// Package audit 是审计领域服务（R-03 全变更记审计的进程内 OHS）：
// 全部业务模块经 AuditService.Record 写审计；本包同时承载条件查询/CSV 导出
// 数据源与 90 天保留清理任务。审计枚举取值与动作码以
// api-account-auth.md §3.4 为准（audit_action/audit_result/audit_source）。
package audit

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// 审计类别（audit_action 枚举）。
const (
	CategoryChange   = "change"
	CategorySecurity = "security"
)

// 审计结果（audit_result 枚举）。
const (
	ResultSuccess = "success"
	ResultFailure = "failure"
)

// 审计来源（audit_source 枚举）。
const (
	SourceWeb    = "web"
	SourceCLI    = "cli"
	SourceSystem = "system"
)

// 细粒度动作码（业务常量，非枚举池条目；api-account-auth.md §3.4）。
const (
	ActionLoginSuccess         = "login_success"
	ActionLoginFailed          = "login_failed"
	ActionAccountLocked        = "account_locked"
	ActionAccountUnlocked      = "account_unlocked"
	ActionLogout               = "logout"
	ActionForceLogout          = "force_logout"
	ActionUserCreated          = "user_created"
	ActionUserDeleted          = "user_deleted"
	ActionPasswordReset        = "password_reset"
	ActionPasswordChanged      = "password_changed"
	ActionUsernameChanged      = "username_changed"
	ActionTotpEnabled          = "totp_enabled"
	ActionTotpDisabled         = "totp_disabled"
	ActionInitCompleted        = "init_completed"
	ActionAuditExported        = "audit_exported"
	ActionAuditPurged          = "audit_purged"
	ActionSecondFactorDisabled = "second_factor_disabled"
)

// 审计对象类型（object_type 常用取值，业务模块可扩展）。
const (
	ObjectTypeUser     = "user"
	ObjectTypeSession  = "session"
	ObjectTypeAuditLog = "audit_log"
	ObjectTypeInit     = "init"
)

// RetentionDays 审计保留天数（ADR-020 固定 90 天，编译期常量，非配置键不入库）。
const RetentionDays = 90

// MaxExportRows CSV 导出行数上限（防失控，超出提示缩小范围）。
const MaxExportRows = 100000

// Entry 是审计写入侧载荷（= storeauth.AuditEntry 别名，保持 OHS 消费方
// 仅依赖本包；四要素 I-15：操作者/时间/对象/结果）。
type Entry = storeauth.AuditEntry

// Record 是审计查询侧行。
type Record = storeauth.AuditRecord

// Condition 是审计条件查询过滤。
type Condition = storeauth.AuditCondition

// Repository 是审计仓储契约（实现在 internal/store/auth，接口定义在消费方）。
// Append 的 at 参数由 Service 统一注入时钟，保证同事务内时间一致。
type Repository interface {
	Append(ctx context.Context, e Entry, at time.Time) error
	Search(ctx context.Context, cond Condition, offset, limit int) ([]Record, int64, error)
	DeleteBefore(ctx context.Context, cutoff time.Time, limit int) (int64, error)
}

// Service 是审计应用服务：Record 供全模块同事务写入；Query/Stream 供
// account-auth 审计页面与导出；Purge 供每日清理任务。
type Service struct {
	repo  Repository
	clock func() time.Time
}

// New 构造审计服务；clock 为 nil 时用 time.Now。
func New(repo Repository, clock func() time.Time) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{repo: repo, clock: clock}
}

// Record 校验四要素后追加一条审计（时间戳取 clock）。独立事务场景使用；
// 需与业务变更同事务（I-05）时由调用方在事务内直接使用仓储。
func (s *Service) Record(ctx context.Context, e Entry) error {
	if err := validateEntry(e); err != nil {
		return err
	}
	if err := s.repo.Append(ctx, e, s.clock()); err != nil {
		return fmt.Errorf("audit record %s: %w", e.Action, err)
	}
	return nil
}

