package fileman

import (
	"context"
	"database/sql"
	"fmt"

	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// Store 聚合 file-manager 仓储（trash_entry/fs_path_scope）+ 复用 account-auth
// 的 audit_log 仓储（同事务审计写入，D-02/D-12 例外语义），并提供事务视图
// BeginTx：入站登记/还原/清空/受管根修改与审计原子落库（I-01）。
type Store struct {
	db *sql.DB
}

// New 构造绑定连接池的仓储聚合。
func New(db *sql.DB) *Store { return &Store{db: db} }

// Trash 返回 trash_entry 仓储。
func (s *Store) Trash() *TrashRepo { return &TrashRepo{x: s.db} }

// Scope 返回 fs_path_scope 仓储。
func (s *Store) Scope() *ScopeRepo { return &ScopeRepo{x: s.db} }

// Audit 返回 audit_log 仓储（跨模块复用，store/auth 实现）。
func (s *Store) Audit() *storeauth.AuditRepo { return storeauth.NewAuditRepo(s.db) }

// BeginTx 开启事务并返回事务绑定仓储视图（service 层经接口消费，
// 保证 trash 入站登记/还原/清空与受管根修改同审计原子落库，I-01/D-02/D-15）。
func (s *Store) BeginTx(ctx context.Context) (*TxStore, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store/fileman: begin tx: %w", err)
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
		return fmt.Errorf("store/fileman: commit: %w", err)
	}
	return nil
}

// Rollback 回滚事务（已结束时错误由调用方按语义判断）。
func (t *TxStore) Rollback() error {
	if err := t.tx.Rollback(); err != nil {
		return fmt.Errorf("store/fileman: rollback: %w", err)
	}
	return nil
}

// Trash 返回事务内 trash_entry 仓储。
func (t *TxStore) Trash() *TrashRepo { return &TrashRepo{x: t.tx} }

// Scope 返回事务内 fs_path_scope 仓储。
func (t *TxStore) Scope() *ScopeRepo { return &ScopeRepo{x: t.tx} }

// Audit 返回事务内 audit_log 仓储。
func (t *TxStore) Audit() *storeauth.AuditRepo { return storeauth.NewAuditRepo(t.tx) }
