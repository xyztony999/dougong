package auth

import (
	"context"
	"database/sql"
	"errors"
)

// TotpRepo 实现 panel_user_totp 表访问（1:1 子表）。
type TotpRepo struct{ x Executor }

// GetByUser 取用户 TOTP 凭证；无绑定行返回 sql.ErrNoRows。
func (r *TotpRepo) GetByUser(ctx context.Context, userID int64) (TotpCredential, error) {
	var t TotpCredential
	var enabled int
	var confirmedAt any
	err := r.x.QueryRowContext(ctx,
		`SELECT user_id, secret, enabled, confirmed_at FROM panel_user_totp WHERE user_id = ?`,
		userID).Scan(&t.UserID, &t.Secret, &enabled, &confirmedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return TotpCredential{}, errors.Join(errNoTotp, sql.ErrNoRows)
	}
	if err != nil {
		return TotpCredential{}, wrapErr("totp get", err)
	}
	t.Enabled = enabled == 1
	ct, err := scanTimePtr(confirmedAt)
	if err != nil {
		return TotpCredential{}, wrapErr("totp scan confirmed_at", err)
	}
	t.ConfirmedAt = ct
	return t, nil
}

// Upsert 写入凭证：未激活旧值可覆盖重生成（设计文档 §4.1.4 totp-setup）。
func (r *TotpRepo) Upsert(ctx context.Context, t TotpCredential) error {
	var confirmed any
	if t.ConfirmedAt != nil {
		confirmed = formatTime(*t.ConfirmedAt)
	}
	_, err := r.x.ExecContext(ctx,
		`INSERT INTO panel_user_totp (user_id, secret, enabled, confirmed_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET secret = excluded.secret, enabled = excluded.enabled, confirmed_at = excluded.confirmed_at`,
		t.UserID, t.Secret, boolInt(t.Enabled), confirmed)
	return wrapErr("totp upsert", err)
}

// Delete 删除凭证行（totp-disable）。
func (r *TotpRepo) Delete(ctx context.Context, userID int64) error {
	_, err := r.x.ExecContext(ctx, `DELETE FROM panel_user_totp WHERE user_id = ?`, userID)
	return wrapErr("totp delete", err)
}

// errNoTotp 便于上层识别“无绑定行”场景（同时携带 sql.ErrNoRows 保持 errors.Is 兼容）。
var errNoTotp = errors.New("store/auth: totp credential not found")

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
