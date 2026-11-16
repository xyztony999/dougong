package panel

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// 审计四要素字面量（category/result/source 取值域由 audit_log DDL CHECK
// 兜底；常量定义在 service/audit，store 层测试不跨层引用）。
const (
	testCategoryChange = "change"
	testResultSuccess  = "success"
	testSourceWeb      = "web"

	// panel-core 审计动作/对象字面量（常量定义在 service/panel，store 层
	// 测试不跨层引用）。
	testActionConfigChanged = "panel_config_changed"
	testObjectTypeConfig    = "panel_config"
)

// newTestDB 以真实 :memory: SQLite 建库并应用全部迁移（不 mock 数据库，
// backend-03 §6）。
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if cerr := db.Close(); cerr != nil {
			t.Errorf("close db: %v", cerr)
		}
	})
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func strPtr(v string) *string { return &v }

func TestEnsureSingletonIdempotent(t *testing.T) {
	db := newTestDB(t)
	st := New(db)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := st.Config().EnsureSingleton(ctx, 23456, now); err != nil {
		t.Fatalf("ensure singleton: %v", err)
	}
	cfg, err := st.Config().Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if cfg.ID != 1 || cfg.ListenScope != ListenScopeAll || cfg.ListenPort != 23456 ||
		cfg.EntryPathEnabled || cfg.UpdateCheck || cfg.Lang != "zh-CN" ||
		cfg.HTTPSMode != HTTPSModeOff || cfg.ListenAddress != nil || cfg.EntryPath != nil {
		t.Fatalf("singleton defaults mismatch: %+v", cfg)
	}

	// 幂等：再次 Ensure（不同端口）不改既有行（I-P01）。
	if err := st.Config().EnsureSingleton(ctx, 99999, now.Add(time.Minute)); err != nil {
		t.Fatalf("ensure singleton again: %v", err)
	}
	cfg2, err := st.Config().Get(ctx)
	if err != nil {
		t.Fatalf("get again: %v", err)
	}
	if cfg2.ListenPort != 23456 {
		t.Fatalf("ensure singleton mutated existing row: port=%d", cfg2.ListenPort)
	}
}

func TestConfigUpdatePartial(t *testing.T) {
	db := newTestDB(t)
	st := New(db)
	ctx := context.Background()
	now := time.Now().UTC()
	if err := st.Config().EnsureSingleton(ctx, 20000, now); err != nil {
		t.Fatalf("ensure: %v", err)
	}

	// 部分更新：仅 scope/address/port（其余列不动）。
	port := 20500
	if err := st.Config().Update(ctx, ConfigChanges{
		ListenScope:   strPtr(ListenScopeCustom),
		ListenAddress: strPtr("10.0.0.5"),
		ListenPort:    &port,
	}, now.Add(time.Second)); err != nil {
		t.Fatalf("update: %v", err)
	}
	cfg, err := st.Config().Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if cfg.ListenScope != ListenScopeCustom || cfg.ListenAddress == nil || *cfg.ListenAddress != "10.0.0.5" || cfg.ListenPort != 20500 {
		t.Fatalf("partial update mismatch: %+v", cfg)
	}
	if cfg.Lang != "zh-CN" || cfg.HTTPSMode != HTTPSModeOff || cfg.UpdateCheck {
		t.Fatalf("untouched columns mutated: %+v", cfg)
	}

	// 空串 listen_address = 显式清空（NULL）。
	if err := st.Config().Update(ctx, ConfigChanges{ListenAddress: strPtr("")}, now.Add(2*time.Second)); err != nil {
		t.Fatalf("clear address: %v", err)
	}
	cfg, err = st.Config().Get(ctx)
	if err != nil {
		t.Fatalf("get after clear: %v", err)
	}
	if cfg.ListenAddress != nil {
		t.Fatalf("listen_address not cleared: %v", *cfg.ListenAddress)
	}

	// 空变更集为 no-op。
	before, _ := st.Config().Get(ctx)
	if err := st.Config().Update(ctx, ConfigChanges{}, now.Add(3*time.Second)); err != nil {
		t.Fatalf("noop update: %v", err)
	}
	after, _ := st.Config().Get(ctx)
	if !before.UpdatedAt.Equal(after.UpdatedAt) {
		t.Fatal("empty changes must not touch updated_at")
	}
}

