package fileman_test

import (
	"archive/tar"
	"archive/zip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/adapter/fslocal"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	. "github.com/dougong/dougong/internal/service/fileman" // 外部测试包 dot-import：用例主体沿用包内符号名
	"github.com/dougong/dougong/internal/store"
	storefileman "github.com/dougong/dougong/internal/store/fileman"
)

// fakeAudit 是审计 OHS 内存替身（断言调用次数与内容；同事务路径走真库，
// 由 store 测试单独覆盖）。
type fakeAudit struct {
	entries []audit.Entry
	fail    bool
}

func (f *fakeAudit) Record(_ context.Context, e audit.Entry) error {
	if f.fail {
		return fmt.Errorf("fake audit down")
	}
	f.entries = append(f.entries, e)
	return nil
}

func (f *fakeAudit) byAction(action string) []audit.Entry {
	var out []audit.Entry
	for _, e := range f.entries {
		if e.Action == action {
			out = append(out, e)
		}
	}
	return out
}

// testEnv 聚合 service 层测试依赖（真实 fs + :memory: SQLite，不 mock 两者）。
type testEnv struct {
	svc     *Service
	db      *sql.DB
	root    string
	outside string // 受管根外目录（越界用例）
	rec     *fakeAudit
	guard   *PathScopeGuard
}

var testActor = &authsvc.Identity{UserID: 1, Username: "admin", Role: "admin"}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	st := storefileman.New(db)
	if err := st.Scope().EnsureRow(ctx, time.Now()); err != nil {
		t.Fatalf("ensure scope: %v", err)
	}
	root := t.TempDir()
	excluded := filepath.Join(root, "excluded")
	if err := os.MkdirAll(excluded, 0o755); err != nil {
		t.Fatalf("mkdir excluded: %v", err)
	}
	outside := t.TempDir()
	guard := NewGuard(ScopeConfig{RootPath: root, ExcludedDirs: []string{excluded}})
	rec := &fakeAudit{}
	svc := New(fslocal.New(), st, rec, guard)
	return &testEnv{svc: svc, db: db, root: root, outside: outside, rec: rec, guard: guard}
}

// wantFileErr 断言错误码符号。
func wantFileErr(t *testing.T, err error, symbol string) {
	t.Helper()
	if err == nil {
		t.Fatalf("err = nil, want %s", symbol)
	}
	var ec *errcode.Error
	if !errors.As(err, &ec) {
		t.Fatalf("err = %v (%T), want errcode %s", err, err, symbol)
	}
	if ec.Symbol() != symbol {
		t.Fatalf("err symbol = %s (%s), want %s", ec.Symbol(), ec.Message(), symbol)
	}
}

