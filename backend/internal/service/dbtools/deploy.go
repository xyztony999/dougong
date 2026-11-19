package dbtools

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dougong/dougong/internal/adapter/phpfpm"
	"github.com/dougong/dougong/internal/adapter/sysd"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storedbt "github.com/dougong/dougong/internal/store/dbtools"
)

// ---- 同步入口（D-04：校验 → 创建 pending 与发起审计同事务 → 立即返回
// taskId → worker 创建即触发）----

// Install POST /dbtool/tool/install：toolType（DBT_002）→ 依赖前置
// （DBT_001）→ mongo_express 实例就绪（DBT_006）→ 未装（已装 DBT_004 语义
// 分流升级）→ 互斥（DBT_003）。
func (s *Service) Install(ctx context.Context, actor *authsvc.Identity, toolType string, instanceID *int64) (*TaskCreatedData, error) {
	entry := CatalogByType(toolType)
	if entry == nil {
		return nil, errcode.ErrDBTInvalidToolType
	}
	if missing := s.depsMissing(ctx, toolType); len(missing) > 0 {
		return nil, errcode.ErrDBTDependencyMissing.WithData(map[string]any{"missing": missing})
	}
	if toolType == storedbt.ToolMongoExpress {
		if instanceID == nil {
			return nil, errcode.ErrValidation.WithFields(errcode.FieldError{
				Field: "instanceId", Message: "mongo_express 安装须绑定目标实例"})
		}
		if err := s.checkInstanceReady(ctx, *instanceID); err != nil {
			return nil, err
		}
	}
	a, err := s.store.Tool().GetByType(ctx, toolType)
	if err != nil {
		return nil, fmt.Errorf("dbtools: install get archive: %w", err)
	}
	if a != nil {
		return nil, errcode.ErrDBTNotInstalled.WithData(map[string]any{"installed": true})
	}
	taskID, err := s.createTask(ctx, actor, toolType, storedbt.ActionInstall,
		AuditActionInstallStarted, instanceID, map[string]any{"toolType": toolType, "action": storedbt.ActionInstall})
	if err != nil {
		return nil, err
	}
	s.dispatchWorker(taskID)
	return &TaskCreatedData{TaskID: taskID, Action: storedbt.ActionInstall}, nil
}

// Upgrade POST /dbtool/tool/upgrade：已装（DBT_004）→ 目录版本≠已装版本
// （DBT_011 不虚构升级）→ 互斥（DBT_003）。
func (s *Service) Upgrade(ctx context.Context, actor *authsvc.Identity, toolType string) (*TaskCreatedData, error) {
	entry := CatalogByType(toolType)
	if entry == nil {
		return nil, errcode.ErrDBTInvalidToolType
	}
	a, err := s.store.Tool().GetByType(ctx, toolType)
	if err != nil {
		return nil, fmt.Errorf("dbtools: upgrade get archive: %w", err)
	}
	if a == nil {
		return nil, errcode.ErrDBTNotInstalled.WithData(map[string]any{"installed": false})
	}
	if a.InstalledVersion == entry.CatalogVersion {
		return nil, errcode.ErrDBTAlreadyCurrentVersion
	}
	taskID, err := s.createTask(ctx, actor, toolType, storedbt.ActionUpgrade,
		AuditActionUpgradeStarted, nil, map[string]any{"toolType": toolType, "action": storedbt.ActionUpgrade})
	if err != nil {
		return nil, err
	}
	s.dispatchWorker(taskID)
	return &TaskCreatedData{TaskID: taskID, Action: storedbt.ActionUpgrade}, nil
}

// Uninstall POST /dbtool/tool/uninstall：已装（DBT_004）→ 互斥（DBT_003）。
// 强确认归前端（D-15 服务端不硬阻断）；执行链白名单不触及引擎数据（I-C03）。
func (s *Service) Uninstall(ctx context.Context, actor *authsvc.Identity, toolType string) (*TaskCreatedData, error) {
	if CatalogByType(toolType) == nil {
		return nil, errcode.ErrDBTInvalidToolType
	}
	a, err := s.store.Tool().GetByType(ctx, toolType)
	if err != nil {
		return nil, fmt.Errorf("dbtools: uninstall get archive: %w", err)
	}
	if a == nil {
		return nil, errcode.ErrDBTNotInstalled.WithData(map[string]any{"installed": false})
	}
	taskID, err := s.createTask(ctx, actor, toolType, storedbt.ActionUninstall,
		AuditActionUninstallStarted, nil, map[string]any{"toolType": toolType, "action": storedbt.ActionUninstall})
	if err != nil {
		return nil, err
	}
	s.dispatchWorker(taskID)
	return &TaskCreatedData{TaskID: taskID, Action: storedbt.ActionUninstall}, nil
}

