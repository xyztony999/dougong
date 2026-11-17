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

	"github.com/dougong/dougong/internal/adapter/acme"
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

// CertRequest POST /sites/cert/request（v2.0 扩展）：域名来源 siteId/domains
// 二选一（CERT_010）→ challenge 三值（CERT_007；dns_01 附 dnsCredentialId
// 实况 CERT_009）→ I-03 防重（CERT_003）→ nginx 前置仅 http_01/tls_alpn
// （dns_01 TXT 在服务商侧免通路前置，D-20）→ INSERT pending → 签发通道。
func (s *Service) CertRequest(ctx context.Context, actor *authsvc.Identity, req CertRequestRequest) (*CertRequestData, error) {
	switch req.Challenge {
	case storesitemgmt.ChallengeHTTP01, storesitemgmt.ChallengeTLSALPN, storesitemgmt.ChallengeDNS01:
	default:
		return nil, errcode.ErrInvalidChallenge
	}
	if (req.SiteID == "") == (len(req.Domains) == 0) {
		return nil, errcode.ErrCertInvalidDomainInput.WithData(ReasonData{
			Reason: "siteId 与 domains 必须二选一"})
	}
	var entry *SiteEntry
	var domains []string
	if req.SiteID != "" {
		e, err := s.resolveSite(ctx, req.SiteID)
		if err != nil {
			return nil, err
		}
		entry = e
		domains = append([]string(nil), e.Block.ServerNames...)
	} else {
		domains = append([]string(nil), req.Domains...)
	}
	if e := validateCertDomains(domains); e != nil {
		return nil, e
	}
	var dnsOpts *dnsIssueOpts
	var dnsCredID *int64
	if req.Challenge == storesitemgmt.ChallengeDNS01 {
		if req.DNSCredentialID == nil {
			return nil, errcode.ErrDnsCredentialNotFound
		}
		cred, err := s.store.DnsCred().GetByID(ctx, *req.DNSCredentialID)
		if err != nil {
			if errors.Is(err, storesitemgmt.ErrDnsCredentialNotFound) {
				return nil, errcode.ErrDnsCredentialNotFound
			}
			return nil, fmt.Errorf("site-mgmt: get dns credential: %w", err)
		}
		opts, err := s.dnsOpts(cred)
		if err != nil {
			return nil, err
		}
		dnsOpts = opts
		id := cred.ID
		dnsCredID = &id
	}
	primary := domains[0]
	// I-03 防重（CERT_003，v2.1 语义修正）：仅拦 **acme 源** active 证书（已有
	// 「更新证书」续签通道，重复申请无意义）与 **pending** 进行中（防并发双击）；
	// **imported 源同域名集合放行**——这正是 ACME 接管续期的入口（用户实测
	// 死路反馈：导入证书不能续签、CERT_003 又拦重新签发、被引用还删不掉）。
	if existing := s.findActiveByDomains(ctx, domains); existing != nil &&
		(existing.Source == storesitemgmt.SourceAcme || existing.Status == storesitemgmt.StatusPending) {
		return nil, errcode.ErrCertDuplicate.WithData(ExistingCertData{ExistingCertID: existing.ID})
	}
	if req.Challenge != storesitemgmt.ChallengeDNS01 {
		if err := s.requireNginx(); err != nil {
			return nil, err
		}
	}
	fullchain, _ := s.certStoragePaths(primary)
	sp := fullchain
	id, err := s.store.Cert().Create(ctx, storesitemgmt.Certificate{
		PrimaryDomain: primary, Domains: domains,
		Source: storesitemgmt.SourceAcme, Status: storesitemgmt.StatusPending,
		Challenge: &req.Challenge, StoragePath: &sp, DnsCredentialID: dnsCredID,
	})
	if err != nil {
		return nil, fmt.Errorf("site-mgmt: cert create pending: %w", err)
	}
	return s.issueForSite(ctx, actor, entry, id, domains, req.Challenge,
		AuditActionCertRequested, audit.SourceWeb, dnsOpts, storesitemgmt.StatusRenewFailed)
}

// validateCertDomains 证书申请域名校验（复用 validDomain 单层通配形态；
// 错误码承载 CERT_010——证书域输入而非站点 server_name 域）。
func validateCertDomains(domains []string) *errcode.Error {
	if len(domains) == 0 {
		return errcode.ErrCertInvalidDomainInput
	}
	for _, d := range domains {
		if !validDomain(d) {
			return errcode.ErrCertInvalidDomainInput
		}
	}
	return nil
}

