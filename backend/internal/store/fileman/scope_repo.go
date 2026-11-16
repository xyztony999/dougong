package fileman

import (
	"context"
	"strings"
	"time"
)

// ScopeRepo 实现 fs_path_scope 单行表访问（SQL 清单 §5.2 #1~#3，全参数化）。
// 修改 Update 与审计同事务由调用方（service BeginTx + TxStore.Audit）保证（D-02）。
type ScopeRepo struct{ x Executor }

// NewScopeRepo 构造绑定给定执行器的受管根仓储。
func NewScopeRepo(x Executor) *ScopeRepo { return &ScopeRepo{x: x} }

// EnsureRow 启动自检（SQL #1）：INSERT OR IGNORE 保证 id=1 行存在（幂等）。
func (r *ScopeRepo) EnsureRow(ctx context.Context, at time.Time) error {
	_, err := r.x.ExecContext(ctx,
		`INSERT OR IGNORE INTO fs_path_scope (id, root_path, excluded_dirs, updated_at)
		 VALUES (1, '/', '/proc,/sys,/dev', ?)`, formatTime(at))
	return wrapErr("scope ensure row", err)
}

// Get 读取单行（SQL #2：启动加载内存副本 + path-scope/get 数据源）；
// 行不存在返回包装后的 sql.ErrNoRows（EnsureRow 前不可达）。
func (r *ScopeRepo) Get(ctx context.Context) (PathScope, error) {
	var s PathScope
	var excluded, updatedAt any
	err := r.x.QueryRowContext(ctx,
		`SELECT id, root_path, excluded_dirs, updated_at FROM fs_path_scope WHERE id = 1`).
		Scan(new(any), &s.RootPath, &excluded, &updatedAt)
	if err != nil {
		return PathScope{}, wrapErr("scope get", err)
	}
	s.ExcludedDirs = splitExcluded(asString(excluded))
	t, err := scanTime(updatedAt)
	if err != nil {
		return PathScope{}, wrapErr("scope scan updated_at", err)
	}
	s.UpdatedAt = t
	return s, nil
}

// Update 修改受管根与排除目录（SQL #3：与审计同事务，D-02；调用方事务视图内执行）。
func (r *ScopeRepo) Update(ctx context.Context, rootPath string, excludedDirs []string, at time.Time) error {
	_, err := r.x.ExecContext(ctx,
		`UPDATE fs_path_scope SET root_path = ?, excluded_dirs = ?, updated_at = ? WHERE id = 1`,
		rootPath, joinExcluded(excludedDirs), formatTime(at))
	return wrapErr("scope update", err)
}

// splitExcluded 将逗号分隔存储拆为切片（空串→nil）。
func splitExcluded(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// joinExcluded 将切片合为逗号分隔存储（与 DDL 默认值同形）。
func joinExcluded(dirs []string) string {
	return strings.Join(dirs, ",")
}

// asString 宽容取串（驱动可能返回 []byte）。
func asString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	case nil:
		return ""
	default:
		return ""
	}
}
