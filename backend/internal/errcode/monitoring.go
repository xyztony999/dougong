package errcode

// monitoring 模块错误码（api-monitoring.md §3.4，MON_001~004；MON_ 前缀为本
// 模块新增，认证类复用 AUTH 码族）。symbol 即契约 code 列原值，经响应包络
// code 字段上线符号串。
var (
	// ErrMonitoringInvalidWindow MON_001 window 不在 metric_window 取值内（last_1h|last_24h）。
	ErrMonitoringInvalidWindow = New("MON_001", "window 取值非法（metric_window：last_1h|last_24h）")
	// ErrMonitoringInvalidDimension MON_002 dimension 不在 metric_dimension 取值内（cpu|memory|disk|network|load）。
	ErrMonitoringInvalidDimension = New("MON_002", "dimension 取值非法（metric_dimension：cpu|memory|disk|network|load）")
	// ErrMonitoringInvalidScope MON_003 scope 非法，或 dimension≠cpu 时携带 scope（D-04）。
	ErrMonitoringInvalidScope = New("MON_003", "scope 取值非法，或仅 dimension=cpu 时允许携带")
	// ErrMonitoringNoData MON_004 启动后尚无成功采样（快照/TOP/趋势均可能；code=0 空序列优先，见 D-07）。
	ErrMonitoringNoData = New("MON_004", "监控数据尚未就绪（启动后首轮采样未完成）")
)
