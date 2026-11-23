// consts.go 承载模块业务常量（api §3.4）：审计动作码（13 个，非枚举池
// 条目）、审计对象类型、tail 参数域、动作序。
package dockermgmt

import "github.com/dougong/dougong/internal/adapter/dockerx"

// 操作审计细粒度动作码（api §3.4 业务常量；action 列存此码，
// category=audit_action.change）。
const (
	AuditContainerStarted   = "container_started"
	AuditContainerStopped   = "container_stopped"
	AuditContainerRestarted = "container_restarted"
	AuditContainerPaused    = "container_paused"
	AuditContainerUnpaused  = "container_unpaused"
	AuditContainerDeleted   = "container_deleted"
	AuditImagePulled        = "image_pulled"
	AuditImageDeleted       = "image_deleted"
	AuditVolumeDeleted      = "volume_deleted"
	AuditNetworkDeleted     = "network_deleted"
	AuditComposeUp          = "compose_up"
	AuditComposeDown        = "compose_down"
	AuditComposeRestarted   = "compose_restarted"
)

// 审计对象类型（object_type，api §3.4）。
const (
	ObjectTypeContainer      = "container"
	ObjectTypeImage          = "image"
	ObjectTypeVolume         = "volume"
	ObjectTypeNetwork        = "network"
	ObjectTypeComposeProject = "compose_project"
)

// tail 参数域（D1 口径-3 参数化：缺省 500、取值 1-2000；非法 DK_001）。
const (
	TailDefault = 500
	TailMin     = 1
	TailMax     = 2000
)

// 动作/动词 re-export（api 层薄壳消费，保持 api→service→adapter 依赖单向）。
const (
	ActionStart   = dockerx.ActionStart
	ActionStop    = dockerx.ActionStop
	ActionRestart = dockerx.ActionRestart
	ActionPause   = dockerx.ActionPause
	ActionUnpause = dockerx.ActionUnpause
	ActionDelete  = dockerx.ActionDelete

	VerbUp      = dockerx.VerbUp
	VerbDown    = dockerx.VerbDown
	VerbRestart = dockerx.VerbRestart
	VerbStop    = dockerx.VerbStop
)

// containerActionOrder 动作权威序（supportedActions 计算按此序过滤输出，D-02）。
var containerActionOrder = []string{
	dockerx.ActionStart, dockerx.ActionStop, dockerx.ActionRestart,
	dockerx.ActionPause, dockerx.ActionUnpause, dockerx.ActionDelete,
}

// 状态筛选合法值域（container_status 三态 + 过渡态原始值，D-02）。
var validStateFilters = map[string]bool{
	dockerx.StatusRunning: true,
	dockerx.StatusPaused:  true,
	dockerx.StatusStopped: true,
	"restarting":          true,
	"removing":            true,
}

// v1.1 创建类审计动作（api-docker-mgmt.md v1.1 §3.4）。
const (
	AuditContainerCreated      = "container_created"
	AuditVolumeCreated         = "volume_created"
	AuditNetworkCreated        = "network_created"
	AuditComposeProjectCreated = "compose_project_created"
)

// v1.2 面板应用与 Compose 生命周期审计动作（ADR-033，api-docker-mgmt.md v1.2）。
const (
	AuditAppCreated            = "app_created"
	AuditAppService            = "app_service_op"
	AuditAppDeleted            = "app_deleted"
	AuditAppSiteLinked         = "app_site_linked"
	AuditComposeProjectDeleted = "compose_project_deleted"
)

// ObjectTypeApp 面板应用的审计对象类型。
const ObjectTypeApp = "docker_app"

// 应用状态（docker_app_state 枚举承载值；实况聚合，非状态机）。
const (
	AppStateRunning = "running"
	AppStateStopped = "stopped"
	AppStatePartial = "partial"
)