// checkInstanceReady 实例就绪校验（D-10 → DBT_006 data.instanceId/reason）。
func (s *Service) checkInstanceReady(ctx context.Context, instanceID int64) error {
	info, err := s.creds.InstanceReadyForTool(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("dbtools: instance ready check: %w", err)
	}
	if !info.Ready {
		return errcode.ErrDBTInstanceNotReady.WithData(map[string]any{
			"instanceId": instanceID, "reason": info.Reason})
	}
	return nil
}

// createTask 互斥判定与任务创建+发起审计同事务（I-06/I-08；SQLite 单连接
// 写串行保证原子）。
func (s *Service) createTask(ctx context.Context, actor *authsvc.Identity, toolType, action, auditAction string,
	instanceID *int64, detail map[string]any) (int64, error) {
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return 0, fmt.Errorf("dbtools: create task tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if inflight, err := tx.Task().InFlight(ctx, toolType); err != nil {
		return 0, fmt.Errorf("dbtools: in-flight check: %w", err)
	} else if inflight != nil {
		return 0, errcode.ErrDBTTaskInFlight.WithData(map[string]any{"taskId": inflight.ID})
	}
	id, err := tx.Task().Create(ctx, storedbt.Task{
		ToolType: toolType, Action: action, InstanceID: instanceID, RequestedBy: actorName(actor),
	})
	if err != nil {
		return 0, fmt.Errorf("dbtools: task create: %w", err)
	}
	if err := tx.Audit().Append(ctx, auditEntry(actor, auditAction, ObjectTypeToolTask,
		fmt.Sprintf("%d", id), detail), s.clock()); err != nil {
		return 0, fmt.Errorf("dbtools: task create audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("dbtools: task create commit: %w", err)
	}
	return id, nil
}

// ---- worker（D-04：goroutine 生命周期=命令链结束，有限时自然终止 G-06）----

// dispatchWorker 创建即触发（ctx 独立于请求——请求返回后任务继续）。
func (s *Service) dispatchWorker(taskID int64) {
	if s.disableDispatch {
		return // 测试同步执行路径（Options.DisableDispatch）
	}
	go s.runTask(taskID)
}

// runTask 单任务执行链：running 落库 → 执行链 → 终态+档案+审计同事务。
func (s *Service) runTask(taskID int64) {
	ctx := context.Background()
	task, err := s.store.Task().GetByID(ctx, taskID)
	if err != nil || task.Status != storedbt.TaskPending {
		return // 不存在/已被恢复流程终态化——不执行
	}
	if err := s.store.Task().MarkRunning(ctx, taskID); err != nil {
		slog.Error("dbtools: mark running", "task", taskID, "err", err)
		return
	}
	ring := newOutputRing()
	var archive *storedbt.Tool
	var newVersion string
	var opErr error
	switch task.Action {
	case storedbt.ActionInstall:
		if task.ToolType == storedbt.ToolPHPMyAdmin {
			archive, opErr = s.installPHPMyAdmin(ctx, task, ring)
		} else {
			archive, opErr = s.installMongoExpress(ctx, task, ring)
		}
	case storedbt.ActionUpgrade:
		if task.ToolType == storedbt.ToolPHPMyAdmin {
			newVersion, opErr = s.upgradePHPMyAdmin(ctx, ring)
		} else {
			newVersion, opErr = s.upgradeMongoExpress(ctx, ring)
		}
	case storedbt.ActionUninstall:
		if task.ToolType == storedbt.ToolPHPMyAdmin {
			opErr = s.uninstallPHPMyAdmin(ctx, ring)
		} else {
			opErr = s.uninstallMongoExpress(ctx, ring)
		}
	default:
		opErr = fmt.Errorf("dbtools: unknown action %q", task.Action)
	}
	status := storedbt.TaskSucceeded
	var taskErr *string
	if opErr != nil {
		status = storedbt.TaskFailed
		msg := opErr.Error()
		// v1.3 D-22 reason 可见：*errcode.Error 且 Data()["reason"] 非空
		// string → 追加 ": " + reason（任务详情可见具体失败原因；无 reason
		// 载荷沿既有 symbol+message 不虚构）。
		var ec *errcode.Error
		if errors.As(opErr, &ec) {
			if m, ok := ec.Data().(map[string]any); ok {
				if reason, ok := m["reason"].(string); ok && reason != "" {
					msg += ": " + reason
				}
			}
		}
		taskErr = &msg
	}
	_ = s.store.Task().AppendOutput(ctx, taskID, tailLines(ring.String(), TaskOutputTailLines))
	if err := s.finishTask(ctx, task, status, taskErr, archive, newVersion); err != nil {
		slog.Error("dbtools: finish task", "task", taskID, "err", err)
	}
}

// finishTask 终态落库+档案登记/更新/删除+终态审计同事务（I-C01/I-08：
// install succeeded→Create；upgrade succeeded→UpdateVersion；uninstall
// succeeded→Delete；failed→档案不变，不留半档案）。
func (s *Service) finishTask(ctx context.Context, task storedbt.Task, status string, taskErr *string,
	archive *storedbt.Tool, newVersion string) error {
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("dbtools: finish tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := tx.Task().Finish(ctx, task.ID, status, taskErr); err != nil {
		return fmt.Errorf("dbtools: finish: %w", err)
	}
	if status == storedbt.TaskSucceeded {
		switch task.Action {
		case storedbt.ActionInstall:
			if archive == nil {
				return fmt.Errorf("dbtools: install succeeded without archive (invariant I-C01)")
			}
			if _, err := tx.Tool().Create(ctx, *archive); err != nil {
				return fmt.Errorf("dbtools: archive create: %w", err)
			}
		case storedbt.ActionUpgrade:
			if err := tx.Tool().UpdateVersion(ctx, task.ToolType, newVersion); err != nil {
				return fmt.Errorf("dbtools: archive version update: %w", err)
			}
		case storedbt.ActionUninstall:
			if err := tx.Tool().Delete(ctx, task.ToolType); err != nil {
				return fmt.Errorf("dbtools: archive delete: %w", err)
			}
		}
	}
	result := audit.ResultSuccess
	if status != storedbt.TaskSucceeded {
		result = audit.ResultFailure
	}
	detail := map[string]any{"toolType": task.ToolType, "action": task.Action, "status": status}
	if taskErr != nil {
		detail["error"] = *taskErr
	}
	if err := tx.Audit().Append(ctx, audit.Entry{
		ActorName: task.RequestedBy, Category: audit.CategoryChange,
		Action: AuditActionTaskFinished, ObjectType: ObjectTypeToolTask,
		ObjectID: fmt.Sprintf("%d", task.ID), Detail: detail, Result: result, Source: audit.SourceWeb,
	}, s.clock()); err != nil {
		return fmt.Errorf("dbtools: finish audit: %w", err)
	}
	return tx.Commit()
}

// RecoverInFlight 启动恢复（I-09：running/pending→failed「面板重启中断」；
// main 启动时调用一次——下载/npm 中断态不安全，worker 不续跑）。
func (s *Service) RecoverInFlight(ctx context.Context) {
	const reason = "面板重启中断"
	if n, err := s.store.Task().FailRunningAll(ctx, reason); err != nil {
		slog.Error("dbtools: recover in-flight", "err", err)
	} else if n > 0 {
		slog.Warn("dbtools: recovered in-flight tasks after restart", "count", n)
	}
}

// ---- 安装执行链 ----

// installPHPMyAdmin D-05/D-06：pool 实况 → 下载+sha256 强制 → 解压管理目录
// → config.inc.php/signon.php/fastcgi.conf 生成 → 落点注入 → 凭据密文。
func (s *Service) installPHPMyAdmin(ctx context.Context, task storedbt.Task, ring *outputRing) (*storedbt.Tool, error) {
	pools, err := s.phpfpm.ListPools(ctx)
	if err != nil {
		return nil, fmt.Errorf("dbtools: list php-fpm pools: %w", err)
	}
	if len(pools) == 0 {
		return nil, errors.New("dbtools: 未检出 PHP-FPM pool（请先经环境管理安装 PHP-FPM）")
	}
	pool := pickPool(pools)
	ring.append("[pool] " + pool.Name + " " + pool.Socket)

	localPath, err := s.deploy.DownloadTarball(ctx, PHPMyAdminTarballURL, s.mirror, DownloadTimeout)
	if err != nil {
		return nil, err
	}
	defer func() { _ = s.deploy.Remove(localPath) }()
	ok, err := s.deploy.VerifySha256(localPath, PHPMyAdminSha256)
	if err != nil {
		return nil, err
	}
	if !ok {
		// D-05 强制：失配=任务 failed，不降级不跳过不安装（文件已清理）。
		return nil, errors.New("sha256_mismatch")
	}
	ring.append("[sha256] verified " + PHPMyAdminSha256)

	dest := filepath.Join(s.toolDir, "phpmyadmin")
	if err := s.deploy.RemoveAll(dest); err != nil { // 清洁落位（白名单工具目录）
		return nil, err
	}
	if err := s.deploy.ExtractTar(localPath, dest); err != nil {
		return nil, err
	}
	_ = s.deploy.RemoveAll(filepath.Join(dest, "setup")) // 加固：移除 setup 目录
	if err := s.deploy.MkdirAll(filepath.Join(dest, "tmp"), 0o777); err != nil {
		return nil, err
	}
	ring.append("[extract] " + dest)

	pa, err := s.panel.PanelAccess(ctx)
	if err != nil {
		return nil, fmt.Errorf("dbtools: panel access: %w", err)
	}
	ref, err := s.selectLanding(ctx, pa)
	if err != nil {
		return nil, err
	}
	secret, err := s.ensureInternalSecret()
	if err != nil {
		return nil, err
	}
	// 凭据生成：signon 密钥 + blowfish_secret（密文落档，I-02）。
	keyRaw, err := randomBytes(SignonKeyBytes)
	if err != nil {
		return nil, err
	}
	blowRaw, err := randomBytes(32)
	if err != nil {
		return nil, err
	}
	signonKey := hex.EncodeToString(keyRaw)
	blowfish := base64.StdEncoding.EncodeToString(blowRaw)
	cipher, err := s.vault.Encrypt(marshalToolCreds(toolCredJSON{SignonKey: signonKey, BlowfishSecret: blowfish}))
	if err != nil {
		return nil, err
	}
	// 配置文件（0644 PHP-FPM worker 可读，A-DBT5）。SignonURL 指向面板后端
	// reopen 端点——自动块形态面板 Go 直连基址含监听端口（A-DBT11）。
	signonBase, signonDynamic := s.signonBaseHost(ref, pa)
	signonExpr := signonURLExpr(signonBase, pa.EntryPath, signonDynamic)
	if ref.Port != 0 {
		signonExpr = signonURLExprPanelPort(pa, signonBase)
	}
	cfg := configIncPHP(configIncPHPParams{
		BlowfishSecret:   blowfish,
		SignonScriptPath: filepath.Join(dest, "signon.php"),
		SignonURLExpr:    signonExpr,
		TempDir:          filepath.Join(dest, "tmp"),
	})
	if err := s.deploy.WriteFile(filepath.Join(dest, "config.inc.php"), []byte(cfg), 0o644); err != nil {
		return nil, err
	}
	signon := signonPHP(panelLoopbackBase(pa)+"/dbtool/signon/verify", secret)
	if err := s.deploy.WriteFile(filepath.Join(dest, "signon.php"), []byte(signon), 0o644); err != nil {
		return nil, err
	}
	if err := s.deploy.WriteFile(filepath.Join(s.toolDir, PHPMyAdminFastCGIConfName), []byte(fastCGIConf()), 0o644); err != nil {
		return nil, err
	}
	if err := s.injectToolLocations(ctx, storedbt.ToolPHPMyAdmin, ref, pa, secret, pool.Socket); err != nil {
		return nil, errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
			"reason": "工具 location 注入失败: " + err.Error()})
	}
	ring.append("[inject] " + ref.File + " server#" + fmt.Sprintf("%d", ref.Index))
	appendLandingHint(ring, ref)
	return &storedbt.Tool{
		ToolType: storedbt.ToolPHPMyAdmin, InstalledVersion: PHPMyAdminVersion,
		DeployPath: dest, SiteRef: marshalSiteRef(ref), CredCipher: cipher,
	}, nil
}