func TestCertRepoUpsertGetDelete(t *testing.T) {
	db := newTestDB(t)
	st := New(db)
	ctx := context.Background()
	// DATETIME 列以 RFC3339 秒精度存储，测试基线截断到秒保证往返相等。
	now := time.Now().UTC().Truncate(time.Second)

	// 无行 → ErrNoRows（https_mode=off 且从未导入时常态）。
	if _, err := st.Cert().Get(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("get empty want ErrNoRows, got %v", err)
	}

	issuer, notBefore, notAfter := "Test CA", now.Add(-time.Hour), now.Add(24*time.Hour)
	if err := st.Cert().Upsert(ctx, Cert{
		CertPEM: "PEM-A", KeyPEM: "KEY-A", Source: CertSourceImported,
		Issuer: &issuer, NotBefore: &notBefore, NotAfter: &notAfter,
	}, now); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := st.Cert().Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.CertPEM != "PEM-A" || got.KeyPEM != "KEY-A" || got.Source != CertSourceImported ||
		got.Issuer == nil || *got.Issuer != issuer || got.NotAfter == nil || !got.NotAfter.Equal(notAfter) {
		t.Fatalf("cert roundtrip mismatch: %+v", got)
	}

	// 替换式更新（D-P04：自签/导入共用一行）。
	if err := st.Cert().Upsert(ctx, Cert{
		CertPEM: "PEM-B", KeyPEM: "KEY-B", Source: CertSourceSelfSigned,
	}, now.Add(time.Second)); err != nil {
		t.Fatalf("upsert replace: %v", err)
	}
	got, _ = st.Cert().Get(ctx)
	if got.CertPEM != "PEM-B" || got.Source != CertSourceSelfSigned || got.Issuer != nil {
		t.Fatalf("replace mismatch: %+v", got)
	}

	if err := st.Cert().Delete(ctx); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := st.Cert().Get(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("get after delete want ErrNoRows, got %v", err)
	}
}

// TestTxAuditSameTx 验证 panel 事务视图内配置更新与审计写入原子落库
// （I-P07 + I-05 同事务语义）。
func TestTxAuditSameTx(t *testing.T) {
	db := newTestDB(t)
	st := New(db)
	ctx := context.Background()
	now := time.Now().UTC()
	if err := st.Config().EnsureSingleton(ctx, 21000, now); err != nil {
		t.Fatalf("ensure: %v", err)
	}

	// 回滚路径：配置更新 + 审计写入后回滚 → 双双不落。
	tx, err := st.BeginTx(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := tx.Config().Update(ctx, ConfigChanges{Lang: strPtr("zh-CN-x")}, now); err != nil {
		t.Fatalf("tx update: %v", err)
	}
	if err := tx.Audit().Append(ctx, storeauth.AuditEntry{
		ActorName: "admin", Category: testCategoryChange,
		Action: testActionConfigChanged, ObjectType: testObjectTypeConfig,
		Result: testResultSuccess, Source: testSourceWeb,
	}, now); err != nil {
		t.Fatalf("tx audit: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_log`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("audit leaked after rollback: n=%d err=%v", n, err)
	}
	cfg, _ := st.Config().Get(ctx)
	if cfg.Lang != "zh-CN" {
		t.Fatalf("config mutated after rollback: %s", cfg.Lang)
	}

	// 提交路径：配置更新 + 审计同事务提交。
	tx, err = st.BeginTx(ctx)
	if err != nil {
		t.Fatalf("begin tx 2: %v", err)
	}
	if err := tx.Config().Update(ctx, ConfigChanges{UpdateCheck: boolPtr(true)}, now.Add(time.Second)); err != nil {
		t.Fatalf("tx update 2: %v", err)
	}
	if err := tx.Audit().Append(ctx, storeauth.AuditEntry{
		ActorName: "admin", Category: testCategoryChange,
		Action: testActionConfigChanged, ObjectType: testObjectTypeConfig,
		Detail: map[string]any{"changes": []map[string]any{{"field": "updateCheck", "before": false, "after": true}}},
		Result: testResultSuccess, Source: testSourceWeb,
	}, now.Add(time.Second)); err != nil {
		t.Fatalf("tx audit 2: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = ?`, testActionConfigChanged).Scan(&n); err != nil || n != 1 {
		t.Fatalf("audit after commit: n=%d err=%v", n, err)
	}
	cfg, _ = st.Config().Get(ctx)
	if !cfg.UpdateCheck {
		t.Fatal("config not committed")
	}
	// detail 序列化为 JSON 且包含字段级 before/after。
	var detail sql.NullString
	if err := db.QueryRow(`SELECT detail FROM audit_log WHERE action = ?`, testActionConfigChanged).Scan(&detail); err != nil {
		t.Fatalf("read detail: %v", err)
	}
	if !strings.Contains(detail.String, `"updateCheck"`) || !strings.Contains(detail.String, `"after":true`) {
		t.Fatalf("detail payload mismatch: %s", detail.String)
	}
}

func boolPtr(b bool) *bool { return &b }
