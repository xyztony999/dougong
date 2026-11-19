package dbtools

import (
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"

	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storedbt "github.com/dougong/dougong/internal/store/dbtools"
)

// ---- 工具访问（D-09/D-08；admin 专属 ⚠️A10 由 api 层 RoleRequired 承载）----

// requireInstalledRunning 已装+运行态就绪前置（open/reopen 共用；运行态
// false/null→DBT_012 服务未运行，不虚构可打开）。
func (s *Service) requireInstalledRunning(ctx context.Context, toolType string) (*storedbt.Tool, error) {
	a, err := s.store.Tool().GetByType(ctx, toolType)
	if err != nil {
		return nil, fmt.Errorf("dbtools: get archive: %w", err)
	}
	if a == nil {
		return nil, errcode.ErrDBTNotInstalled.WithData(map[string]any{"installed": false})
	}
	if r := s.probeRunning(ctx, toolType, a); r == nil || !*r {
		return nil, errcode.ErrDBTDeployOpFailed.WithData(map[string]any{"reason": "工具服务未运行"})
	}
	return a, nil
}

// Open POST /dbtool/access/open：phpmyadmin 签发 signon 令牌（实例绑定）+
// 工具 URL 附令牌片段；mongo_express gating+basic auth 服务端注入直达。
// 令牌明文仅此次响应下发 admin 浏览器；凭据永不下发（I-02）。
func (s *Service) Open(ctx context.Context, actor *authsvc.Identity, toolType string,
	instanceID *int64, requestHost string) (*OpenData, error) {
	if CatalogByType(toolType) == nil {
		return nil, errcode.ErrDBTInvalidToolType
	}
	a, err := s.requireInstalledRunning(ctx, toolType)
	if err != nil {
		return nil, err
	}
	pa, err := s.panel.PanelAccess(ctx)
	if err != nil {
		return nil, fmt.Errorf("dbtools: panel access: %w", err)
	}
	ref, err := parseSiteRef(a.SiteRef)
	if err != nil {
		return nil, err
	}
	// v1.3 惰性校正（D-20④）：自动块 server_name 与当前 BindDomain 比对，
	// 不一致刷新（自愈钩子失败/重启窗口）；失败不阻断打开（slog 降级）。
	s.lazyRefreshLanding(ctx, ref, pa)
	base, dynamic := s.toolBaseHost(ref, pa, requestHost)
	if dynamic {
		return nil, errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
			"reason": "无法确定工具访问域名（落点无具体 server_name 且未绑定域名）"})
	}
	data := &OpenData{ToolType: toolType}
	detail := map[string]any{"toolType": toolType}
	if toolType == storedbt.ToolPHPMyAdmin {
		if instanceID == nil {
			return nil, errcode.ErrValidation.WithFields(errcode.FieldError{
				Field: "instanceId", Message: "phpmyadmin 打开须指定目标实例"})
		}
		if err := s.checkInstanceReady(ctx, *instanceID); err != nil {
			return nil, err
		}
		token, exp, ierr := s.signon.Issue(ctx, actorSessionDigest(actor), *instanceID)
		if ierr != nil {
			return nil, ierr
		}
		data.URL = base + ToolPathPrefixPHPMyAdmin + "?dg_signon=" + token
		expStr := formatTS(exp)
		data.TokenExpiresAt = &expStr
		detail["instanceId"] = *instanceID
	} else {
		data.URL = base + ToolPathPrefixMongoExpress
	}
	// 访问开启审计（D-16：detail 不含令牌/凭据明文）。
	s.recordAudit(ctx, actor, AuditActionToolAccessed, ObjectTypeTool, toolType, detail)
	return data, nil
}

// Reopen GET /dbtool/access/reopen（v1.2 W3：phpMyAdmin SignonURL 重认证
// GET 落地载体）：admin 会话 gating（api 层）+ 实例就绪 + 重签新令牌 →
// 返回工具 URL（handler 302）。与 open 同一令牌规约，不放宽任何校验。
func (s *Service) Reopen(ctx context.Context, actor *authsvc.Identity, instanceID int64, requestHost string) (string, error) {
	a, err := s.requireInstalledRunning(ctx, storedbt.ToolPHPMyAdmin)
	if err != nil {
		return "", err
	}
	if err := s.checkInstanceReady(ctx, instanceID); err != nil {
		return "", err
	}
	pa, err := s.panel.PanelAccess(ctx)
	if err != nil {
		return "", fmt.Errorf("dbtools: panel access: %w", err)
	}
	ref, err := parseSiteRef(a.SiteRef)
	if err != nil {
		return "", err
	}
	// v1.3 惰性校正（D-20④，同 access/open 口径）。
	s.lazyRefreshLanding(ctx, ref, pa)
	base, dynamic := s.toolBaseHost(ref, pa, requestHost)
	if dynamic {
		return "", errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
			"reason": "无法确定工具访问域名"})
	}
	token, _, ierr := s.signon.Issue(ctx, actorSessionDigest(actor), instanceID)
	if ierr != nil {
		return "", ierr
	}
	s.recordAudit(ctx, actor, AuditActionToolAccessed, ObjectTypeTool, storedbt.ToolPHPMyAdmin,
		map[string]any{"toolType": storedbt.ToolPHPMyAdmin, "instanceId": instanceID, "reopen": true})
	return base + ToolPathPrefixPHPMyAdmin + "?dg_signon=" + token, nil
}