// installMongoExpress D-07：npm 精确版本 → systemd 单元（env 注入 vault
// Mongo 凭据+面板 basic auth+baseUrl+bind 127.0.0.1）→ 秘密文件 → 落点注入。
// 失败清理：单元已创建后失败→尽力停服/移除单元（不留孤儿服务）。
func (s *Service) installMongoExpress(ctx context.Context, task storedbt.Task, ring *outputRing) (archive *storedbt.Tool, retErr error) {
	if task.InstanceID == nil {
		return nil, errors.New("dbtools: mongo_express install missing instanceId")
	}
	creds, err := s.creds.InstanceCredentialsForTool(ctx, *task.InstanceID)
	if err != nil {
		return nil, err
	}
	out, err := s.deploy.NpmInstall(ctx, MongoExpressPackage, MongoExpressVersion, s.npmRegistry, DeployOpTimeout)
	ring.append(out)
	if err != nil {
		return nil, err
	}
	globalRoot, err := s.deploy.NpmGlobalRoot(ctx)
	if err != nil {
		return nil, err
	}
	appPath := globalRoot + "/mongo-express/app.js"
	passRaw, err := randomBytes(BasicAuthPasswordBytes)
	if err != nil {
		return nil, err
	}
	password := base64.RawURLEncoding.EncodeToString(passRaw) // 24B→32 字符
	cookieSecret, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	sessionSecret, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	unit := mongoExpressUnit(mongoUnitParams{
		AppPath: appPath, MongoURL: buildMongoURL(creds),
		BasicUsername: BasicAuthUsername, BasicPassword: password,
		CookieSecret: cookieSecret, SessionSecret: sessionSecret,
	})
	if err := s.deploy.WriteSecretFile(MongoExpressUnitPath, []byte(unit)); err != nil {
		return nil, err
	}
	unitCreated := true
	defer func() {
		if retErr != nil && unitCreated {
			s.cleanupMongoUnit(ctx, ring) // 尽力清理（失败记日志不掩盖主错误）
		}
	}()
	if err := s.sysd.DaemonReload(ctx); err != nil {
		return nil, fmt.Errorf("dbtools: daemon-reload: %w", err)
	}
	if err := s.sysd.EnableUnit(ctx, MongoExpressUnitName); err != nil {
		return nil, fmt.Errorf("dbtools: enable unit: %w", err)
	}
	job, err := s.sysd.StartUnit(ctx, MongoExpressUnitName)
	if err != nil {
		return nil, fmt.Errorf("dbtools: start unit: %w", err)
	}
	if !job.Succeeded {
		return nil, fmt.Errorf("dbtools: start unit job result=%s", job.Reason)
	}
	ring.append("[systemd] " + MongoExpressUnitName + " started (bind " + MongoExpressBind + ":" + fmt.Sprintf("%d", MongoExpressPort) + ")")

	if err := s.deploy.WriteSecretFile(filepath.Join(s.toolDir, MongoExpressAuthConfName),
		[]byte(mongoExpressAuthConf(password))); err != nil {
		return nil, err
	}
	secret, err := s.ensureInternalSecret()
	if err != nil {
		return nil, err
	}
	pa, err := s.panel.PanelAccess(ctx)
	if err != nil {
		return nil, fmt.Errorf("dbtools: panel access: %w", err)
	}
	ref, err := s.selectLanding(ctx, pa)
	if err != nil {
		return nil, err
	}
	if err := s.injectToolLocations(ctx, storedbt.ToolMongoExpress, ref, pa, secret, ""); err != nil {
		return nil, errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
			"reason": "工具 location 注入失败: " + err.Error()})
	}
	appendLandingHint(ring, ref)
	cipher, err := s.vault.Encrypt(marshalToolCreds(toolCredJSON{BasicAuthPassword: password}))
	if err != nil {
		return nil, err
	}
	unitName := MongoExpressUnitName
	return &storedbt.Tool{
		ToolType: storedbt.ToolMongoExpress, InstalledVersion: MongoExpressVersion,
		DeployPath: globalRoot + "/mongo-express", SiteRef: marshalSiteRef(ref),
		UnitName: &unitName, InstanceID: task.InstanceID, CredCipher: cipher,
	}, nil
}

