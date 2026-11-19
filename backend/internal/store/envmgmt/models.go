// Package envmgmt 是环境管理（env-mgmt）模块的数据访问层（api-env-mgmt.md
// §5.2/§5.3；SQL 只允许出现在本包）。env_install_task 为本模块唯一持久化
// 对象（四态任务台账 ⚠️A3）；环境条目实况零台账（D-01 实时探测）。
package envmgmt

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dougong/dougong/internal/store"
)

// Executor 是 store.Executor 的别名（*sql.DB 与 *sql.Tx 共同满足）。
type Executor = store.Executor

// env_type / env_task_action / env_task_status 取值域（DDL CHECK 兜底）。
const (
	EnvNginx   = "nginx"
	EnvApache  = "apache"
	EnvDocker  = "docker"
	EnvMySQL   = "mysql"
	EnvRedis   = "redis"
	EnvPHPFPM  = "php_fpm"
	EnvMongoDB = "mongodb" // v1.1 W1 修复轮：七环境目录（⚠️A1/迁移 010）
	EnvNode    = "node"    // v1.3 Node 增量（prd FR-08/⚠️A16：八环境目录，迁移 014；运行时型无服务单元 ⚠️A17）

	ActionInstall   = "install"
	ActionUpgrade   = "upgrade"
	ActionUninstall = "uninstall"

	TaskPending   = "pending"
	TaskRunning   = "running"
	TaskSucceeded = "succeeded"
	TaskFailed    = "failed"

	// bootstrap_status 取值域（v1.2 安装后配置就绪，api D-18；DDL CHECK 兜底）。
	// NULL=无引导步骤（非数据库环境/卸载/包操作失败未达引导），非枚举值。
	BootstrapSucceeded = "succeeded"
	BootstrapFailed    = "failed"
	BootstrapSkipped   = "skipped"
)

// ErrTaskNotFound 任务不存在（ENV_005 语义）。
var ErrTaskNotFound = errors.New("store/envmgmt: task not found")

// Task 对应 env_install_task 一行（聚合根 env_install_task，api §4.1.1）。
type Task struct {
	ID          int64
	EnvType     string
	Action      string
	Status      string
	OutputTail  string
	Error       *string
	RequestedBy string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	StartedAt   *time.Time
	FinishedAt  *time.Time
	// 引导结果（v1.2 安装后配置就绪，D-18）：nil=无引导步骤；终态定格。
	BootstrapStatus *string
	BootstrapNote   *string
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func scanTime(src any) (time.Time, error) {
	switch v := src.(type) {
	case string:
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return time.Time{}, fmt.Errorf("store/envmgmt: parse time %q: %w", v, err)
		}
		return t, nil
	case time.Time:
		return v, nil
	default:
		return time.Time{}, fmt.Errorf("store/envmgmt: unexpected time column type %T", src)
	}
}

func scanTimePtr(src any) (*time.Time, error) {
	if src == nil {
		return nil, nil
	}
	t, err := scanTime(src)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func wrapErr(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("store/envmgmt: %s: %w", op, err)
}

// Store 聚合 env-mgmt 仓储 + 复用 account-auth audit_log 仓储（任务创建/
// 终态与审计同事务 I-03）。
type Store struct{ db *sql.DB }

// New 构造仓储聚合。
func New(db *sql.DB) *Store { return &Store{db: db} }

// Task 返回任务仓储。
func (s *Store) Task() *TaskRepo { return &TaskRepo{x: s.db} }

// BeginTx 开启事务视图（任务创建+发起审计 / 终态+审计 同事务）。
type TxStore struct{ tx *sql.Tx }

func (s *Store) BeginTx(ctx context.Context) (*TxStore, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store/envmgmt: begin tx: %w", err)
	}
	return &TxStore{tx: tx}, nil
}

// Commit 提交事务。
func (t *TxStore) Commit() error {
	if err := t.tx.Commit(); err != nil {
		return fmt.Errorf("store/envmgmt: commit: %w", err)
	}
	return nil
}

// Rollback 回滚（已结束时错误由调用方按语义判断）。
func (t *TxStore) Rollback() error {
	if err := t.tx.Rollback(); err != nil {
		return fmt.Errorf("store/envmgmt: rollback: %w", err)
	}
	return nil
}

// Task 返回事务内任务仓储。
func (t *TxStore) Task() *TaskRepo { return &TaskRepo{x: t.tx} }
