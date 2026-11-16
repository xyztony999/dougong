package panel

// 面板证书 ACME 编排（v2.0，ADR-029；api-panel-core.md §4.1.3/4.1.4 D-P13/D-P15）：
// PanelCertAcmeService——cert/request、cert/retry-renewal 与 4.1.3 每日自动续期
// 三者共用同一通道（injectChallengeLocation → AcmeGateway.Issue ≤ACME_TIMEOUT →
// 撤除+`nginx -t`）；挑战 location 临时注入经 nginx 安全写回（备份→注入→
// `nginx -t`→失败恢复原字节，R-06 四步；不新增独立 80 端口监听）。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dougong/dougong/internal/adapter/acme"
	"github.com/dougong/dougong/internal/adapter/nginxx"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storepanel "github.com/dougong/dougong/internal/store/panel"
)

// acmeMu 串行化面板证书 ACME 编排（手动 request/retry 与每日调度并发互斥，
// 同一账户与同一挑战 location 不重入）。
var acmeMu sync.Mutex

// CertStatus GET /panel/cert/status（两角色）：面板证书状态视图（来源/推算
// 状态/失败原因/ACME 概要，D-P13）；cert=null 即未配置证书；查看类不审计。
func (s *Service) CertStatus(ctx context.Context) (*CertStatusResult, error) {
	cfg, err := s.st.Config().Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("panel: cert status get config: %w", err)
	}
	res := &CertStatusResult{
		Acme: CertAcmeInfo{Domain: cfg.BindDomain, Challenge: ChallengePanelCert, AutoRenew: true},
	}
	cert, err := s.st.Cert().Get(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return res, nil // 未配置证书（cert=null）
	}
	if err != nil {
		return nil, fmt.Errorf("panel: cert status get cert: %w", err)
	}
	if st := CalcCertStatus(cert, s.now()); st != "" {
		res.Cert = &CertStatusCert{
			Source: cert.Source, Status: &st, Issuer: cert.Issuer,
			NotBefore: cert.NotBefore, NotAfter: cert.NotAfter, LastError: cert.LastError,
		}
	} else {
		res.Cert = &CertStatusCert{ // source≠acme：status=null（无续期语义，D-P13）
			Source: cert.Source, Issuer: cert.Issuer,
			NotBefore: cert.NotBefore, NotAfter: cert.NotAfter, LastError: cert.LastError,
		}
	}
	return res, nil
}

