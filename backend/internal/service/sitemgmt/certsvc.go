package sitemgmt

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dougong/dougong/internal/adapter/nginxx"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storesitemgmt "github.com/dougong/dougong/internal/store/sitemgmt"
)

// 证书六接口编排（api §4.1.4）：list（D-08 推算 + D-11 引用推导）/request
// （D-09/D-10）/deploy/undeploy（I-05/D-11，经 WritePipeline）/retry-renewal
// （进入 4.1.3 同一通道）/delete（I-02 守卫 + I-04 归属）。certificate 表写与
// 审计同事务（D-12 例外）。

// CertList GET /sites/cert/list：读全表 → 逐行推算（imported 元数据读时刷新）
// → 引用推导 → 筛选（source/status 推算后过滤）→ not_after 升序。
func (s *Service) CertList(ctx context.Context, source, status string) (*CertListData, error) {
	if source != "" && source != storesitemgmt.SourceAcme && source != storesitemgmt.SourceImported {
		return nil, fieldError("source", "source 筛选值非法（acme|imported）")
	}
	knownStatus := map[string]bool{
		storesitemgmt.StatusPending: true, storesitemgmt.StatusValid: true,
		storesitemgmt.StatusExpiringSoon: true, storesitemgmt.StatusExpired: true,
		storesitemgmt.StatusRenewFailed: true,
	}
	if status != "" && !knownStatus[status] {
		return nil, fieldError("status", "status 筛选值非法（cert_status 取值域）")
	}
	rows, err := s.store.Cert().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("site-mgmt: cert list: %w", err)
	}
	scan, err := s.scanSites(ctx)
	if err != nil {
		return nil, err
	}
	now := s.clock()
	items := make([]CertItem, 0, len(rows))
	for _, row := range rows {
		c := s.computeCert(row, now)
		if source != "" && row.Source != source {
			continue
		}
		if status != "" && c.Status != status {
			continue
		}
		items = append(items, CertItem{
			ID:              row.ID,
			PrimaryDomain:   row.PrimaryDomain,
			Domains:         nonEmpty(row.Domains),
			Issuer:          c.Issuer,
			Source:          row.Source,
			Status:          c.Status,
			Challenge:       row.Challenge,
			NotBefore:       timePtrStr(c.NotBefore),
			NotAfter:        timePtrStr(c.NotAfter),
			OriginPath:      row.OriginPath,
			LastError:       row.LastError,
			ReferencedSites: referencedSites(scan, row),
			CreatedAt:       formatTS(row.CreatedAt),
		})
	}
	sortCertItems(items)
	return &CertListData{Items: items, GeneratedAt: formatTS(now)}, nil
}

// CertRequest POST /sites/cert/request：siteId/challenge 校验（CERT_007）→
// I-03 同域名防重（CERT_003）→ nginx 可用（SITE_012，80/443 通路经既有站点
// 配置）→ INSERT pending（机制无审计，A-SM9）→ AcmeGateway 签发（同步 ≤120s，
// http-01 经 A-SM10 临时 location 注入/移除）→ 终态落库与审计同事务。
func (s *Service) CertRequest(ctx context.Context, actor *authsvc.Identity, req CertRequestRequest) (*CertRequestData, error) {
	entry, err := s.resolveSite(ctx, req.SiteID)
	if err != nil {
		return nil, err
	}
	switch req.Challenge {
	case storesitemgmt.ChallengeHTTP01, storesitemgmt.ChallengeTLSALPN:
	default:
		return nil, errcode.ErrInvalidChallenge
	}
	if e := validateDomainList(entry.Block.ServerNames); e != nil {
		return nil, e
	}
	domains := append([]string(nil), entry.Block.ServerNames...)
	primary := domains[0]
	if existing := s.findActiveByDomains(ctx, domains); existing != nil {
		return nil, errcode.ErrCertDuplicate.WithData(ExistingCertData{ExistingCertID: existing.ID})
	}
	if err := s.requireNginx(); err != nil {
		return nil, err
	}
	fullchain, _ := s.certStoragePaths(primary)
	sp := fullchain
	id, err := s.store.Cert().Create(ctx, storesitemgmt.Certificate{
		PrimaryDomain: primary, Domains: domains,
		Source: storesitemgmt.SourceAcme, Status: storesitemgmt.StatusPending,
		Challenge: &req.Challenge, StoragePath: &sp,
	})
	if err != nil {
		return nil, fmt.Errorf("site-mgmt: cert create pending: %w", err)
	}
	return s.issueForSite(ctx, actor, entry, id, domains, req.Challenge,
		AuditActionCertRequested, audit.SourceWeb)
}

