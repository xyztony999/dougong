package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate 将内嵌迁移（internal/store/migrations/NNN_*.sql，按文件名序）应用到
// 未执行过的版本。每个迁移在独立事务中执行并记录版本号，重复调用幂等。
// 后续模块新增表时只需追加 NNN_*.sql 文件，共用同一 schema_migrations 版本表。
func Migrate(ctx context.Context, db *sql.DB) error {
	return MigrateFS(ctx, db, migrationsFS)
}

// MigrateFS 对给定嵌入文件系统执行迁移（测试可注入独立 FS）。
func MigrateFS(ctx context.Context, db *sql.DB, fsys embed.FS) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    TEXT PRIMARY KEY,
		applied_at DATETIME NOT NULL
	)`); err != nil {
		return fmt.Errorf("migrate: create version table: %w", err)
	}
	names, err := fs.Glob(fsys, "migrations/*.sql")
	if err != nil {
		return fmt.Errorf("migrate: glob migrations: %w", err)
	}
	sort.Strings(names)
	for _, name := range names {
		version := strings.TrimSuffix(filepath.Base(name), ".sql")
		var applied int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version,
		).Scan(&applied); err != nil {
			return fmt.Errorf("migrate: check %s: %w", version, err)
		}
		if applied > 0 {
			continue
		}
		body, err := fsys.ReadFile(name)
		if err != nil {
			return fmt.Errorf("migrate: read %s: %w", name, err)
		}
		if err := applyMigration(ctx, db, version, string(body)); err != nil {
			return err
		}
	}
	return nil
}

// applyMigration 在单事务内执行迁移体并登记版本；任何失败整体回滚。
func applyMigration(ctx context.Context, db *sql.DB, version, body string) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("migrate %s: begin: %w", version, err)
	}
	defer func() {
		if rerr := tx.Rollback(); rerr != nil && !errors.Is(rerr, sql.ErrTxDone) {
			err = errors.Join(fmt.Errorf("migrate %s: rollback: %w", version, rerr))
		}
	}()
	if _, err := tx.ExecContext(ctx, body); err != nil {
		return fmt.Errorf("migrate %s: exec: %w", version, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)`,
		version, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("migrate %s: record version: %w", version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migrate %s: commit: %w", version, err)
	}
	return nil
}
