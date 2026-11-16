package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// InitRepo 实现 init_state 单行表访问（I-02：单行单向 0→1 不可逆）。
type InitRepo struct{ x Executor }

// EnsureRow 启动自检：INSERT OR IGNORE 保证 id=1 行存在（SQL 清单 #1，幂等）。
func (r *InitRepo) EnsureRow(ctx context.Context) error {
	_, err := r.x.ExecContext(ctx,
		`INSERT OR IGNORE INTO init_state (id, initialized) VALUES (1, 0)`)
	return wrapErr("init ensure row", err)
}

// Get 读取初始化状态；行不存在（迁移后未自检）视作未初始化零值。
func (r *InitRepo) Get(ctx context.Context) (InitState, error) {
	var initialized int
	var method sql.NullString
	var initializedAt any
	err := r.x.QueryRowContext(ctx,
		`SELECT initialized, method, initialized_at FROM init_state WHERE id = 1`).
		Scan(&initialized, &method, &initializedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return InitState{}, nil
	}
	if err != nil {
		return InitState{}, wrapErr("init get", err)
	}
	st := InitState{Initialized: initialized == 1, Method: scanStringPtr(method)}
	at, err := scanTimePtr(initializedAt)
	if err != nil {
		return InitState{}, wrapErr("init scan initialized_at", err)
	}
	st.InitializedAt = at
	return st, nil
}

// TryClaim 初始化仲裁抢占（SQL 清单 #2）：仅当 initialized=0 时置 1 并记录方式/时间，
// 返回是否抢占成功；后到者 false（I-02，上层转 AUTH_009）。须在事务内调用。
func (r *InitRepo) TryClaim(ctx context.Context, method string, at time.Time) (bool, error) {
	res, err := r.x.ExecContext(ctx,
		`UPDATE init_state SET initialized = 1, method = ?, initialized_at = ?
		 WHERE id = 1 AND initialized = 0`,
		method, formatTime(at))
	if err != nil {
		return false, wrapErr("init try claim", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, wrapErr("init try claim rows", err)
	}
	return n == 1, nil
}