// RequestCert POST /panel/cert/request（admin，D-P13/D-P15）：校验链
// bind_domain 非空（PANEL_007）→ nginx 可用（PANEL_008）→ 防重 PANEL_011
// （source=acme 且推算 valid/expiring_soon；self_signed/imported 或无行=替换式
// 放行）；执行同步 ≤ACME_TIMEOUT；成功 UPSERT panel_cert（source=acme）+
// https_mode='acme' 联动（D-P14）+ 审计 panel_cert_requested；失败不落证书行
// （D-P13 首次申请失败无行），审计 failure 并返回 PANEL_008/009（data.reason）。
func (s *Service) RequestCert(ctx context.Context, actor *authsvc.Identity) (*CertAcmeResult, error) {
	domain, err := s.requireBoundDomain(ctx)
	if err != nil {
		return nil, err // PANEL_007
	}
	// 防重（PANEL_011）：source=acme 且推算 ∈ {valid, expiring_soon}（renew_failed/
	// expired 不阻塞重新申请；self_signed/imported 行=替换式放行；无行=放行）。
	cert, cerr := s.st.Cert().Get(ctx)
	switch {
	case cerr != nil && !errors.Is(cerr, sql.ErrNoRows):
		return nil, fmt.Errorf("panel: cert request get cert: %w", cerr)
	case cerr == nil && cert.Source == storepanel.CertSourceAcme:
		if st := CalcCertStatus(cert, s.now()); st == storepanel.CertStatusValid ||
			st == storepanel.CertStatusExpiringSoon {
			if cert.NotAfter != nil {
				return nil, errcode.ErrAcmeCertExists.WithData(NotAfterData{NotAfter: *cert.NotAfter})
			}
			return nil, errcode.ErrAcmeCertExists
		}
	}

	res, failErr := s.runAcmeIssuance(ctx, domain)
	if failErr != nil {
		s.auditAcmeAction(ctx, actor, ActionPanelCertRequested, false, domain, failErr)
		return nil, failErr
	}
	// 成功：证书行 + https_mode 联动 + 审计同事务（I-P10/D-P14）。
	notBefore, notAfter := res.NotBefore, res.NotAfter
	issuer := res.Issuer
	err = s.withTx(ctx, func(tx *storepanel.TxStore) error {
		if err := tx.Cert().Upsert(ctx, storepanel.Cert{
			CertPEM: string(res.CertPEM), KeyPEM: string(res.KeyPEM),
			Source: storepanel.CertSourceAcme, Issuer: &issuer,
			NotBefore: &notBefore, NotAfter: &notAfter,
		}, s.now()); err != nil {
			return fmt.Errorf("panel: upsert acme cert: %w", err)
		}
		if err := tx.Config().Update(ctx, storepanel.ConfigChanges{
			HTTPSMode: strPtr(storepanel.HTTPSModeAcme), // D-P14 唯一自动切换路径
		}, s.now()); err != nil {
			return fmt.Errorf("panel: link https_mode=acme: %w", err)
		}
		return tx.Audit().Append(ctx, auditEntry(actor, ActionPanelCertRequested, ObjectTypePanelCert,
			map[string]any{"domain": domain, "result": audit.ResultSuccess}), s.now())
	})
	if err != nil {
		return nil, err
	}
	return &CertAcmeResult{
		Cert: CertAcmeResultCert{
			Source: storepanel.CertSourceAcme, Status: storepanel.CertStatusValid,
			Issuer: &issuer, NotBefore: &notBefore, NotAfter: &notAfter,
		},
		HTTPSMode: storepanel.HTTPSModeAcme,
	}, nil
}

// RetryRenewal POST /panel/cert/retry-renewal（admin，4.1.3 同一通道）：校验链
// panel_cert 行存在且 source=acme（PANEL_010 data.currentSource）→ 推算态=
// renew_failed（valid/expiring_soon 走自动调度、expired 重新 request，
// PANEL_010 data.currentStatus）→ bind_domain 非空（PANEL_007）；执行同步
// ≤ACME_TIMEOUT（续期域名=当前 bind_domain，域名变更后续期即换域名证书，
// D-P04 替换式）；成功回写证书行（last_error=NULL，推算回 valid）+ 审计；
// 失败经 UpdateAcmeResult 落 last_error（保持 renew_failed 呈现）+ 审计 + PANEL_009。
func (s *Service) RetryRenewal(ctx context.Context, actor *authsvc.Identity) (*CertAcmeResult, error) {
	cert, err := s.st.Cert().Get(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errcode.ErrAcmeStateInvalid.WithData(CurrentSourceData{CurrentSource: nil})
	}
	if err != nil {
		return nil, fmt.Errorf("panel: retry renewal get cert: %w", err)
	}
	if cert.Source != storepanel.CertSourceAcme {
		src := cert.Source
		return nil, errcode.ErrAcmeStateInvalid.WithData(CurrentSourceData{CurrentSource: &src})
	}
	status := CalcCertStatus(cert, s.now())
	if status != storepanel.CertStatusRenewFailed {
		return nil, errcode.ErrAcmeStateInvalid.WithData(CurrentStatusData{CurrentStatus: status})
	}
	domain, err := s.requireBoundDomain(ctx)
	if err != nil {
		return nil, err // PANEL_007
	}
	res, failErr := s.runAcmeIssuance(ctx, domain)
	if failErr != nil {
		s.recordRenewalFailure(ctx, actor, ActionPanelCertRenewalRetried, domain, failErrReason(failErr))
		return nil, errcode.ErrPanelAcmeFailed.WithData(ReasonData{Reason: failErrReason(failErr)})
	}
	notBefore, notAfter := res.NotBefore, res.NotAfter
	issuer := res.Issuer
	err = s.withTx(ctx, func(tx *storepanel.TxStore) error {
		// 续期产物整行替换（D-P04 替换式：lego 每次签发新密钥对，材料必须随行
		// 更新——not_before/not_after/issuer/last_error=NULL 一并落定，SQL #6 语义
		// 由 Upsert 覆盖）。
		if err := tx.Cert().Upsert(ctx, storepanel.Cert{
			CertPEM: string(res.CertPEM), KeyPEM: string(res.KeyPEM),
			Source: storepanel.CertSourceAcme, Issuer: &issuer,
			NotBefore: &notBefore, NotAfter: &notAfter,
		}, s.now()); err != nil {
			return fmt.Errorf("panel: retry renewal upsert: %w", err)
		}
		return tx.Audit().Append(ctx, auditEntry(actor, ActionPanelCertRenewalRetried, ObjectTypePanelCert,
			map[string]any{"domain": domain, "result": audit.ResultSuccess}), s.now())
	})
	if err != nil {
		return nil, err
	}
	return &CertAcmeResult{
		Cert: CertAcmeResultCert{
			Source: storepanel.CertSourceAcme, Status: storepanel.CertStatusValid,
			Issuer: &issuer, NotBefore: &notBefore, NotAfter: &notAfter,
		},
		HTTPSMode: storepanel.HTTPSModeAcme,
	}, nil
}

