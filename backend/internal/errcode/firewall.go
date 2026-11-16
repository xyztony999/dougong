package errcode

// firewall 模块错误码（api-firewall.md §3.4，FW_001~010；FW_ 前缀为本模块
// 新增 10 个，鉴权/CSRF/越界/参数绑定复用 AUTH 码族）。symbol 即契约 code
// 列原值，经响应包络 code 字段上线符号串。
var (
	// ErrFwInvalidRuleParam FW_001 端口/端口段/协议/动作取值非法（1-65535、N≤M、枚举值域；R-14/D-05）。
	ErrFwInvalidRuleParam = New("FW_001", "端口/端口段/协议/动作取值非法")
	// ErrFwInvalidSource FW_002 来源限制 CIDR 非法（IPv4/IPv6 网段格式，D-05）。
	ErrFwInvalidSource = New("FW_002", "来源限制 CIDR 非法（须为 IPv4/IPv6 网段格式）")
	// ErrFwRuleNotFound FW_003 删除目标不存在（面板外已删，D-07；不静默成功）。
	ErrFwRuleNotFound = New("FW_003", "删除目标规则不存在（可能已在面板外被删除）")
	// ErrFwActionNotSupported FW_004 动作不在当前后端支持面（firewalld×deny/reject/limit，I-04/D-01）。
	ErrFwActionNotSupported = New("FW_004", "该动作不在当前防火墙后端支持面内")
	// ErrFwPanelPortAtRisk FW_005 面板端口涉险且未确认（I-05/D-06；data 携带命中详情，
	// confirmDanger=true 后重提交放行——保护不阻止操作本身）。
	ErrFwPanelPortAtRisk = New("FW_005", "操作命中面板监听端口，需二次确认")
	// ErrFwRuleAlreadyExists FW_006 等价规则已存在（ufw skipping/firewalld already enabled 语义透传，D-07）。
	ErrFwRuleAlreadyExists = New("FW_006", "等价规则已存在")
	// ErrFwRuntimeApplyFailed FW_007 firewalld permanent 已写入、runtime 应用失败（D-08 两层状态；不回滚）。
	ErrFwRuntimeApplyFailed = New("FW_007", "规则已写入永久配置，但运行态应用失败（重启后生效）")
	// ErrFwBackendExecFailed FW_008 后端命令执行失败（data.reason 如实携带）。
	ErrFwBackendExecFailed = New("FW_008", "防火墙后端命令执行失败")
	// ErrFwNoBackend FW_009 backend=none 时访问规则面（I-01：不返回空规则列表——无后端≠无规则）。
	ErrFwNoBackend = New("FW_009", "未检测到防火墙后端（ufw/firewalld）")
	// ErrFwBackendNotActive FW_010 后端已识别但不可用（如 firewalld 服务停止）；不返回过期数据（I-01）。
	ErrFwBackendNotActive = New("FW_010", "防火墙后端已识别但当前不可用（未运行）")
)
