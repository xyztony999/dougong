package panel

import (
	"context"
	"database/sql"
	"fmt"

	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// Store 聚合 panel-core 仓储（panel_config/panel_cert）+ 复用 account-auth
// 的 audit_log 仓储（同事务审计写入，I-05 语义/R-03），并提供事务视图
// BeginTx：配置原子保存（I-P07）与审计/证书写入同事务。
type Store struct {
	db *sql.DB
}

// New 构造绑定连接池的仓储聚合。
func New(db *sql.DB) *Store { return &Store{db: db} }

// Config 返回 panel_config 仓储。
func (s *Store) Config() *ConfigRepo { return &ConfigRepo{x: s.db} }

// Cert 返回 panel_cert 仓储。
func (s *Store) Cert() *CertRepo { return &CertRepo{x: s.db} }

// Audit 返回 audit_log 仓储（跨模块复用，store/auth 实现）。
func (s *Store) Audit() *storeauth.AuditRepo { return storeauth.NewAuditRepo(s.db) }

// BeginTx 开启事务并返回事务绑定仓储视图（service 层经接口消费，
// 保证配置/证书变更与审计原子落库）。
func (s *Store) BeginTx(ctx context.Context) (*TxStore, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store/panel: begin tx: %w", err)
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
		return fmt.Errorf("store/panel: commit: %w", err)
	}
	return nil
}

// Rollback 回滚事务（已结束时错误由调用方按语义判断）。
func (t *TxStore) Rollback() error {
	if err := t.tx.Rollback(); err != nil {
		return fmt.Errorf("store/panel: rollback: %w", err)
	}
	return nil
}

// Config 返回事务内 panel_config 仓储。
func (t *TxStore) Config() *ConfigRepo { return &ConfigRepo{x: t.tx} }

// Cert 返回事务内 panel_cert 仓储。
func (t *TxStore) Cert() *CertRepo { return &CertRepo{x: t.tx} }

// Audit 返回事务内 audit_log 仓储。
func (t *TxStore) Audit() *storeauth.AuditRepo { return storeauth.NewAuditRepo(t.tx) }
