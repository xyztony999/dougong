package errcode

// panel-core 模块错误码（api-panel-core.md §3.4，PANEL_001~006；PANEL_ 前缀
// 为本模块新增，鉴权/CSRF/参数校验复用 AUTH 码族）。symbol 即契约 code 列
// 原值，经响应包络 code 字段上线符号串。
var (
	// ErrCustomAddressRequired PANEL_001 listen_scope=custom 但监听地址缺失/非法（I-P02）。
	ErrCustomAddressRequired = New("PANEL_001", "监听范围为 custom 时必须提供合法监听地址")
	// ErrCertInvalid PANEL_002 证书 PEM 解析失败或证书/私钥不配对（I-P04，R-14）。
	ErrCertInvalid = New("PANEL_002", "证书解析失败或与私钥不配对，请检查 PEM 内容")
	// ErrCertRequired PANEL_003 https_mode 切至 imported 但尚无证书行（I-P04）。
	ErrCertRequired = New("PANEL_003", "尚未导入证书，请先上传证书与私钥")
	// ErrUpdateCheckDisabled PANEL_004 update_check=0（默认）时触发检查被拒（I-P05，R-09）。
	ErrUpdateCheckDisabled = New("PANEL_004", "更新检查未开启")
	// ErrUpdateCheckFailed PANEL_005 版本源不可达/超时/清单格式不符；不自动重试（D-P09）。
	ErrUpdateCheckFailed = New("PANEL_005", "版本源不可达或清单格式不符")
	// ErrLangNotSupported PANEL_006 语言取值不受支持（MVP 仅 zh-CN，I-P08）。
	ErrLangNotSupported = New("PANEL_006", "界面语言取值不受支持")
)
