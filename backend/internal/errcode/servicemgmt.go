package errcode

// service-mgmt 模块错误码（api-service-mgmt.md §3.4，SVC_001~008；SVC_ 前缀
// 为本模块新增，鉴权/CSRF/越界/参数绑定复用 AUTH 码族）。symbol 即契约 code
// 列原值，经响应包络 code 字段上线符号串。
var (
	// ErrSvcInvalidUnitName SVC_001 unit 名称形态非法（非 名称.类型后缀 官方格式，D-09/R-14）。
	ErrSvcInvalidUnitName = New("SVC_001", "unit 名称形态非法（须为 名称.类型后缀 官方格式）")
	// ErrSvcInvalidStateFilter SVC_002 state 过滤值非法（不在 service_active_state 取值内）。
	ErrSvcInvalidStateFilter = New("SVC_002", "state 过滤值非法（active|inactive|failed 多值）")
	// ErrSvcUnitNotFound SVC_003 目标 unit 不存在（已卸载或名称错误，I-03 实况校验）。
	ErrSvcUnitNotFound = New("SVC_003", "目标 unit 不存在")
	// ErrSvcUnitTypeNotActionable SVC_004 非 .service 类型发起操作（I-04 动作边界）。
	ErrSvcUnitTypeNotActionable = New("SVC_004", "仅 .service 类型 unit 支持此操作")
	// ErrSvcAutostartNotToggleable SVC_005 自启状态不可切换（enabledState 非两态主轴，⚠️A14/D-05）。
	ErrSvcAutostartNotToggleable = New("SVC_005", "自启状态不可切换（非 enabled/disabled 两态）")
	// ErrSvcOperationTimeout SVC_006 等待终态超 90s 上限（D-01）；Job 仍在执行，终态经列表刷新确认。
	ErrSvcOperationTimeout = New("SVC_006", "操作超时（任务仍在执行，终态经列表刷新确认）")
	// ErrSvcOperationFailed SVC_007 systemd 侧 Job 执行失败；data 携带失败原因与实际终态。
	ErrSvcOperationFailed = New("SVC_007", "操作执行失败")
	// ErrSvcSystemdUnavailable SVC_008 dbus 通道/systemd 不可达（I-01：不返回过期数据）。
	ErrSvcSystemdUnavailable = New("SVC_008", "systemd 通道不可用")
)
