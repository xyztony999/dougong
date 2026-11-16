package sitemgmt

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	storesitemgmt "github.com/dougong/dougong/internal/store/sitemgmt"
)

// CertStatusCalculator（§4.1.1 领域服务，D-08 读时惰性推算）：valid/
// expiring_soon/expired 由 not_after 与当前时间即时推算（CERT_EXPIRING_DAYS=30
// 编译期常量）；pending/renew_failed 为落库态如实呈现；imported 证书元数据
// 读时重读源站证书文件刷新（不落库过期值，防面板外替换漂移）。

// ComputedCert 读时推算后的证书视图（呈现层数据源）。
type ComputedCert struct {
	Row          storesitemgmt.Certificate
	Status       string
	Issuer       string
	NotBefore    *time.Time
	NotAfter     *time.Time
	RefreshError string // imported 文件读取/解析失败时的如实提示（不虚构元数据）
}

// computeCert 逐行推算（imported 元数据读时刷新；acme 落库值呈现）。
func (s *Service) computeCert(row storesitemgmt.Certificate, now time.Time) ComputedCert {
	c := ComputedCert{Row: row, Status: row.Status, Issuer: row.Issuer,
		NotBefore: row.NotBefore, NotAfter: row.NotAfter}
	if row.Source == storesitemgmt.SourceImported && row.OriginPath != nil {
		if leaf, err := parseCertFile(s.gw, *row.OriginPath); err == nil {
			c.Issuer = issuerDisplayName(leaf)
			nb, na := leaf.NotBefore, leaf.NotAfter
			c.NotBefore, c.NotAfter = &nb, &na // 读时刷新，不落库（D-08）
		} else {
			c.RefreshError = err.Error() // 文件缺失/替换为非证书内容：保持落库呈现
		}
	}
	if c.NotAfter != nil && c.Status != storesitemgmt.StatusPending && c.Status != storesitemgmt.StatusRenewFailed {
		c.Status = deriveStatus(*c.NotAfter, now)
	}
	return c
}

// computeStatus 轻量状态推算（无元数据刷新；list 项投影用）。
func (s *Service) computeStatus(row storesitemgmt.Certificate, now time.Time) string {
	if row.Source == storesitemgmt.SourceImported && row.OriginPath != nil {
		if leaf, err := parseCertFile(s.gw, *row.OriginPath); err == nil {
			return deriveStatus(leaf.NotAfter, now)
		}
	}
	if row.NotAfter != nil && row.Status != storesitemgmt.StatusPending && row.Status != storesitemgmt.StatusRenewFailed {
		return deriveStatus(*row.NotAfter, now)
	}
	return row.Status
}

// deriveStatus 由 not_after 推算：到期→expired；≤30 天→expiring_soon；否则 valid。
func deriveStatus(notAfter time.Time, now time.Time) string {
	if !now.Before(notAfter) {
		return storesitemgmt.StatusExpired
	}
	if notAfter.Sub(now) <= CertExpiringDays*24*time.Hour {
		return storesitemgmt.StatusExpiringSoon
	}
	return storesitemgmt.StatusValid
}

// parseCertFile 读取并解析 PEM 证书文件叶证书（imported 刷新与纳管校验共用；
// 经网关文件原语——测试可注入）。
func parseCertFile(gw NginxGateway, path string) (*x509.Certificate, error) {
	content, _, err := gw.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read cert %s: %w", path, err)
	}
	rest := []byte(content)
	for len(rest) > 0 {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		leaf, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse cert %s: %w", path, err)
		}
		return leaf, nil
	}
	return nil, fmt.Errorf("no CERTIFICATE block in %s", path)
}

// issuerDisplayName 签发者可读名（CN 优先，缺省 Organization）。
func issuerDisplayName(c *x509.Certificate) string {
	if c.Issuer.CommonName != "" {
		return c.Issuer.CommonName
	}
	if len(c.Issuer.Organization) > 0 {
		return c.Issuer.Organization[0]
	}
	return ""
}

// CertReferencedResolver（§4.1.1 领域服务，D-11）：引用关系实时推导不落冗余
// 列——扫描站点 ssl_cert 值与证书 storage_path（acme）/origin_path（imported）
// 匹配；cert/delete 守卫与 cert/list 的 referencedSites 均按此推导。

// referencedSites 推导引用该证书的站点清单（含 serverNames）。
func referencedSites(scan *ScanResult, row storesitemgmt.Certificate) []SiteRef {
	var refPath string
	switch row.Source {
	case storesitemgmt.SourceAcme:
		if row.StoragePath != nil {
			refPath = *row.StoragePath
		}
	case storesitemgmt.SourceImported:
		if row.OriginPath != nil {
			refPath = *row.OriginPath
		}
	}
	if refPath == "" {
		return []SiteRef{}
	}
	var out []SiteRef
	for i := range scan.Entries {
		e := &scan.Entries[i]
		if e.Block.SSLCert == refPath {
			out = append(out, SiteRef{SiteID: e.SiteID, ServerNames: nonEmpty(e.Block.ServerNames)})
		}
	}
	if out == nil {
		out = []SiteRef{}
	}
	return out
}

// domainSetEqual SAN 全集集合等值比较（I-03 同域名集合判定的集合语义——
// 顺序无关，大小写归一）。
func domainSetEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]bool, len(a))
	for _, d := range a {
		set[lower(d)] = true
	}
	for _, d := range b {
		if !set[lower(d)] {
			return false
		}
	}
	return true
}

func lower(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'A' && r <= 'Z' {
			out[i] = r + ('a' - 'A')
		}
	}
	return string(out)
}
