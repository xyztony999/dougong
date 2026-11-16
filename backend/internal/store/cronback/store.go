package cronback

import (
	"context"
	"database/sql"
	"fmt"

	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// Store 聚合 cron-backup 仓储（backup_job/backup_run）+ 复用 account-auth 的
// audit_log 仓储（同事务审计：手动触发 INSERT run 与 audit 同事务 R-03/A-C3；
// 终态回写 + 轮转 DELETE + audit backup_rotated 同事务 D-09）。
type Store struct {
	db *sql.DB
}

// New 构造绑定连接池的仓储聚合。
func New(db *sql.DB) *Store { return &Store{db: db} }

// Job 返回 backup_job 仓储。
func (s *Store) Job() *JobRepo { return &JobRepo{x: s.db} }

// Run 返回 backup_run 仓储。
func (s *Store) Run() *RunRepo { return &RunRepo{x: s.db} }

// Audit 返回 audit_log 仓储（跨模块复用，store/auth 实现）。
func (s *Store) Audit() *storeauth.AuditRepo { return storeauth.NewAuditRepo(s.db) }

// BeginTx 开启事务并返回事务绑定仓储视图（service 层经接口消费，保证终态
// 回写/轮转/审计原子落库，D-09）。
func (s *Store) BeginTx(ctx context.Context) (*TxStore, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("cronback: begin tx: %w", err)
	}
	return &TxStore{tx: tx}, nil
}

// TxStore 是事务绑定的仓储视图。
type TxStore struct {
	tx *sql.Tx
}

// Commit 提交事务。
func (t *TxStore) Commit() error {
	if err := t.tx.Commit(); err != nil {
		return fmt.Errorf("cronback: commit: %w", err)
	}
	return nil
}

// Rollback 回滚事务（已结束时错误由调用方按语义判断）。
func (t *TxStore) Rollback() error {
	if err := t.tx.Rollback(); err != nil {
		return fmt.Errorf("cronback: rollback: %w", err)
	}
	return nil
}

// Job 返回事务内 backup_job 仓储。
func (t *TxStore) Job() *JobRepo { return &JobRepo{x: t.tx} }

// Run 返回事务内 backup_run 仓储。
func (t *TxStore) Run() *RunRepo { return &RunRepo{x: t.tx} }

// Audit 返回事务内 audit_log 仓储（同事务写入，R-03）。
func (t *TxStore) Audit() *storeauth.AuditRepo { return storeauth.NewAuditRepo(t.tx) }
