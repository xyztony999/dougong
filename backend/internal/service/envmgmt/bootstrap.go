package envmgmt

import (
	"context"
	"log/slog"

	"github.com/dougong/dougong/internal/service/audit"
	dbmgmtsvc "github.com/dougong/dougong/internal/service/dbmgmt"
	storeenv "github.com/dougong/dougong/internal/store/envmgmt"
)

// 本文件是 env-mgmt 安装后配置就绪引导触发器（api-env-mgmt.md D-15~D-19，
// v1.2 增量）：worker 在数据库类环境 install/upgrade 包操作成功后执行后置引导
// 步骤——服务就绪（SysdGateway 幂等 start，⚠️A15）→ 消费 db-mgmt
// BootstrapAfterInstall OHS（建账户/设密码/注册实例凭据归 db-mgmt 实例层，
// 单向依赖）→ 引导结果回写任务台账（bootstrap_status/note，D-18）。引导失败
// 不改变包安装终态（独立维度，⚠️A14）。

// BootstrapGateway 是 db-mgmt 安装后配置就绪 OHS 消费契约（api §4.1.5；
// db-mgmt Service 实现，main 装配注入；测试注入替身，零 runner 实况依赖）。
type BootstrapGateway interface {
	BootstrapAfterInstall(ctx context.Context, engine, actorName string) (dbmgmtsvc.BootstrapOutcome, error)
}

// bootstrapEnvTypes 触发安装后配置就绪的环境集合（D-15，编译期常量；
// PHP/nginx/Apache/Docker 装完即用无引导）。
var bootstrapEnvTypes = map[string]bool{
	storeenv.EnvMySQL:   true,
	storeenv.EnvRedis:   true,
	storeenv.EnvMongoDB: true,
}

// runBootstrap worker 后置引导步骤（仅数据库类 install/upgrade 成功后调用）。
// 返回 bootstrap_status/note 指针（nil=无引导步骤）；引导输出追加 ring。
func (s *Service) runBootstrap(ctx context.Context, task storeenv.Task, ring *outputRing) (*string, *string) {
	if !bootstrapEnvTypes[task.EnvType] {
		return nil, nil
	}
	if s.bootstrap == nil {
		s.recordBootstrapAudit(ctx, task, storeenv.BootstrapFailed)
		return bootstrapResult(storeenv.BootstrapFailed, "引导通道未装配（database-mgmt OHS 不可用）", ring, task)
	}
	// 服务就绪：幂等 start 目录 DB 单元（⚠️A15；start 对已运行无副作用，enable
	// 不强制沿 ⚠️A5）。启动失败不阻断——db-mgmt 引导有界重试等待就绪，最终如实。
	if entry := CatalogByType(task.EnvType); entry != nil {
		ch := s.pack.DetectChannel()
		if unit, _, _ := s.unitFor(ctx, entry, ch.Family); unit != "" {
			if _, err := s.sysd.StartUnit(ctx, unit); err != nil {
				ring.append("[bootstrap] 服务启动尝试失败（继续等待引擎就绪）: " + err.Error())
			}
		}
	}
	ring.append("[bootstrap] 开始安装后配置就绪引导（" + task.EnvType + "）")
	out, err := s.bootstrap.BootstrapAfterInstall(ctx, task.EnvType, task.RequestedBy)
	if err != nil {
		s.recordBootstrapAudit(ctx, task, storeenv.BootstrapFailed)
		return bootstrapResult(storeenv.BootstrapFailed, "引导失败: "+err.Error(), ring, task)
	}
	s.recordBootstrapAudit(ctx, task, out.Status)
	return bootstrapResult(out.Status, out.Note, ring, task)
}

// bootstrapResult 定格引导结果（追加输出行 + env_bootstrap 审计，detail 不含密码）。
func bootstrapResult(status, note string, ring *outputRing, task storeenv.Task) (*string, *string) {
	ring.append("[bootstrap] " + status + ": " + note)
	st, n := status, note
	return &st, &n
}

// recordBootstrapAudit 引导动作审计（env_bootstrap，D-15；失败重试一次后日志
// 兜底，与 recordRepoAudit 同口径；detail 含 envType/bootstrapStatus，不含密码）。
func (s *Service) recordBootstrapAudit(ctx context.Context, task storeenv.Task, status string) {
	e := audit.Entry{
		ActorName: task.RequestedBy, Category: audit.CategoryChange,
		Action: AuditActionEnvBootstrap, ObjectType: ObjectTypeEnv, ObjectID: task.EnvType,
		Detail: map[string]any{"envType": task.EnvType, "bootstrapStatus": status},
		Result: audit.ResultSuccess, Source: audit.SourceWeb,
	}
	if status == storeenv.BootstrapFailed {
		e.Result = audit.ResultFailure
	}
	if err := s.audit(ctx, e); err != nil {
		if err := s.audit(ctx, e); err != nil {
			slog.WarnContext(ctx, "envmgmt: bootstrap audit degraded", "envType", task.EnvType, "err", err)
		}
	}
}
