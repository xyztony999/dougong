package sitemgmt

import (
	"context"
	"database/sql"
	"fmt"

	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// Store 聚合 site-mgmt 仓储（certificate）+ 复用 account-auth 的 audit_log
// 仓储（同事务审计写入，D-12 例外语义：certificate 表写与审计同事务），并提供
// 事务视图 BeginTx。
type Store struct {
	db *sql.DB
}

// New 构造绑定连接池的仓储聚合。
func New(db *sql.DB) *Store { return &Store{db: db} }

// Cert 返回 certificate 仓储。
func (s *Store) Cert() *CertRepo { return &CertRepo{x: s.db} }

// DnsCred 返回 dns_credential 仓储（v2.0，A20）。
func (s *Store) DnsCred() *DnsCredentialRepo { return &DnsCredentialRepo{x: s.db} }

// Audit 返回 audit_log 仓储（跨模块复用，store/auth 实现）。
func (s *Store) Audit() *storeauth.AuditRepo { return storeauth.NewAuditRepo(s.db) }

// BeginTx 开启事务并返回事务绑定仓储视图（service 层经接口消费，保证
// 签发/续期终态落库、删除与审计原子提交，D-12 例外）。
func (s *Store) BeginTx(ctx context.Context) (*TxStore, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store/sitemgmt: begin tx: %w", err)
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
		return fmt.Errorf("store/sitemgmt: commit: %w", err)
	}
	return nil
}

// Rollback 回滚事务（已结束时错误由调用方按语义判断）。
func (t *TxStore) Rollback() error {
	if err := t.tx.Rollback(); err != nil {
		return fmt.Errorf("store/sitemgmt: rollback: %w", err)
	}
	return nil
}

// Cert 返回事务内 certificate 仓储。
func (t *TxStore) Cert() *CertRepo { return &CertRepo{x: t.tx} }

// DnsCred 返回事务内 dns_credential 仓储（v2.0）。
func (t *TxStore) DnsCred() *DnsCredentialRepo { return &DnsCredentialRepo{x: t.tx} }

// Audit 返回事务内 audit_log 仓储。
func (t *TxStore) Audit() *storeauth.AuditRepo { return storeauth.NewAuditRepo(t.tx) }
