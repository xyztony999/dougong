package sitemgmt

import (
	"context"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storesitemgmt "github.com/dougong/dougong/internal/store/sitemgmt"
)

// v1.2/v2.0 增量编排：cert/renew（D-18 手动立即续期+失败分流）、cert/replace
//（D-18 imported 文件替换）、dns-credential 三接口（D-20 凭证管理）。

// dnsIssueOpts 是 dns_01 签发的内存态凭据（service 层解密产物；明文不落库
// 不回显不入审计 detail，I-D01）。
type dnsIssueOpts struct {
	Provider   string
	Credential DnsCredentialPayload
}

// dnsOpts 解密凭证行 → 签发实参（D-20）。
func (s *Service) dnsOpts(c storesitemgmt.DnsCredential) (*dnsIssueOpts, error) {
	if s.dnsVault == nil {
		return nil, fmt.Errorf("site-mgmt: dns vault not configured")
	}
	plain, err := s.dnsVault.Decrypt(string(c.CredentialEnc))
	if err != nil {
		return nil, fmt.Errorf("site-mgmt: decrypt dns credential %d: %w", c.ID, err)
	}
	var payload DnsCredentialPayload
	if err := json.Unmarshal([]byte(plain), &payload); err != nil {
		return nil, fmt.Errorf("site-mgmt: parse dns credential %d: %w", c.ID, err)
	}
	return &dnsIssueOpts{Provider: c.Provider, Credential: payload}, nil
}

// dnsCredPayloadFor 校验凭据材料按 provider 形态（R-14：形态校验非有效性
// 验证——有效性在签发时经服务商 API 验证，不缓存校验态，D-20/W8）。
func dnsCredPayloadFor(provider string, p DnsCredentialPayload) error {
	switch provider {
	case storesitemgmt.ProviderCloudflare:
		if p.APIToken == "" {
			return errcode.ErrCertInvalidDomainInput.WithData(ReasonData{Reason: "cloudflare 凭据须含 apiToken"})
		}
	case storesitemgmt.ProviderAliyun:
		if p.AccessKeyID == "" || p.AccessKeySecret == "" {
			return errcode.ErrCertInvalidDomainInput.WithData(ReasonData{Reason: "aliyun 凭据须含 accessKeyId 与 accessKeySecret"})
		}
	case storesitemgmt.ProviderDNSPod:
		if p.SecretID == "" || p.SecretKey == "" {
			return errcode.ErrCertInvalidDomainInput.WithData(ReasonData{Reason: "dnspod 凭据须含 secretId 与 secretKey"})
		}
	default:
		return errcode.ErrCertInvalidDomainInput.WithData(ReasonData{Reason: "provider 取值非法（cloudflare|aliyun|dnspod）"})
	}
	return nil
}

// DnsCredentialList GET /sites/dns-credential/list（D-20：零回显；引用计数
// 实时推导 I-C02——certificate.dns_credential_id 扫描）。
func (s *Service) DnsCredentialList(ctx context.Context) (*DnsCredentialListData, error) {
	rows, err := s.store.DnsCred().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("site-mgmt: dns credential list: %w", err)
	}
	certs, cerr := s.store.Cert().List(ctx)
	refCount := map[int64]int{}
	if cerr == nil {
		for _, c := range certs {
			if c.DnsCredentialID != nil {
				refCount[*c.DnsCredentialID]++
			}
		}
	} else {
		slog.WarnContext(ctx, "site-mgmt: dns credential ref count degraded", "err", cerr)
	}
	items := make([]DnsCredentialItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, DnsCredentialItem{
			ID: r.ID, Name: r.Name, Provider: r.Provider,
			CreatedAt: formatTS(r.CreatedAt), ReferencedCertCount: refCount[r.ID],
		})
	}
	return &DnsCredentialListData{Items: items}, nil
}

