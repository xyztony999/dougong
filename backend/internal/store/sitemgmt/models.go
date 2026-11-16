// Package sitemgmt 是网站管理（site-mgmt）模块的数据访问层（SQL 只允许出现在
// 本包；行结构体即 certificate 聚合根持久化模型，供 service 层接口引用——依赖
// 方向 api → service → store，本包不感知 HTTP 与业务编排）。
//
// certificate 为本模块唯一持久化对象（api-site-mgmt.md §5.3）；site 为配置文件
// 实时投影零持久化（D2 判定），不在此层。
package sitemgmt

import (
	"fmt"
	"time"

	"github.com/dougong/dougong/internal/store"
)

// Executor 是 store.Executor 的别名（*sql.DB 与 *sql.Tx 共同满足）。
type Executor = store.Executor

// cert_source / cert_status / acme_challenge 取值域（DDL CHECK 兜底，api §3.4）。
const (
	SourceAcme     = "acme"
	SourceImported = "imported"

	StatusPending      = "pending"
	StatusValid        = "valid"
	StatusExpiringSoon = "expiring_soon"
	StatusExpired      = "expired"
	StatusRenewFailed  = "renew_failed"

	ChallengeHTTP01  = "http_01"
	ChallengeTLSALPN = "tls_alpn"
	ChallengeDNS01   = "dns_01" // v2.0（⚠️A20/Gate 1 改判）

	// dns_provider 取值域（v2.0，DDL CHECK 兜底；初始清单=lego 官方内置 provider）。
	ProviderCloudflare = "cloudflare"
	ProviderAliyun     = "aliyun"
	ProviderDNSPod     = "dnspod"
)

// DeployRef 是 deploy_prev_refs JSON map 的单站点值：部署前 ssl_certificate/
// ssl_certificate_key 成对原值（A-SM14：成对快照保证 undeploy 后配对可用）。
type DeployRef struct {
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

// Certificate 对应 certificate 表一行（api §4.1.1 字段清单）。
// Issuer/NotBefore/NotAfter：acme 落库；imported 读时刷新呈现（D-08）。
type Certificate struct {
	ID              int64
	PrimaryDomain   string
	Domains         []string // SAN 列表（JSON 数组列的解析态）
	Issuer          string
	Source          string
	Status          string
	Challenge       *string // acme 必填（I-01 应用层校验）
	StoragePath     *string
	OriginPath      *string
	NotBefore       *time.Time
	NotAfter        *time.Time
	DeployPrevRefs  map[string]DeployRef // siteId → 部署前原值（D-11）
	DnsCredentialID *int64               // v2.0：DNS 凭证引用（仅 dns_01 行非空，I-01/I-C01）
	LastError       *string
	AcmeAccountRef  *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ---- 时间映射：与 001_account_auth/002_panel_core 同法，DATETIME 列以
// TEXT(RFC3339, UTC) 存储，字典序即时间序。 ----

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func formatTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatTime(*t)
}

func scanTime(src any) (time.Time, error) {
	switch v := src.(type) {
	case string:
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return time.Time{}, fmt.Errorf("store/sitemgmt: parse time %q: %w", v, err)
		}
		return t, nil
	case time.Time:
		return v, nil
	default:
		return time.Time{}, fmt.Errorf("store/sitemgmt: unexpected time column type %T", src)
	}
}

func scanTimePtr(src any) (*time.Time, error) {
	if src == nil {
		return nil, nil
	}
	t, err := scanTime(src)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// wrapErr 统一包装存储错误（保持 %w 解链）。
func wrapErr(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("store/sitemgmt: %s: %w", op, err)
}

// DnsCredential 对应 dns_credential 表一行（v2.0，⚠️A20；api §4.3.1）。
// CredentialEnc 为凭据材料密文（JSON→AES-256-GCM 单列；明文仅 service 层
// 签发/续期内存态解密，任何接口零回显 I-D01）。
type DnsCredential struct {
	ID            int64
	Name          string
	Provider      string
	CredentialEnc []byte
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
