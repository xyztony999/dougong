package store

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
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

// ---- 带数据升级（迁移 018：app 表重建）----
//
// 这组测试对应已部署主机的升级路径：库里已经有旧版形态的项目与任务，再升到 018。
// 此前几次表重建只有空库测试。

const upgradeTS = "2026-11-20T10:00:00Z"

// upgradedDB 得到一个「旧版库升级到当前」的内存库：先逐个应用 018 之前的迁移，
// 用 seed 写入旧版形态的数据，再跑完整迁移（此时只剩 018 及之后未应用）。
func upgradedDB(t *testing.T, seed func(exec func(q string, args ...any))) (*sql.DB, context.Context) {
	t.Helper()
	names, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
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
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("exec failed: %v\n%s", err, q)
		}
	}
	// 版本表平时由 MigrateFS 先建；这里直接用 applyMigration，所以先建出来（DDL 同 MigrateFS）。
	exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    TEXT PRIMARY KEY,
		applied_at DATETIME NOT NULL
	)`)
	for _, name := range names {
		version := strings.TrimSuffix(filepath.Base(name), ".sql")
		if version >= "018" {
			continue
		}
		body, err := migrationsFS.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if err := applyMigration(ctx, db, version, string(body)); err != nil {
			t.Fatalf("apply %s: %v", version, err)
		}
	}
	if cols := appColumnDefs(ctx, db); strings.Contains(cols, "runtime") {
		t.Fatalf("升级前不应有 runtime 列，实际: %s", cols)
	}
	if seed != nil {
		seed(exec)
	}
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db, ctx
}

// insertApp 新建一个 Node 项目并返回它拿到的 id（同一事务内取 last_insert_rowid，确保是同一连接）。
func insertApp(t *testing.T, ctx context.Context, db *sql.DB, name string, port int) int {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() {
		_ = tx.Rollback() // 已提交后返回 ErrTxDone，无需处理
	}()
	unit := fmt.Sprintf("dougong-app-%s.service", name)
	if _, err := tx.ExecContext(ctx, `INSERT INTO app (name, dir, entry, runtime, start_mode, start_spec, listen_port, unit_name, created_by, created_at, updated_at)
		VALUES (?, '/opt/x', 'server.js', 'node', 'node_file', '{"file":"server.js"}', ?, ?, 'admin', ?, ?)`,
		name, port, unit, upgradeTS, upgradeTS); err != nil {
		t.Fatalf("升级后新建项目 %s: %v", name, err)
	}
	var id int
	if err := tx.QueryRowContext(ctx, `SELECT last_insert_rowid()`).Scan(&id); err != nil {
		t.Fatalf("last_insert_rowid: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return id
}

// TestMigrate018PreservesExistingApps 带数据升级的主场景：既有项目、已下线的容器形态
// 条目、任务台账一行不丢且逐字段不变；自增序号不回落；新列与新约束生效。
func TestMigrate018PreservesExistingApps(t *testing.T) {
	db, ctx := upgradedDB(t, func(exec func(q string, args ...any)) {
		// 旧版形态的数据：FastAPI 预设、已下线的容器形态、自定义命令各一条
		exec(`INSERT INTO app (id, name, dir, entry, start_mode, start_spec, listen_port, unit_name, created_by, created_at, updated_at) VALUES
			(1, 'nim-proxy', '/opt/nim', 'app.py', 'preset_fastapi', '{"module":"app","app":"app"}', 8100, 'dougong-app-nim-proxy.service', 'admin', ?, ?),
			(2, 'legacy-web', '/opt/web', 'server.js', 'container', '{"image":"nginx:latest"}', 8080, 'dougong-app-legacy-web.service', 'admin', ?, ?),
			(3, 'worker', '/opt/worker', 'main.py', 'custom', '{"command":".venv/bin/python main.py"}', 8090, 'dougong-app-worker.service', 'admin', ?, ?)`,
			upgradeTS, upgradeTS, upgradeTS, upgradeTS, upgradeTS, upgradeTS)
		// worker 后来被删除：自增序号停在 3
		exec(`DELETE FROM app WHERE id = 3`)
		exec(`INSERT INTO app_task (id, app_id, app_name, action, status, requested_by, created_at, updated_at) VALUES
			(1, 1, 'nim-proxy', 'create', 'succeeded', 'admin', ?, ?),
			(2, 2, 'legacy-web', 'create', 'succeeded', 'admin', ?, ?)`,
			upgradeTS, upgradeTS, upgradeTS, upgradeTS)
	})
	t.Cleanup(func() { _ = db.Close() })

	// 既有行逐字段保留，runtime 取缺省值
	got := dumpRows(t, ctx, db, `SELECT id, name, dir, entry, runtime, start_mode, start_spec, listen_port, unit_name, created_by, created_at, updated_at FROM app ORDER BY id`)
	want := [][]string{
		{"1", "nim-proxy", "/opt/nim", "app.py", "python", "preset_fastapi", `{"module":"app","app":"app"}`, "8100", "dougong-app-nim-proxy.service", "admin", upgradeTS, upgradeTS},
		{"2", "legacy-web", "/opt/web", "server.js", "python", "container", `{"image":"nginx:latest"}`, "8080", "dougong-app-legacy-web.service", "admin", upgradeTS, upgradeTS},
	}
	if len(got) != len(want) {
		t.Fatalf("升级后 app 行数 = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if strings.Join(got[i], "|") != strings.Join(want[i], "|") {
			t.Errorf("app 第 %d 行升级后变了:\n got  %v\n want %v", i, got[i], want[i])
		}
	}

	// 任务台账不受重建影响
	if n := queryCount(ctx, db, `SELECT COUNT(*) FROM app_task WHERE app_id IN (1, 2) AND status = 'succeeded'`); n != 2 {
		t.Fatalf("app_task 行数 = %d, want 2", n)
	}

	// 自增序号不回落：下一个 id 是 4，不会把已删除的 worker 的 id 发给新项目
	if id := insertApp(t, ctx, db, "web", 3000); id != 4 {
		t.Fatalf("升级后新建项目的 id = %d, want 4（重建把自增序号重置了）", id)
	}

	// 新约束：运行时取值域、启动方式取值域、unit_name 唯一
	for name, q := range map[string]string{
		"runtime 取值域":    `INSERT INTO app (name, dir, entry, runtime, start_mode, listen_port, unit_name, created_by, created_at, updated_at) VALUES ('bad1', '/x', 'a.py', 'java', 'custom', 9001, 'dougong-app-bad1.service', 'admin', ?, ?)`,
		"start_mode 取值域": `INSERT INTO app (name, dir, entry, runtime, start_mode, listen_port, unit_name, created_by, created_at, updated_at) VALUES ('bad2', '/x', 'a.py', 'python', 'bogus', 9002, 'dougong-app-bad2.service', 'admin', ?, ?)`,
		"unit_name 唯一索引": `INSERT INTO app (name, dir, entry, runtime, start_mode, listen_port, unit_name, created_by, created_at, updated_at) VALUES ('bad3', '/x', 'a.py', 'python', 'custom', 9003, 'dougong-app-nim-proxy.service', 'admin', ?, ?)`,
	} {
		if _, err := db.ExecContext(ctx, q, upgradeTS, upgradeTS); err == nil {
			t.Errorf("%s：应被拒绝的写入成功了", name)
		}
	}

	// 库完整性；runtime 列带 NOT NULL 与缺省值；版本表记齐
	if integrity := queryScalar(ctx, db, `PRAGMA integrity_check`); integrity != "ok" {
		t.Fatalf("integrity_check = %q", integrity)
	}
	if cols := appColumnDefs(ctx, db); !strings.Contains(cols, "runtime TEXT notnull=1 dflt=['python']") {
		t.Fatalf("runtime 列定义不符: %s", cols)
	}
	if versions := queryCount(ctx, db, `SELECT COUNT(*) FROM schema_migrations`); versions <= 0 {
		t.Fatalf("schema_migrations 行数 = %d", versions)
	}
}

// TestMigrate018KeepsAutoincrementHighWaterMark 表重建时自增序号的两端：
// 表里的项目全部删除过（序号停在更大的数上）、以及从未建过项目。
func TestMigrate018KeepsAutoincrementHighWaterMark(t *testing.T) {
	const fiveApps = `INSERT INTO app (id, name, dir, entry, start_mode, start_spec, listen_port, unit_name, created_by, created_at, updated_at) VALUES
		(1, 'a1', '/opt/a1', 'app.py', 'preset_fastapi', '{"module":"app","app":"app"}', 8101, 'dougong-app-a1.service', 'admin', ?, ?),
		(2, 'a2', '/opt/a2', 'app.py', 'preset_fastapi', '{"module":"app","app":"app"}', 8102, 'dougong-app-a2.service', 'admin', ?, ?),
		(3, 'a3', '/opt/a3', 'app.py', 'preset_fastapi', '{"module":"app","app":"app"}', 8103, 'dougong-app-a3.service', 'admin', ?, ?),
		(4, 'a4', '/opt/a4', 'app.py', 'preset_fastapi', '{"module":"app","app":"app"}', 8104, 'dougong-app-a4.service', 'admin', ?, ?),
		(5, 'a5', '/opt/a5', 'app.py', 'preset_fastapi', '{"module":"app","app":"app"}', 8105, 'dougong-app-a5.service', 'admin', ?, ?)`

	t.Run("项目全部删除后升级：新项目不沿用已删除的 id", func(t *testing.T) {
		db, ctx := upgradedDB(t, func(exec func(q string, args ...any)) {
			exec(fiveApps, upgradeTS, upgradeTS, upgradeTS, upgradeTS, upgradeTS, upgradeTS, upgradeTS, upgradeTS, upgradeTS, upgradeTS)
			exec(`DELETE FROM app`)
		})
		t.Cleanup(func() { _ = db.Close() })
		if n := queryCount(ctx, db, `SELECT COUNT(*) FROM app`); n != 0 {
			t.Fatalf("升级后 app 行数 = %d, want 0", n)
		}
		if id := insertApp(t, ctx, db, "new", 3001); id != 6 {
			t.Fatalf("升级后新建项目的 id = %d, want 6（已删除项目的 id 被重新发放）", id)
		}
	})

	t.Run("从未建过项目：新项目从 1 开始", func(t *testing.T) {
		db, ctx := upgradedDB(t, nil)
		t.Cleanup(func() { _ = db.Close() })
		if id := insertApp(t, ctx, db, "first", 3002); id != 1 {
			t.Fatalf("空库升级后新建项目的 id = %d, want 1", id)
		}
	})
}

// appColumnDefs 返回 app 表各列的「名 类型 是否非空 缺省值」描述（按列序），供断言。
func appColumnDefs(ctx context.Context, db *sql.DB) string {
	rows, err := db.QueryContext(ctx, `SELECT name, type, "notnull", COALESCE(dflt_value, '') FROM pragma_table_info('app') ORDER BY cid`)
	if err != nil {
		return "query failed: " + err.Error()
	}
	defer rows.Close()
	var parts []string
	for rows.Next() {
		var name, typ string
		var notNull int
		var dflt string
		if err := rows.Scan(&name, &typ, &notNull, &dflt); err != nil {
			return "scan failed: " + err.Error()
		}
		parts = append(parts, fmt.Sprintf("%s %s notnull=%d dflt=[%s]", name, typ, notNull, dflt))
	}
	return strings.Join(parts, "; ")
}

// dumpRows 把查询结果取成字符串矩阵（NULL 视为测试失败）。
func dumpRows(t *testing.T, ctx context.Context, db *sql.DB, q string) [][]string {
	t.Helper()
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		t.Fatalf("query failed: %v\n%s", err, q)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("columns: %v", err)
	}
	var out [][]string
	for rows.Next() {
		vals := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatalf("scan: %v", err)
		}
		row := make([]string, len(cols))
		for i, v := range vals {
			if !v.Valid {
				t.Fatalf("列 %s 为 NULL", cols[i])
			}
			row[i] = v.String
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

func queryCount(ctx context.Context, db *sql.DB, q string) int {
	var n int
	if err := db.QueryRowContext(ctx, q).Scan(&n); err != nil {
		return -1
	}
	return n
}

func queryScalar(ctx context.Context, db *sql.DB, q string) string {
	var s string
	if err := db.QueryRowContext(ctx, q).Scan(&s); err != nil {
		return "query failed: " + err.Error()
	}
	return s
}