// DnsCredentialCreate POST /sites/dns-credential/create（D-20：形态校验→
// AES 密文→同事务落库+审计〔凭据材料零入 detail〕）。
func (s *Service) DnsCredentialCreate(ctx context.Context, actor *authsvc.Identity, req DnsCredentialCreateRequest) (*DnsCredentialCreateData, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 64 {
		return nil, fieldError("name", "显示名须为 1~64 字符")
	}
	if err := dnsCredPayloadFor(req.Provider, req.Credential); err != nil {
		return nil, err
	}
	if s.dnsVault == nil {
		return nil, fmt.Errorf("site-mgmt: dns vault not configured")
	}
	raw, err := json.Marshal(req.Credential)
	if err != nil {
		return nil, fmt.Errorf("site-mgmt: marshal dns credential: %w", err)
	}
	enc, err := s.dnsVault.Encrypt(string(raw))
	if err != nil {
		return nil, err
	}
	actorName, actorID := actorIdentity(actor)
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("site-mgmt: dns cred tx: %w", err)
	}
	defer func() {
		if rerr := tx.Rollback(); rerr != nil && !errors.Is(rerr, sql.ErrTxDone) {
			slog.WarnContext(ctx, "site-mgmt: dns cred tx rollback", "err", rerr)
		}
	}()
	id, err := tx.DnsCred().Create(ctx, storesitemgmt.DnsCredential{
		Name: name, Provider: req.Provider, CredentialEnc: []byte(enc),
	})
	if err != nil {
		return nil, fmt.Errorf("site-mgmt: dns cred create: %w", err)
	}
	if err := tx.Audit().Append(ctx, audit.Entry{
		ActorID: actorID, ActorName: actorName,
		Category: audit.CategoryChange, Action: AuditActionDnsCredCreated,
		ObjectType: ObjectTypeDnsCredential, ObjectID: strconv.FormatInt(id, 10),
		Detail: map[string]any{"provider": req.Provider}, // 凭据材料零入 detail（I-D01）
		Result: audit.ResultSuccess, Source: audit.SourceWeb,
	}, s.clock()); err != nil {
		return nil, fmt.Errorf("site-mgmt: dns cred audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("site-mgmt: dns cred commit: %w", err)
	}
	return &DnsCredentialCreateData{ID: id, Name: name, Provider: req.Provider}, nil
}

// DnsCredentialDelete POST /sites/dns-credential/delete（D-20：引用证书集合
// 实时推导警示返回〔不硬阻断，I-C01〕→ 同事务删行+审计）。
func (s *Service) DnsCredentialDelete(ctx context.Context, actor *authsvc.Identity, id int64) (*DnsCredentialDeleteData, error) {
	if _, err := s.store.DnsCred().GetByID(ctx, id); err != nil {
		if errors.Is(err, storesitemgmt.ErrDnsCredentialNotFound) {
			return nil, errcode.ErrDnsCredentialNotFound
		}
		return nil, fmt.Errorf("site-mgmt: dns cred get: %w", err)
	}
	certs, err := s.store.Cert().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("site-mgmt: cert list for cred delete: %w", err)
	}
	refs := []DnsCertRefItem{}
	for _, c := range certs {
		if c.DnsCredentialID != nil && *c.DnsCredentialID == id {
			refs = append(refs, DnsCertRefItem{CertID: c.ID, PrimaryDomain: c.PrimaryDomain})
		}
	}
	actorName, actorID := actorIdentity(actor)
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("site-mgmt: dns cred del tx: %w", err)
	}
	defer func() {
		if rerr := tx.Rollback(); rerr != nil && !errors.Is(rerr, sql.ErrTxDone) {
			slog.WarnContext(ctx, "site-mgmt: dns cred del tx rollback", "err", rerr)
		}
	}()
	if err := tx.DnsCred().Delete(ctx, id); err != nil {
		return nil, fmt.Errorf("site-mgmt: dns cred delete: %w", err)
	}
	if err := tx.Audit().Append(ctx, audit.Entry{
		ActorID: actorID, ActorName: actorName,
		Category: audit.CategoryChange, Action: AuditActionDnsCredDeleted,
		ObjectType: ObjectTypeDnsCredential, ObjectID: strconv.FormatInt(id, 10),
		Detail: map[string]any{"referencedCertCount": len(refs)},
		Result: audit.ResultSuccess, Source: audit.SourceWeb,
	}, s.clock()); err != nil {
		return nil, fmt.Errorf("site-mgmt: dns cred del audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("site-mgmt: dns cred del commit: %w", err)
	}
	return &DnsCredentialDeleteData{ID: id, ReferencedCerts: refs}, nil
}

// CertRenew POST /sites/cert/renew（v1.2 D-18 手动立即续期：acme 来源
// valid/expiring_soon/renew_failed；失败迁移分流——当前 valid 保持 valid+
// last_error，其余转 renew_failed）。
func (s *Service) CertRenew(ctx context.Context, actor *authsvc.Identity, certID int64) (*CertRenewData, error) {
	row, err := s.certByID(ctx, certID)
	if err != nil {
		return nil, err
	}
	status := s.computeStatus(row, s.clock())
	if row.Source != storesitemgmt.SourceAcme || row.Challenge == nil ||
		(status != storesitemgmt.StatusValid && status != storesitemgmt.StatusExpiringSoon &&
			status != storesitemgmt.StatusRenewFailed) {
		return nil, errcode.ErrCertInvalidState.WithData(StatusData{CurrentStatus: status})
	}
	failStatus := storesitemgmt.StatusRenewFailed
	if status == storesitemgmt.StatusValid {
		failStatus = storesitemgmt.StatusValid // D-18 分流：旧证书仍在用，不标风险态
	}
	if *row.Challenge != storesitemgmt.ChallengeDNS01 {
		if err := s.requireNginx(); err != nil {
			return nil, err
		}
	}
	return s.renewCertWith(ctx, actor, row, AuditActionCertManualRenewed, audit.SourceWeb, failStatus)
}