// RunPanelRenewalScheduler 阻塞运行面板证书自动续期调度（4.1.3：启动 1 分钟
// 后首轮 + 24h ticker；panel_cert.source=acme 且读时推算 expiring_soon 时发起
// 单次尝试；失败转 renew_failed 等待次日或手动 retry，不自动无限重试）。
// goroutine 由 main 统一编排（ctx 取消退出，G-06）；单轮 panic 隔离。
func (s *Service) RunPanelRenewalScheduler(ctx context.Context) error {
	select {
	case <-time.After(time.Minute):
	case <-ctx.Done():
		return nil
	}
	for {
		s.runPanelRenewalPass(ctx)
		select {
		case <-time.After(24 * time.Hour):
		case <-ctx.Done():
			return nil
		}
	}
}

// runPanelRenewalPass 单轮调度：读证书行与 bind_domain，expiring_soon 的 acme
// 证书发起单次续期（4.1.3 同一通道）；成功/失败均写审计 panel_cert_renewed
// （source=system，R-03）；nginx 不可用（PANEL_008 语义）记 last_error 跳过。
func (s *Service) runPanelRenewalPass(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil { // 单轮 panic 隔离（调度循环存活）
			slog.ErrorContext(ctx, "panel: cert renewal pass panic", "panic", r)
		}
	}()
	cert, err := s.st.Cert().Get(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return
	}
	if err != nil {
		slog.ErrorContext(ctx, "panel: renewal pass get cert", "err", err)
		return
	}
	if cert.Source != storepanel.CertSourceAcme ||
		CalcCertStatus(cert, s.now()) != storepanel.CertStatusExpiringSoon {
		return // 仅 expiring_soon 发起（valid 未到窗、renew_failed 等手动/次日、expired 重新申请）
	}
	domain, err := s.BindDomainState(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "panel: renewal pass get bind domain", "err", err)
		return
	}
	if domain == "" { // 续期域名=bind_domain（4.1.3 输入）；未绑定即无法续期，如实落 last_error
		reason := "bind_domain 未绑定，无法自动续期"
		s.recordRenewalFailure(ctx, nil, ActionPanelCertRenewed, "", reason)
		return
	}
	res, failErr := s.runAcmeIssuance(ctx, domain)
	if failErr != nil {
		s.recordRenewalFailure(ctx, nil, ActionPanelCertRenewed, domain, failErrReason(failErr))
		return
	}
	notBefore, notAfter := res.NotBefore, res.NotAfter
	issuer := res.Issuer
	err = s.withTx(ctx, func(tx *storepanel.TxStore) error {
		if err := tx.Cert().Upsert(ctx, storepanel.Cert{
			CertPEM: string(res.CertPEM), KeyPEM: string(res.KeyPEM),
			Source: storepanel.CertSourceAcme, Issuer: &issuer,
			NotBefore: &notBefore, NotAfter: &notAfter,
		}, s.now()); err != nil {
			return fmt.Errorf("panel: renewal upsert: %w", err)
		}
		return tx.Audit().Append(ctx, auditSystemEntry(ActionPanelCertRenewed, ObjectTypePanelCert,
			map[string]any{"domain": domain, "result": audit.ResultSuccess}), s.now())
	})
	if err != nil {
		slog.ErrorContext(ctx, "panel: certificate renewal finalize", "domain", domain, "err", err)
		return
	}
	slog.InfoContext(ctx, "panel: certificate auto renewed", "domain", domain)
}

