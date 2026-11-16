package envmgmt

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	storeenv "github.com/dougong/dougong/internal/store/envmgmt"
)

// 业务常量（api-env-mgmt.md §3.4）。
const (
	// TaskOutputTailLines 任务输出尾部采集行数（D-08）。
	TaskOutputTailLines = 200
	// TaskListPageSize 任务列表默认分页。
	TaskListPageSize = 50
)

// 审计动作码与对象类型（api §3.4 业务常量）。
const (
	AuditActionEnvInstallStarted   = "env_install_started"
	AuditActionEnvUpgradeStarted   = "env_upgrade_started"
	AuditActionEnvUninstallStarted = "env_uninstall_started"
	AuditActionEnvTaskFinished     = "env_task_finished"
	AuditActionEnvServiceStarted   = "env_service_started"
	AuditActionEnvServiceStopped   = "env_service_stopped"
	AuditActionEnvServiceRestarted = "env_service_restarted"
	AuditActionEnvServiceEnabled   = "env_service_enabled"
	AuditActionEnvServiceDisabled  = "env_service_disabled"
	ObjectTypeEnvTask              = "env_install_task"
	ObjectTypeEnv                  = "env"
)

// ---- DTO（api §4.1.4）----

// ChannelDTO 通道状态。
type ChannelDTO struct {
	Family    string `json:"family"`
	Available bool   `json:"available"`
}

// EnvItem 环境全景条目（目录+实况+在途标记；探测不可用字段为 null，D-01）。
type EnvItem struct {
	EnvType        string  `json:"envType"`
	DisplayName    string  `json:"displayName"`
	Description    string  `json:"description"`
	RelatedModule  string  `json:"relatedModule"`
	Installed      *bool   `json:"installed"`
	Version        *string `json:"version"`
	ServiceRunning *bool   `json:"serviceRunning"`
	ServiceEnabled *bool   `json:"serviceEnabled"`
	UnitName       string  `json:"unitName"`
	RunningTaskID  *int64  `json:"runningTaskId"`
}

// EnvListData GET /env/env/list 响应。
type EnvListData struct {
	Channel     ChannelDTO `json:"channel"`
	Items       []EnvItem  `json:"items"`
	GeneratedAt string     `json:"generatedAt"`
}

// TaskCreatedData install/uninstall 响应。
type TaskCreatedData struct {
	TaskID int64  `json:"taskId"`
	Action string `json:"action"`
}

// TaskItem 任务清单项。
type TaskItem struct {
	ID          int64   `json:"id"`
	EnvType     string  `json:"envType"`
	Action      string  `json:"action"`
	Status      string  `json:"status"`
	RequestedBy string  `json:"requestedBy"`
	CreatedAt   string  `json:"createdAt"`
	StartedAt   *string `json:"startedAt"`
	FinishedAt  *string `json:"finishedAt"`
	Error       *string `json:"error"`
}

// TaskListData GET /env/task/list 响应。
type TaskListData struct {
	Items []TaskItem `json:"items"`
	Total int        `json:"total"`
}

// TaskDetailData GET /env/task/detail 响应（+输出尾部）。
type TaskDetailData struct {
	TaskItem
	OutputTail string `json:"outputTail"`
}

// ServiceOpData service/{start,...} 响应。
type ServiceOpData struct {
	EnvType string `json:"envType"`
	Unit    string `json:"unit"`
}

// ---- 任务与服务操作编排 ----

// TaskList 任务台账（筛选+分页，created_at 倒序）。
func (s *Service) TaskList(ctx context.Context, envType, status string, page, pageSize int) (*TaskListData, error) {
	if envType != "" && CatalogByType(envType) == nil {
		return nil, errcodeInvalidEnvType()
	}
	known := map[string]bool{storeenv.TaskPending: true, storeenv.TaskRunning: true,
		storeenv.TaskSucceeded: true, storeenv.TaskFailed: true}
	if status != "" && !known[status] {
		return nil, errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "status", Message: "status 筛选值非法（env_task_status 取值域）"})
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = TaskListPageSize
	}
	rows, total, err := s.store.Task().List(ctx, envType, status, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, fmt.Errorf("envmgmt: task list: %w", err)
	}
	items := make([]TaskItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, taskItemOf(r))
	}
	return &TaskListData{Items: items, Total: total}, nil
}

// TaskDetail 单任务详情（输出尾部排障）。
func (s *Service) TaskDetail(ctx context.Context, taskID int64) (*TaskDetailData, error) {
	r, err := s.store.Task().GetByID(ctx, taskID)
	if err != nil {
		return nil, errcode.ErrEnvTaskNotFound
	}
	return &TaskDetailData{TaskItem: taskItemOf(r), OutputTail: r.OutputTail}, nil
}

