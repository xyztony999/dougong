package panel

import (
	"encoding/json"
	"time"
)

// DTO（camelCase JSON；时间 RFC3339 UTC 由 time.Time 默认编码满足）。
// 结构与 api-panel-core.md §4.1.4 接口详情逐字段对齐，禁止自行增删。

// DetailView 是 GET /panel/config/detail 与 POST /panel/config/update 的
// data 结构（配置单例全量视图，含生效分类与会话透传）。
type DetailView struct {
	Listen         ListenView      `json:"listen"`
	EntryPath      EntryPathView   `json:"entryPath"`
	HTTPS          HTTPSView       `json:"https"`
	UpdateCheck    UpdateCheckView `json:"updateCheck"`
	Lang           LangView        `json:"lang"`
	Session        SessionView     `json:"session"`
	PendingRestart bool            `json:"pendingRestart"`
}

// ListenView 监听配置组（唯一 restart_required 组，D-P02）。
type ListenView struct {
	Scope      string  `json:"scope"`
	Address    *string `json:"address"`
	Port       int     `json:"port"`
	EffectMode string  `json:"effectMode"`
}

// EntryPathView 安全入口组（修改即时生效——EntryPathGuard 下一请求即用新值）。
type EntryPathView struct {
	Enabled    bool    `json:"enabled"`
	Path       *string `json:"path"`
	EffectMode string  `json:"effectMode"`
}

// CertSummary 证书摘要（I-P06：仅 source/issuer/有效期，key_pem 永不下发）。
type CertSummary struct {
	Source    string     `json:"source"`
	Issuer    *string    `json:"issuer"`
	NotBefore *time.Time `json:"notBefore"`
	NotAfter  *time.Time `json:"notAfter"`
}

// HTTPSView 面板自身 HTTPS 组（证书材料热读取即时生效，D-P06）。
type HTTPSView struct {
	Mode       string       `json:"mode"`
	Cert       *CertSummary `json:"cert"`
	EffectMode string       `json:"effectMode"`
}

// UpdateCheckView 更新检查开关组（默认关，R-09/I-P05）。
type UpdateCheckView struct {
	Enabled    bool   `json:"enabled"`
	EffectMode string `json:"effectMode"`
}

// LangView 界面语言组（MVP 单值 zh-CN，I-P08）。
type LangView struct {
	Value      string   `json:"value"`
	Supported  []string `json:"supported"`
	EffectMode string   `json:"effectMode"`
}

// SessionView 会话参数只读透传组（D-P08：载体=配置文件，无 API 修改通道；
// 值为秒数整数——tasks.md 裁定 A-P2）。
type SessionView struct {
	AbsoluteTimeout int64 `json:"absoluteTimeout"`
	IdleTimeout     int64 `json:"idleTimeout"`
}

// UpdateConfigRequest POST /panel/config/update 入参：部分字段集（nil/缺省
// = 不更新）；entry_path 值不可经此接口设置（仅 regenerate，api 4.1.4）；
// 携带 session 字段即拒（D-P08）。
type UpdateConfigRequest struct {
	ListenScope      *string `json:"listenScope"`
	ListenAddress    *string `json:"listenAddress"`
	ListenPort       *int    `json:"listenPort"`
	EntryPathEnabled *bool   `json:"entryPathEnabled"`
	HTTPSMode        *string `json:"httpsMode"`
	UpdateCheck      *bool   `json:"updateCheck"`
	Lang             *string `json:"lang"`
	// Session 捕获请求中是否出现 session 字段（出现即 AUTH_012，不解析内容）。
	Session *json.RawMessage `json:"session"`
}

// AnySet 报告是否携带至少一个可更新字段。
func (r *UpdateConfigRequest) AnySet() bool {
	return r.ListenScope != nil || r.ListenAddress != nil || r.ListenPort != nil ||
		r.EntryPathEnabled != nil || r.HTTPSMode != nil || r.UpdateCheck != nil ||
		r.Lang != nil
}

// RegenerateEntryPathResult POST /panel/config/regenerate-entry-path 响应。
type RegenerateEntryPathResult struct {
	EntryPath string `json:"entryPath"`
}

// UploadCertificateRequest POST /panel/config/upload-certificate 入参
// （PEM 文本 JSON 字段，D-P05）。
type UploadCertificateRequest struct {
	CertPEM string `json:"certPem" binding:"required"`
	KeyPEM  string `json:"keyPem" binding:"required"`
}

// UploadCertificateResult 证书导入响应（摘要，无 PEM 材料）。
type UploadCertificateResult struct {
	Cert CertSummary `json:"cert"`
}

// UpdateCheckResult POST /panel/system/update-check 响应（过程数据不落库）。
type UpdateCheckResult struct {
	CurrentVersion  string    `json:"currentVersion"`
	LatestVersion   *string   `json:"latestVersion"`
	UpdateAvailable bool      `json:"updateAvailable"`
	CheckedAt       time.Time `json:"checkedAt"`
}
