// Package servicemgmt 是 systemd 服务管理应用服务（api-service-mgmt.md §4.1）：
// ServiceProjector（清单投影 + 过滤搜索）、UnitActionService（动作编排：
// 校验 → 提交 → 等待终态 → 审计）、SysdGateway 契约消费（adapter/sysd 实现）。
// 零持久化（D-07）：无表无 SQL，投影恒为当次 dbus 读取快照（I-01 无缓存层）。
package servicemgmt

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/dougong/dougong/internal/adapter/sysd"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
)

// Service 是服务管理门面（7 接口的业务承载；handler 薄壳，不感知 HTTP）。
type Service struct {
	gw    Gateway
	audit AuditRecorder
	clock func() time.Time
}

// New 构造服务（clock 为 nil 用 time.Now；rec 为审计 OHS，R-03）。
func New(gw Gateway, rec AuditRecorder) *Service {
	return &Service{gw: gw, audit: rec, clock: time.Now}
}

// List GET /services/unit/list（admin+readonly）：即时读取已装全集（I-01/I-03）
// → 服务端过滤/搜索（SVC_002/D-02）→ 名称字典序升序；generatedAt=投影生成
// 时刻（数据新旧可辨识）。
func (s *Service) List(ctx context.Context, stateFilter, search string) (*ListData, error) {
	states, err := parseStateFilter(stateFilter)
	if err != nil {
		return nil, err
	}
	units, err := s.gw.ListUnits(ctx)
	if err != nil {
		return nil, mapGatewayError(err)
	}
	items := projectList(units, states, search)
	return &ListData{Items: items, Total: len(items), GeneratedAt: formatTS(s.clock())}, nil
}

// Detail GET /services/unit/detail（admin+readonly）：名称形态（SVC_001）→
// 存在性（SVC_003）→ 属性快照（I-01 实况）。
func (s *Service) Detail(ctx context.Context, name string) (*DetailData, error) {
	if e := guardUnitName(name); e != nil {
		return nil, e
	}
	d, err := s.gw.GetUnit(ctx, name)
	if err != nil {
		return nil, mapGatewayError(err)
	}
	return projectDetail(d), nil
}

// Start POST /services/unit/start（admin）：提交并等待 Job 终态 ≤90s（D-01）。
func (s *Service) Start(ctx context.Context, actor *authsvc.Identity, name string) (*ActionData, error) {
	return s.jobAction(ctx, actor, AuditActionServiceStarted, name, s.gw.StartUnit)
}

// Stop POST /services/unit/stop（admin）：同 start 结构（等待终态）。
func (s *Service) Stop(ctx context.Context, actor *authsvc.Identity, name string) (*ActionData, error) {
	return s.jobAction(ctx, actor, AuditActionServiceStopped, name, s.gw.StopUnit)
}

// Restart POST /services/unit/restart（admin）：active/inactive/failed 均可发起。
func (s *Service) Restart(ctx context.Context, actor *authsvc.Identity, name string) (*ActionData, error) {
	return s.jobAction(ctx, actor, AuditActionServiceRestarted, name, s.gw.RestartUnit)
}

// Enable POST /services/unit/enable（admin）：开机自启启用（轻量同步）。
func (s *Service) Enable(ctx context.Context, actor *authsvc.Identity, name string) (*AutostartData, error) {
	return s.toggleAutostart(ctx, actor, AuditActionServiceEnabled, name, EnabledStateEnabled)
}

// Disable POST /services/unit/disable（admin）：开机自启禁用（轻量同步）。
func (s *Service) Disable(ctx context.Context, actor *authsvc.Identity, name string) (*AutostartData, error) {
	return s.toggleAutostart(ctx, actor, AuditActionServiceDisabled, name, EnabledStateDisabled)
}

