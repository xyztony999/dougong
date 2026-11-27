package auth

import (
	"context"
	"database/sql"
	"errors"
)

// RecoveryRepo 实现 panel_user_recovery_codes 表访问（一次性恢复码，哈希存储）。
type RecoveryRepo struct{ x Executor }

// ReplaceUnused 作废用户全部未用码并批量写入新码（同事务，generate 语义）。
func (r *RecoveryRepo) ReplaceUnused(ctx context.Context, userID int64, hashes []string, now string) error {
	if _, err := r.x.ExecContext(ctx,
		`DELETE FROM panel_user_recovery_codes WHERE user_id = ? AND used_at IS NULL`, userID); err != nil {
		return wrapErr("recovery delete unused", err)
	}
	for _, h := range hashes {
		if _, err := r.x.ExecContext(ctx,
			`INSERT INTO panel_user_recovery_codes (user_id, code_hash, used_at, created_at) VALUES (?, ?, NULL, ?)`,
			userID, h, now); err != nil {
			return wrapErr("recovery insert", err)
		}
	}
	return nil
}

// GetUnusedByHash 取未用码（恢复码登录校验）；无行返回 sql.ErrNoRows。
func (r *RecoveryRepo) GetUnusedByHash(ctx context.Context, userID int64, hash string) (RecoveryCode, error) {
	var c RecoveryCode
	var usedAt any
	var createdAt any
	err := r.x.QueryRowContext(ctx,
		`SELECT id, user_id, code_hash, used_at, created_at FROM panel_user_recovery_codes
		 WHERE user_id = ? AND code_hash = ? AND used_at IS NULL`, userID, hash).
		Scan(&c.ID, &c.UserID, &c.CodeHash, &usedAt, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return RecoveryCode{}, err
	}
	if err != nil {
		return RecoveryCode{}, wrapErr("recovery get", err)
	}
	ct, err := scanTime(createdAt)
	if err != nil {
		return RecoveryCode{}, err
	}
	c.CreatedAt = ct
	return c, nil
}

// MarkUsed 标记一码已用（一次性）。
func (r *RecoveryRepo) MarkUsed(ctx context.Context, id int64, now string) error {
	_, err := r.x.ExecContext(ctx,
		`UPDATE panel_user_recovery_codes SET used_at = ? WHERE id = ? AND used_at IS NULL`, now, id)
	return wrapErr("recovery mark used", err)
}

// CountUnused 剩余未用码数。
func (r *RecoveryRepo) CountUnused(ctx context.Context, userID int64) (int, error) {
	var n int
	err := r.x.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM panel_user_recovery_codes WHERE user_id = ? AND used_at IS NULL`, userID).Scan(&n)
	return n, wrapErr("recovery count", err)
}