// Query 条件分页查询：page 从 1 起，pageSize 默认 20、上限 100（公共分页约定）。
// 返回当页 items 与 total；时间字段为 RFC3339 UTC 字符串（透传）。
func (s *Service) Query(ctx context.Context, cond Condition, page, pageSize int) (items []Record, total int64, err error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	items, total, err = s.repo.Search(ctx, cond, (page-1)*pageSize, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("audit query: %w", err)
	}
	return items, total, nil
}

// StreamBatch 以固定批大小顺序遍历满足条件的全部记录（CSV 导出用），
// 每批回调 fn；fn 返回错误即中止。遍历批次按 created_at 倒序。
func (s *Service) StreamBatch(ctx context.Context, cond Condition, batchSize int, fn func(batch []Record) error) error {
	if batchSize < 1 {
		batchSize = 1000
	}
	for offset := 0; ; offset += batchSize {
		batch, _, err := s.repo.Search(ctx, cond, offset, batchSize)
		if err != nil {
			return fmt.Errorf("audit stream offset=%d: %w", offset, err)
		}
		if len(batch) == 0 {
			return nil
		}
		if err := fn(batch); err != nil {
			return fmt.Errorf("audit stream callback offset=%d: %w", offset, err)
		}
		if len(batch) < batchSize {
			return nil
		}
	}
}

// Purge 执行一轮保留期清理：分批删除 created_at < now-90d 的记录，
// 并追加一条 audit_purged 审计（独立事务，source=system）。
// 批次失败即返回错误（已删批次保留，次日重试——设计文档 §4.3.3）。
func (s *Service) Purge(ctx context.Context) (int64, error) {
	cutoff := s.clock().UTC().AddDate(0, 0, -RetentionDays)
	const batch = 500
	var deleted int64
	for {
		n, err := s.repo.DeleteBefore(ctx, cutoff, batch)
		if err != nil {
			// 失败也要留下失败审计（尽力而为，记录失败不影响返回原始错误）。
			if rerr := s.Record(ctx, Entry{
				ActorName:  "system",
				Category:   CategoryChange,
				Action:     ActionAuditPurged,
				ObjectType: ObjectTypeAuditLog,
				Detail:     map[string]any{"deletedCount": deleted, "cutoff": cutoff.Format(time.RFC3339), "error": err.Error()},
				Result:     ResultFailure,
				Source:     SourceSystem,
			}); rerr != nil {
				slog.WarnContext(ctx, "audit purge: record failure entry", "err", rerr)
			}
			return deleted, fmt.Errorf("audit purge batch: %w", err)
		}
		deleted += n
		if n < batch {
			break
		}
	}
	if err := s.Record(ctx, Entry{
		ActorName:  "system",
		Category:   CategoryChange,
		Action:     ActionAuditPurged,
		ObjectType: ObjectTypeAuditLog,
		Detail:     map[string]any{"deletedCount": deleted, "cutoff": cutoff.Format(time.RFC3339)},
		Result:     ResultSuccess,
		Source:     SourceSystem,
	}); err != nil {
		return deleted, fmt.Errorf("audit purge record: %w", err)
	}
	return deleted, nil
}

// validateEntry 强制四要素（I-15：操作者/对象/结果 + 动作与类别合法）。
func validateEntry(e Entry) error {
	if strings.TrimSpace(e.ActorName) == "" {
		return fmt.Errorf("audit: actor name is empty")
	}
	if e.Category != CategoryChange && e.Category != CategorySecurity {
		return fmt.Errorf("audit: invalid category %q", e.Category)
	}
	if strings.TrimSpace(e.Action) == "" {
		return fmt.Errorf("audit: action is empty")
	}
	if strings.TrimSpace(e.ObjectType) == "" {
		return fmt.Errorf("audit: object type is empty")
	}
	if e.Result != ResultSuccess && e.Result != ResultFailure {
		return fmt.Errorf("audit: invalid result %q", e.Result)
	}
	if e.Source != SourceWeb && e.Source != SourceCLI && e.Source != SourceSystem {
		return fmt.Errorf("audit: invalid source %q", e.Source)
	}
	return nil
}