// recordRenewalFailure 续期失败落库（SQL #7：last_error，renew_failed 呈现，
// D-P13）+ 审计 failure（与落库同事务）。
func (s *Service) recordRenewalFailure(ctx context.Context, actor *authsvc.Identity,
	action, domain, reason string) {
	now := s.now()
	err := s.withTx(ctx, func(tx *storepanel.TxStore) error {
		if err := tx.Cert().UpdateAcmeResult(ctx, nil, nil, nil, &reason, now); err != nil {
			return fmt.Errorf("panel: renewal failure persist: %w", err)
		}
		detail := map[string]any{"result": audit.ResultFailure, "reason": reason}
		if domain != "" {
			detail["domain"] = domain
		}
		var entry audit.Entry
		if actor != nil {
			entry = auditEntry(actor, action, ObjectTypePanelCert, detail)
		} else {
			entry = auditSystemEntry(action, ObjectTypePanelCert, detail)
		}
		entry.Result = audit.ResultFailure
		return tx.Audit().Append(ctx, entry, now)
	})
	if err != nil { // 落库失败仅日志（调度/响应主语义已定），下次读态重算
		slog.ErrorContext(ctx, "panel: renewal failure finalize", "action", action, "err", err)
	}
}

// auditAcmeAction 手动通道失败审计（request 失败不落证书行，D-P13——审计
// 独立写入，重试一次后降级日志，沿 D-06 口径）。
func (s *Service) auditAcmeAction(ctx context.Context, actor *authsvc.Identity,
	action string, success bool, domain string, failErr *errcode.Error) {
	result := audit.ResultSuccess
	detail := map[string]any{"domain": domain, "result": result}
	if !success {
		result = audit.ResultFailure
		detail = map[string]any{"domain": domain, "result": result, "reason": failErrReason(failErr)}
	}
	entry := auditEntry(actor, action, ObjectTypePanelCert, detail)
	entry.Result = result
	if err := s.st.Audit().Append(ctx, entry, s.now()); err != nil {
		if err := s.st.Audit().Append(ctx, entry, s.now()); err != nil {
			slog.WarnContext(ctx, "panel: acme audit record degraded to runtime log",
				"action", action, "err", err)
		}
	}
}

// requireBoundDomain 前置 bind_domain 非空（PANEL_007，D-P12/D-P13）。
func (s *Service) requireBoundDomain(ctx context.Context) (string, error) {
	domain, err := s.BindDomainState(ctx)
	if err != nil {
		return "", fmt.Errorf("panel: get bind domain: %w", err)
	}
	if domain == "" {
		return "", errcode.ErrBindDomainRequired
	}
	return domain, nil
}

