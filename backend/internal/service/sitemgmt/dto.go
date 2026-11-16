package sitemgmt

import "time"

// ---- site/list · site/detail（§4.2.4）----

// SiteListData 是 GET /sites/site/list 的 data（实时投影 D-01；duplicates[]
// 重复定义呈现 D-03；nginx 缺席时空列表 + false，I-06 空态非错误）。
type SiteListData struct {
	NginxAvailable bool        `json:"nginxAvailable"`
	Items          []SiteItem  `json:"items"`
	Duplicates     []Duplicate `json:"duplicates"`
	GeneratedAt    string      `json:"generatedAt"`
}

// SiteItem 是 list 项（v2.0 D-17 聚合条目：同源文件且 server_name 全集相同
// 的多个 server 块合并——listen 合并去重、enabled=任一块生效、证书状态=组内
// 最优〔443/ssl 块优先〕、root=首个非空、proxy=组内首个反代段；寻址经
// blocks[].siteId，全部写接口仍单块寻址）。
type SiteItem struct {
	ServerNames []string    `json:"serverNames"`
	Listen      []string    `json:"listen"`
	Root        string      `json:"root"`
	Enabled     string      `json:"enabled"`
	CertRef     *string     `json:"certRef"`
	CertID      *int64      `json:"certId,omitempty"`
	CertStatus  *string     `json:"certStatus,omitempty"`
	Proxy       *ProxyDTO   `json:"proxy,omitempty"`
	SourceFile  string      `json:"sourceFile"`
	Blocks      []SiteBlock `json:"blocks"`
}

// SiteBlock 是聚合条目的块级子清单项（单块条目为 1 元素数组，结构统一）。
type SiteBlock struct {
	SiteID     string   `json:"siteId"`
	Listen     []string `json:"listen"`
	Enabled    string   `json:"enabled"`
	CertRef    *string  `json:"certRef"`
	CertID     *int64   `json:"certId,omitempty"`
	CertStatus *string  `json:"certStatus,omitempty"`
}

// SiteDetailData 是 GET /sites/site/detail 的 data（list 项全字段 + locations
// /Raw 块，§4.2.4）。
type SiteDetailData struct {
	SiteItem
	Locations []LocationDTO `json:"locations"`
	RawBlocks []RawBlockDTO `json:"rawBlocks"`
}

// ProxyDTO 是主反代段参数（§4.2.1 proxy 值对象；非反代站点为 null）。
type ProxyDTO struct {
	Upstream   string `json:"upstream"`
	HostHeader string `json:"hostHeader"`
	Websocket  bool   `json:"websocket"`
}

// LocationDTO 是 location 清单项（§4.2.1 locations）。
type LocationDTO struct {
	Path        string `json:"path"`
	ProxyPass   string `json:"proxyPass,omitempty"`
	FastCGIPass string `json:"fastcgiPass,omitempty"`
}

// RawBlockDTO 是 Raw 未识别指令块原文（R-07/A-SM4）。
type RawBlockDTO struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

// Duplicate 是重复定义组（同签名 server 块多文件/多处并存，D-03/A-SM17）。
type Duplicate struct {
	ServerNames []string `json:"serverNames"`
	Listen      []string `json:"listen"`
	Files       []string `json:"files"`
}

// ---- site/create（§4.2.4）----

// CreateSiteRequest 是 POST /sites/site/create 请求（三模板，FR-14~16）。
type CreateSiteRequest struct {
	Type        string   `json:"type"`
	ServerNames []string `json:"serverNames"`
	Listen      string   `json:"listen"`
	Root        string   `json:"root"`
	PoolName    string   `json:"poolName"`
	Upstream    string   `json:"upstream"`
	HostHeader  string   `json:"hostHeader"`
	Websocket   *bool    `json:"websocket"`
}

// CreateSiteData 是 create 响应。
type CreateSiteData struct {
	SiteID      string   `json:"siteId"`
	Type        string   `json:"type"`
	ServerNames []string `json:"serverNames"`
	Listen      string   `json:"listen"`
	SourceFile  string   `json:"sourceFile"`
}

// ---- site/update · proxy/update（§4.2.4）----

// UpdateSiteRequest 是 POST /sites/site/update 请求（结构化编辑）。
type UpdateSiteRequest struct {
	SiteID      string   `json:"siteId"`
	ServerNames []string `json:"serverNames"`
	Listen      []string `json:"listen"`
	Root        string   `json:"root"`
}

// UpdateSiteData 是 update 响应（新值）。
type UpdateSiteData struct {
	SiteID      string   `json:"siteId"`
	ServerNames []string `json:"serverNames"`
	Listen      []string `json:"listen"`
	Root        string   `json:"root"`
}

// UpdateProxyRequest 是 POST /sites/proxy/update 请求（M3 验收主场景）。
type UpdateProxyRequest struct {
	SiteID     string `json:"siteId"`
	Upstream   string `json:"upstream"`
	HostHeader string `json:"hostHeader"`
	Websocket  *bool  `json:"websocket"`
}

