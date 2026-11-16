package dbmgmt

import (
	"context"
	"database/sql"
	"fmt"

	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// Store 聚合 database-mgmt 仓储（db_instance）+ 复用 account-auth 的 audit_log
// 仓储（同事务审计写入：探测登记/凭据落库/移除与审计原子提交，R-03/§3.2），
// 并提供事务视图 BeginTx。
type Store struct {
	db *sql.DB
}

// New 构造绑定连接池的仓储聚合。
func New(db *sql.DB) *Store { return &Store{db: db} }

// Instance 返回 db_instance 仓储。
func (s *Store) Instance() *InstanceRepo { return NewInstanceRepo(s.db) }

// Audit 返回 audit_log 仓储（跨模块复用，store/auth 实现）。
func (s *Store) Audit() *storeauth.AuditRepo { return storeauth.NewAuditRepo(s.db) }

// BeginTx 开启事务并返回事务绑定仓储视图（service 层经接口消费，保证档案
// 变更与审计原子提交）。
func (s *Store) BeginTx(ctx context.Context) (*TxStore, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store/dbmgmt: begin tx: %w", err)
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
		return fmt.Errorf("store/dbmgmt: commit: %w", err)
	}
	return nil
}

// Rollback 回滚事务（已结束时错误由调用方按语义判断）。
func (t *TxStore) Rollback() error {
	if err := t.tx.Rollback(); err != nil {
		return fmt.Errorf("store/dbmgmt: rollback: %w", err)
	}
	return nil
}

// Instance 返回事务内 db_instance 仓储。
func (t *TxStore) Instance() *InstanceRepo { return NewInstanceRepo(t.tx) }

// Audit 返回事务内 audit_log 仓储。
func (t *TxStore) Audit() *storeauth.AuditRepo { return storeauth.NewAuditRepo(t.tx) }
