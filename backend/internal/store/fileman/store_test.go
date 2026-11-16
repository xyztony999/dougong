package fileman

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
)

// auditEntryForTest 构造合法审计四要素（audit_log 表随 001 迁移就位）。
func auditEntryForTest(action string) storeauth.AuditEntry {
	return storeauth.AuditEntry{
		ActorName: "admin", Category: "change", Action: action,
		ObjectType: "file_entry", ObjectID: "/srv/x", Result: "success", Source: "web",
	}
}

// newTestDB 打开 :memory: 并应用全部迁移（真实 schema，不 mock）。
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

func TestScopeEnsureRowDefaultsAndIdempotent(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	s := New(db)
	now := time.Now()
	if err := s.Scope().EnsureRow(ctx, now); err != nil {
		t.Fatalf("ensure row: %v", err)
	}
	if err := s.Scope().EnsureRow(ctx, now.Add(time.Minute)); err != nil {
		t.Fatalf("ensure row again: %v", err) // SQL #1 幂等
	}
	got, err := s.Scope().Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.RootPath != "/" {
		t.Fatalf("default root = %q, want /", got.RootPath)
	}
	wantEx := []string{"/proc", "/sys", "/dev"}
	if len(got.ExcludedDirs) != len(wantEx) {
		t.Fatalf("default excluded = %v, want %v", got.ExcludedDirs, wantEx)
	}
	for i := range wantEx {
		if got.ExcludedDirs[i] != wantEx[i] {
			t.Fatalf("default excluded = %v, want %v", got.ExcludedDirs, wantEx)
		}
	}
}

func TestScopeUpdateRoundtrip(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	s := New(db)
	if err := s.Scope().EnsureRow(ctx, time.Now()); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if err := s.Scope().Update(ctx, "/srv", []string{"/srv/exclude", "/tmp"}, time.Now()); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := s.Scope().Get(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.RootPath != "/srv" || len(got.ExcludedDirs) != 2 ||
		got.ExcludedDirs[0] != "/srv/exclude" || got.ExcludedDirs[1] != "/tmp" {
		t.Fatalf("roundtrip = %+v", got)
	}
}

func TestTrashLifecycle(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	s := New(db)
	if err := s.Scope().EnsureRow(ctx, time.Now()); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	base := time.Now().UTC().Add(-time.Hour)
	mk := func(name, path string, at time.Time) *TrashEntry {
		uid := int64(7)
		return &TrashEntry{
			OriginalPath: path, TrashName: name, EntryType: EntryTypeFile,
			Size: 10, OperatorID: &uid, OperatorName: "admin",
		}
	}
	e1 := mk("100-aaaa", "/srv/a.txt", base)
	e2 := mk("200-bbbb", "/srv/b.txt", base.Add(time.Minute))
	if err := s.Trash().Insert(ctx, e1, base); err != nil {
		t.Fatalf("insert e1: %v", err)
	}
	if err := s.Trash().Insert(ctx, e2, base.Add(time.Minute)); err != nil {
		t.Fatalf("insert e2: %v", err)
	}
	if e1.ID == 0 || e2.ID == 0 {
		t.Fatalf("insert should backfill ids (%d,%d)", e1.ID, e2.ID)
	}
	if e1.Status != TrashStatusInTrash {
		t.Fatalf("insert status = %q", e1.Status)
	}
	// GetByID。
	got, err := s.Trash().GetByID(ctx, e1.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.TrashName != "100-aaaa" || got.OperatorName != "admin" || got.OperatorID == nil || *got.OperatorID != 7 {
		t.Fatalf("get by id = %+v", got)
	}
	// 列表 deleted_at 倒序（D-03）。
	rows, err := s.Trash().ListInTrash(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 || rows[0].TrashName != "200-bbbb" {
		t.Fatalf("list order = %+v", rows)
	}
	// 还原：in_trash→restored 一次成功、二次 false（I-02 终态不可逆）。
	ok, err := s.Trash().UpdateRestored(ctx, e1.ID, time.Now())
	if err != nil || !ok {
		t.Fatalf("restore first: ok=%v err=%v", ok, err)
	}
	ok, err = s.Trash().UpdateRestored(ctx, e1.ID, time.Now())
	if err != nil {
		t.Fatalf("restore second: %v", err)
	}
	if ok {
		t.Fatal("restore second should not update (terminal state)")
	}
	// restored 行不再出现在 in_trash 列表。
	rows, err = s.Trash().ListInTrash(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("list after restore = %+v err=%v", rows, err)
	}
	// DeleteAllInTrash（SQL #7）。
	n, err := s.Trash().DeleteAllInTrash(ctx)
	if err != nil || n != 1 {
		t.Fatalf("delete all = %d err=%v", n, err)
	}
}

func TestTrashGetByIDNotFound(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	s := New(db)
	if err := s.Scope().EnsureRow(ctx, time.Now()); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	_, err := s.Trash().GetByID(ctx, 999)
	if err == nil || !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("err = %v, want wrapped sql.ErrNoRows", err)
	}
}

// TestDeleteTxSameTransactionAudit 验证入站登记与删除审计同事务（D-12/D-15）：
// tx 内 Insert + audit_log Append 同提交；Rollback 则两者俱无。
func TestDeleteTxSameTransactionAudit(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	s := New(db)
	if err := s.Scope().EnsureRow(ctx, time.Now()); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	now := time.Now().UTC()
	uid := int64(1)

	tx, err := s.BeginTx(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	entry := &TrashEntry{
		OriginalPath: "/srv/x", TrashName: "300-cccc", EntryType: EntryTypeDirectory,
		Size: 100, OperatorID: &uid, OperatorName: "admin",
	}
	if err := tx.Trash().Insert(ctx, entry, now); err != nil {
		t.Fatalf("tx insert: %v", err)
	}
	if err := tx.Audit().Append(ctx, auditEntryForTest("file_deleted"), now); err != nil {
		t.Fatalf("tx audit: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	var nAudit int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_log WHERE action = ?`, "file_deleted").Scan(&nAudit); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if nAudit != 1 {
		t.Fatalf("audit rows = %d, want 1", nAudit)
	}

	// Rollback 路径：两者俱无。
	tx2, err := s.BeginTx(ctx)
	if err != nil {
		t.Fatalf("begin2: %v", err)
	}
	e2 := &TrashEntry{OriginalPath: "/srv/y", TrashName: "400-dddd",
		EntryType: EntryTypeFile, OperatorName: "admin"}
	if err := tx2.Trash().Insert(ctx, e2, now); err != nil {
		t.Fatalf("tx2 insert: %v", err)
	}
	if err := tx2.Audit().Append(ctx, auditEntryForTest("file_moved"), now); err != nil {
		t.Fatalf("tx2 audit: %v", err)
	}
	if err := tx2.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	var nTrash int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM trash_entry WHERE trash_name = ?`, "400-dddd").Scan(&nTrash); err != nil {
		t.Fatalf("count trash: %v", err)
	}
	if nTrash != 0 {
		t.Fatal("rollback should remove pending insert")
	}
	var nMoved int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_log WHERE action = ?`, "file_moved").Scan(&nMoved); err != nil {
		t.Fatalf("count moved: %v", err)
	}
	if nMoved != 0 {
		t.Fatal("rollback should remove pending audit")
	}
}