// Rotate POST /dbtool/access/rotate（D-08 四步链路 I-C02：生成新口令→
// 秘密文件+systemd env 更新→重启服务→成功后密文替换+审计同事务；失败
// 回滚文件旧口令保持有效）。仅 mongo_express（phpmyadmin→DBT_009）。
func (s *Service) Rotate(ctx context.Context, actor *authsvc.Identity, toolType string) (*RotateData, error) {
	if CatalogByType(toolType) == nil {
		return nil, errcode.ErrDBTInvalidToolType
	}
	if toolType != storedbt.ToolMongoExpress {
		return nil, errcode.ErrDBTRotateNotSupported
	}
	a, err := s.store.Tool().GetByType(ctx, toolType)
	if err != nil {
		return nil, fmt.Errorf("dbtools: rotate get archive: %w", err)
	}
	if a == nil {
		return nil, errcode.ErrDBTNotInstalled.WithData(map[string]any{"installed": false})
	}
	if a.InstanceID == nil {
		return nil, errcode.ErrDBTDeployOpFailed.WithData(map[string]any{"reason": "档案缺少目标实例引用"})
	}
	var oldCreds toolCredJSON
	if err := s.decryptToolCreds(a.CredCipher, &oldCreds); err != nil {
		return nil, err
	}
	creds, err := s.creds.InstanceCredentialsForTool(ctx, *a.InstanceID)
	if err != nil {
		return nil, err
	}
	authConfPath := filepath.Join(s.toolDir, MongoExpressAuthConfName)
	oldAuthConf, err := s.deploy.ReadFile(authConfPath)
	if err != nil {
		return nil, errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
			"reason": "读取 basic auth 秘密文件失败: " + err.Error()})
	}
	oldUnit, err := s.deploy.ReadFile(MongoExpressUnitPath)
	if err != nil {
		return nil, errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
			"reason": "读取 systemd 单元文件失败: " + err.Error()})
	}
	globalRoot, err := s.deploy.NpmGlobalRoot(ctx)
	if err != nil {
		return nil, err
	}
	passRaw, err := randomBytes(BasicAuthPasswordBytes)
	if err != nil {
		return nil, err
	}
	newPassword := base64.RawURLEncoding.EncodeToString(passRaw)
	cookieSecret, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	sessionSecret, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	newUnit := mongoExpressUnit(mongoUnitParams{
		AppPath: globalRoot + "/mongo-express/app.js", MongoURL: buildMongoURL(creds),
		BasicUsername: BasicAuthUsername, BasicPassword: newPassword,
		CookieSecret: cookieSecret, SessionSecret: sessionSecret,
	})
	// 四步链路：秘密文件+单元更新 → daemon-reload → 重启；失败回滚。
	rollback := func(cause error) error {
		if rerr := s.deploy.WriteSecretFile(authConfPath, oldAuthConf); rerr != nil {
			cause = fmt.Errorf("%v（回滚 auth.conf 亦失败: %v）", cause, rerr)
		}
		if rerr := s.deploy.WriteSecretFile(MongoExpressUnitPath, oldUnit); rerr != nil {
			cause = fmt.Errorf("%v（回滚单元文件亦失败: %v）", cause, rerr)
		}
		if rerr := s.sysd.DaemonReload(ctx); rerr != nil {
			cause = fmt.Errorf("%v（回滚 daemon-reload 失败: %v）", cause, rerr)
		}
		if _, rerr := s.sysd.RestartUnit(ctx, MongoExpressUnitName); rerr != nil {
			cause = fmt.Errorf("%v（回滚重启失败: %v）", cause, rerr)
		}
		return errcode.ErrDBTDeployOpFailed.WithData(map[string]any{"reason": cause.Error()})
	}
	if err := s.deploy.WriteSecretFile(authConfPath, []byte(mongoExpressAuthConf(newPassword))); err != nil {
		return nil, rollback(err)
	}
	if err := s.deploy.WriteSecretFile(MongoExpressUnitPath, []byte(newUnit)); err != nil {
		return nil, rollback(err)
	}
	if err := s.sysd.DaemonReload(ctx); err != nil {
		return nil, rollback(err)
	}
	job, err := s.sysd.RestartUnit(ctx, MongoExpressUnitName)
	if err != nil {
		return nil, rollback(err)
	}
	if !job.Succeeded {
		return nil, rollback(fmt.Errorf("重启服务 job result=%s", job.Reason))
	}
	// 成功后密文替换+审计同事务（I-C02/I-03；detail 不含口令）。
	newCipher, err := s.vault.Encrypt(marshalToolCreds(toolCredJSON{BasicAuthPassword: newPassword}))
	if err != nil {
		return nil, rollback(err)
	}
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return nil, rollback(fmt.Errorf("dbtools: rotate tx: %w", err))
	}
	defer func() { _ = tx.Rollback() }()
	if err := tx.Tool().UpdateCredCipher(ctx, toolType, newCipher); err != nil {
		return nil, rollback(err)
	}
	if err := tx.Audit().Append(ctx, auditEntry(actor, AuditActionCredentialRotated,
		ObjectTypeTool, toolType, map[string]any{"toolType": toolType}), s.clock()); err != nil {
		return nil, rollback(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, rollback(err)
	}
	return &RotateData{ToolType: toolType, RotatedAt: formatTS(s.clock())}, nil
}

func actorSessionDigest(actor *authsvc.Identity) string {
	if actor == nil {
		return ""
	}
	return actor.SessionDigest
}