// issueForSite 签发/续期共用通道（request 与 4.1.3/retry 统一 AcmeGateway，
// D-09）：http-01 临时 location 注入 → Issue → 恢复 → 托管文件写 → 终态落库
// +审计同事务。resultAction/auditSource 随调用方（web/system）。
func (s *Service) issueForSite(ctx context.Context, actor *authsvc.Identity, entry *SiteEntry,
	certID int64, domains []string, challenge, resultAction, auditSource string) (*CertRequestData, error) {
	actorName, actorID := actorIdentity(actor)
	if auditSource == "" {
		auditSource = audit.SourceWeb
	}
	cleanup := func() {}
	webroot := s.panelWebroot()
	if challenge == storesitemgmt.ChallengeHTTP01 && entry != nil {
		c, err := s.prepareHTTP01(ctx, entry)
		if err != nil {
			return nil, err
		}
		cleanup = c
		webroot = s.acmeWebroot(entry, challenge)
	}
	actx, cancel := context.WithTimeout(ctx, ACMETimeout)
	res, ierr := s.acme.Issue(actx, domains, challenge, webroot)
	cancel()
	cleanup() // 移除临时 location（恢复原样）——先于终态落库
	if ierr != nil {
		reason := ierr.Error()
		if err := s.finalizeRenewal(ctx, certID, storesitemgmt.StatusRenewFailed, &reason,
			nil, nil, "", "", resultAction, auditSource, actorName, actorID,
			map[string]any{"challenge": challenge, "error": errcode.ErrAcmeFailed.Symbol(), "reason": reason}); err != nil {
			return nil, err
		}
		return nil, errcode.ErrAcmeFailed.WithData(ReasonData{Reason: reason})
	}
	fullchain, keyPath := s.certStoragePaths(domains[0])
	if err := s.gw.MkdirAll(s.certsDir); err != nil {
		return nil, fmt.Errorf("site-mgmt: certs dir: %w", err)
	}
	if err := s.gw.WriteSecretFileAtomic(fullchain, res.CertPEM); err != nil {
		return nil, fmt.Errorf("site-mgmt: write cert file: %w", err)
	}
	if err := s.gw.WriteSecretFileAtomic(keyPath, res.KeyPEM); err != nil {
		return nil, fmt.Errorf("site-mgmt: write key file: %w", err)
	}
	if err := s.finalizeRenewal(ctx, certID, storesitemgmt.StatusValid, nil,
		&res.NotBefore, &res.NotAfter, res.Issuer, res.AccountURI,
		resultAction, auditSource, actorName, actorID,
		map[string]any{"challenge": challenge, "result": audit.ResultSuccess}); err != nil {
		return nil, err
	}
	return &CertRequestData{CertID: certID, Status: storesitemgmt.StatusValid,
		NotAfter: formatTS(res.NotAfter)}, nil
}