// jobAction 是 start/stop/restart 共用编排（UnitActionService，I-05/I-06）：
// 校验链（D-09：形态→存在性→类型）→ 提交并等待终态 → 操作后快照 →
// 恰好一条审计（A-S5：guard 拒绝也记 failure，detail.error=错误码）。
func (s *Service) jobAction(ctx context.Context, actor *authsvc.Identity, action, name string,
	submit func(context.Context, string) (sysd.JobResult, error)) (*ActionData, error) {
	if e := guardUnitName(name); e != nil {
		s.auditAction(ctx, actor, action, name, audit.ResultFailure, detailError(e))
		return nil, e
	}
	pre, err := s.gw.GetUnit(ctx, name) // 存在性实况校验（SVC_003，I-03；D-09 逐级第二）
	if err != nil {
		gerr := mapGatewayError(err)
		s.auditAction(ctx, actor, action, name, audit.ResultFailure, detailError(gerr))
		return nil, gerr
	}
	if e := guardUnitType(name); e != nil { // 类型边界（SVC_004；D-09 逐级第三）
		s.auditAction(ctx, actor, action, name, audit.ResultFailure, detailError(e))
		return nil, e
	}
	res, err := submit(ctx, name)
	if err != nil {
		if errors.Is(err, sysd.ErrJobTimeout) {
			// 超时（D-01）：Job 仍在执行，终态经列表刷新确认；审计 failure+timeout。
			s.auditAction(ctx, actor, action, name, audit.ResultFailure,
				map[string]any{"error": errcode.ErrSvcOperationTimeout.Symbol(), "timeout": true})
			return nil, errcode.ErrSvcOperationTimeout.WithData(TimeoutData{Name: name, Note: TimeoutNote})
		}
		gerr := mapGatewayErrorWithReason(err, name)
		s.auditAction(ctx, actor, action, name, audit.ResultFailure, detailError(gerr))
		return nil, gerr
	}
	if !res.Succeeded {
		// Job 失败（SVC_007）：失败原因 + 实际终态（终态取实况读取，读取失败
		// 退 JobResult 终态）。
		active := res.ActiveState
		if post, perr := s.gw.GetUnit(ctx, name); perr == nil {
			active = post.ActiveState
		}
		s.auditAction(ctx, actor, action, name, audit.ResultFailure,
			map[string]any{"error": errcode.ErrSvcOperationFailed.Symbol(), "reason": res.Reason})
		return nil, errcode.ErrSvcOperationFailed.WithData(ActionFailureData{
			Name: name, Reason: res.Reason, ActiveState: strPtr(active),
		})
	}
	// 成功：操作后投影快照（免二次查询）；快照读取失败降级用 JobResult 终态 +
	// 前置 enabledState（A-S6：不因快照读失败翻转已成功操作的结果）。
	data := ActionData{
		Name: name, ActiveState: res.ActiveState,
		SubState: res.SubState, EnabledState: pre.EnabledState,
	}
	if post, perr := s.gw.GetUnit(ctx, name); perr == nil {
		data.ActiveState, data.SubState, data.EnabledState = post.ActiveState, post.SubState, post.EnabledState
	} else {
		slog.WarnContext(ctx, "service-mgmt: post-action snapshot degraded",
			"unit", name, "action", action, "err", perr)
	}
	s.auditAction(ctx, actor, action, name, audit.ResultSuccess, nil)
	return &data, nil
}

// toggleAutostart 是 enable/disable 共用编排：校验链（D-09：形态→存在性→
// 类型→两态主轴）→ 轻量同步调用 → 刷新值响应 → 恰好一条审计。
func (s *Service) toggleAutostart(ctx context.Context, actor *authsvc.Identity,
	action, name, targetState string) (*AutostartData, error) {
	if e := guardUnitName(name); e != nil {
		s.auditAction(ctx, actor, action, name, audit.ResultFailure, detailError(e))
		return nil, e
	}
	d, err := s.gw.GetUnit(ctx, name) // 存在性实况校验（SVC_003；D-09 逐级第二）
	if err != nil {
		gerr := mapGatewayError(err)
		s.auditAction(ctx, actor, action, name, audit.ResultFailure, detailError(gerr))
		return nil, gerr
	}
	if e := guardUnitType(name); e != nil { // 类型边界（SVC_004；D-09 逐级第三）
		s.auditAction(ctx, actor, action, name, audit.ResultFailure, detailError(e))
		return nil, e
	}
	if e := guardToggleable(d.EnabledState); e != nil { // 两态主轴（SVC_005，⚠️A14）
		s.auditAction(ctx, actor, action, name, audit.ResultFailure, detailError(e))
		return nil, e
	}
	var opErr error
	if targetState == EnabledStateEnabled {
		opErr = s.gw.EnableUnit(ctx, name)
	} else {
		opErr = s.gw.DisableUnit(ctx, name)
	}
	if opErr != nil {
		// systemd 拒绝（如配置只读）→ SVC_007（api §4.1.4 enable）。
		gerr := mapGatewayErrorWithReason(opErr, name)
		s.auditAction(ctx, actor, action, name, audit.ResultFailure, detailError(gerr))
		return nil, gerr
	}
	// 成功：enabledState=刷新值（快照读取失败退目标字面值，A-S6 同口径）。
	state := targetState
	if post, perr := s.gw.GetUnit(ctx, name); perr == nil && post.EnabledState != "" {
		state = post.EnabledState
	} else if perr != nil {
		slog.WarnContext(ctx, "service-mgmt: post-toggle snapshot degraded",
			"unit", name, "action", action, "err", perr)
	}
	s.auditAction(ctx, actor, action, name, audit.ResultSuccess, nil)
	return &AutostartData{Name: name, EnabledState: state}, nil
}

