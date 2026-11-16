// Package store 提供 SQLite 存储基座与迁移框架，是全部业务模块仓储的公共依赖。
//
// 连接策略（api-account-auth.md §3.2）：SQLite 单连接写串行；PRAGMA 经 DSN
// 连接级下发（外键强制 / WAL / busy_timeout / synchronous=NORMAL）。
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // 纯 Go SQLite 驱动（免 CGO），注册驱动名 "sqlite"
)

// Executor 是 *sql.DB 与 *sql.Tx 共同满足的最小执行接口。仓储实现依赖本接口，
// 从而同一仓储即可绑定连接池、也可绑定事务（I-05：审计与业务变更同事务）。
type Executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Open 打开（必要时创建）SQLite 数据库。path 为文件路径或 ":memory:"（测试用），
// 父目录不存在时自动创建。
func Open(path string) (*sql.DB, error) {
	dsn, err := buildDSN(path)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store.Open(%s): %w", path, err)
	}
	// 设计文档 §3.2「单连接写串行」：单机面板流量下读写单连接足够，
	// 同时天然规避 SQLite 写锁竞争。
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	return db, nil
}

// buildDSN 组装 modernc.org/sqlite DSN，PRAGMA 以连接级参数下发（对 :memory: 同样生效）。
func buildDSN(path string) (string, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		return "", fmt.Errorf("store: db path is empty")
	}
	if p != ":memory:" {
		dir := filepath.Dir(p)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("store.Open mkdir %s: %w", dir, err)
		}
		p = "file:" + filepath.ToSlash(p)
	}
	return p + "?_pragma=foreign_keys(1)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)", nil
}