// finalizeRenewal 证书终态落库与审计同事务（D-12 例外；成功/失败均恰好一条）。
func (s *Service) finalizeRenewal(ctx context.Context, certID int64, status string, lastErr *string,
	notBefore, notAfter *time.Time, issuer, accountURI, resultAction, auditSource, actorName string,
	actorID *int64, detail map[string]any) error {
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("site-mgmt: begin finalize tx: %w", err)
	}
	defer func() {
		if rerr := tx.Rollback(); rerr != nil && !errors.Is(rerr, sql.ErrTxDone) {
			slog.WarnContext(ctx, "site-mgmt: finalize tx rollback", "cert", certID, "err", rerr)
		}
	}()
	if status == storesitemgmt.StatusValid {
		if err := tx.Cert().UpdateRenewalMeta(ctx, certID, status, lastErr, notBefore, notAfter, issuer); err != nil {
			return fmt.Errorf("site-mgmt: finalize renewal meta: %w", err)
		}
		if accountURI != "" {
			if err := tx.Cert().SetAccountRef(ctx, certID, accountURI); err != nil {
				return fmt.Errorf("site-mgmt: finalize account ref: %w", err)
			}
		}
	} else {
		if err := tx.Cert().UpdateStatus(ctx, certID, status, lastErr); err != nil {
			return fmt.Errorf("site-mgmt: finalize status: %w", err)
		}
	}
	result := audit.ResultSuccess
	if status != storesitemgmt.StatusValid {
		result = audit.ResultFailure
	}
	entry := audit.Entry{
		ActorID: actorID, ActorName: actorName,
		Category: audit.CategoryChange, Action: resultAction,
		ObjectType: ObjectTypeCertificate, ObjectID: strconv.FormatInt(certID, 10),
		Detail: detail, Result: result, Source: auditSource,
	}
	if err := tx.Audit().Append(ctx, entry, s.clock()); err != nil {
		return fmt.Errorf("site-mgmt: finalize audit: %w", err)
	}
	return tx.Commit()
}

// findActiveByDomains I-03：同域名集合的 active 证书（D-10；renew_failed/
// expired 不阻塞重新申请）。
func (s *Service) findActiveByDomains(ctx context.Context, domains []string) *storesitemgmt.Certificate {
	rows, err := s.store.Cert().ListActive(ctx)
	if err != nil {
		slog.WarnContext(ctx, "site-mgmt: list active certs failed", "err", err)
		return nil
	}
	for i := range rows {
		if domainSetEqual(rows[i].Domains, domains) {
			return &rows[i]
		}
	}
	return nil
}

// prepareHTTP01 A-SM10：已有 /.well-known/acme-challenge/ location → 复用其
// root 为 webroot（零写回）；无 → 经 WritePipeline 注入临时 location（root=
// 面板 webroot，^~ 前缀）→ 返回 cleanup（恢复原字节，同管线）。
func (s *Service) prepareHTTP01(ctx context.Context, entry *SiteEntry) (func(), error) {
	if loc := findAcmeLocation(entry.Block.Node); loc != nil {
		if root := directiveArgIn(loc, "root"); root != "" {
			return func() {}, nil // 复用既有 location（其 root 即挑战文件落点）
		}
	}
	orig, _, err := s.gw.ReadFile(entry.File)
	if err != nil {
		return nil, fmt.Errorf("site-mgmt: read for challenge: %w", err)
	}
	webroot := s.panelWebroot()
	if err := s.gw.MkdirAll(webroot); err != nil {
		return nil, fmt.Errorf("site-mgmt: webroot mkdir: %w", err)
	}
	tgt, _, err := s.openEdit(ctx, entry.SiteID)
	if err != nil {
		return nil, err
	}
	indent := indentOfBlock(tgt.tree, tgt.block)
	loc := indent + "location ^~ /.well-known/acme-challenge/ {\n" +
		indent + "    root " + webroot + ";\n" +
		indent + "}"
	pos := insertPos(tgt.tree, tgt.block)
	if err := tgt.tree.Apply(pos, pos, loc+"\n"+indent); err != nil {
		return nil, fmt.Errorf("site-mgmt: inject challenge location: %w", err)
	}
	content, err := tgt.serialize()
	if err != nil {
		return nil, err
	}
	if err := s.applyPipeline(ctx, entry.File, content, false); err != nil {
		return nil, err
	}
	return func() { // 恢复原字节（同管线；失败记日志——配置保持注入态可服务，排障可辨）
		if rerr := s.applyPipeline(context.WithoutCancel(ctx), entry.File, orig, false); rerr != nil {
			slog.ErrorContext(ctx, "site-mgmt: challenge location restore failed",
				"file", entry.File, "err", rerr)
		}
	}, nil
}