// UpdateProxyData 是 proxy/update 响应。
type UpdateProxyData struct {
	SiteID string   `json:"siteId"`
	Proxy  ProxyDTO `json:"proxy"`
}

// ---- site/source · source-save（§4.2.4，D-06 乐观锁）----

// SourceData 是 GET /sites/site/source 响应（Monaco 打开；baseMtime 基线）。
type SourceData struct {
	SiteID     string `json:"siteId"`
	SourceFile string `json:"sourceFile"`
	Content    string `json:"content"`
	BaseMtime  string `json:"baseMtime"`
	Size       int64  `json:"size"`
}

// SourceSaveRequest 是 POST /sites/site/source-save 请求。
type SourceSaveRequest struct {
	SiteID    string `json:"siteId"`
	Content   string `json:"content"`
	BaseMtime string `json:"baseMtime"`
}

// SourceSaveData 是 source-save 响应（新基线）。
type SourceSaveData struct {
	SiteID    string `json:"siteId"`
	BaseMtime string `json:"baseMtime"`
}

// ---- site/enable · site/disable（§4.2.4，D-16）----

// ToggleData 是启停响应（新投影值）。
type ToggleData struct {
	SiteID  string `json:"siteId"`
	Enabled string `json:"enabled"`
}

// ---- log/tail（§4.2.4，D-07）----

// LogTailData 是 GET /sites/log/tail 响应。
type LogTailData struct {
	SiteID    string    `json:"siteId"`
	Type      string    `json:"type"`
	LogPath   string    `json:"logPath"`
	Lines     []LogLine `json:"lines"`
	Truncated bool      `json:"truncated"`
}

// LogLine 是日志行（ts? 可选——无时间戳行原样呈现）。
type LogLine struct {
	TS   *string `json:"ts,omitempty"`
	Text string  `json:"text"`
}

// ---- nginx/reload（§4.2.4，D-05）----

// ReloadData 是 POST /sites/nginx/reload 响应。
type ReloadData struct {
	Reloaded bool `json:"reloaded"`
}

// ---- php-pools/list（§4.2.4，D-14）----

// PhpPoolsData 是 GET /sites/php-pools/list 响应。
type PhpPoolsData struct {
	Pools []PhpPoolDTO `json:"pools"`
}

// PhpPoolDTO 是 pool 候选项。
type PhpPoolDTO struct {
	Name   string `json:"name"`
	Socket string `json:"socket"`
}

// ---- cert/*（§4.1.4）----

// CertListData 是 GET /sites/cert/list 响应（读时推算 D-08 + 引用推导 D-11；
// storage_path/acme_account_ref 不下发——内部实现细节）。
type CertListData struct {
	Items       []CertItem `json:"items"`
	GeneratedAt string     `json:"generatedAt"`
}

// CertItem 是证书清单项。
type CertItem struct {
	ID              int64     `json:"id"`
	PrimaryDomain   string    `json:"primaryDomain"`
	Domains         []string  `json:"domains"`
	Issuer          string    `json:"issuer"`
	Source          string    `json:"source"`
	Status          string    `json:"status"`
	Challenge       *string   `json:"challenge,omitempty"`
	NotBefore       *string   `json:"notBefore,omitempty"`
	NotAfter        *string   `json:"notAfter,omitempty"`
	OriginPath      *string   `json:"originPath,omitempty"`
	LastError       *string   `json:"lastError,omitempty"`
	ReferencedSites []SiteRef `json:"referencedSites"`
	CreatedAt       string    `json:"createdAt"`
}

// SiteRef 是引用站点的推导结果（D-11）。
type SiteRef struct {
	SiteID      string   `json:"siteId"`
	ServerNames []string `json:"serverNames"`
}

// CertRequestRequest 是 POST /sites/cert/request 请求（v2.0 扩展：域名来源
// siteId 与 domains 二选一——domains 支持手动指定含泛域名 *.domain〔CERT_010〕；
// challenge=dns_01 时 dnsCredentialId 必填〔CERT_009〕）。
type CertRequestRequest struct {
	SiteID          string   `json:"siteId"`
	Domains         []string `json:"domains"`
	Challenge       string   `json:"challenge"`
	DNSCredentialID *int64   `json:"dnsCredentialId"`
}

// CertRequestData 是签发成功响应（同步终态，D-09）。
type CertRequestData struct {
	CertID   int64  `json:"certId"`
	Status   string `json:"status"`
	NotAfter string `json:"notAfter"`
}

// CertRefRequest 是 deploy/undeploy 共用请求。
type CertRefRequest struct {
	CertID int64  `json:"certId"`
	SiteID string `json:"siteId"`
}

// CertRefData 是 deploy/undeploy 响应（certRef=新引用值/恢复后的原引用值）。
type CertRefData struct {
	CertID  int64  `json:"certId"`
	SiteID  string `json:"siteId"`
	CertRef string `json:"certRef"`
}

