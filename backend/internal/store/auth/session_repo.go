package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SessionRepo 实现 panel_session 表访问。会话有效性为计算态（I-11/D-03）：
// revoked_at IS NULL AND now < expires_at AND now < idle_expires_at，全部以
// RFC3339 UTC 字符串参数化比较（字典序=时间序）。
type SessionRepo struct{ x Executor }

const sessionSelect = `SELECT s.token_hash, s.user_id, u.username, s.csrf_token,
	s.created_at, s.last_active_at, s.expires_at, s.idle_expires_at,
	s.revoked_at, s.end_reason, s.source_ip, s.user_agent
	FROM panel_session s JOIN panel_user u ON u.id = s.user_id`

func scanSession(row interface{ Scan(...any) error }) (Session, error) {
	var s Session
	var createdAt, lastActive, expiresAt, idleExpiresAt any
	var revokedAt any
	var endReason, sourceIP, userAgent sql.NullString
	if err := row.Scan(&s.TokenHash, &s.UserID, &s.Username, &s.CsrfToken,
		&createdAt, &lastActive, &expiresAt, &idleExpiresAt,
		&revokedAt, &endReason, &sourceIP, &userAgent); err != nil {
		return Session{}, err
	}
	var err error
	if s.CreatedAt, err = scanTime(createdAt); err != nil {
		return Session{}, fmt.Errorf("scan session created_at: %w", err)
	}
	if s.LastActiveAt, err = scanTime(lastActive); err != nil {
		return Session{}, fmt.Errorf("scan session last_active_at: %w", err)
	}
	if s.ExpiresAt, err = scanTime(expiresAt); err != nil {
		return Session{}, fmt.Errorf("scan session expires_at: %w", err)
	}
	if s.IdleExpiresAt, err = scanTime(idleExpiresAt); err != nil {
		return Session{}, fmt.Errorf("scan session idle_expires_at: %w", err)
	}
	if s.RevokedAt, err = scanTimePtr(revokedAt); err != nil {
		return Session{}, fmt.Errorf("scan session revoked_at: %w", err)
	}
	s.EndReason = scanStringPtr(endReason)
	s.SourceIP = scanStringPtr(sourceIP)
	s.UserAgent = scanStringPtr(userAgent)
	return s, nil
}

// Create 插入新会话（revoked_at/end_reason 初始为 NULL）。
func (r *SessionRepo) Create(ctx context.Context, s Session) error {
	_, err := r.x.ExecContext(ctx,
		`INSERT INTO panel_session (token_hash, user_id, csrf_token, created_at, last_active_at,
		 expires_at, idle_expires_at, revoked_at, end_reason, source_ip, user_agent)
		 VALUES (?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?, ?)`,
		s.TokenHash, s.UserID, s.CsrfToken, formatTime(s.CreatedAt), formatTime(s.LastActiveAt),
		formatTime(s.ExpiresAt), formatTime(s.IdleExpiresAt),
		stringPtrArg(s.SourceIP), stringPtrArg(s.UserAgent))
	return wrapErr("session create", err)
}

// GetByTokenHash 按 token 哈希取会话（含用户名）；无行返回 sql.ErrNoRows。
func (r *SessionRepo) GetByTokenHash(ctx context.Context, tokenHash string) (Session, error) {
	s, err := scanSession(r.x.QueryRowContext(ctx,
		sessionSelect+` WHERE s.token_hash = ?`, tokenHash))
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, fmt.Errorf("session %q: %w", tokenHash, sql.ErrNoRows)
	}
	return s, wrapErr("session get", err)
}

// Touch 滑动续期：刷新 last_active_at 与 idle_expires_at（ADR-019）。
func (r *SessionRepo) Touch(ctx context.Context, tokenHash string, lastActiveAt, idleExpiresAt time.Time) error {
	_, err := r.x.ExecContext(ctx,
		`UPDATE panel_session SET last_active_at = ?, idle_expires_at = ? WHERE token_hash = ?`,
		formatTime(lastActiveAt), formatTime(idleExpiresAt), tokenHash)
	return wrapErr("session touch", err)
}