// acmeWebroot Issue 调用的 webroot 实参（http-01；复用既有 location root 时
// 取该值，否则面板 webroot；tls_alpn 忽略）。
func (s *Service) acmeWebroot(entry *SiteEntry, challenge string) string {
	if challenge != storesitemgmt.ChallengeHTTP01 {
		return ""
	}
	if loc := findAcmeLocation(entry.Block.Node); loc != nil {
		if root := directiveArgIn(loc, "root"); root != "" {
			return root
		}
	}
	return s.panelWebroot()
}

// findAcmeLocation 定位既有 acme-challenge location 块（A-SM10 ①复用判定）。
func findAcmeLocation(block *nginxx.Node) *nginxx.Node {
	for _, ch := range block.Children {
		if ch.Kind == nginxx.KindBlock && strings.EqualFold(ch.Name, "location") &&
			strings.Contains(strings.Join(ch.Args, " "), ".well-known/acme-challenge") {
			return ch
		}
	}
	return nil
}

// directiveArgIn 取块内首条同名指令首参。
func directiveArgIn(n *nginxx.Node, name string) string {
	if d := findDirective(n, name); d != nil && len(d.Args) > 0 {
		return d.Args[0]
	}
	return ""
}

// CertDeploy POST /sites/cert/deploy：证书存在（CERT_001）且状态推算 ∈
// {valid, expiring_soon}（CERT_002 data.currentStatus）→ 站点存在（SITE_002）
// → 快照原引用入 deploy_prev_refs（I-05）→ WritePipeline 写回 ssl_cert 引用
// → reload；幂等（引用已为目标值直接成功）。
func (s *Service) CertDeploy(ctx context.Context, actor *authsvc.Identity, req CertRefRequest) (*CertRefData, error) {
	row, err := s.certByID(ctx, req.CertID)
	if err != nil {
		return nil, err
	}
	status := s.computeStatus(row, s.clock())
	if status != storesitemgmt.StatusValid && status != storesitemgmt.StatusExpiringSoon {
		return nil, errcode.ErrCertInvalidState.WithData(StatusData{CurrentStatus: status})
	}
	if row.StoragePath == nil || *row.StoragePath == "" {
		return nil, errcode.ErrCertInvalidState.WithData(StatusData{CurrentStatus: status})
	}
	entry, err := s.resolveSite(ctx, req.SiteID)
	if err != nil {
		return nil, err
	}
	if err := s.requireNginx(); err != nil {
		return nil, err
	}
	if entry.Block.SSLCert == *row.StoragePath { // 幂等：引用已为目标值
		return &CertRefData{CertID: req.CertID, SiteID: req.SiteID, CertRef: *row.StoragePath}, nil
	}
	prev := storesitemgmt.DeployRef{Cert: entry.Block.SSLCert, Key: entry.Block.SSLCertKey}
	if err := s.store.Cert().SetDeployPrevRef(ctx, req.CertID, req.SiteID, prev); err != nil {
		return nil, fmt.Errorf("site-mgmt: snapshot prev refs: %w", err)
	}
	tgt, _, err := s.openEdit(ctx, req.SiteID)
	if err != nil {
		return nil, err
	}
	if err := setSSLRefs(tgt.tree, tgt.block, *row.StoragePath, certKeyPathOf(*row.StoragePath)); err != nil {
		return nil, fmt.Errorf("site-mgmt: edit ssl refs: %w", err)
	}
	content, err := tgt.serialize()
	if err != nil {
		return nil, err
	}
	if err := s.applyPipeline(ctx, tgt.file, content, false); err != nil {
		// 补偿：清除快照键，维持 I-05 引用一致性（配置已回滚原样）。
		if cerr := s.store.Cert().ClearDeployPrevRef(ctx, req.CertID, req.SiteID); cerr != nil {
			slog.WarnContext(ctx, "site-mgmt: clear prev ref after pipeline failure",
				"cert", req.CertID, "site", req.SiteID, "err", cerr)
		}
		return nil, err
	}
	s.auditAction(ctx, actor, AuditActionCertDeployed, ObjectTypeCertificate,
		strconv.FormatInt(req.CertID, 10), map[string]any{"siteId": req.SiteID}, true)
	if rerr := s.reloadAfterWrite(ctx); rerr != nil {
		return &CertRefData{CertID: req.CertID, SiteID: req.SiteID, CertRef: *row.StoragePath}, rerr
	}
	return &CertRefData{CertID: req.CertID, SiteID: req.SiteID, CertRef: *row.StoragePath}, nil
}

