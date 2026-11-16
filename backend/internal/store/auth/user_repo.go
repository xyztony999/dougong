package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// UserRepo 实现 panel_user 表访问。所有语句参数化（? 占位），
// 不存在行返回包装后的 sql.ErrNoRows，由 service 层转换业务语义。
type UserRepo struct{ x Executor }

const userCols = `id, username, password_hash, role, status, fail_count, locked_until, created_at, updated_at`

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var u User
	var lockedUntil, createdAt, updatedAt any
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Status,
		&u.FailCount, &lockedUntil, &createdAt, &updatedAt); err != nil {
		return User{}, err
	}
	var err error
	if u.LockedUntil, err = scanTimePtr(lockedUntil); err != nil {
		return User{}, fmt.Errorf("scan user locked_until: %w", err)
	}
	if u.CreatedAt, err = scanTime(createdAt); err != nil {
		return User{}, fmt.Errorf("scan user created_at: %w", err)
	}
	if u.UpdatedAt, err = scanTime(updatedAt); err != nil {
		return User{}, fmt.Errorf("scan user updated_at: %w", err)
	}
	return u, nil
}

// GetByID 按主键取用户。
func (r *UserRepo) GetByID(ctx context.Context, id int64) (User, error) {
	u, err := scanUser(r.x.QueryRowContext(ctx,
		`SELECT `+userCols+` FROM panel_user WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, fmt.Errorf("user id=%d: %w", id, sql.ErrNoRows)
	}
	return u, wrapErr("user get by id", err)
}

// GetByUsername 按用户名取用户（COLLATE NOCASE，I-06 大小写不敏感唯一）。
func (r *UserRepo) GetByUsername(ctx context.Context, username string) (User, error) {
	u, err := scanUser(r.x.QueryRowContext(ctx,
		`SELECT `+userCols+` FROM panel_user WHERE username = ? COLLATE NOCASE`, username))
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, fmt.Errorf("user %q: %w", username, sql.ErrNoRows)
	}
	return u, wrapErr("user get by username", err)
}

// List 全量用户（用户量极小，无分页，设计文档 §4.1.4）。
func (r *UserRepo) List(ctx context.Context) ([]User, error) {
	rows, err := r.x.QueryContext(ctx, `SELECT `+userCols+` FROM panel_user ORDER BY id`)
	if err != nil {
		return nil, wrapErr("user list", err)
	}
	defer closeRows(rows)
	var users []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("user list scan: %w", err)
		}
		users = append(users, u)
	}
	return users, wrapErr("user list rows", rows.Err())
}

// Create 插入用户（新用户 status 恒 active、fail_count 恒 0），返回自增 id。
func (r *UserRepo) Create(ctx context.Context, u User, now time.Time) (int64, error) {
	res, err := r.x.ExecContext(ctx,
		`INSERT INTO panel_user (username, password_hash, role, status, fail_count, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 0, ?, ?)`,
		u.Username, u.PasswordHash, u.Role, UserStatusActive, formatTime(now), formatTime(now))
	if err != nil {
		return 0, wrapErr("user create", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("user create last id: %w", err)
	}
	return id, nil
}

// Update 更新用户全量可变列（用户名/密码/角色）。
func (r *UserRepo) Update(ctx context.Context, u User, now time.Time) error {
	_, err := r.x.ExecContext(ctx,
		`UPDATE panel_user SET username = ?, password_hash = ?, role = ?, updated_at = ? WHERE id = ?`,
		u.Username, u.PasswordHash, u.Role, formatTime(now), u.ID)
	return wrapErr("user update", err)
}

// UpdatePassword 仅更新密码哈希。
func (r *UserRepo) UpdatePassword(ctx context.Context, id int64, passwordHash string, now time.Time) error {
	_, err := r.x.ExecContext(ctx,
		`UPDATE panel_user SET password_hash = ?, updated_at = ? WHERE id = ?`,
		passwordHash, formatTime(now), id)
	return wrapErr("user update password", err)
}

// UpdateLockState 更新内嵌锁定字段组（R-01：fail_count/locked_until/status 同步迁移）。
func (r *UserRepo) UpdateLockState(ctx context.Context, id int64, failCount int, lockedUntil *time.Time, status string, now time.Time) error {
	_, err := r.x.ExecContext(ctx,
		`UPDATE panel_user SET fail_count = ?, locked_until = ?, status = ?, updated_at = ? WHERE id = ?`,
		failCount, formatTimePtr(lockedUntil), status, formatTime(now), id)
	return wrapErr("user update lock state", err)
}

// Delete 删除用户（panel_session 经 FK ON DELETE CASCADE 级联清理，
// 删除前由 service 先按 end_reason=cascade 显式吊销，见编排 §2.3）。
func (r *UserRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.x.ExecContext(ctx, `DELETE FROM panel_user WHERE id = ?`, id)
	return wrapErr("user delete", err)
}

// CountAdmins 统计 admin 数量（I-03 唯一管理员保护）。
func (r *UserRepo) CountAdmins(ctx context.Context) (int64, error) {
	var n int64
	err := r.x.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM panel_user WHERE role = ?`, RoleAdmin).Scan(&n)
	return n, wrapErr("user count admins", err)
}

func wrapErr(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("store/auth: %s: %w", op, err)
}
