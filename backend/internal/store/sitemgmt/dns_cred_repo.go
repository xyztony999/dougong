package sitemgmt

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrDnsCredentialNotFound DNS 凭证不存在（CERT_009 语义；v2.0 ⚠️A20）。
var ErrDnsCredentialNotFound = errors.New("store/sitemgmt: dns credential not found")

// DnsCredentialRepo 实现 dns_credential 表访问（api v2.0 §4.3.4/§5.2 SQL #7~#9
// 全参数化——标识符静态字面量，G-02；凭据材料密文列对 SQL 层不透明）。
type DnsCredentialRepo struct{ x Executor }

// NewDnsCredentialRepo 构造绑定给定执行器的凭证仓储。
func NewDnsCredentialRepo(x Executor) *DnsCredentialRepo { return &DnsCredentialRepo{x: x} }

// Create 插入一行凭证（SQL #7；密文落库与审计同事务由 service 层 TxStore 保证）。
func (r *DnsCredentialRepo) Create(ctx context.Context, c DnsCredential) (int64, error) {
	now := formatTime(time.Now().UTC())
	res, err := r.x.ExecContext(ctx,
		`INSERT INTO dns_credential (name, provider, credential_enc, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		c.Name, c.Provider, c.CredentialEnc, now, now)
	if err != nil {
		return 0, wrapErr("dns credential create", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store/sitemgmt: dns credential create last id: %w", err)
	}
	return id, nil
}

// GetByID 按 id 取单行（含密文——仅 service 层签发/续期解密消费）；不存在
// 返回 ErrDnsCredentialNotFound。
func (r *DnsCredentialRepo) GetByID(ctx context.Context, id int64) (DnsCredential, error) {
	row := r.x.QueryRowContext(ctx,
		`SELECT id, name, provider, credential_enc, created_at, updated_at
		 FROM dns_credential WHERE id = ?`, id)
	var c DnsCredential
	var createdAt, updatedAt any
	if err := row.Scan(&c.ID, &c.Name, &c.Provider, &c.CredentialEnc, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DnsCredential{}, ErrDnsCredentialNotFound
		}
		return DnsCredential{}, wrapErr("dns credential get by id", err)
	}
	var err error
	if c.CreatedAt, err = scanTime(createdAt); err != nil {
		return DnsCredential{}, fmt.Errorf("scan dns credential created_at: %w", err)
	}
	if c.UpdatedAt, err = scanTime(updatedAt); err != nil {
		return DnsCredential{}, fmt.Errorf("scan dns credential updated_at: %w", err)
	}
	return c, nil
}

// List 全量清单（SQL #8 零回显形态：不含 credential_enc——呈现层永不解密）。
func (r *DnsCredentialRepo) List(ctx context.Context) ([]DnsCredential, error) {
	rows, err := r.x.QueryContext(ctx,
		`SELECT id, name, provider, created_at, updated_at
		 FROM dns_credential ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, wrapErr("dns credential list", err)
	}
	defer rows.Close()
	var out []DnsCredential
	for rows.Next() {
		var c DnsCredential
		var createdAt, updatedAt any
		if err := rows.Scan(&c.ID, &c.Name, &c.Provider, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("store/sitemgmt: dns credential list scan: %w", err)
		}
		if c.CreatedAt, err = scanTime(createdAt); err != nil {
			return nil, fmt.Errorf("scan dns credential created_at: %w", err)
		}
		if c.UpdatedAt, err = scanTime(updatedAt); err != nil {
			return nil, fmt.Errorf("scan dns credential updated_at: %w", err)
		}
		out = append(out, c)
	}
	return out, wrapErr("dns credential list rows", rows.Err())
}

// Delete 删除行（SQL #9；密文随行删除；引用警示与 I-C01 悬空语义在 service 层）。
func (r *DnsCredentialRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.x.ExecContext(ctx, `DELETE FROM dns_credential WHERE id = ?`, id)
	return wrapErr("dns credential delete", err)
}