// runAcmeIssuance 签发/续期共用通道（request / retry-renewal / 4.1.3 调度统一，
// D-P13）：挑战 location 注入（D-P15 落点，R-06 备份→注入→`nginx -t`→生效
// reload）→ AcmeGateway.Issue（≤ACME_TIMEOUT 同步）→ 撤除（恢复原字节 +
// `nginx -t` + reload，成败皆然）；失败返回 PANEL_008（通道/落点不可用）或
// PANEL_009（签发失败），均携带 data.reason。
func (s *Service) runAcmeIssuance(ctx context.Context, domain string) (*acme.IssueResult, *errcode.Error) {
	if err := s.requirePanelAcmeChannel(); err != nil {
		return nil, err // PANEL_008
	}
	acmeMu.Lock()
	defer acmeMu.Unlock()
	webroot, cleanup, perr := s.injectChallengeLocation(ctx, domain)
	if perr != nil {
		return nil, errcode.ErrPanelNginxUnavailable.WithData(ReasonData{Reason: perr.Error()})
	}
	actx, cancel := context.WithTimeout(ctx, AcmeTimeout)
	res, ierr := s.acme.Issue(actx, []string{domain}, ChallengePanelCert, webroot)
	cancel()
	cleanup() // 撤除挑战 location（恢复原状；失败在内部记日志并尽力回滚，I-P10）
	if ierr != nil {
		slog.WarnContext(ctx, "panel: acme issue failed", "domain", domain, "err", ierr)
		return nil, errcode.ErrPanelAcmeFailed.WithData(ReasonData{Reason: ierr.Error()})
	}
	return res, nil
}

// requirePanelAcmeChannel 面板 ACME 通道可用性前置（PANEL_008 data.reason）：
// 通道未装配（测试/裁配环境）或 nginx 二进制未检出均拒绝。
func (s *Service) requirePanelAcmeChannel() *errcode.Error {
	if s.acme == nil || s.nginx == nil {
		return errcode.ErrPanelNginxUnavailable.WithData(ReasonData{Reason: "面板证书 ACME 通道未装配"})
	}
	if err := s.nginx.Available(); err != nil {
		return errcode.ErrPanelNginxUnavailable.WithData(ReasonData{Reason: err.Error()})
	}
	return nil
}

