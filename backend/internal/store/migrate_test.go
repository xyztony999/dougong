package store

import (
	"context"
	"path/filepath"
	"testing"
)

// TestMigrateIdempotent 迁移可重复执行（版本表跳过已应用项）。
// 迁移数量动态对齐 migrations/*.sql（模块无关的骨架断言：后续模块新增
// 迁移文件无须改此测试；测试 CWD 为本包目录，相对路径稳定）。
func TestMigrateIdempotent(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("migrations", "*.sql"))
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no migration files found")
	}
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		if cerr := db.Close(); cerr != nil {
			t.Errorf("close: %v", cerr)
		}
	})
	ctx := context.Background()
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	var versions int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&versions); err != nil {
		t.Fatalf("count versions: %v", err)
	}
	if versions != len(files) {
		t.Fatalf("schema_migrations rows = %d, want %d（= migrations/*.sql 文件数）", versions, len(files))
	}
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	// 幂等：二次执行不新增版本行。
	var versions2 int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&versions2); err != nil {
		t.Fatalf("recount versions: %v", err)
	}
	if versions2 != versions {
		t.Fatalf("second migrate changed rows: %d -> %d", versions, versions2)
	}
	// 表结构就位（基线子集：account-auth 5 表 + panel-core 2 表 + file-manager 2 表）。
	for _, table := range []string{"panel_user", "panel_user_totp", "panel_session", "audit_log", "init_state",
		"panel_config", "panel_cert", "trash_entry", "fs_path_scope"} {
		var n int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&n); err != nil { //nolint:gosec // 表名来自本测试静态白名单
			t.Fatalf("table %s missing: %v", table, err)
		}
	}
}