// auditAction 记录一条操作审计（I-06：每个动作接口调用恰好一条；D-06：
// 写入失败重试一次，仍失败记面板运行日志兜底——不影响操作结果返回）。
func (s *Service) auditAction(ctx context.Context, actor *authsvc.Identity,
	action, name, result string, detail any) {
	entry := audit.Entry{
		Category:   audit.CategoryChange,
		Action:     action,
		ObjectType: ObjectTypeSystemdService,
		ObjectID:   name,
		Detail:     detail,
		Result:     result,
		Source:     audit.SourceWeb,
	}
	if actor != nil {
		uid := actor.UserID
		entry.ActorID = &uid
		entry.ActorName = actor.Username
	} else { // 防御：AuthRequired 链路下不可达
		entry.ActorName = "unknown"
	}
	if err := s.audit.Record(ctx, entry); err != nil {
		if err := s.audit.Record(ctx, entry); err != nil { // 重试一次（D-06）
			slog.WarnContext(ctx, "service-mgmt: audit record degraded to runtime log",
				"action", action, "unit", name, "result", result, "err", err)
		}
	}
}

// mapGatewayError 网关哨兵错误 → SVC 错误码（A-S8）；未知错误包装为内部
// 错误（经 response.Fail 脱敏为 INTERNAL，细节进日志）。
func mapGatewayError(err error) error {
	switch {
	case errors.Is(err, sysd.ErrUnitNotFound):
		return errcode.ErrSvcUnitNotFound
	case errors.Is(err, sysd.ErrUnavailable):
		return errcode.ErrSvcSystemdUnavailable
	case errors.Is(err, sysd.ErrJobTimeout):
		return errcode.ErrSvcOperationTimeout
	case errors.Is(err, sysd.ErrRejected):
		return errcode.ErrSvcOperationFailed
	default:
		return fmt.Errorf("service-mgmt gateway: %w", err)
	}
}

// mapGatewayErrorWithReason 同 mapGatewayError，但 SVC_007 携带 {name,
// reason} 载荷（动作侧失败原因外显；调用期拒绝无终态，activeState=null）。
func mapGatewayErrorWithReason(err error, name string) error {
	switch {
	case errors.Is(err, sysd.ErrUnitNotFound):
		return errcode.ErrSvcUnitNotFound
	case errors.Is(err, sysd.ErrUnavailable):
		return errcode.ErrSvcSystemdUnavailable
	case errors.Is(err, sysd.ErrJobTimeout):
		return errcode.ErrSvcOperationTimeout
	case errors.Is(err, sysd.ErrRejected):
		return errcode.ErrSvcOperationFailed.WithData(ActionFailureData{Name: name, Reason: err.Error()})
	default:
		return fmt.Errorf("service-mgmt gateway: %w", err)
	}
}

// detailError 构造审计 detail 的 {error: SVC 码} 载荷（A-S5：guard 拒绝与
// 网关错误均记错误码符号）。
func detailError(err error) map[string]any {
	var ec *errcode.Error
	if errors.As(err, &ec) {
		return map[string]any{"error": ec.Symbol()}
	}
	return map[string]any{"error": err.Error()}
}

// strPtr 空串落 nil（JSON null），非空取指针。
func strPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