// cleanupMongoUnit 安装失败后尽力清理已创建单元（停服→禁用→移除文件→
// daemon-reload；各步失败记日志继续——主错误不被掩盖）。
func (s *Service) cleanupMongoUnit(ctx context.Context, ring *outputRing) {
	if _, err := s.sysd.StopUnit(ctx, MongoExpressUnitName); err != nil && !errors.Is(err, sysd.ErrUnitNotFound) {
		slog.WarnContext(ctx, "dbtools: cleanup stop unit", "err", err)
	}
	if err := s.sysd.DisableUnit(ctx, MongoExpressUnitName); err != nil && !errors.Is(err, sysd.ErrUnitNotFound) {
		slog.WarnContext(ctx, "dbtools: cleanup disable unit", "err", err)
	}
	if err := s.deploy.Remove(MongoExpressUnitPath); err != nil {
		slog.WarnContext(ctx, "dbtools: cleanup remove unit file", "err", err)
	}
	if err := s.sysd.DaemonReload(ctx); err != nil {
		slog.WarnContext(ctx, "dbtools: cleanup daemon-reload", "err", err)
	}
	ring.append("[cleanup] 安装失败，已尽力清理 systemd 单元")
}

// ---- 升级执行链（D-14：目录钉死新版本；配置/凭据保留）----

