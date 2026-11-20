package appmgmt

// 审计细粒度动作码与对象类型（api-app-mgmt.md §3.4 业务常量，非枚举池条目；
// category 均为 change；detail 不含环境变量值与启动参数值 ⚠️A9）。
const (
	AuditActionTaskStarted      = "app_task_started"
	AuditActionTaskFinished     = "app_task_finished"
	AuditActionServiceStarted   = "app_service_started"
	AuditActionServiceStopped   = "app_service_stopped"
	AuditActionServiceRestarted = "app_service_restarted"
	AuditActionServiceEnabled   = "app_service_enabled"
	AuditActionServiceDisabled  = "app_service_disabled"
	AuditActionParamsUpdated    = "app_params_updated"
	AuditActionSiteLinked       = "app_site_linked"

	ObjTypeTask = "app_task"
	ObjTypeApp  = "app"
)

// 创建计划承载（D-05：create 参数经任务输出首行 plan:{json} 传递——worker 与
// 发起解耦；重启后 pending 不续跑〔I-08 保守〕，参数不丢台账语义）。
const planPrefix = "plan:"
