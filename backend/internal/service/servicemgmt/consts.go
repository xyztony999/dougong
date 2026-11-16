package servicemgmt

import (
	"regexp"
	"time"
)

// ActionWaitLimit 操作终态等待上限（D-01：90s 编译期常量，非配置键——
// api §3.4「无新增配置键」；对齐 systemd 默认 TimeoutStartSec/TimeoutStopSec
// 上限情形）。
const ActionWaitLimit = 90 * time.Second

// TimeoutNote SVC_006 提示文案（api §4.1.4：note=「任务仍在执行，请稍后刷新
// 列表确认终态」）。
const TimeoutNote = "任务仍在执行，请稍后刷新列表确认终态"

// unitNamePattern 名称形态正则（D-09/SVC_001 校验依据；后缀集=unit_type
// 官方全集，api §3.4 业务常量表）。
const unitNamePattern = `^[A-Za-z0-9@._-]+\.(service|socket|timer|mount|path|target|automount|slice|swap|scope|device)$`

var unitNameRe = regexp.MustCompile(unitNamePattern)

// service_active_state 枚举（引用 01-solution/04-business-enum.md，无新增）。
const (
	ActiveStateActive   = "active"
	ActiveStateInactive = "inactive"
	ActiveStateFailed   = "failed"
)

// service_enabled_state 两态主轴（实际 enabledState 透传可含非两态值，D-05）。
const (
	EnabledStateEnabled  = "enabled"
	EnabledStateDisabled = "disabled"
)

// TypeService unit_type 枚举中唯一可操作类型（I-04 动作边界）。
const TypeService = "service"

// 审计业务常量（api §3.4：操作审计细粒度动作码——action 列存此码，
// category=change，object_type=systemd_service，object_id=unit 全名）。
const (
	AuditActionServiceStarted   = "service_started"
	AuditActionServiceStopped   = "service_stopped"
	AuditActionServiceRestarted = "service_restarted"
	AuditActionServiceEnabled   = "service_enabled"
	AuditActionServiceDisabled  = "service_disabled"
	ObjectTypeSystemdService    = "systemd_service"
)