func writeFileT(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// ---- PathScopeGuard（R-04 两步校验；路径逃逸为 M2 验收硬项） ----

func TestGuardScopeRejects(t *testing.T) {
	env := newTestEnv(t)
	g := env.guard

	// 相对路径 → FILE_001。
	_, err := g.Resolve("relative/path")
	wantFileErr(t, err, "FILE_001")

	// 不在受管根内 → FILE_002 reason=not_under_root。
	_, err = g.Resolve(filepath.Join(env.outside, "a.txt"))
	wantFileErr(t, err, "FILE_002")
	var ec *errcode.Error
	if !errors.As(err, &ec) {
		t.Fatal("unreachable")
	}
	if rd, ok := ec.Data().(ReasonData); !ok || rd.Reason != ReasonNotUnderRoot {
		t.Fatalf("reason = %+v, want not_under_root", ec.Data())
	}

	// 排除目录 → FILE_002 reason=excluded。
	_, err = g.Resolve(filepath.Join(env.root, "excluded", "a.txt"))
	wantFileErr(t, err, "FILE_002")
	errors.As(err, &ec)
	if rd, ok := ec.Data().(ReasonData); !ok || rd.Reason != ReasonExcluded {
		t.Fatalf("reason = %+v, want excluded", ec.Data())
	}

	// 不存在 → fs.ErrNotExist（调用方映射 FILE_003/FILE_012）。
	if _, err := g.Resolve(filepath.Join(env.root, "nope")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want fs.ErrNotExist", err)
	}

	// ResolveParent：目标不存在但父目录合法。
	if err := os.MkdirAll(filepath.Join(env.root, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	dir, name, err := g.ResolveParent(filepath.Join(env.root, "sub", "new.txt"))
	if err != nil {
		t.Fatalf("resolve parent: %v", err)
	}
	if name != "new.txt" || !strings.HasPrefix(dir, env.root) {
		t.Fatalf("resolve parent = %q,%q", dir, name)
	}

	// 根本身放行。
	if real, err := g.Resolve(env.root); err != nil || real != filepath.Clean(env.root) {
		t.Fatalf("root resolve = %q err=%v", real, err)
	}
}

// TestGuardSymlinkEscape 路径逃逸硬项（D0 §2）：受管根内符号链接指向根外
// → Resolve 解析真实路径后 FILE_002（Windows 无符号链接权限则跳过）。
func TestGuardSymlinkEscape(t *testing.T) {
	env := newTestEnv(t)
	link := filepath.Join(env.root, "escape")
	if err := os.Symlink(env.outside, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink not permitted on this windows host: %v", err)
		}
		t.Fatalf("symlink: %v", err)
	}
	_, err := env.guard.Resolve(link)
	wantFileErr(t, err, "FILE_002")
}

func TestGuardRefreshTakesEffectImmediately(t *testing.T) { // D-02：提交后刷新即予生效
	env := newTestEnv(t)
	sub := filepath.Join(env.root, "later")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := env.guard.Resolve(sub); err != nil {
		t.Fatalf("before refresh: %v", err)
	}
	env.guard.Refresh(ScopeConfig{RootPath: env.root, ExcludedDirs: []string{sub}})
	_, err := env.guard.Resolve(sub)
	wantFileErr(t, err, "FILE_002")
}

// ---- 投影与排序（D-03/I-06/I-10） ----

func TestListDirProjectionAndSort(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	writeFileT(t, filepath.Join(env.root, "b.txt"), "bb")
	writeFileT(t, filepath.Join(env.root, "a.txt"), "aaaa")
	if err := os.MkdirAll(filepath.Join(env.root, ".hidden"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := env.svc.ListDir(ctx, env.root, "", "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// I-10 隐藏条目全量：.hidden a.txt b.txt excluded 共 4。
	if data.Total != 4 {
		t.Fatalf("total = %d, want 4 (items=%+v)", data.Total, data.Items)
	}
	if data.GeneratedAt == "" {
		t.Fatal("generatedAt required")
	}
	// 默认 name asc：'.'(46) < 'a' < 'b' < 'e'。
	if data.Items[0].Name != ".hidden" || data.Items[1].Name != "a.txt" ||
		data.Items[3].Name != "excluded" {
		t.Fatalf("default name asc order = %+v", namesOf(data.Items))
	}
	if data.Items[1].EntryType != TypeFile || data.Items[3].EntryType != TypeDirectory {
		t.Fatalf("entry types = %+v", data.Items)
	}
	if runtime.GOOS != "windows" { // 权限位断言仅 unix（Windows stat 恒 0666/0777）
		if data.Items[1].Mode != "0644" {
			t.Fatalf("a.txt mode = %q, want 0644", data.Items[1].Mode)
		}
	}
	if data.Items[1].Path != filepath.Join(env.root, "a.txt") {
		t.Fatalf("item path = %q", data.Items[1].Path)
	}
	// size 排序：a.txt(4) 最大 → desc 首位（目录 size=0 不参与大值）。
	bySize, err := env.svc.ListDir(ctx, env.root, "size", "desc")
	if err != nil {
		t.Fatalf("list size: %v", err)
	}
	if bySize.Items[0].Name != "a.txt" {
		t.Fatalf("size desc first = %s", bySize.Items[0].Name)
	}
	// 非法排序参数 → FILE_001。
	_, err = env.svc.ListDir(ctx, env.root, "bogus", "")
	wantFileErr(t, err, "FILE_001")
	_, err = env.svc.ListDir(ctx, env.root, "name", "bogus")
	wantFileErr(t, err, "FILE_001")
	// 目录不存在/目标为文件 → FILE_012（A-F16）。
	_, err = env.svc.ListDir(ctx, filepath.Join(env.root, "none"), "", "")
	wantFileErr(t, err, "FILE_012")
	_, err = env.svc.ListDir(ctx, filepath.Join(env.root, "a.txt"), "", "")
	wantFileErr(t, err, "FILE_012")
}

func namesOf(items []EntryItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Name)
	}
	return out
}

// TestListDirDefaultPathIsRoot Gate 2 E1 回归：path 缺省=受管根（A-F20），
// 与显式受管根路径的投影完全一致。
func TestListDirDefaultPathIsRoot(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	writeFileT(t, filepath.Join(env.root, "seed.txt"), "x")

	implicit, err := env.svc.ListDir(ctx, "", "", "")
	if err != nil {
		t.Fatalf("implicit path list: %v", err)
	}
	explicit, err := env.svc.ListDir(ctx, env.root, "", "")
	if err != nil {
		t.Fatalf("explicit path list: %v", err)
	}
	if implicit.Path != explicit.Path || implicit.Total != explicit.Total ||
		implicit.GeneratedAt == "" {
		t.Fatalf("default path mismatch: implicit=%+v explicit=%+v", implicit.Path, explicit.Path)
	}
	// path-scope/update 收紧后，缺省跟随新根（D-02 副本即时生效联动 A-F20）。
	sub := filepath.Join(env.root, "fresh")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if _, err := env.svc.UpdateScope(ctx, testActor, sub, nil); err != nil {
		t.Fatalf("update scope: %v", err)
	}
	after, err := env.svc.ListDir(ctx, "", "", "")
	if err != nil {
		t.Fatalf("implicit after scope update: %v", err)
	}
	if after.Path != sub || after.Total != 0 {
		t.Fatalf("default should follow refreshed root, got %+v (%d)", after.Path, after.Total)
	}
}

// ---- 整理动作（create/rename/move） ----

func TestCreateRenameMove(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// create：非法名 FILE_007；非法 type FILE_001（A-F9）；重名 FILE_004。
	_, err := env.svc.Create(ctx, testActor, env.root, "a/b", TypeFile)
	wantFileErr(t, err, "FILE_007")
	_, err = env.svc.Create(ctx, testActor, env.root, "x", "socket")
	wantFileErr(t, err, "FILE_001")
	d, err := env.svc.Create(ctx, testActor, env.root, "newdir", TypeDirectory)
	if err != nil || d.EntryType != TypeDirectory {
		t.Fatalf("create dir: %v %+v", err, d)
	}
	if fi, err := os.Stat(filepath.Join(env.root, "newdir")); err != nil || !fi.IsDir() {
		t.Fatalf("created dir missing: %v", err)
	}
	_, err = env.svc.Create(ctx, testActor, env.root, "newdir", TypeDirectory)
	wantFileErr(t, err, "FILE_004")

	// rename：D-13 同名直接错误；详情 detail={oldPath}。
	writeFileT(t, filepath.Join(env.root, "one.txt"), "1")
	writeFileT(t, filepath.Join(env.root, "two.txt"), "2")
	rd, err := env.svc.Rename(ctx, testActor, filepath.Join(env.root, "one.txt"), "uno.txt")
	if err != nil || !strings.HasSuffix(rd.Path, "uno.txt") {
		t.Fatalf("rename: %v %+v", err, rd)
	}
	_, err = env.svc.Rename(ctx, testActor, filepath.Join(env.root, "uno.txt"), "two.txt")
	wantFileErr(t, err, "FILE_004")

	// move：目标目录不存在 FILE_012；成功移动；同位置=无操作成功。
	_, err = env.svc.Move(ctx, testActor, filepath.Join(env.root, "uno.txt"),
		filepath.Join(env.root, "nodir"), "")
	wantFileErr(t, err, "FILE_012")
	mv, err := env.svc.Move(ctx, testActor, filepath.Join(env.root, "uno.txt"),
		filepath.Join(env.root, "newdir"), "")
	if err != nil || len(mv.Moved) != 1 || !strings.HasSuffix(mv.Moved[0].To, "uno.txt") {
		t.Fatalf("move: %v %+v", err, mv)
	}
	mv, err = env.svc.Move(ctx, testActor, filepath.Join(env.root, "newdir", "uno.txt"),
		filepath.Join(env.root, "newdir"), "")
	if err != nil || len(mv.Moved) != 0 { // 同位置移动=无操作
		t.Fatalf("same-position move: %v %+v", err, mv)
	}
	// skip：目标被占时跳过。
	writeFileT(t, filepath.Join(env.root, "uno.txt"), "occupied")
	mv, err = env.svc.Move(ctx, testActor, filepath.Join(env.root, "newdir", "uno.txt"),
		env.root, ConflictSkip)
	if err != nil || mv.SkippedCount != 1 || len(mv.Moved) != 0 {
		t.Fatalf("move skip: %v %+v", err, mv)
	}
	if b, _ := os.ReadFile(filepath.Join(env.root, "uno.txt")); string(b) != "occupied" {
		t.Fatalf("skip must not overwrite, got %q", b)
	}
	// 审计恰好一条原则（A-F10：失败尝试也留痕，此处只数 success）：created 1 /
	// renamed 1 / moved 2（1 成功 + 1 skip；同位置无审计）。
	if n := countSuccess(env.rec.byAction(ActionFileCreated)); n != 1 {
		t.Fatalf("file_created success audit = %d", n)
	}
	if n := countSuccess(env.rec.byAction(ActionFileRenamed)); n != 1 {
		t.Fatalf("file_renamed success audit = %d", n)
	}
	if n := countSuccess(env.rec.byAction(ActionFileMoved)); n != 2 {
		t.Fatalf("file_moved success audit = %d", n)
	}
}

func countSuccess(entries []audit.Entry) int {
	n := 0
	for _, e := range entries {
		if e.Result == audit.ResultSuccess {
			n++
		}
	}
	return n
}

func TestMoveOverwriteAndAsk(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	src := filepath.Join(env.root, "sub")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFileT(t, filepath.Join(env.root, "f.txt"), "src")
	writeFileT(t, filepath.Join(src, "f.txt"), "dst")
	// ask → FILE_005 且 data.conflicts=[{path}] 零动作。
	_, err := env.svc.Move(ctx, testActor, filepath.Join(env.root, "f.txt"), src, "")
	wantFileErr(t, err, "FILE_005")
	var ec *errcode.Error
	if !errors.As(err, &ec) {
		t.Fatal("unreachable")
	}
	if cd, ok := ec.Data().(ConflictData); !ok || len(cd.Conflicts) != 1 ||
		!strings.HasSuffix(cd.Conflicts[0].Path, "f.txt") {
		t.Fatalf("conflicts = %+v", ec.Data())
	}
	if b, _ := os.ReadFile(filepath.Join(src, "f.txt")); string(b) != "dst" {
		t.Fatalf("ask must not touch disk, got %q", b)
	}
	// overwrite → 覆盖。
	if _, err := env.svc.Move(ctx, testActor, filepath.Join(env.root, "f.txt"), src, ConflictOverwrite); err != nil {
		t.Fatalf("move overwrite: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(src, "f.txt")); string(b) != "src" {
		t.Fatalf("overwrite content = %q", b)
	}
	// 越界拒绝零审计（D-01/A-F10）。
	before := len(env.rec.entries)
	if _, err := env.svc.Move(ctx, testActor, filepath.Join(env.outside, "x"), src, ""); err == nil {
		t.Fatal("outside rejected")
	}
	if len(env.rec.entries) != before {
		t.Fatal("scope rejection must not audit")
	}
}

// ---- 在线编辑（D-05） ----

func TestContentEditFlow(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	p := filepath.Join(env.root, "note.md")
	writeFileT(t, p, "# 斗拱\nhello")
	c, err := env.svc.Content(ctx, p)
	if err != nil || c.Content != "# 斗拱\nhello" || c.Size != int64(len("# 斗拱\nhello")) {
		t.Fatalf("content: %v %+v", err, c)
	}
	// 保存全量覆盖（最后写者胜）。
	sv, err := env.svc.ContentSave(ctx, testActor, p, "# 斗拱\nworld")
	if err != nil || sv.Size != int64(len("# 斗拱\nworld")) {
		t.Fatalf("save: %v %+v", err, sv)
	}
	if b, _ := os.ReadFile(p); string(b) != "# 斗拱\nworld" {
		t.Fatalf("saved = %q", b)
	}
	// 二进制（空字节）→ FILE_006。
	bin := filepath.Join(env.root, "bin.dat")
	if err := os.WriteFile(bin, []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatalf("write bin: %v", err)
	}
	_, err = env.svc.Content(ctx, bin)
	wantFileErr(t, err, "FILE_006")
	// 无效 UTF-8 → FILE_006。
	bad := filepath.Join(env.root, "bad.txt")
	if err := os.WriteFile(bad, []byte{0xff, 0xfe, 0x41}, 0o644); err != nil {
		t.Fatalf("write bad: %v", err)
	}
	_, err = env.svc.Content(ctx, bad)
	wantFileErr(t, err, "FILE_006")
	// 超限 → FILE_011（读取与保存两侧）。
	big := filepath.Join(env.root, "big.txt")
	if err := os.WriteFile(big, make([]byte, EditSizeLimit+1), 0o644); err != nil {
		t.Fatalf("write big: %v", err)
	}
	_, err = env.svc.Content(ctx, big)
	wantFileErr(t, err, "FILE_011")
	_, err = env.svc.ContentSave(ctx, testActor, p, strings.Repeat("x", EditSizeLimit+1))
	wantFileErr(t, err, "FILE_011")
	// 目录 → FILE_012。
	_, err = env.svc.Content(ctx, env.root)
	wantFileErr(t, err, "FILE_012")
}

// ---- 回收站（D-15/I-01/I-02） ----

func TestTrashDeleteRestorePurge(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	p := filepath.Join(env.root, "doc.txt")
	writeFileT(t, p, "content")

	dd, err := env.svc.Delete(ctx, testActor, p)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if dd.TrashID == 0 || dd.TrashName == "" {
		t.Fatalf("delete data = %+v", dd)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("original should be gone")
	}
	trashDir := filepath.Join(env.root, TrashDirName)
	if _, err := os.Stat(filepath.Join(trashDir, dd.TrashName)); err != nil {
		t.Fatalf("trash content missing: %v", err)
	}
	// 入站登记与删除审计同事务（D-12/D-15）：audit_log 一行 file_deleted success。
	var n int
	if err := env.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_log WHERE action = ? AND result = 'success'`, ActionFileDeleted).
		Scan(&n); err != nil || n != 1 {
		t.Fatalf("same-tx audit rows = %d err=%v", n, err)
	}

	// list：一条在站，operator_name 快照（D-12）。
	list, err := env.svc.TrashList(ctx)
	if err != nil || list.Total != 1 || list.Items[0].OperatorName != "admin" {
		t.Fatalf("trash list: %v %+v", err, list)
	}
	if list.Warning != "" {
		t.Fatalf("no orphan expected, warning = %q", list.Warning)
	}

	// 还原到被占原路径：ask → FILE_005；overwrite → 覆盖还原。
	writeFileT(t, p, "newer")
	_, err = env.svc.TrashRestore(ctx, testActor, dd.TrashID, "")
	wantFileErr(t, err, "FILE_005")
	rs, err := env.svc.TrashRestore(ctx, testActor, dd.TrashID, ConflictOverwrite)
	if err != nil || rs.Status != "restored" || rs.ID != dd.TrashID {
		t.Fatalf("restore: %v %+v", err, rs)
	}
	if b, _ := os.ReadFile(p); string(b) != "content" {
		t.Fatalf("restored content = %q", b)
	}
	// 终态不可逆：再还原 → FILE_014（I-02）。
	_, err = env.svc.TrashRestore(ctx, testActor, dd.TrashID, "")
	wantFileErr(t, err, "FILE_014")

	// purge：空站幂等（PRD N3）→ 有站 purge 物理删除。
	pd, err := env.svc.TrashPurge(ctx, testActor)
	if err != nil || pd.PurgedCount != 0 {
		t.Fatalf("empty purge: %v %+v", err, pd)
	}
	if _, err := env.svc.Delete(ctx, testActor, p); err != nil {
		t.Fatalf("delete2: %v", err)
	}
	pd, err = env.svc.TrashPurge(ctx, testActor)
	if err != nil || pd.PurgedCount != 1 {
		t.Fatalf("purge: %v %+v", err, pd)
	}
	if entries, _ := os.ReadDir(trashDir); len(entries) != 0 {
		t.Fatalf("trash dir should be empty, got %d", len(entries))
	}
	var rows int
	if err := env.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM trash_entry`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("only restored row should remain, got %d err=%v", rows, err)
	}
}

func TestTrashListReconcilesOrphans(t *testing.T) { // PRD N2：面板外干预对账
	env := newTestEnv(t)
	ctx := context.Background()
	p := filepath.Join(env.root, "gone.txt")
	writeFileT(t, p, "x")
	dd, err := env.svc.Delete(ctx, testActor, p)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(env.root, TrashDirName, dd.TrashName)); err != nil {
		t.Fatalf("remove: %v", err)
	}
	list, err := env.svc.TrashList(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if list.Total != 0 || list.Warning == "" {
		t.Fatalf("orphan reconcile: %+v", list)
	}
}

// ---- 上传与分块（D-04/D-10） ----

func TestUploadDirectFlow(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	up, err := env.svc.Upload(ctx, testActor, env.root, []UploadPayload{
		{Name: "u1.txt", Size: 3, Data: []byte("aaa")},
		{Name: "u2.txt", Size: 3, Data: []byte("bbb")},
	}, "")
	if err != nil || up.UploadedCount != 2 || len(up.Results) != 2 {
		t.Fatalf("upload: %v %+v", err, up)
	}
	// ask 冲突 → FILE_005 零落盘（内容不变）。
	_, err = env.svc.Upload(ctx, testActor, env.root, []UploadPayload{
		{Name: "u1.txt", Size: 3, Data: []byte("zzz")},
	}, "")
	wantFileErr(t, err, "FILE_005")
	if b, _ := os.ReadFile(filepath.Join(env.root, "u1.txt")); string(b) != "aaa" {
		t.Fatalf("ask must not write, got %q", b)
	}
	// skip → skipped；overwrite → 覆盖。
	up, err = env.svc.Upload(ctx, testActor, env.root, []UploadPayload{
		{Name: "u1.txt", Size: 3, Data: []byte("ccc")},
	}, ConflictSkip)
	if err != nil || up.UploadedCount != 0 || up.Results[0].Status != "skipped" {
		t.Fatalf("upload skip: %v %+v", err, up)
	}
	up, err = env.svc.Upload(ctx, testActor, env.root, []UploadPayload{
		{Name: "u1.txt", Size: 3, Data: []byte("ddd")},
	}, ConflictOverwrite)
	if err != nil || up.UploadedCount != 1 {
		t.Fatalf("upload overwrite: %v %+v", err, up)
	}
	if b, _ := os.ReadFile(filepath.Join(env.root, "u1.txt")); string(b) != "ddd" {
		t.Fatalf("overwrite = %q", b)
	}
	// 超限 → FILE_011（A-F8 整体拒绝零落盘）。
	_, err = env.svc.Upload(ctx, testActor, env.root, []UploadPayload{
		{Name: "big.bin", Size: DirectUploadLimit + 1, Data: make([]byte, DirectUploadLimit+1)},
	}, "")
	wantFileErr(t, err, "FILE_011")
	if _, serr := os.Stat(filepath.Join(env.root, "big.bin")); !os.IsNotExist(serr) {
		t.Fatal("oversize must not land")
	}
}

func TestChunkedUploadFlow(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	total := int64(UploadChunkSize)*2 + 10 // 2 满块 + 10 字节 → ceil=3 块（D-04）
	// totalChunks 不匹配 → FILE_010。
	_, err := env.svc.ChunkInit(ctx, env.root, "movie.bin", total, 2)
	wantFileErr(t, err, "FILE_010")
	// 同名预检 ask → FILE_005 零动作。
	writeFileT(t, filepath.Join(env.root, "exists.bin"), "x")
	_, err = env.svc.ChunkInit(ctx, env.root, "exists.bin", int64(UploadChunkSize), 1)
	wantFileErr(t, err, "FILE_005")

	init, err := env.svc.ChunkInit(ctx, env.root, "movie.bin", total, 3)
	if err != nil || init.UploadID == "" {
		t.Fatalf("chunk init: %v %+v", err, init)
	}
	part1 := bytesRepeat('a', UploadChunkSize)
	part2 := bytesRepeat('b', UploadChunkSize)
	part3 := bytesRepeat('c', 10)
	pr, err := env.svc.ChunkUpload(ctx, init.UploadID, 0, part1)
	if err != nil || pr.Received != 1 || pr.Total != 3 {
		t.Fatalf("chunk 0: %v %+v", err, pr)
	}
	// 重复块/越界 index → FILE_010。
	_, err = env.svc.ChunkUpload(ctx, init.UploadID, 0, part1)
	wantFileErr(t, err, "FILE_010")
	_, err = env.svc.ChunkUpload(ctx, init.UploadID, 5, part3)
	wantFileErr(t, err, "FILE_010")
	// 无效任务 → FILE_010。
	_, err = env.svc.ChunkUpload(ctx, "deadbeef", 0, part3)
	wantFileErr(t, err, "FILE_010")
	// 未齐 complete → FILE_010。
	_, err = env.svc.ChunkComplete(ctx, testActor, init.UploadID)
	wantFileErr(t, err, "FILE_010")
	if _, err := env.svc.ChunkUpload(ctx, init.UploadID, 1, part2); err != nil {
		t.Fatalf("chunk 1: %v", err)
	}
	if _, err := env.svc.ChunkUpload(ctx, init.UploadID, 2, part3); err != nil {
		t.Fatalf("chunk 2: %v", err)
	}
	cc, err := env.svc.ChunkComplete(ctx, testActor, init.UploadID)
	if err != nil || cc.Size != total {
		t.Fatalf("complete: %v %+v", err, cc)
	}
	got, _ := os.ReadFile(filepath.Join(env.root, "movie.bin"))
	if int64(len(got)) != total || got[0] != 'a' ||
		got[UploadChunkSize] != 'b' || got[UploadChunkSize*2] != 'c' {
		t.Fatalf("merged content wrong (len=%d)", len(got))
	}
	// 临时目录已清理；任务作废。
	if entries, _ := os.ReadDir(env.root); len(entries) != 3 { // exists.bin movie.bin + excluded/
		t.Fatalf("tmp chunk dir should be removed, entries=%d", len(entries))
	}
	_, err = env.svc.ChunkComplete(ctx, testActor, init.UploadID)
	wantFileErr(t, err, "FILE_010")
	// 审计 detail={chunked:true,totalSize}。
	found := false
	for _, e := range env.rec.byAction(ActionFileUploaded) {
		if m, ok := e.Detail.(map[string]any); ok && m["chunked"] == true {
			found = true
		}
	}
	if !found {
		t.Fatal("chunked audit detail missing")
	}
}

// bytesRepeat 显式构造字节块。
func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

// ---- 元数据（D-06/D-14） ----

func TestChmodValidation(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	p := filepath.Join(env.root, "perm.txt")
	writeFileT(t, p, "x")
	_, err := env.svc.Chmod(ctx, testActor, p, "0999")
	wantFileErr(t, err, "FILE_015")
	_, err = env.svc.Chmod(ctx, testActor, p, "1777")
	wantFileErr(t, err, "FILE_015")
	_, err = env.svc.Chmod(ctx, testActor, p, "abc")
	wantFileErr(t, err, "FILE_015")
	cd, err := env.svc.Chmod(ctx, testActor, p, "0600")
	if err != nil || cd.Mode != "0600" {
		t.Fatalf("chmod: %v %+v", err, cd)
	}
	if runtime.GOOS != "windows" { // 位断言仅 unix（Windows chmod 只读位语义）
		fi, _ := os.Stat(p)
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %v", fi.Mode().Perm())
		}
	}
}

func TestChownAtLeastOneAndCandidates(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	p := filepath.Join(env.root, "own.txt")
	writeFileT(t, p, "x")
	// 至少一项（§4.2.4）。
	_, err := env.svc.Chown(ctx, testActor, p, "", "")
	wantFileErr(t, err, "FILE_015")
	// 候选不存在 → FILE_015（Windows 网关退化 → FILE_013，A-F7）。
	_, err = env.svc.Chown(ctx, testActor, p, "___no_such_user___", "")
	if runtime.GOOS == "windows" {
		wantFileErr(t, err, "FILE_013")
	} else {
		wantFileErr(t, err, "FILE_015")
	}
	// system-accounts：Linux 真实读取；Windows 不支持。
	if _, err := env.svc.SystemAccounts(ctx); err != nil && runtime.GOOS != "windows" {
		t.Fatalf("system accounts: %v", err)
	}
}

// ---- 压缩/解压（D-07/FR-14/15） ----

func TestArchiveRoundtripZip(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	writeFileT(t, filepath.Join(env.root, "z1.txt"), "one")
	if err := os.MkdirAll(filepath.Join(env.root, "zsub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFileT(t, filepath.Join(env.root, "zsub", "z2.txt"), "two")
	ad, err := env.svc.Archive(ctx, testActor,
		[]string{filepath.Join(env.root, "z1.txt"), filepath.Join(env.root, "zsub")}, FormatZip, "")
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if !strings.HasSuffix(ad.Path, "archive.zip") || ad.Size == 0 || ad.Format != FormatZip {
		t.Fatalf("archive data = %+v", ad)
	}
	// 同名产物 → FILE_004（D-13）。
	_, err = env.svc.Archive(ctx, testActor, []string{filepath.Join(env.root, "z1.txt")}, FormatZip, "")
	wantFileErr(t, err, "FILE_004")
	// 解压到新目录：roundtrip。
	dst := filepath.Join(env.root, "out")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatalf("mkdir dst: %v", err)
	}
	ud, err := env.svc.Unarchive(ctx, testActor, ad.Path, dst, "")
	if err != nil || ud.ExtractedCount != 3 { // z1.txt + zsub/ + zsub/z2.txt
		t.Fatalf("unarchive: %v %+v", err, ud)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "zsub", "z2.txt")); string(b) != "two" {
		t.Fatalf("extracted = %q", b)
	}
	// 再次解压同目录 ask → FILE_005 零落盘；overwrite → 成功。
	_, err = env.svc.Unarchive(ctx, testActor, ad.Path, dst, "")
	wantFileErr(t, err, "FILE_005")
	if _, err := env.svc.Unarchive(ctx, testActor, ad.Path, dst, ConflictOverwrite); err != nil {
		t.Fatalf("unarchive overwrite: %v", err)
	}
	// 非法 format → FILE_008。
	_, err = env.svc.Archive(ctx, testActor, []string{filepath.Join(env.root, "z1.txt")}, "rar", "")
	wantFileErr(t, err, "FILE_008")
}

func TestArchiveTarGzRoundtrip(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	writeFileT(t, filepath.Join(env.root, "t1.txt"), "tar-content")
	ad, err := env.svc.Archive(ctx, testActor,
		[]string{filepath.Join(env.root, "t1.txt")}, FormatTarGz, "")
	if err != nil || !strings.HasSuffix(ad.Path, "archive.tar.gz") { // 缺省名=archive.{ext}（§4.2.4）
		t.Fatalf("archive tar.gz: %v %+v", err, ad)
	}
	dst := filepath.Join(env.root, "tout")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := env.svc.Unarchive(ctx, testActor, ad.Path, dst, ""); err != nil {
		t.Fatalf("unarchive tar.gz: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "t1.txt")); string(b) != "tar-content" {
		t.Fatalf("extracted = %q", b)
	}
	// 无扩展名产物（自定义 targetName 直用）无法识别 → FILE_008（如实）。
	custom := filepath.Join(env.root, "noext")
	if _, err := env.svc.Archive(ctx, testActor,
		[]string{filepath.Join(env.root, "t1.txt")}, FormatTar, "noext"); err != nil {
		t.Fatalf("archive noext: %v", err)
	}
	_, err = env.svc.Unarchive(ctx, testActor, custom, dst, "")
	wantFileErr(t, err, "FILE_008")
}

func TestUnarchiveZipSlipRejected(t *testing.T) { // M2 验收硬项（R-04 归档延伸）
	env := newTestEnv(t)
	ctx := context.Background()
	evil := filepath.Join(env.root, "evil.zip")
	writeEvilZip(t, evil, "../evil.txt", "ok.txt")
	dst := filepath.Join(env.root, "safe")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := env.svc.Unarchive(ctx, testActor, evil, dst, "")
	wantFileErr(t, err, "FILE_009")
	// 整体不落盘：ok.txt 也不得出现（D-07 预检在先）。
	if _, serr := os.Stat(filepath.Join(dst, "ok.txt")); !os.IsNotExist(serr) {
		t.Fatal("zip slip must abort whole extraction")
	}
	if _, serr := os.Stat(filepath.Join(env.root, "evil.txt")); !os.IsNotExist(serr) {
		t.Fatal("no escape beyond target dir")
	}
}

func TestUnarchiveSymlinkEntryRejected(t *testing.T) { // A-F17
	env := newTestEnv(t)
	ctx := context.Background()
	evil := filepath.Join(env.root, "link.tar")
	writeEvilTarSymlink(t, evil)
	dst := filepath.Join(env.root, "safe2")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := env.svc.Unarchive(ctx, testActor, evil, dst, "")
	wantFileErr(t, err, "FILE_009")
}

// ---- 杂项 ----

func TestAuditDegradedToLog(t *testing.T) { // D-09：审计失败不影响操作结果
	env := newTestEnv(t)
	env.rec.fail = true
	ctx := context.Background()
	p := filepath.Join(env.root, "d.txt")
	writeFileT(t, p, "x")
	if _, err := env.svc.Chmod(ctx, testActor, p, "0644"); err != nil {
		t.Fatalf("chmod should succeed despite audit down: %v", err)
	}
}

// writeEvilZip 构造携带越界条目名的 zip。
func writeEvilZip(t *testing.T, path string, names ...string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = f.Close() }()
	zw := zip.NewWriter(f)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatalf("create entry %s: %v", n, err)
		}
		if _, err := w.Write([]byte("payload")); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
}

// writeEvilTarSymlink 构造含符号链接条目的 tar（A-F17 拒绝面）。
func writeEvilTarSymlink(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = f.Close() }()
	tw := tar.NewWriter(f)
	hdr := &tar.Header{Name: "evil-link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd", Mode: 0o777}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("header: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
}
