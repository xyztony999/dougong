package auth

import (
	"context"
	"database/sql"
	"errors"
)

// DeviceRepo 实现 panel_user_devices 表访问（新设备识别）。
type DeviceRepo struct{ x Executor }

// Touch 登录成功时 upsert 设备；返回是否首次（newDevice）。
func (r *DeviceRepo) Touch(ctx context.Context, userID int64, deviceHash, userAgent, now string) (bool, error) {
	var existing int
	err := r.x.QueryRowContext(ctx,
		`SELECT id FROM panel_user_devices WHERE user_id = ? AND device_hash = ?`, userID, deviceHash).Scan(&existing)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := r.x.ExecContext(ctx,
			`INSERT INTO panel_user_devices (user_id, device_hash, user_agent, first_seen_at, last_seen_at) VALUES (?, ?, ?, ?, ?)`,
			userID, deviceHash, userAgent, now, now); err != nil {
			return false, wrapErr("device insert", err)
		}
		return true, nil
	case err != nil:
		return false, wrapErr("device get", err)
	}
	if _, err := r.x.ExecContext(ctx,
		`UPDATE panel_user_devices SET last_seen_at = ? WHERE id = ?`, now, existing); err != nil {
		return false, wrapErr("device touch", err)
	}
	return false, nil
}