// injectChallengeLocation D-P15 挑战 location 注入：落点选择（优先 server_name
// 匹配 bind_domain 的 80 端口 server 块 → listen 80 default_server → 首个
// listen 80 块；跨惯例目录按扫描序）→ 既有 acme-challenge location 则复用其
// root/alias 派生 webroot（零写回，A-SM10 先例）→ 否则注入临时 location
// （alias 指向面板挑战目录）经 R-06 四步写回并 reload 生效；返回 webroot 与
// cleanup（撤除+`nginx -t`+reload，恢复原字节；回滚失败记日志不掩盖主错误）。
func (s *Service) injectChallengeLocation(ctx context.Context, domain string) (string, func(), error) {
	files, err := s.nginx.ScanConfDirs()
	if err != nil {
		return "", nil, fmt.Errorf("扫描 nginx 配置目录: %w", err)
	}
	type candidate struct {
		file  nginxx.FileSnapshot
		tree  *nginxx.Tree
		block *nginxx.ServerBlock
	}
	var nameMatch, defaultMatch, first80 *candidate
	for i := range files {
		tree := nginxx.Parse(files[i].Content)
		for _, blk := range nginxx.ExtractServers(tree) {
			if !listensOnPort80(blk) {
				continue
			}
			c := &candidate{file: files[i], tree: tree, block: blk}
			if first80 == nil {
				first80 = c
			}
			if hasDefaultServerFlag(blk) && defaultMatch == nil {
				defaultMatch = c
			}
			if nameMatch == nil && serverNameMatches(blk, domain) {
				nameMatch = c
			}
		}
	}
	target := nameMatch
	if target == nil {
		target = defaultMatch
	}
	if target == nil {
		target = first80
	}
	if target == nil {
		return "", nil, fmt.Errorf("未发现监听 80 端口的 server 块（%s）", domain)
	}

	// 既有 acme-challenge location → 复用（零写回）。
	if loc := findPanelAcmeLocation(target.block.Node); loc != nil {
		if root := panelDirectiveArg(loc, "root"); root != "" {
			return root, func() {}, nil
		}
		if alias := panelDirectiveArg(loc, "alias"); alias != "" {
			if parent, ok := stripChallengeSuffix(alias); ok {
				return parent, func() {}, nil
			}
		}
		return "", nil, fmt.Errorf("目标 server 块已有不兼容的 acme-challenge location，无法注入")
	}

	// 注入：备份 → 原子写 → `nginx -t`（失败恢复原字节）→ reload 生效。
	orig, _, err := s.nginx.ReadFile(target.file.Path)
	if err != nil {
		return "", nil, fmt.Errorf("读取目标配置 %s: %w", target.file.Path, err)
	}
	challengeDir := filepath.Join(s.webrootDir, ".well-known", "acme-challenge")
	if err := s.nginx.MkdirAll(challengeDir); err != nil {
		return "", nil, fmt.Errorf("创建挑战目录: %w", err)
	}
	bak, err := s.challengeBackupPath(target.file.Path)
	if err != nil {
		return "", nil, err
	}
	if err := s.nginx.CopyFile(target.file.Path, bak); err != nil {
		return "", nil, fmt.Errorf("写前备份: %w", err)
	}

	indent := target.tree.IndentOf(target.block.Node)
	insert := "\n" + indent + "    location ^~ /.well-known/acme-challenge/ {\n" +
		indent + "        alias " + filepath.ToSlash(challengeDir) + "/;\n" +
		indent + "    }"
	if err := target.tree.Apply(target.tree.EndOfBlockBody(target.block.Node),
		target.tree.EndOfBlockBody(target.block.Node), insert); err != nil {
		return "", nil, fmt.Errorf("注入挑战 location: %w", err)
	}
	content, err := target.tree.Serialize()
	if err != nil {
		return "", nil, fmt.Errorf("序列化注入结果: %w", err)
	}
	restore := func() error { // 恢复原字节 + `nginx -t`（异常再回滚备份）
		if err := s.nginx.WriteFileAtomic(target.file.Path, []byte(orig)); err != nil {
			if rerr := s.nginx.CopyFile(bak, target.file.Path); rerr != nil {
				return fmt.Errorf("撤除失败（备份回滚亦失败，配置可能残留挑战 location）: %v / %v", err, rerr)
			}
			return fmt.Errorf("撤除失败（已回滚备份）: %v", err)
		}
		if tr, terr := s.nginx.TestConfig(ctx); terr != nil || !tr.OK {
			if rerr := s.nginx.CopyFile(bak, target.file.Path); rerr != nil {
				return fmt.Errorf("撤除后 nginx -t 未通过且回滚失败: %v", rerr)
			}
			return fmt.Errorf("撤除后 nginx -t 未通过（已回滚备份）")
		}
		if out, rerr := s.nginx.Reload(ctx); rerr != nil {
			return fmt.Errorf("撤除后 reload 失败（配置已恢复，下次重载生效）: %v / %s", rerr, out)
		}
		return nil
	}

	if err := s.nginx.WriteFileAtomic(target.file.Path, []byte(content)); err != nil {
		return "", nil, fmt.Errorf("写入挑战 location: %w", err)
	}
	if tr, terr := s.nginx.TestConfig(ctx); terr != nil || !tr.OK {
		if rerr := restore(); rerr != nil {
			slog.ErrorContext(ctx, "panel: challenge inject rollback failed", "err", rerr)
		}
		return "", nil, fmt.Errorf("注入后 nginx -t 未通过，已恢复原状")
	}
	if out, rerr := s.nginx.Reload(ctx); rerr != nil {
		if rerr2 := restore(); rerr2 != nil {
			slog.ErrorContext(ctx, "panel: challenge inject rollback failed", "err", rerr2)
		}
		return "", nil, fmt.Errorf("注入后 reload 失败: %v / %s", rerr, out)
	}
	cleanup := func() {
		if rerr := restore(); rerr != nil { // 撤除失败：尽力回滚已做，日志留痕（I-P10）
			slog.ErrorContext(ctx, "panel: challenge location remove failed", "err", rerr)
		}
	}
	return filepath.ToSlash(s.webrootDir), cleanup, nil
}