// upgradePHPMyAdmin 下载新 tarball（sha256）→ staging 解压 → 保留
// config.inc.php/signon.php → 替换程序目录（signon 密钥 vault 不变）。
func (s *Service) upgradePHPMyAdmin(ctx context.Context, ring *outputRing) (string, error) {
	localPath, err := s.deploy.DownloadTarball(ctx, PHPMyAdminTarballURL, s.mirror, DownloadTimeout)
	if err != nil {
		return "", err
	}
	defer func() { _ = s.deploy.Remove(localPath) }()
	ok, err := s.deploy.VerifySha256(localPath, PHPMyAdminSha256)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errors.New("sha256_mismatch")
	}
	dest := filepath.Join(s.toolDir, "phpmyadmin")
	staging := dest + ".new"
	if err := s.deploy.RemoveAll(staging); err != nil {
		return "", err
	}
	if err := s.deploy.ExtractTar(localPath, staging); err != nil {
		return "", err
	}
	_ = s.deploy.RemoveAll(filepath.Join(staging, "setup"))
	// 配置保留（D-14 conffile 保守先例）。
	oldCfg, err := s.deploy.ReadFile(filepath.Join(dest, "config.inc.php"))
	if err != nil {
		return "", fmt.Errorf("dbtools: preserve config.inc.php: %w", err)
	}
	oldSignon, err := s.deploy.ReadFile(filepath.Join(dest, "signon.php"))
	if err != nil {
		return "", fmt.Errorf("dbtools: preserve signon.php: %w", err)
	}
	if err := s.deploy.RemoveAll(dest); err != nil {
		return "", err
	}
	if err := s.deploy.Rename(staging, dest); err != nil {
		return "", err
	}
	if err := s.deploy.WriteFile(filepath.Join(dest, "config.inc.php"), oldCfg, 0o644); err != nil {
		return "", err
	}
	if err := s.deploy.WriteFile(filepath.Join(dest, "signon.php"), oldSignon, 0o644); err != nil {
		return "", err
	}
	if err := s.deploy.MkdirAll(filepath.Join(dest, "tmp"), 0o777); err != nil {
		return "", err
	}
	ring.append("[upgrade] 程序目录已替换，config.inc.php/signon.php 保留")
	return PHPMyAdminVersion, nil
}