// CertIDRequest 是 retry-renewal/delete 共用请求。
type CertIDRequest struct {
	CertID int64 `json:"certId"`
}

// CertDeleteData 是 delete 响应。
type CertDeleteData struct {
	CertID int64 `json:"certId"`
}

// ---- cert/renew · cert/replace（v1.2，D-18）----

// CertRenewData 是 cert/renew 响应（与 CertRequestData 同形）。
type CertRenewData = CertRequestData

// CertReplaceRequest 是 POST /sites/cert/replace 请求（imported 文件替换）。
type CertReplaceRequest struct {
	CertID  int64  `json:"certId"`
	CertPEM string `json:"certPem"`
}

// CertReplaceData 是 replace 响应（新证书读时解析元数据；域名差异由前端呈现）。
type CertReplaceData struct {
	CertID    int64    `json:"certId"`
	NotBefore *string  `json:"notBefore,omitempty"`
	NotAfter  *string  `json:"notAfter,omitempty"`
	Issuer    string   `json:"issuer"`
	Domains   []string `json:"domains"`
}

// ---- dns-credential（v2.0，⚠️A20/D-20）----

// DnsCredentialItem 是凭证清单项（凭据材料零回显 I-D01）。
type DnsCredentialItem struct {
	ID                  int64  `json:"id"`
	Name                string `json:"name"`
	Provider            string `json:"provider"`
	CreatedAt           string `json:"createdAt"`
	ReferencedCertCount int    `json:"referencedCertCount"`
}

// DnsCredentialListData 是 GET /sites/dns-credential/list 响应。
type DnsCredentialListData struct {
	Items []DnsCredentialItem `json:"items"`
}

// DnsCredentialPayload 是凭据材料（按 provider 形态；明文仅本请求到达 service
// 即加密，任何响应不回显）。
type DnsCredentialPayload struct {
	APIToken        string `json:"apiToken,omitempty"`        // cloudflare
	AccessKeyID     string `json:"accessKeyId,omitempty"`     // aliyun
	AccessKeySecret string `json:"accessKeySecret,omitempty"` // aliyun
	SecretID        string `json:"secretId,omitempty"`        // dnspod
	SecretKey       string `json:"secretKey,omitempty"`       // dnspod
}

// DnsCredentialCreateRequest 是 POST /sites/dns-credential/create 请求。
type DnsCredentialCreateRequest struct {
	Name       string               `json:"name"`
	Provider   string               `json:"provider"`
	Credential DnsCredentialPayload `json:"credential"`
}

// DnsCredentialCreateData 是 create 响应（零凭据材料）。
type DnsCredentialCreateData struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

// DnsCredentialDeleteData 是 delete 响应（referencedCerts 引用警示不硬阻断，I-C01）。
type DnsCredentialDeleteData struct {
	ID              int64            `json:"id"`
	ReferencedCerts []DnsCertRefItem `json:"referencedCerts"`
}

// DnsCertRefItem 是凭证删除时的引用证书呈现项（I-C02 实时推导）。
type DnsCertRefItem struct {
	CertID        int64  `json:"certId"`
	PrimaryDomain string `json:"primaryDomain"`
}

// ---- 错误 data 载荷（§3.4）----

// SiteErrorData 是 SITE_003/SITE_004 的 data（errors[]+diff / output）。
type SiteErrorData struct {
	Errors []string `json:"errors,omitempty"`
	Diff   string   `json:"diff,omitempty"`
	Output string   `json:"output,omitempty"`
}

// ReasonData 是单 reason 载荷（SITE_010 currentMtime / SITE_012 reason /
// SITE_013 reason / CERT_004 reason 等）。
type ReasonData struct {
	Reason       string `json:"reason,omitempty"`
	CurrentMtime string `json:"currentMtime,omitempty"`
	Status       string `json:"status,omitempty"` // v1.2：cert/renew 失败迁移后状态（D-18 分流）
}

// ExistingSiteData 是 SITE_005 载荷（data.existingSiteId）。
type ExistingSiteData struct {
	ExistingSiteID string `json:"existingSiteId"`
}

// ExistingCertData 是 CERT_003 载荷（data.existingCertId）。
type ExistingCertData struct {
	ExistingCertID int64 `json:"existingCertId"`
}

// StatusData 是 CERT_002 载荷（data.currentStatus；v2.0：acme 调用 replace
// 时附 reason=source）。
type StatusData struct {
	CurrentStatus string `json:"currentStatus"`
	Reason        string `json:"reason,omitempty"`
}

// CertRefSitesData 是 CERT_005 载荷（data.sites[] 含 siteId/serverNames）。
type CertRefSitesData struct {
	Sites []SiteRef `json:"sites"`
}

// formatTS 统一 RFC3339 UTC 字符串（api §3.1 时间格式）。
func formatTS(t time.Time) string { return t.UTC().Format(time.RFC3339) }