func taskItemOf(r storeenv.Task) TaskItem {
	return TaskItem{
		ID: r.ID, EnvType: r.EnvType, Action: r.Action, Status: r.Status,
		RequestedBy: r.RequestedBy, CreatedAt: formatTS(r.CreatedAt),
		StartedAt: timePtrStr(r.StartedAt), FinishedAt: timePtrStr(r.FinishedAt),
		Error: r.Error,
	}
}

// ServiceOp service/{start,stop,restart,enable,disable}（D-09：目录单元限定、
// 已装前置、SysdGateway 复用；结果如实 ENV_006）。
func (s *Service) ServiceOp(ctx context.Context, actor *authsvc.Identity, envType, op string) (*ServiceOpData, error) {
	entry := CatalogByType(envType)
	if entry == nil {
		return nil, errcodeInvalidEnvType()
	}
	ch := s.pack.DetectChannel()
	unit := entry.UnitByFamily[ch.Family]
	if unit == "" {
		return nil, errcodeNotInstalled()
	}
	if ch.Available {
		if _, ok, err := s.pack.QueryInstalled(ctx, ch.Family, entry.PkgByFamily[ch.Family]); err == nil && !ok {
			return nil, errcodeNotInstalled()
		}
	}
	var opErr error
	switch op {
	case "start":
		_, opErr = s.sysd.StartUnit(ctx, unit)
	case "stop":
		_, opErr = s.sysd.StopUnit(ctx, unit)
	case "restart":
		_, opErr = s.sysd.RestartUnit(ctx, unit)
	case "enable":
		opErr = s.sysd.EnableUnit(ctx, unit)
	case "disable":
		opErr = s.sysd.DisableUnit(ctx, unit)
	default:
		return nil, errcode.ErrValidation.WithFields(errcode.FieldError{
			Field: "op", Message: "服务动作非法"})
	}
	if opErr != nil {
		return nil, errcode.ErrEnvUnitOpFailed.WithData(map[string]any{"reason": opErr.Error()})
	}
	s.recordAudit(ctx, actor, serviceAuditAction(op), ObjectTypeEnv, envType,
		map[string]any{"envType": envType, "unit": unit})
	return &ServiceOpData{EnvType: envType, Unit: unit}, nil
}

func serviceAuditAction(op string) string {
	switch op {
	case "start":
		return AuditActionEnvServiceStarted
	case "stop":
		return AuditActionEnvServiceStopped
	case "restart":
		return AuditActionEnvServiceRestarted
	case "enable":
		return AuditActionEnvServiceEnabled
	}
	return AuditActionEnvServiceDisabled
}

func auditActionFor(action string, _ bool) string {
	switch action {
	case storeenv.ActionUpgrade:
		return AuditActionEnvUpgradeStarted
	case storeenv.ActionUninstall:
		return AuditActionEnvUninstallStarted
	}
	return AuditActionEnvInstallStarted
}

// recordAudit 服务操作审计（D-12 常规口径：失败重试一次后日志兜底，不影响
// 操作结果返回）。
func (s *Service) recordAudit(ctx context.Context, actor *authsvc.Identity,
	action, objectType, objectID string, detail map[string]any) {
	e := auditEntry(actor, action, objectType, objectID, detail)
	if err := s.audit(ctx, e); err != nil {
		if err := s.audit(ctx, e); err != nil {
			slog.WarnContext(ctx, "envmgmt: audit record degraded", "action", action, "err", err)
		}
	}
}

// slog 引用（recordAudit 兜底日志）。

// auditEntry 组装审计条目。
func auditEntry(actor *authsvc.Identity, action, objectType, objectID string, detail map[string]any) audit.Entry {
	e := audit.Entry{
		Category: audit.CategoryChange, Action: action,
		ObjectType: objectType, ObjectID: objectID,
		Detail: detail, Result: audit.ResultSuccess, Source: audit.SourceWeb,
	}
	if actor != nil {
		id := actor.UserID
		e.ActorID = &id
		e.ActorName = actor.Username
	} else {
		e.ActorName = "unknown"
	}
	return e
}

// ---- errcode 便捷构造 ----

func errcodeInvalidEnvType() *errcode.Error { return errcode.ErrEnvInvalidType }
func errcodeChannelUnavailable(reason string) *errcode.Error {
	return errcode.ErrEnvChannelUnavailable.WithData(map[string]any{"reason": reason})
}
func errcodeTaskInFlight(taskID int64) *errcode.Error {
	return errcode.ErrEnvTaskInFlight.WithData(map[string]any{"taskId": taskID})
}
func errcodeNotInstalled() *errcode.Error {
	return errcode.ErrEnvNotInstalled.WithData(map[string]any{"installed": false})
}

func formatTS(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func timePtrStr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	v := formatTS(*t)
	return &v
}