// upgradeMongoExpress npm 重装新版本 → 单元重写（凭据形态不变：basic auth
// 口令取档案密文，Mongo URL 取 vault 当前凭据）→ 重启服务。
func (s *Service) upgradeMongoExpress(ctx context.Context, ring *outputRing) (string, error) {
	a, err := s.store.Tool().GetByType(ctx, storedbt.ToolMongoExpress)
	if err != nil || a == nil || a.InstanceID == nil {
		return "", errors.New("dbtools: mongo_express archive missing for upgrade")
	}
	creds, err := s.creds.InstanceCredentialsForTool(ctx, *a.InstanceID)
	if err != nil {
		return "", err
	}
	var cj toolCredJSON
	if err := s.decryptToolCreds(a.CredCipher, &cj); err != nil {
		return "", err
	}
	out, err := s.deploy.NpmInstall(ctx, MongoExpressPackage, MongoExpressVersion, s.npmRegistry, DeployOpTimeout)
	ring.append(out)
	if err != nil {
		return "", err
	}
	globalRoot, err := s.deploy.NpmGlobalRoot(ctx)
	if err != nil {
		return "", err
	}
	cookieSecret, err := randomHex(32)
	if err != nil {
		return "", err
	}
	sessionSecret, err := randomHex(32)
	if err != nil {
		return "", err
	}
	unit := mongoExpressUnit(mongoUnitParams{
		AppPath: globalRoot + "/mongo-express/app.js", MongoURL: buildMongoURL(creds),
		BasicUsername: BasicAuthUsername, BasicPassword: cj.BasicAuthPassword,
		CookieSecret: cookieSecret, SessionSecret: sessionSecret,
	})
	if err := s.deploy.WriteSecretFile(MongoExpressUnitPath, []byte(unit)); err != nil {
		return "", err
	}
	if err := s.sysd.DaemonReload(ctx); err != nil {
		return "", fmt.Errorf("dbtools: daemon-reload: %w", err)
	}
	job, err := s.sysd.RestartUnit(ctx, MongoExpressUnitName)
	if err != nil {
		return "", fmt.Errorf("dbtools: restart unit: %w", err)
	}
	if !job.Succeeded {
		return "", fmt.Errorf("dbtools: restart unit job result=%s", job.Reason)
	}
	ring.append("[upgrade] mongo-express 已重装并重启")
	return MongoExpressVersion, nil
}

// ---- 卸载执行链（D-15/I-C03：白名单仅工具目录/单元/注入配置/本域档案——
// 绝不触及引擎数据目录与 db_instance 档案）----

