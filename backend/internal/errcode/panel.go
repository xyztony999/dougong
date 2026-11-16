package errcode

// panel-core 模块错误码（api-panel-core.md v2.1 §3.4，PANEL_001~012；PANEL_ 前缀
// 为本模块新增，鉴权/CSRF/参数校验复用 AUTH 码族）。symbol 即契约 code 列
// 原值，经响应包络 code 字段上线符号串。PANEL_007~011 为 v2.0 增量（ADR-028/029）；
// PANEL_012 为 v2.1 增量（ADR-030/D-P17 CLI 通道）。
var (
	// ErrCustomAddressRequired PANEL_001 listen_scope=custom 但监听地址缺失/非法（I-P02）。
	ErrCustomAddressRequired = New("PANEL_001", "监听范围为 custom 时必须提供合法监听地址")
	// ErrCertInvalid PANEL_002 证书 PEM 解析失败或证书/私钥不配对（I-P04，R-14）。
	ErrCertInvalid = New("PANEL_002", "证书解析失败或与私钥不配对，请检查 PEM 内容")
	// ErrCertRequired PANEL_003 https_mode 切至 imported/acme 但尚无对应来源的可用证书行（I-P04；v2.0 扩展 acme 分支，D-P14）。
	ErrCertRequired = New("PANEL_003", "尚无对应来源的可用证书，请先上传/申请证书")
	// ErrUpdateCheckDisabled PANEL_004 update_check=0（默认）时触发检查被拒（I-P05，R-09）。
	ErrUpdateCheckDisabled = New("PANEL_004", "更新检查未开启")
	// ErrUpdateCheckFailed PANEL_005 版本源不可达/超时/清单格式不符；不自动重试（D-P09）。
	ErrUpdateCheckFailed = New("PANEL_005", "版本源不可达或清单格式不符")
	// ErrLangNotSupported PANEL_006 语言取值不受支持（MVP 仅 zh-CN，I-P08）。
	ErrLangNotSupported = New("PANEL_006", "界面语言取值不受支持")
	// ErrBindDomainRequired PANEL_007 cert/request·retry-renewal 前置 bind_domain 未绑定（D-P12/D-P13，ADR-028）。
	ErrBindDomainRequired = New("PANEL_007", "尚未绑定域名，请先在面板设置中绑定域名")
	// ErrPanelNginxUnavailable PANEL_008 nginx 未检出或 80 挑战通道不可用（D-P15；
	// 变体名避开 site-mgmt SITE_012 同名哨兵）。
	ErrPanelNginxUnavailable = New("PANEL_008", "nginx 未检出或 80 挑战通道不可用")
	// ErrPanelAcmeFailed PANEL_009 ACME 签发/续期失败（挑战不通/CA 拒绝/超时，D-P13；
	// 变体名避开 site-mgmt CERT_004 同名哨兵）。
	ErrPanelAcmeFailed = New("PANEL_009", "ACME 签发或续期失败")
	// ErrAcmeStateInvalid PANEL_010 证书非 acme 来源或当前状态不允许该操作（D-P13）。
	ErrAcmeStateInvalid = New("PANEL_010", "证书当前状态不允许该操作")
	// ErrAcmeCertExists PANEL_011 已有 valid/expiring_soon 的 acme 证书再次申请（防 CA 配额浪费）。
	ErrAcmeCertExists = New("PANEL_011", "已有有效的 ACME 证书，续期请走续期通道")
	// ErrInitPasswordMissing PANEL_012 CLI reset-password 未设置 DOUGONG_INIT_PASSWORD
	// 环境变量（v2.1，ADR-030/D-P17：新密码仅经环境变量注入、不经 argv，同 init
	// 口径；CLI 退出码 2）。
	ErrInitPasswordMissing = New("PANEL_012", "未设置 DOUGONG_INIT_PASSWORD 环境变量（新密码仅经环境变量注入，不经命令行参数）")
)
