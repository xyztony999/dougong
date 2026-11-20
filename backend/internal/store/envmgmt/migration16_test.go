package envmgmt

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dougong/dougong/internal/store"
)

// TestMigrate16PythonCheck 迁移 016 回归（v1.4 ⚠️A18）：env_type CHECK 九值
// 重建后 python 可插入、非法值仍拒（表重建沿 010/014 先例，迁移全量自动执行）。
func TestMigrate16PythonCheck(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO env_install_task (env_type, action, status, requested_by, created_at, updated_at)
		 VALUES ('python','install','pending','admin','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("insert python（迁移 016 CHECK 应放行九值）: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO env_install_task (env_type, action, status, requested_by, created_at, updated_at)
		 VALUES ('perl','install','pending','admin','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`); err == nil {
		t.Fatal("perl 应被 CHECK 拒绝（九值外取值域 R-14）")
	}
}