// uninstallPHPMyAdmin 撤除注入恢复原字节 → 删除程序目录与 fastcgi 模板。
func (s *Service) uninstallPHPMyAdmin(ctx context.Context, ring *outputRing) error {
	a, err := s.store.Tool().GetByType(ctx, storedbt.ToolPHPMyAdmin)
	if err != nil || a == nil {
		return errors.New("dbtools: phpmyadmin archive missing for uninstall")
	}
	ref, err := parseSiteRef(a.SiteRef)
	if err != nil {
		return err
	}
	if err := s.removeToolInjection(ctx, storedbt.ToolPHPMyAdmin, ref); err != nil {
		return errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
			"reason": "撤除注入失败: " + err.Error()})
	}
	// v1.3 清块判定（D-19⑥/I-C04）：两工具均卸载且 autoCreated → 移除整块。
	if err := s.cleanupAutoLanding(ctx, storedbt.ToolPHPMyAdmin, ref, ring); err != nil {
		return err
	}
	if err := s.deploy.RemoveAll(filepath.Join(s.toolDir, "phpmyadmin")); err != nil {
		return err
	}
	if err := s.deploy.Remove(filepath.Join(s.toolDir, PHPMyAdminFastCGIConfName)); err != nil {
		return err
	}
	ring.append("[uninstall] 工具部署已解除（引擎数据与 db_instance 档案不受影响，I-C03）")
	return nil
}

// uninstallMongoExpress 停止并移除 systemd 服务 → 撤除注入 → npm 移除 →
// 删除 auth 秘密文件。
func (s *Service) uninstallMongoExpress(ctx context.Context, ring *outputRing) error {
	a, err := s.store.Tool().GetByType(ctx, storedbt.ToolMongoExpress)
	if err != nil || a == nil {
		return errors.New("dbtools: mongo_express archive missing for uninstall")
	}
	if _, err := s.sysd.StopUnit(ctx, MongoExpressUnitName); err != nil && !errors.Is(err, sysd.ErrUnitNotFound) {
		return fmt.Errorf("dbtools: stop unit: %w", err)
	}
	if err := s.sysd.DisableUnit(ctx, MongoExpressUnitName); err != nil && !errors.Is(err, sysd.ErrUnitNotFound) {
		return fmt.Errorf("dbtools: disable unit: %w", err)
	}
	if err := s.deploy.Remove(MongoExpressUnitPath); err != nil {
		return err
	}
	if err := s.sysd.DaemonReload(ctx); err != nil {
		return fmt.Errorf("dbtools: daemon-reload: %w", err)
	}
	ref, err := parseSiteRef(a.SiteRef)
	if err != nil {
		return err
	}
	if err := s.removeToolInjection(ctx, storedbt.ToolMongoExpress, ref); err != nil {
		return errcode.ErrDBTDeployOpFailed.WithData(map[string]any{
			"reason": "撤除注入失败: " + err.Error()})
	}
	// v1.3 清块判定（D-19⑥/I-C04）：两工具均卸载且 autoCreated → 移除整块。
	if err := s.cleanupAutoLanding(ctx, storedbt.ToolMongoExpress, ref, ring); err != nil {
		return err
	}
	out, err := s.deploy.NpmRemove(ctx, MongoExpressPackage, DeployOpTimeout)
	ring.append(out)
	if err != nil {
		return err
	}
	if err := s.deploy.Remove(filepath.Join(s.toolDir, MongoExpressAuthConfName)); err != nil {
		return err
	}
	ring.append("[uninstall] 工具部署已解除（MongoDB 数据与 db_instance 档案不受影响，I-C03）")
	return nil
}

// ---- 共享辅助 ----

// ensureInternalSecret 内部秘密（D-11：crypto/rand 32B hex，0600；首装生成，
// 后续复用——两工具共享同一 gate 秘密）。
func (s *Service) ensureInternalSecret() (string, error) {
	p := filepath.Join(s.toolDir, InternalSecretName)
	if b, err := s.deploy.ReadFile(p); err == nil {
		if v := strings.TrimSpace(string(b)); v != "" {
			return v, nil
		}
	}
	secret, err := randomHex(32)
	if err != nil {
		return "", err
	}
	if err := s.deploy.WriteSecretFile(p, []byte(secret)); err != nil {
		return "", err
	}
	return secret, nil
}