// CertUndeploy POST /sites/cert/undeploy：I-05 校验（siteId 须在
// deploy_prev_refs，否则 CERT_006）→ WritePipeline 恢复部署前原引用值 →
// 清除该键 → reload。
func (s *Service) CertUndeploy(ctx context.Context, actor *authsvc.Identity, req CertRefRequest) (*CertRefData, error) {
	row, err := s.certByID(ctx, req.CertID)
	if err != nil {
		return nil, err
	}
	ref, ok := row.DeployPrevRefs[req.SiteID]
	if !ok {
		return nil, errcode.ErrUndeployNotDeployed
	}
	if _, err := s.resolveSite(ctx, req.SiteID); err != nil {
		return nil, err
	}
	if err := s.requireNginx(); err != nil {
		return nil, err
	}
	tgt, _, err := s.openEdit(ctx, req.SiteID)
	if err != nil {
		return nil, err
	}
	if err := setSSLRefs(tgt.tree, tgt.block, ref.Cert, ref.Key); err != nil {
		return nil, fmt.Errorf("site-mgmt: restore ssl refs: %w", err)
	}
	content, err := tgt.serialize()
	if err != nil {
		return nil, err
	}
	if err := s.applyPipeline(ctx, tgt.file, content, false); err != nil {
		return nil, err
	}
	if cerr := s.store.Cert().ClearDeployPrevRef(ctx, req.CertID, req.SiteID); cerr != nil {
		slog.WarnContext(ctx, "site-mgmt: clear prev ref after undeploy",
			"cert", req.CertID, "site", req.SiteID, "err", cerr)
	}
	s.auditAction(ctx, actor, AuditActionCertUndeployed, ObjectTypeCertificate,
		strconv.FormatInt(req.CertID, 10), map[string]any{"siteId": req.SiteID}, true)
	if rerr := s.reloadAfterWrite(ctx); rerr != nil {
		return &CertRefData{CertID: req.CertID, SiteID: req.SiteID, CertRef: ref.Cert}, rerr
	}
	return &CertRefData{CertID: req.CertID, SiteID: req.SiteID, CertRef: ref.Cert}, nil
}

// CertRetryRenewal POST /sites/cert/retry-renewal：status=renew_failed 专属
// （CERT_002），进入 4.1.3 同一续期通道（同步返回终态）。
func (s *Service) CertRetryRenewal(ctx context.Context, actor *authsvc.Identity, certID int64) (*CertRequestData, error) {
	row, err := s.certByID(ctx, certID)
	if err != nil {
		return nil, err
	}
	if row.Source != storesitemgmt.SourceAcme || row.Challenge == nil {
		st := s.computeStatus(row, s.clock())
		return nil, errcode.ErrCertInvalidState.WithData(StatusData{CurrentStatus: st})
	}
	if s.computeStatus(row, s.clock()) != storesitemgmt.StatusRenewFailed {
		return nil, errcode.ErrCertInvalidState.WithData(StatusData{
			CurrentStatus: s.computeStatus(row, s.clock())})
	}
	if err := s.requireNginx(); err != nil {
		return nil, err
	}
	return s.renewCert(ctx, actor, row, AuditActionCertRenewalRetried, audit.SourceWeb)
}