// challengeBackupPath 挑战注入写前备份路径（{backupRoot}/{文件基名}/{unixnano}.conf，
// R-06 同法于 site-mgmt backupPathFor）。
func (s *Service) challengeBackupPath(file string) (string, error) {
	bucket := filepath.Join(s.backupRoot, filepath.Base(file))
	if err := s.nginx.MkdirAll(bucket); err != nil {
		return "", fmt.Errorf("创建备份目录: %w", err)
	}
	return filepath.Join(bucket, strconv.FormatInt(s.now().UnixNano(), 10)+".conf"), nil
}

// ---- D-P15 落点判定与 location 复用纯函数 ----

// listensOnPort80 报告 server 块是否监听 80 端口（listen 声明首参为 80 或
// 以 :80 结尾——含 [::]:80）。
func listensOnPort80(blk *nginxx.ServerBlock) bool {
	for _, l := range blk.Listen {
		fields := strings.Fields(l)
		if len(fields) == 0 {
			continue
		}
		bind := fields[0]
		if bind == "80" || strings.HasSuffix(bind, ":80") {
			return true
		}
	}
	return false
}

// hasDefaultServerFlag 报告 80 端口 listen 声明是否携带 default_server。
func hasDefaultServerFlag(blk *nginxx.ServerBlock) bool {
	for _, l := range blk.Listen {
		fields := strings.Fields(l)
		if len(fields) > 1 && (fields[0] == "80" || strings.HasSuffix(fields[0], ":80")) {
			for _, f := range fields[1:] {
				if f == "default_server" || f == "default" {
					return true
				}
			}
		}
	}
	return false
}

// serverNameMatches 报告 server_name 集合是否包含目标域名（大小写不敏感）。
func serverNameMatches(blk *nginxx.ServerBlock, domain string) bool {
	for _, n := range blk.ServerNames {
		if strings.EqualFold(n, domain) {
			return true
		}
	}
	return false
}

// findPanelAcmeLocation 定位既有 acme-challenge location 块（复用判定，
// site-mgmt findAcmeLocation 同法）。
func findPanelAcmeLocation(block *nginxx.Node) *nginxx.Node {
	for _, ch := range block.Children {
		if ch.Kind == nginxx.KindBlock && strings.EqualFold(ch.Name, "location") &&
			strings.Contains(strings.Join(ch.Args, " "), ".well-known/acme-challenge") {
			return ch
		}
	}
	return nil
}

// panelDirectiveArg 取块内首条同名指令首参。
func panelDirectiveArg(n *nginxx.Node, name string) string {
	for _, ch := range n.Children {
		if ch.Kind == nginxx.KindDirective && ch.Name == name && len(ch.Args) > 0 {
			return ch.Args[0]
		}
	}
	return ""
}

// stripChallengeSuffix 从 alias 值派生 webroot：alias 以 /.well-known/acme-challenge
// 结尾时返回其父目录，否则报告不可派生。
func stripChallengeSuffix(alias string) (string, bool) {
	const suffix = "/.well-known/acme-challenge"
	a := strings.TrimSuffix(filepath.ToSlash(strings.TrimRight(alias, "/")), "/")
	if strings.HasSuffix(a, suffix) {
		return strings.TrimSuffix(a, suffix), true
	}
	return "", false
}

// failErrReason 提取 *errcode.Error 携带的 reason（PANEL_008/009 data.reason）。
func failErrReason(e *errcode.Error) string {
	if d, ok := e.Data().(ReasonData); ok {
		return d.Reason
	}
	return e.Message()
}

// strPtr 字符串指针速记。
func strPtr(v string) *string { return &v }