// InternalSecret 读取内部秘密（api 内部端点守卫消费；每请求实时读取——
// 秘密轮换即时生效，文件 65B 页缓存命中开销可忽略）。
func (s *Service) InternalSecret() (string, error) {
	b, err := s.deploy.ReadFile(filepath.Join(s.toolDir, InternalSecretName))
	if err != nil {
		return "", fmt.Errorf("dbtools: internal secret unavailable: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}

// toolBaseHost 工具 URL 基址（v1.3 D-23 端口分支）：
// port≠0（自动落地块）→ scheme://hostname:port——hostname=BindDomain 优先
// （PanelAccess 实时读），否则 requestHost 去端口（open/reopen 浏览器语境；
// 安装期无请求语境且未绑域名→动态回退沿 A-DBT6）；
// port=0（既有面板域块 80/443）沿旧形态：落点具体域名优先 → bind_domain →
// 请求 Host → 动态 PHP Host 回退。
func (s *Service) toolBaseHost(ref siteRef, pa PanelAccess, requestHost string) (base string, dynamic bool) {
	if ref.Port != 0 {
		host := pa.BindDomain
		if host == "" {
			host = stripHostPort(requestHost)
		}
		if host == "" {
			host = ref.ServerName
		}
		if host == "" {
			return "", true
		}
		return ref.Scheme + "://" + net.JoinHostPort(host, strconv.Itoa(ref.Port)), false
	}
	host := ref.ServerName
	if host == "" {
		host = pa.BindDomain
	}
	if host == "" {
		host = requestHost
	}
	if host == "" {
		return "", true
	}
	return ref.Scheme + "://" + host, false
}

// signonBaseHost SignonURL 基址（reopen 为面板后端 API，非工具落点；
// A-DBT11 落码裁定）：port=0 既有块→沿旧口径 toolBaseHost（nginx 80/443
// 承载面板）；port≠0 自动块→面板 Go 直连基址 {panelScheme}://{hostname}:
// {ListenPort}（0 匹配推断面板不经 nginx 承载；hostname=BindDomain 优先，
// 未绑安装期无请求语境→动态 PHP 回退；panelScheme 按面板 HTTPS 态，工具
// 落地块恒 http 不决定面板 scheme）。
func (s *Service) signonBaseHost(ref siteRef, pa PanelAccess) (base string, dynamic bool) {
	if ref.Port == 0 {
		return s.toolBaseHost(ref, pa, "")
	}
	scheme := "http"
	if pa.HTTPSEnabled {
		scheme = "https"
	}
	if pa.BindDomain != "" {
		return scheme + "://" + net.JoinHostPort(pa.BindDomain, strconv.Itoa(pa.ListenPort)), false
	}
	return "", true
}

// stripHostPort 去除 Host 头的可选 :port（D-23；IPv6 字面量形如 [::1]:8080；
// SplitHostPort 失败视为不含端口——带方括号无端口形态剥括号供 JoinHostPort
// 重新规范化）。
func stripHostPort(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		return host[1 : len(host)-1]
	}
	return host
}

// pickPool 默认 pool 选择（D-06 复用已装 PHP-FPM 默认 pool：www 优先，
// 否则首个——不新建 pool）。
func pickPool(pools []phpfpm.Pool) phpfpm.Pool {
	for _, p := range pools {
		if p.Name == "www" {
			return p
		}
	}
	return pools[0]
}

// toolCredJSON 工具凭据密文内嵌 JSON（mongo_express={basicAuthPassword}；
// phpmyadmin={signonKey,blowfishSecret}，§4.1.1）。
type toolCredJSON struct {
	BasicAuthPassword string `json:"basicAuthPassword,omitempty"`
	SignonKey         string `json:"signonKey,omitempty"`
	BlowfishSecret    string `json:"blowfishSecret,omitempty"`
}

func marshalToolCreds(c toolCredJSON) string {
	b, _ := json.Marshal(c) // 基础类型结构体，marshal 不可能失败
	return string(b)
}

func (s *Service) decryptToolCreds(cipher string, out *toolCredJSON) error {
	plain, err := s.vault.Decrypt(cipher)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(plain), out); err != nil {
		return fmt.Errorf("dbtools: tool cred json: %w", err)
	}
	return nil
}

func randomHex(n int) (string, error) {
	b, err := randomBytes(n)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// basicAuthHeader Basic 认证头值（base64(user:pass)，D-08 服务端注入）。
func basicAuthHeader(user, pass string) string {
	return base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
}

// ---- 输出尾部采集（D-08 同构，I-10 ≤200 行）----

type outputRing struct{ lines []string }

func newOutputRing() *outputRing { return &outputRing{lines: []string{}} }

func (r *outputRing) append(s string) {
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		if l == "" && len(r.lines) == 0 {
			continue
		}
		r.lines = append(r.lines, l)
	}
	if len(r.lines) > TaskOutputTailLines {
		r.lines = append([]string(nil), r.lines[len(r.lines)-TaskOutputTailLines:]...)
	}
}

func (r *outputRing) String() string { return strings.Join(r.lines, "\n") }

func tailLines(s string, n int) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