// Revoke 吊销单个会话并写 end_reason（revoked_at 非空时必填，I-13 立即生效）。
func (r *SessionRepo) Revoke(ctx context.Context, tokenHash string, revokedAt time.Time, reason string) error {
	_, err := r.x.ExecContext(ctx,
		`UPDATE panel_session SET revoked_at = ?, end_reason = ? WHERE token_hash = ? AND revoked_at IS NULL`,
		formatTime(revokedAt), reason, tokenHash)
	return wrapErr("session revoke", err)
}

// RevokeByUser 级联吊销某用户会话（I-04 编排 §2.3：删除/重置=全部，改密=除当前外）。
func (r *SessionRepo) RevokeByUser(ctx context.Context, userID int64, revokedAt time.Time, reason, exceptTokenHash string) error {
	q := `UPDATE panel_session SET revoked_at = ?, end_reason = ? WHERE user_id = ? AND revoked_at IS NULL`
	args := []any{formatTime(revokedAt), reason, userID}
	if exceptTokenHash != "" {
		q += ` AND token_hash != ?`
		args = append(args, exceptTokenHash)
	}
	_, err := r.x.ExecContext(ctx, q, args...)
	return wrapErr("session revoke by user", err)
}

// ListActive 列出全部有效会话（I-11），按 last_active_at 倒序（§4.2.4）。
func (r *SessionRepo) ListActive(ctx context.Context, now time.Time) ([]Session, error) {
	rows, err := r.x.QueryContext(ctx,
		sessionSelect+` WHERE s.revoked_at IS NULL AND s.expires_at > ? AND s.idle_expires_at > ?
		 ORDER BY s.last_active_at DESC`,
		formatTime(now), formatTime(now))
	if err != nil {
		return nil, wrapErr("session list active", err)
	}
	defer closeRows(rows)
	var out []Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("session list scan: %w", err)
		}
		out = append(out, s)
	}
	return out, wrapErr("session list rows", rows.Err())
}

// FindActiveByDigest 按会话摘要（token_hash 前 8 位）取有效会话（revoke 接口入参）。
func (r *SessionRepo) FindActiveByDigest(ctx context.Context, digest string, now time.Time) (Session, error) {
	s, err := scanSession(r.x.QueryRowContext(ctx,
		sessionSelect+` WHERE substr(s.token_hash, 1, 8) = ?
		 AND s.revoked_at IS NULL AND s.expires_at > ? AND s.idle_expires_at > ?`,
		digest, formatTime(now), formatTime(now)))
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, fmt.Errorf("session digest=%s: %w", digest, sql.ErrNoRows)
	}
	return s, wrapErr("session find by digest", err)
}

// DeleteByUser 物理删除某用户全部会话行（仓储契约 §4.2.5）。
func (r *SessionRepo) DeleteByUser(ctx context.Context, userID int64) error {
	_, err := r.x.ExecContext(ctx, `DELETE FROM panel_session WHERE user_id = ?`, userID)
	return wrapErr("session delete by user", err)
}

// DeleteFinished 物理删除已完成会话（已吊销早于 cutoff，或已过期），分批限量（仓储契约 §4.2.5）。
func (r *SessionRepo) DeleteFinished(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	res, err := r.x.ExecContext(ctx,
		`DELETE FROM panel_session WHERE token_hash IN (
			SELECT token_hash FROM panel_session
			WHERE (revoked_at IS NOT NULL AND revoked_at < ?) OR expires_at < ?
			LIMIT ?)`,
		formatTime(cutoff), formatTime(cutoff), limit)
	if err != nil {
		return 0, wrapErr("session delete finished", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("session delete finished rows: %w", err)
	}
	return n, nil
}

func stringPtrArg(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

// closeRows 在 defer 中关闭结果集；Close 失败无副作用（连接由池回收），可安全忽略。
func closeRows(rows *sql.Rows) {
	rows.Close() //nolint:errcheck // defer 场景，失败不影响正确性
}
