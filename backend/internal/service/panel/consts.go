package panel

import "time"

// 配置生效方式（config_effect_mode 枚举，D1 AA 附录C：仅 listen.* 为
// restart_required，其余全部 immediate——api-panel-core.md §3.4）。
const (
	EffectImmediate       = "immediate"
	EffectRestartRequired = "restart_required"
)

// 监听端口范围（I-P02；与 DDL CHECK 同值）。
const (
	ListenPortMin = 1024
	ListenPortMax = 65535
)

// 界面语言（I-P08：MVP 仅 zh-CN；多语言二期再登记 panel_lang 枚举）。
const (
	LangZhCN = "zh-CN"
)

// SupportedLangs 是 lang.supported 响应值（detail 只读下发）。
var SupportedLangs = []string{LangZhCN}

// 安全入口路径（I-P03：生成器输出空间 ^[A-Za-z0-9_-]{8,32}$）。
const (
	entryPathLen = 16 // 字符集 64 项 → 96 bit 熵（模 256 无偏）
)

// 自签证书参数（D-P06：切换 self_signed 即时生成；825 天为自签证书常规
// 有效期，非契约值——到期经「重新生成/重新导入」更换）。
const selfSignedValidity = 825 * 24 * time.Hour

// UpdateCheckTimeout 更新检查外呼超时上限（编译期常量，api §3.2 N1：
// 待 B 阶段实测校准；单次触发单次请求，无自动重试——D-P09/R-09）。
const UpdateCheckTimeout = 10 * time.Second

// 面板证书 ACME 常量（v2.0，api §3.4 业务常量表，ADR-029）。
const (
	// AcmeTimeout ACME 签发/续期同步等待上限，超时即失败（D-P13；
	// 值沿 site-mgmt D-09 同值先例）。
	AcmeTimeout = 120 * time.Second
	// CertExpiringDays expiring_soon 推算阈值（D-P13；ADR-024 阈值同值，不可配）。
	CertExpiringDays = 30
	// ChallengePanelCert 面板证书挑战方式固定 HTTP-01（ACME_CHALLENGE 面板侧，
	// ADR-029；非请求参数）。
	ChallengePanelCert = "http_01"
)

// 审计细粒度动作码与对象类型（api-panel-core.md v2.0 §3.4 业务常量，
// category=audit_action.change）。
const (
	ActionPanelConfigChanged        = "panel_config_changed"
	ActionPanelCertUpdated          = "panel_cert_updated"
	ActionPanelEntryPathRegenerated = "panel_entry_path_regenerated"
	ActionPanelCertRequested        = "panel_cert_requested"       // v2.0（D-P13）
	ActionPanelCertRenewed          = "panel_cert_renewed"         // v2.0（source=system 后台动作）
	ActionPanelCertRenewalRetried   = "panel_cert_renewal_retried" // v2.0（手动重试）

	ObjectTypePanelConfig = "panel_config"
	ObjectTypePanelCert   = "panel_cert"

	// SingletonObjectID 单行表对象标识（id=1）。
	SingletonObjectID = "1"
)