// issueForSite 签发/续期共用通道（request 与 4.1.3/retry 统一 AcmeGateway，
// D-09）：http-01 临时 location 注入 → Issue → 恢复 → 托管文件写 → 终态落库
// +审计同事务。resultAction/auditSource 随调用方（web/system）。
func (s *Service) issueForSite(ctx context.Context, actor *authsvc.Identity, entry *SiteEntry,
	certID int64, domains []string, challenge, resultAction, auditSource string,
	dnsOpts *dnsIssueOpts, failStatus string) (*CertRequestData, error) {
	actorName, actorID := actorIdentity(actor)
	if auditSource == "" {
		auditSource = audit.SourceWeb
	}
	if failStatus == "" {
		failStatus = storesitemgmt.StatusRenewFailed
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
	var res *acme.IssueResult
	var ierr error
	if challenge == storesitemgmt.ChallengeDNS01 {
		res, ierr = s.acme.IssueDNS(actx, domains, dnsOpts.Provider, acme.DnsCredential{
			APIToken:    dnsOpts.Credential.APIToken,
			AccessKeyID: dnsOpts.Credential.AccessKeyID, AccessKeySecret: dnsOpts.Credential.AccessKeySecret,
			SecretID: dnsOpts.Credential.SecretID, SecretKey: dnsOpts.Credential.SecretKey,
		})
	} else {
		res, ierr = s.acme.Issue(actx, domains, challenge, webroot)
	}
	cancel()
	cleanup() // 移除临时 location（恢复原样）——先于终态落库
	if ierr != nil {
		reason := ierr.Error()
		if err := s.finalizeRenewal(ctx, certID, failStatus, &reason,
			nil, nil, "", "", resultAction, auditSource, actorName, actorID,
			map[string]any{"challenge": challenge, "error": errcode.ErrAcmeFailed.Symbol(), "reason": reason}); err != nil {
			return nil, err
		}
		return nil, errcode.ErrAcmeFailed.WithData(ReasonData{Reason: reason, Status: failStatus})
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
// v2.1 接管语义：同集合多张 active 并存时，优先返回**拦截性**的一张（acme 源
// 或 pending——CERT_003 判定用），imported 仅作兜底返回——避免首张恰好是
// imported 而漏拦共存的 acme 证书。
func (s *Service) findActiveByDomains(ctx context.Context, domains []string) *storesitemgmt.Certificate {
	rows, err := s.store.Cert().ListActive(ctx)
	if err != nil {
		slog.WarnContext(ctx, "site-mgmt: list active certs failed", "err", err)
		return nil
	}
	var fallback *storesitemgmt.Certificate
	for i := range rows {
		if !domainSetEqual(rows[i].Domains, domains) {
			continue
		}
		if rows[i].Source == storesitemgmt.SourceAcme || rows[i].Status == storesitemgmt.StatusPending {
			return &rows[i]
		}
		if fallback == nil {
			fallback = &rows[i]
		}
	}
	return fallback
}

// prepareHTTP01 A-SM10（v2.2 多块注入；**v2.3 跨文件+默认兜底**）：已有
// /.well-known/acme-challenge/ location → 复用其 root（零写回；判定扩至全扫描
// 相关块）；无 → 经 WritePipeline 注入临时挑战 location 到**全部承载该域名的
// server 块**（跨文件：80 与 443 块可能分属不同 conf），并对块内 server 级
// 301/302 跳转做 location / 临时包裹（wrapServerRedirect）；若匹配块中**无 80
// 端口块**（域名仅 443 在服），追加注入 **80 默认兜底块**（default_server 优先、
// 否则扫描序首个 80 块——LE 首跳 http 请求实际落点，常见为全局 `return 301
// https://$host…` catch-all）→ cleanup 逐文件字节级复原（逆序，同管线）。
// 修正动机（两次实机 404）：v2.2 只覆盖 entry 所在单文件——全局 catch-all
// 重定向块/跨文件 443 块仍使 LE 首跳或跟随跳落空。
func (s *Service) prepareHTTP01(ctx context.Context, entry *SiteEntry) (func(), error) {
	domains := entry.Block.ServerNames
	domSet := domainSet(domains)
	scan, err := s.scanSites(ctx)
	if err != nil {
		return nil, fmt.Errorf("site-mgmt: scan for challenge: %w", err)
	}
	for i := range scan.Entries { // 复用判定：全扫描相关块
		e := &scan.Entries[i]
		if e.Block.Dormant || !serverBlockMatches(e.Block.Node, domSet) {
			continue
		}
		if loc := findAcmeLocation(e.Block.Node); loc != nil {
			if root := directiveArgIn(loc, "root"); root != "" {
				return func() {}, nil
			}
		}
	}
	webroot := s.panelWebroot()
	if err := s.gw.MkdirAll(webroot); err != nil {
		return nil, fmt.Errorf("site-mgmt: webroot mkdir: %w", err)
	}
	if entry.Block.Dormant { // 注释块：子树单块注入（域未在服务，签发成败由 LE 判定）
		orig, _, rerr := s.gw.ReadFile(entry.File)
		if rerr != nil {
			return nil, fmt.Errorf("site-mgmt: read for challenge: %w", rerr)
		}
		tgt, _, oerr := s.openEdit(ctx, entry.SiteID)
		if oerr != nil {
			return nil, oerr
		}
		if err := injectChallengeBlocks(tgt.tree, []*nginxx.Node{tgt.block}, webroot); err != nil {
			return nil, fmt.Errorf("site-mgmt: inject challenge location: %w", err)
		}
		content, serr := tgt.serialize()
		if serr != nil {
			return nil, serr
		}
		if err := s.applyPipeline(ctx, entry.File, content, false); err != nil {
			return nil, err
		}
		return s.restoreCleanup(ctx, entry.File, orig), nil
	}
	// 跨文件分组：匹配块按文件聚合；无 80 匹配块时补默认兜底块。
	type fileEdit struct {
		tree   *nginxx.Tree
		blocks []*nginxx.Node
	}
	byFile := map[string]*fileEdit{}
	order := make([]string, 0, 2)
	has80 := false
	for i := range scan.Entries {
		e := &scan.Entries[i]
		if e.Block.Dormant || !serverBlockMatches(e.Block.Node, domSet) {
			continue
		}
		if listenPort80(e.Block) {
			has80 = true
		}
		fe, ok := byFile[e.File]
		if !ok {
			fe = &fileEdit{tree: e.Tree}
			byFile[e.File] = fe
			order = append(order, e.File)
		}
		fe.blocks = append(fe.blocks, e.Block.Node)
	}
	if !has80 {
		if fb := default80Block(scan); fb != nil {
			fe, ok := byFile[fb.File]
			if !ok {
				fe = &fileEdit{tree: fb.Tree}
				byFile[fb.File] = fe
				order = append(order, fb.File)
			}
			fe.blocks = append(fe.blocks, fb.Block.Node)
		}
	}
	written := make([]struct{ file, orig string }, 0, len(order))
	restore := func() { // 逆序逐文件复原（失败记日志——保持注入态可服务可排障）
		for i := len(written) - 1; i >= 0; i-- {
			w := written[i]
			if rerr := s.applyPipeline(context.WithoutCancel(ctx), w.file, w.orig, false); rerr != nil {
				slog.ErrorContext(ctx, "site-mgmt: challenge restore failed",
					"file", w.file, "err", rerr)
			}
		}
	}
	for _, f := range order {
		fe := byFile[f]
		orig, _, rerr := s.gw.ReadFile(f)
		if rerr != nil {
			restore()
			return nil, fmt.Errorf("site-mgmt: read for challenge %s: %w", f, rerr)
		}
		if err := injectChallengeBlocks(fe.tree, fe.blocks, webroot); err != nil {
			restore()
			return nil, fmt.Errorf("site-mgmt: inject challenge locations %s: %w", f, err)
		}
		content, serr := fe.tree.Serialize()
		if serr != nil {
			restore()
			return nil, serr
		}
		if err := s.applyPipeline(ctx, f, content, false); err != nil {
			restore()
			return nil, err
		}
		written = append(written, struct{ file, orig string }{f, orig})
	}
	return func() { restore() }, nil
}

// restoreCleanup 单文件复原闭包（dormant 路径）。
func (s *Service) restoreCleanup(ctx context.Context, file, orig string) func() {
	return func() {
		if rerr := s.applyPipeline(context.WithoutCancel(ctx), file, orig, false); rerr != nil {
			slog.ErrorContext(ctx, "site-mgmt: challenge location restore failed",
				"file", file, "err", rerr)
		}
	}
}

// listenPort80 块是否承载 80 端口（无 listen 声明=nginx 隐式 80）。
func listenPort80(b *nginxx.ServerBlock) bool {
	if len(b.Listen) == 0 {
		return true
	}
	for _, l := range b.Listen {
		first := strings.Fields(l)
		if len(first) == 0 {
			continue
		}
		if first[0] == "80" || strings.HasSuffix(first[0], ":80") {
			return true
		}
	}
	return false
}

// default80Block LE 首跳 http 请求的实际落点：显式 default_server 的 80 块
// 优先，否则扫描序首个 80 块（nginx 无 default_server 时取配置序首个）。
func default80Block(scan *ScanResult) *SiteEntry {
	var first80 *SiteEntry
	for i := range scan.Entries {
		e := &scan.Entries[i]
		if e.Block.Dormant || !listenPort80(e.Block) {
			continue
		}
		if first80 == nil {
			first80 = e
		}
		for _, l := range e.Block.Listen {
			if strings.Contains(l, "default_server") {
				return e
			}
		}
	}
	return first80
}

// reusableChallengeRoot 整文件内任一相关 server 块已有挑战 location 时返回
// 其 root（A-SM10 ①复用，v2.2 从单块扩至全部相关块）。
func reusableChallengeRoot(t *nginxx.Tree, domains []string) string {
	domSet := domainSet(domains)
	for _, top := range t.Nodes {
		if !isServerNode(top) || !serverBlockMatches(top, domSet) {
			continue
		}
		if loc := findAcmeLocation(top); loc != nil {
			if root := directiveArgIn(loc, "root"); root != "" {
				return root
			}
		}
	}
	return ""
}

// prepareChallengeBlocks 在整文件树内为全部相关 server 块注入（包装
// injectChallengeBlocks；单测锁定）。
func prepareChallengeBlocks(t *nginxx.Tree, domains []string, webroot string) error {
	domSet := domainSet(domains)
	var blocks []*nginxx.Node
	for _, top := range t.Nodes {
		if isServerNode(top) && serverBlockMatches(top, domSet) {
			blocks = append(blocks, top)
		}
	}
	return injectChallengeBlocks(t, blocks, webroot)
}

// injectChallengeBlocks 对指定 server 块注入挑战 location 并包裹块内 server 级
// 跳转（纯树变换，单测锁定）。幂等：已有挑战入口的块跳过。
func injectChallengeBlocks(t *nginxx.Tree, blocks []*nginxx.Node, webroot string) error {
	for _, top := range blocks {
		if findAcmeLocation(top) != nil {
			continue
		}
		for _, ch := range top.Children {
			if ch.Kind != nginxx.KindDirective {
				continue
			}
			if (ch.Name == "return" && len(ch.Args) >= 1 &&
				(ch.Args[0] == "301" || ch.Args[0] == "302")) ||
				(ch.Name == "rewrite" && len(ch.Args) >= 2 &&
					(ch.Args[len(ch.Args)-1] == "permanent" || ch.Args[len(ch.Args)-1] == "redirect")) {
				if err := wrapServerRedirect(t, ch); err != nil {
					return err
				}
			}
		}
		indent := indentOfBlock(t, top)
		loc := indent + "location ^~ /.well-known/acme-challenge/ {\n" +
			indent + "    root " + webroot + ";\n" +
			indent + "}"
		pos := insertPos(t, top)
		if err := t.Apply(pos, pos, loc+"\n"+indent); err != nil {
			return err
		}
	}
	return nil
}

// wrapServerRedirect 把 server 级跳转指令临时包裹为 location / { … }：
// nginx 的 server 级 rewrite/return 在 location 选择**之前**执行，不包裹则
// 挑战请求连同 /.well-known/ 一起被重定向到 HTTPS 块（无挑战入口 → 404）。
// 包裹后：`^~ /.well-known/acme-challenge/` 前缀命中挑战 location，其余路径
// 经 location / 保持原跳转语义。挑战完成 cleanup 整文件恢复原字节。
func wrapServerRedirect(t *nginxx.Tree, ch *nginxx.Node) error {
	indent := t.IndentOf(ch)
	wrapped := "location / {\n" + indent + "    " + t.TextOf(ch) + "\n" + indent + "}"
	return t.Apply(ch.Start, ch.End, wrapped)
}

// serverBlockMatches server_name 与申请域名相关（精确命中，或泛域名条目
// 覆盖某申请域名，如 *.tonyxyz.com 覆盖 proxy2.tonyxyz.com）。
func serverBlockMatches(block *nginxx.Node, domSet map[string]bool) bool {
	for _, ch := range block.Children {
		if ch.Kind != nginxx.KindDirective || !strings.EqualFold(ch.Name, "server_name") {
			continue
		}
		for _, a := range ch.Args {
			if domSet[a] {
				return true
			}
			if strings.HasPrefix(a, "*.") {
				suffix := a[1:] // ".tonyxyz.com"
				for d := range domSet {
					if strings.HasSuffix(d, suffix) {
						return true
					}
				}
			}
		}
	}
	return false
}

// domainSet 域名集合（小写归一——server_name 大小写不敏感惯例）。
func domainSet(domains []string) map[string]bool {
	m := make(map[string]bool, len(domains))
	for _, d := range domains {
		m[strings.ToLower(d)] = true
	}
	return m
}

// isServerNode 顶层 server 块判定。
func isServerNode(n *nginxx.Node) bool {
	return n.Kind == nginxx.KindBlock && strings.EqualFold(n.Name, "server")
}

// acmeWebroot Issue 调用的 webroot 实参（http-01；复用既有 location root 时
// 取该值——v2.2 扩至全部相关块，否则面板 webroot；tls_alpn 忽略）。
func (s *Service) acmeWebroot(entry *SiteEntry, challenge string) string {
	if challenge != storesitemgmt.ChallengeHTTP01 {
		return ""
	}
	if !entry.Block.Dormant {
		if root := reusableChallengeRoot(entry.Tree, entry.Block.ServerNames); root != "" {
			return root
		}
	} else if loc := findAcmeLocation(entry.Block.Node); loc != nil {
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

// renewCert 4.1.3 续期通道（retry-renewal 与每日调度共用；失败统一转
// renew_failed——D-18 分流由 CertRenew 走 renewCertWith）。
func (s *Service) renewCert(ctx context.Context, actor *authsvc.Identity,
	row storesitemgmt.Certificate, resultAction, auditSource string) (*CertRequestData, error) {
	return s.renewCertWith(ctx, actor, row, resultAction, auditSource, storesitemgmt.StatusRenewFailed)
}

// renewCertWith 续期通道（v1.2 D-18：failStatus 分流——手动更新对 valid 行
// 失败保持 valid+last_error）：dns_01 行先解密引用凭证（凭证缺失=失败如实，
// ⚠️A20）。
func (s *Service) renewCertWith(ctx context.Context, actor *authsvc.Identity,
	row storesitemgmt.Certificate, resultAction, auditSource, failStatus string) (*CertRequestData, error) {
	if actor == nil { // 调度通道（source=system）
		actor = &authsvc.Identity{Username: "system"}
	}
	var dnsOpts *dnsIssueOpts
	if *row.Challenge == storesitemgmt.ChallengeDNS01 {
		if row.DnsCredentialID == nil {
			reason := "DNS 凭证已删除"
			if err := s.finalizeRenewal(ctx, row.ID, failStatus, &reason,
				nil, nil, "", "", resultAction, auditSource, actor.Username, nil,
				map[string]any{"challenge": *row.Challenge, "reason": reason}); err != nil {
				return nil, err
			}
			return nil, errcode.ErrDnsCredentialNotFound
		}
		cred, err := s.store.DnsCred().GetByID(ctx, *row.DnsCredentialID)
		if err != nil {
			if errors.Is(err, storesitemgmt.ErrDnsCredentialNotFound) {
				reason := "DNS 凭证已删除"
				if ferr := s.finalizeRenewal(ctx, row.ID, failStatus, &reason,
					nil, nil, "", "", resultAction, auditSource, actor.Username, nil,
					map[string]any{"challenge": *row.Challenge, "reason": reason}); ferr != nil {
					return nil, ferr
				}
				return nil, errcode.ErrDnsCredentialNotFound
			}
			return nil, fmt.Errorf("site-mgmt: get dns credential for renewal: %w", err)
		}
		opts, err := s.dnsOpts(cred)
		if err != nil {
			return nil, err
		}
		dnsOpts = opts
	}
	entry := s.findSiteByDomains(ctx, row.Domains)
	data, err := s.issueForSite(ctx, actor, entry, row.ID, row.Domains, *row.Challenge,
		resultAction, auditSource, dnsOpts, failStatus)
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