// CertDelete POST /sites/cert/delete：I-02 引用守卫（CERT_005 data.sites[]）
// → 同事务删行+审计（D-12）→ acme 连带删除托管文件（失败记日志，I-04）；
// imported 仅删行（源站原文件不动，⚠️A16）。
func (s *Service) CertDelete(ctx context.Context, actor *authsvc.Identity, certID int64) (*CertDeleteData, error) {
	row, err := s.certByID(ctx, certID)
	if err != nil {
		return nil, err
	}
	scan, err := s.scanSites(ctx)
	if err != nil {
		return nil, err
	}
	if refs := referencedSites(scan, row); len(refs) > 0 {
		return nil, errcode.ErrCertReferenced.WithData(CertRefSitesData{Sites: refs})
	}
	actorName, actorID := actorIdentity(actor)
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("site-mgmt: delete tx: %w", err)
	}
	defer func() {
		if rerr := tx.Rollback(); rerr != nil && !errors.Is(rerr, sql.ErrTxDone) {
			slog.WarnContext(ctx, "site-mgmt: delete tx rollback", "cert", certID, "err", rerr)
		}
	}()
	if err := tx.Cert().Delete(ctx, certID); err != nil {
		return nil, fmt.Errorf("site-mgmt: delete cert row: %w", err)
	}
	if err := tx.Audit().Append(ctx, audit.Entry{
		ActorID: actorID, ActorName: actorName,
		Category: audit.CategoryChange, Action: AuditActionCertDeleted,
		ObjectType: ObjectTypeCertificate, ObjectID: strconv.FormatInt(certID, 10),
		Detail: map[string]any{"source": row.Source},
		Result: audit.ResultSuccess, Source: audit.SourceWeb,
	}, s.clock()); err != nil {
		return nil, fmt.Errorf("site-mgmt: delete audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("site-mgmt: delete commit: %w", err)
	}
	if row.Source == storesitemgmt.SourceAcme && row.StoragePath != nil {
		// I-04：删行成功后删托管文件；失败记面板日志兜底（不回滚删行）。
		if rerr := s.gw.RemoveFile(*row.StoragePath); rerr != nil {
			slog.ErrorContext(ctx, "site-mgmt: remove managed cert file failed",
				"path", *row.StoragePath, "err", rerr)
		}
		if rerr := s.gw.RemoveFile(certKeyPathOf(*row.StoragePath)); rerr != nil {
			slog.ErrorContext(ctx, "site-mgmt: remove managed key file failed",
				"path", certKeyPathOf(*row.StoragePath), "err", rerr)
		}
	}
	return &CertDeleteData{CertID: certID}, nil
}

