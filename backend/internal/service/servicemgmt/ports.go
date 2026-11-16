package servicemgmt

import (
	"context"

	"github.com/dougong/dougong/internal/adapter/sysd"
	"github.com/dougong/dougong/internal/service/audit"
)

// Gateway 是 systemd dbus 网关契约（api §4.1.5 SysdGateway；接口定义在消费方，
// 实现为 internal/adapter/sysd.Gateway，main 装配注入——backend-01 §1）。
// 全部方法 ctx 首参（backend-rules §1.1）；错误以包级哨兵表达（A-S8 归一）。
// 测试以 fake 实现注入（systemd 交互不出本模块单测边界）。
type Gateway interface {
	// ListUnits system 级全量清单投影（I-01/I-03）。
	ListUnits(ctx context.Context) ([]sysd.UnitSnapshot, error)
	// GetUnit 单 unit 属性（含 description/loadState）。
	GetUnit(ctx context.Context, name string) (sysd.UnitDetail, error)
	// StartUnit/StopUnit/RestartUnit 提交 + 等待 Job 终态（≤ActionWaitLimit，I-05）。
	StartUnit(ctx context.Context, name string) (sysd.JobResult, error)
	StopUnit(ctx context.Context, name string) (sysd.JobResult, error)
	RestartUnit(ctx context.Context, name string) (sysd.JobResult, error)
	// EnableUnit/DisableUnit 轻量同步（无 Job 等待）。
	EnableUnit(ctx context.Context, name string) error
	DisableUnit(ctx context.Context, name string) error
}

// AuditRecorder 是审计写入契约（account-auth OHS：AuditService.Record，R-03；
// D-06 降级语义见 auditAction）。实现为 internal/service/audit.Service。
type AuditRecorder interface {
	Record(ctx context.Context, e audit.Entry) error
}
