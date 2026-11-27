package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// WebauthnRepo 实现 panel_user_webauthn 表访问（1:N passkey 凭据）。
type WebauthnRepo struct{ x Executor }

// Create 写入凭据（注册 finish）。
func (r *WebauthnRepo) Create(ctx context.Context, c WebauthnCredential) (int64, error) {
	transports, err := json.Marshal(c.Transports)
	if err != nil {
		return 0, wrapErr("webauthn marshal transports", err)
	}
	res, err := r.x.ExecContext(ctx,
		`INSERT INTO panel_user_webauthn (user_id, credential_id, public_key, attestation_type, aaguid, sign_count, transports, name, created_at, last_used_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.UserID, c.CredentialID, c.PublicKey, c.AttestationType, c.AAGUID, c.SignCount, string(transports), c.Name,
		formatTime(c.CreatedAt), formatTimePtr(c.LastUsedAt))
	if err != nil {
		return 0, wrapErr("webauthn create", err)
	}
	id, err := res.LastInsertId()
	return id, wrapErr("webauthn lastid", err)
}

// ListByUser 取用户全部凭据（创建序）。
func (r *WebauthnRepo) ListByUser(ctx context.Context, userID int64) ([]WebauthnCredential, error) {
	rows, err := r.x.QueryContext(ctx,
		`SELECT id, user_id, credential_id, public_key, attestation_type, aaguid, sign_count, transports, name, created_at, last_used_at
		 FROM panel_user_webauthn WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, wrapErr("webauthn list", err)
	}
	defer func() { _ = rows.Close() }()
	var out []WebauthnCredential
	for rows.Next() {
		c, err := scanWebauthn(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, wrapErr("webauthn rows", rows.Err())
}

// GetByID 取单条（须属该用户，删/校验用）；无行返回 sql.ErrNoRows。
func (r *WebauthnRepo) GetByID(ctx context.Context, userID, id int64) (WebauthnCredential, error) {
	row := r.x.QueryRowContext(ctx,
		`SELECT id, user_id, credential_id, public_key, attestation_type, aaguid, sign_count, transports, name, created_at, last_used_at
		 FROM panel_user_webauthn WHERE id = ? AND user_id = ?`, id, userID)
	c, err := scanWebauthn(row)
	if errors.Is(err, sql.ErrNoRows) {
		return WebauthnCredential{}, err
	}
	return c, err
}

// GetByCredentialID 按凭据标识取（断言后更新 sign_count 用）。
func (r *WebauthnRepo) GetByCredentialID(ctx context.Context, credentialID []byte) (WebauthnCredential, error) {
	row := r.x.QueryRowContext(ctx,
		`SELECT id, user_id, credential_id, public_key, attestation_type, aaguid, sign_count, transports, name, created_at, last_used_at
		 FROM panel_user_webauthn WHERE credential_id = ?`, credentialID)
	return scanWebauthn(row)
}

// Touch 更新 sign_count/last_used_at（time.Time 由调用方传入 RFC3339 字符串）。
func (r *WebauthnRepo) Touch(ctx context.Context, id int64, signCount uint32, lastUsed string) error {
	_, err := r.x.ExecContext(ctx,
		`UPDATE panel_user_webauthn SET sign_count = ?, last_used_at = ? WHERE id = ?`,
		signCount, lastUsed, id)
	return wrapErr("webauthn touch", err)
}

// Delete 删除本人凭据。
func (r *WebauthnRepo) Delete(ctx context.Context, userID, id int64) error {
	_, err := r.x.ExecContext(ctx, `DELETE FROM panel_user_webauthn WHERE id = ? AND user_id = ?`, id, userID)
	return wrapErr("webauthn delete", err)
}

// CountByUser 凭据数（登录二因素判定用）。
func (r *WebauthnRepo) CountByUser(ctx context.Context, userID int64) (int, error) {
	var n int
	err := r.x.QueryRowContext(ctx, `SELECT COUNT(*) FROM panel_user_webauthn WHERE user_id = ?`, userID).Scan(&n)
	return n, wrapErr("webauthn count", err)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanWebauthn(row rowScanner) (WebauthnCredential, error) {
	var c WebauthnCredential
	var transports string
	var aaguid []byte
	var createdAt, lastUsed any
	err := row.Scan(&c.ID, &c.UserID, &c.CredentialID, &c.PublicKey, &c.AttestationType, &aaguid, &c.SignCount, &transports, &c.Name, &createdAt, &lastUsed)
	if err != nil {
		return WebauthnCredential{}, wrapErr("webauthn scan", err)
	}
	c.AAGUID = aaguid
	ct, err := scanTime(createdAt)
	if err != nil {
		return WebauthnCredential{}, err
	}
	c.CreatedAt = ct
	if err := json.Unmarshal([]byte(transports), &c.Transports); err != nil {
		return WebauthnCredential{}, wrapErr("webauthn unmarshal transports", err)
	}
	lu, err := scanTimePtr(lastUsed)
	if err != nil {
		return WebauthnCredential{}, err
	}
	c.LastUsedAt = lu
	return c, nil
}