// renewCert 4.1.3 续期通道（retry-renewal 与每日调度共用）：AcmeGateway 续期
// → 托管文件原路径替换（I-04）→ deploy_prev_refs 全部站点重执行部署写回 →
// status=valid 落库+审计同事务；失败 renew_failed + last_error。
func (s *Service) renewCert(ctx context.Context, actor *authsvc.Identity,
	row storesitemgmt.Certificate, resultAction, auditSource string) (*CertRequestData, error) {
	if actor == nil { // 调度通道（source=system）
		actor = &authsvc.Identity{Username: "system"}
	}
	entry := s.findSiteByDomains(ctx, row.Domains)
	data, err := s.issueForSite(ctx, actor, entry, row.ID, row.Domains, *row.Challenge,
		resultAction, auditSource)
	if err != nil {
		return nil, err
	}
	// 成功后：重执行部署写回（引用漂移纠偏；未漂移站点 ssl_cert 已指向同路径
	// ——原路径替换语义下无需写回，仅漂移者经管线纠正）。
	scan, serr := s.scanSites(ctx)
	if serr == nil {
		for siteID := range row.DeployPrevRefs {
			e := scan.Find(siteID)
			if e == nil {
				continue
			}
			if e.Block.SSLCert == *row.StoragePath {
				continue
			}
			if rerr := s.redeployRef(ctx, e, *row.StoragePath); rerr != nil {
				slog.WarnContext(ctx, "site-mgmt: redeploy ref after renewal",
					"site", siteID, "err", rerr)
			}
		}
		if len(row.DeployPrevRefs) > 0 {
			if out, rerr := s.gw.Reload(ctx); rerr != nil {
				slog.WarnContext(ctx, "site-mgmt: reload after renewal",
					"output", out, "err", rerr) // D-05：配置保留，日志呈现
			}
		}
	}
	return data, nil
}

// redeployRef 续期后部署引用纠偏（WritePipeline 写回 ssl_cert=storage_path）。
func (s *Service) redeployRef(ctx context.Context, e *SiteEntry, storagePath string) error {
	tgt, _, err := s.openEdit(ctx, e.SiteID)
	if err != nil {
		return err
	}
	if err := setSSLRefs(tgt.tree, tgt.block, storagePath, certKeyPathOf(storagePath)); err != nil {
		return err
	}
	content, err := tgt.serialize()
	if err != nil {
		return err
	}
	return s.applyPipeline(ctx, tgt.file, content, false)
}

// findSiteByDomains 按域名集合定位站点（续期挑战通道复用；找不到返回 nil——
// issueForSite 对 nil entry 需要 http-01 webroot 缺省处理）。
func (s *Service) findSiteByDomains(ctx context.Context, domains []string) *SiteEntry {
	if len(domains) == 0 {
		return nil
	}
	scan, err := s.scanSites(ctx)
	if err != nil {
		return nil
	}
	for i := range scan.Entries {
		e := &scan.Entries[i]
		if len(e.Block.ServerNames) > 0 && strings.EqualFold(e.Block.ServerNames[0], domains[0]) {
			return e
		}
	}
	return nil
}

// certByID 证书存在性（CERT_001）。
func (s *Service) certByID(ctx context.Context, id int64) (storesitemgmt.Certificate, error) {
	row, err := s.store.Cert().GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, storesitemgmt.ErrCertNotFound) {
			return storesitemgmt.Certificate{}, errcode.ErrCertNotFound
		}
		return storesitemgmt.Certificate{}, fmt.Errorf("site-mgmt: get cert %d: %w", id, err)
	}
	return row, nil
}

// certStoragePaths 托管文件路径（A-SM11：{certsDir}/{规范化主域名}-fullchain.pem
// 与 -key.pem，由面板数据目录派生非硬编码）。
func (s *Service) certStoragePaths(primaryDomain string) (string, string) {
	base := filepath.Join(s.certsDir, normalizeDomain(primaryDomain))
	return base + "-fullchain.pem", base + "-key.pem"
}

// certKeyPathOf 由 storage_path 派生私钥路径（成对命名约定）。
func certKeyPathOf(storagePath string) string {
	return strings.TrimSuffix(storagePath, "-fullchain.pem") + "-key.pem"
}

// actorIdentity 审计四要素的操作者拆解（nil 防御：system 通道显式传名）。
func actorIdentity(actor *authsvc.Identity) (string, *int64) {
	if actor == nil {
		return "system", nil
	}
	id := actor.UserID
	return actor.Username, &id
}

// timePtrStr 时间指针 → RFC3339 字符串指针（nil 保持 nil）。
func timePtrStr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	v := formatTS(*t)
	return &v
}
