// Package dbtools 是数据库工具集成（db-tools）模块的数据访问层
// （api-db-tools.md §5.2/§5.3；SQL 只允许出现在本包）。db_tool（部署档案+
// 凭据密文）与 tool_deploy_task（四态任务台账）为双聚合根；工具运行态/
// 依赖就绪度零台账（D-02 实时探测投影不落库）。
package dbtools

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// Executor 是 store.Executor 的别名（*sql.DB 与 *sql.Tx 共同满足）。
type Executor = store.Executor

// db_tool_type / db_tool_task_action / db_tool_task_status 取值域（DDL CHECK 兜底）。
const (
	ToolPHPMyAdmin   = "phpmyadmin"
	ToolMongoExpress = "mongo_express"

	ActionInstall   = "install"
	ActionUpgrade   = "upgrade"
	ActionUninstall = "uninstall"

	TaskPending   = "pending"
	TaskRunning   = "running"
	TaskSucceeded = "succeeded"
	TaskFailed    = "failed"
)

// ErrTaskNotFound 任务不存在（DBT_005 语义）。
var ErrTaskNotFound = errors.New("store/dbtools: task not found")

// Tool 对应 db_tool 一行（聚合根 db_tool，api §4.1.1）。CredCipher 为
// AES-256-GCM 密文（I-02：任何接口响应/日志/审计不含明文/密文/密钥）。
type Tool struct {
	ID               int64
	ToolType         string
	InstalledVersion string
	DeployPath       string
	SiteRef          string // 注入落点标识（来源文件+server 块定位 JSON，D-03）
	UnitName         *string
	InstanceID       *int64
	CredCipher       string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Task 对应 tool_deploy_task 一行（聚合根 tool_deploy_task，api §4.2.1）。
type Task struct {
	ID          int64
	ToolType    string
	Action      string
	Status      string
	InstanceID  *int64
	OutputTail  string
	Error       *string
	RequestedBy string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	StartedAt   *time.Time
	FinishedAt  *time.Time
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func scanTime(src any) (time.Time, error) {
	switch v := src.(type) {
	case string:
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return time.Time{}, fmt.Errorf("store/dbtools: parse time %q: %w", v, err)
		}
		return t, nil
	case []byte:
		t, err := time.Parse(time.RFC3339, string(v))
		if err != nil {
			return time.Time{}, fmt.Errorf("store/dbtools: parse time %q: %w", v, err)
		}
		return t, nil
	case time.Time:
		return v, nil
	default:
		return time.Time{}, fmt.Errorf("store/dbtools: unexpected time column type %T", src)
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
	return fmt.Errorf("store/dbtools: %s: %w", op, err)
}

func strPtrArg(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func int64PtrArg(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

// Store 聚合 db-tools 仓储 + 复用 account-auth audit_log 仓储（任务创建/
// 终态/档案变更与审计同事务 I-03/I-C01）。
type Store struct{ db *sql.DB }

// New 构造仓储聚合。
func New(db *sql.DB) *Store { return &Store{db: db} }

// Tool 返回工具档案仓储。
func (s *Store) Tool() *ToolRepo { return &ToolRepo{x: s.db} }

// Task 返回任务仓储。
func (s *Store) Task() *TaskRepo { return &TaskRepo{x: s.db} }

// Audit 返回审计仓储（非事务通道）。
func (s *Store) Audit() *storeauth.AuditRepo { return storeauth.NewAuditRepo(s.db) }

// TxStore 事务视图（任务创建+发起审计 / 终态+档案+审计 同事务）。
type TxStore struct{ tx *sql.Tx }

// BeginTx 开启事务。
func (s *Store) BeginTx(ctx context.Context) (*TxStore, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store/dbtools: begin tx: %w", err)
	}
	return &TxStore{tx: tx}, nil
}

// Commit 提交事务。
func (t *TxStore) Commit() error {
	if err := t.tx.Commit(); err != nil {
		return fmt.Errorf("store/dbtools: commit: %w", err)
	}
	return nil
}

// Rollback 回滚（已结束时错误由调用方按语义判断）。
func (t *TxStore) Rollback() error {
	if err := t.tx.Rollback(); err != nil {
		return fmt.Errorf("store/dbtools: rollback: %w", err)
	}
	return nil
}

// Tool 返回事务内工具档案仓储。
func (t *TxStore) Tool() *ToolRepo { return &ToolRepo{x: t.tx} }

// Task 返回事务内任务仓储。
func (t *TxStore) Task() *TaskRepo { return &TaskRepo{x: t.tx} }

// Audit 返回事务内审计仓储（同事务写入，R-03）。
func (t *TxStore) Audit() *storeauth.AuditRepo { return storeauth.NewAuditRepo(t.tx) }