// CertReplace POST /sites/cert/replace（v1.2 D-18 imported 文件替换：PEM
// x509 解析校验（CERT_008）→ cert-backup 备份（保留 5 份）→ 原子写回
// origin_path → reload；读时刷新元数据〔D-08〕响应返回）。
func (s *Service) CertReplace(ctx context.Context, actor *authsvc.Identity, req CertReplaceRequest) (*CertReplaceData, error) {
	row, err := s.certByID(ctx, req.CertID)
	if err != nil {
		return nil, err
	}
	if row.Source != storesitemgmt.SourceImported || row.OriginPath == nil {
		return nil, errcode.ErrCertInvalidState.WithData(StatusData{
			CurrentStatus: s.computeStatus(row, s.clock()), Reason: "source"})
	}
	pem := []byte(req.CertPEM)
	if len(pem) == 0 || len(pem) > CertPemSizeLimit {
		return nil, errcode.ErrCertInvalidPEM
	}
	leaf, err := parseLeafCertLocal(pem)
	if err != nil {
		return nil, errcode.ErrCertInvalidPEM
	}
	// 备份原文件（cert-backup/{certId}/{timestamp}.pem，保留 CertBackupKeep 份）。
	backupDir := filepath.Join(s.certBackupRoot(), strconv.FormatInt(row.ID, 10))
	if err := s.gw.MkdirAll(backupDir); err != nil {
		return nil, fmt.Errorf("site-mgmt: cert backup dir: %w", err)
	}
	if orig, _, rerr := s.gw.ReadFile(*row.OriginPath); rerr == nil {
		stamp := s.clock().UTC().Format("20060102T150405Z")
		if err := s.gw.WriteFileAtomic(filepath.Join(backupDir, stamp+".pem"), []byte(orig)); err != nil {
			return nil, fmt.Errorf("site-mgmt: cert backup write: %w", err)
		}
		trimCertBackups(ctx, backupDir, CertBackupKeep)
	} else {
		slog.WarnContext(ctx, "site-mgmt: cert replace backup skipped (origin unreadable)",
			"path", *row.OriginPath, "err", rerr)
	}
	if err := s.gw.WriteFileAtomic(*row.OriginPath, pem); err != nil {
		return nil, fmt.Errorf("site-mgmt: cert replace write: %w", err)
	}
	s.auditAction(ctx, actor, AuditActionCertReplaced, ObjectTypeCertificate,
		strconv.FormatInt(row.ID, 10),
		map[string]any{"size": len(pem), "notAfter": formatTS(leaf.NotAfter)}, true)
	// reload 生效（W5 统一 reload；失败如实返回——文件已替换，D-05 语义）。
	if rerr := s.reloadAfterWrite(ctx); rerr != nil {
		return nil, rerr
	}
	data := &CertReplaceData{CertID: row.ID, Issuer: issuerNameLocal(leaf), Domains: certDomains(leaf)}
	nb, na := formatTS(leaf.NotBefore), formatTS(leaf.NotAfter)
	data.NotBefore, data.NotAfter = &nb, &na
	return data, nil
}

// certBackupRoot 替换备份根目录（{dataDir}/cert-backup，D-18）。
func (s *Service) certBackupRoot() string { return filepath.Join(s.dataDir, "cert-backup") }

// trimCertBackups 保留最近 keep 份备份（按文件名时间戳字典序，超额删最旧）。
func trimCertBackups(ctx context.Context, dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".pem") {
			names = append(names, e.Name())
		}
	}
	if len(names) <= keep {
		return
	}
	sort.Strings(names) // 时间戳字典序=时间序（ReadDir 排序不依赖平台）
	for _, n := range names[:len(names)-keep] {
		if rerr := os.Remove(filepath.Join(dir, n)); rerr != nil {
			slog.WarnContext(ctx, "site-mgmt: trim cert backup", "file", n, "err", rerr)
		}
	}
}

// parseLeafCertLocal 解析 PEM 首证书（cert/replace 服务端校验，R-14/CERT_008）。
func parseLeafCertLocal(pemBytes []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("sitemgmt: no CERTIFICATE PEM block")
	}
	return x509.ParseCertificate(block.Bytes)
}

// issuerNameLocal 签发者 CN/组织呈现（与 adapter/acme 同法）。
func issuerNameLocal(c *x509.Certificate) string {
	if c == nil {
		return ""
	}
	if len(c.Issuer.Organization) > 0 {
		return c.Issuer.Organization[0]
	}
	if c.Issuer.CommonName != "" {
		return c.Issuer.CommonName
	}
	return c.Issuer.String()
}

// certDomains 叶证书 DNS SAN+CN 域名集合（replace 响应差异呈现，D-18）。
func certDomains(c *x509.Certificate) []string {
	out := append([]string(nil), c.DNSNames...)
	if c.Subject.CommonName != "" {
		out = append(out, c.Subject.CommonName)
	}
	if len(out) == 0 {
		return []string{}
	}
	return out
}
