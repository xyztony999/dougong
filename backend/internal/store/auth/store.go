package auth

import (
	"context"
	"database/sql"
	"fmt"
)

// Store 聚合 account-auth 全部仓储，并提供跨仓储事务组合 WithinTx
// （I-05：业务变更与审计写入同一事务）。同一类型既绑定连接池（db 版）
// 也可在事务回调中获得事务绑定视图（txStore 版）。
type Store struct {
	db *sql.DB
}

// New 构造绑定连接池的仓储聚合。
func New(db *sql.DB) *Store { return &Store{db: db} }

// Users 返回 panel_user 仓储。
func (s *Store) Users() *UserRepo { return &UserRepo{x: s.db} }

// Sessions 返回 panel_session 仓储。
func (s *Store) Sessions() *SessionRepo { return &SessionRepo{x: s.db} }

// Totp 返回 panel_user_totp 仓储。
func (s *Store) Totp() *TotpRepo { return &TotpRepo{x: s.db} }

// Audit 返回 audit_log 仓储。
func (s *Store) Audit() *AuditRepo { return &AuditRepo{x: s.db} }

// Init 返回 init_state 仓储。
func (s *Store) Init() *InitRepo { return &InitRepo{x: s.db} }

// BeginTx 开启事务并返回事务绑定仓储视图（TxStore 与 Store 同构 accessor，
// 并提供 Commit/Rollback）；service 层经接口消费，保证业务变更与审计
// 原子落库（I-05）。
func (s *Store) BeginTx(ctx context.Context) (*TxStore, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store/auth: begin tx: %w", err)
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
		return fmt.Errorf("store/auth: commit: %w", err)
	}
	return nil
}

// Rollback 回滚事务（已结束时返回 nil 语义错误由调用方判断）。
func (t *TxStore) Rollback() error {
	if err := t.tx.Rollback(); err != nil {
		return fmt.Errorf("store/auth: rollback: %w", err)
	}
	return nil
}

// Users 返回事务内 panel_user 仓储。
func (t *TxStore) Users() *UserRepo { return &UserRepo{x: t.tx} }

// Sessions 返回事务内 panel_session 仓储。
func (t *TxStore) Sessions() *SessionRepo { return &SessionRepo{x: t.tx} }

// Totp 返回事务内 panel_user_totp 仓储。
func (t *TxStore) Totp() *TotpRepo { return &TotpRepo{x: t.tx} }

// Audit 返回事务内 audit_log 仓储。
func (t *TxStore) Audit() *AuditRepo { return &AuditRepo{x: t.tx} }

// Init 返回事务内 init_state 仓储。
func (t *TxStore) Init() *InitRepo { return &InitRepo{x: t.tx} }
