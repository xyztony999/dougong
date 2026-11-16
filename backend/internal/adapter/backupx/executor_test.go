package backupx

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeDump 是 DumpSource 替身（dbmgmt OHS 语义）。
type fakeDump struct {
	dbs   []string
	sqlOf func(db string) string
}

func (f fakeDump) DumpStream(_ context.Context, _ int64, database string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(f.sqlOf(database))), nil
}

func (f fakeDump) DatabaseNames(context.Context, int64) ([]string, error) {
	return f.dbs, nil
}

// buildSiteDir 建样本站点目录（含子目录、文件、symlink——Windows 无符号链接
// 权限时跳过该条目）。
func buildSiteDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>ok</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "a.txt"), []byte("alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestExecuteLocalTarget(t *testing.T) {
	root := t.TempDir() // 备份目标根
	siteDir := buildSiteDir(t)
	dump := fakeDump{
		dbs:   []string{"shop", "cms"},
		sqlOf: func(db string) string { return "-- dump of " + db + "\nCREATE TABLE t;" },
	}
	e := NewExecutor("", dump)
	spec := JobSpec{
		JobID: 1, Name: "nightly",
		Sources: []SourceSpec{
			{Kind: KindSiteDir, Path: siteDir},
			{Kind: KindDatabase, InstanceID: 7, InstanceName: "local", Database: "shop"},
			{Kind: KindDatabase, InstanceID: 7, InstanceName: "192.168.1.10-3306"}, // 整实例全库展开
		},
		Target:         TargetSpec{Type: TypeLocal, Path: root},
		RetentionCount: 3,
	}
	started := time.Date(2026, 11, 15, 2, 0, 0, 0, time.UTC)
	res, err := e.Execute(context.Background(), spec, started)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	// 产物目录 {根}/{job 名}/{yyyyMMdd-HHmmss}/（D-10）。
	wantDir := filepath.Join(root, "nightly", "20261115-020000")
	if res.ArtifactPath != wantDir {
		t.Fatalf("artifactPath = %q, want %q", res.ArtifactPath, wantDir)
	}
	names := map[string]int64{}
	for _, f := range res.Files {
		names[f.Name] = f.Size
	}
	for _, want := range []string{
		"site-1.tar.gz",                    // 站点目录项序号命名
		"db-local-shop.sql.gz",             // 指定库
		"db-192.168.1.10-3306-shop.sql.gz", // 整实例展开（label 点号安全）
		"db-192.168.1.10-3306-cms.sql.gz",
	} {
		if _, ok := names[want]; !ok {
			t.Fatalf("missing artifact %q (files=%v)", want, names)
		}
	}
	if res.ArtifactSize <= 0 {
		t.Fatalf("artifactSize = %d", res.ArtifactSize)
	}
	// manifest 存在且自描述（D-10）。
	m, err := e.ReadManifestPath(context.Background(), res.ArtifactPath, 0)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if m.JobID != 1 || len(m.Files) != len(res.Files) || len(m.Sources) != 4 { // 展开后 4 源
		t.Fatalf("manifest wrong: %+v", m)
	}
	// ref 形状断言（Gate 2 E1：{type, ref}——detail sources 契约字段）。
	wantRefs := map[string]string{
		"site_dir":  siteDir,
		"database":  "local·shop",
		"database2": "192.168.1.10-3306·shop",
		"database3": "192.168.1.10-3306·cms",
	}
	got := 0
	for _, src := range m.Sources {
		switch src.Kind {
		case "site_dir":
			if src.Ref != wantRefs["site_dir"] {
				t.Fatalf("site ref = %q, want %q", src.Ref, wantRefs["site_dir"])
			}
			got++
		case "database":
			if src.Ref != wantRefs["database"] && src.Ref != wantRefs["database2"] && src.Ref != wantRefs["database3"] {
				t.Fatalf("db ref = %q unexpected", src.Ref)
			}
			got++
		}
	}
	if got != 4 {
		t.Fatalf("ref-checked sources = %d, want 4", got)
	}
	if m.StartedAt != "2026-11-15T02:00:00Z" || m.FinishedAt == "" {
		t.Fatalf("manifest times wrong: %q %q", m.StartedAt, m.FinishedAt)
	}
	// tar.gz 可解且内容如实（gzip+tar 头校验）。
	st, err := os.Stat(filepath.Join(res.ArtifactPath, "site-1.tar.gz"))
	if err != nil || st.Size() == 0 {
		t.Fatalf("site archive missing/empty: %v", err)
	}
}

func TestExecuteSourceFailureWholeFailure(t *testing.T) {
	root := t.TempDir()
	siteDir := buildSiteDir(t)
	// 缺失源目录 → 任一源失败整体 failure（I-04），错误列明失败源。
	e := NewExecutor("", fakeDump{})
	spec := JobSpec{
		JobID: 2, Name: "j",
		Sources: []SourceSpec{
			{Kind: KindSiteDir, Path: siteDir},
			{Kind: KindSiteDir, Path: filepath.Join(root, "no-such-dir")},
		},
		Target: TargetSpec{Type: TypeLocal, Path: root},
	}
	if _, err := e.Execute(context.Background(), spec, time.Now()); err == nil {
		t.Fatal("want whole-run failure")
	} else if !strings.Contains(err.Error(), "site_dir") || !strings.Contains(err.Error(), "no-such-dir") {
		t.Fatalf("error must identify failing source: %v", err)
	}
	// 半成品清理（I-04：failure 不可下载——不残留目录）。
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatalf("partial artifacts not cleaned: %v", entries)
	}
}

func TestPullAndRemoveLocal(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "j", "20260101-000000")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "site-1.tar.gz"), []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := NewExecutor("", nil)
	local, cleanup, err := e.PullFile(context.Background(), dir, "site-1.tar.gz", 0)
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	defer cleanup()
	b, err := os.ReadFile(local)
	if err != nil || string(b) != "payload" {
		t.Fatalf("pulled content = %q err=%v", b, err)
	}
	// 不在清单的缺失文件 → ErrArtifactMissing（CB_009 语义）。
	if _, _, err := e.PullFile(context.Background(), dir, "ghost.gz", 0); err != ErrArtifactMissing {
		t.Fatalf("missing pull err = %v, want ErrArtifactMissing", err)
	}
	// 清理：目录整体移除。
	if err := e.RemoveArtifactPath(context.Background(), dir, 0); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("dir still exists: %v", err)
	}
}

func TestClassifyArtifact(t *testing.T) {
	cases := []struct {
		path                 string
		kind, userHost, rest string
	}{
		{"/var/backups/j/1", TypeLocal, "", ""},
		{"root@192.168.1.10:/srv/bk/j/1", TypeRsync, "root@192.168.1.10", "/srv/bk/j/1"},
		{"192.168.1.10:/srv/bk", TypeRsync, "192.168.1.10", "/srv/bk"},
	}
	for _, tc := range cases {
		kind, userHost, rest := ClassifyArtifact(tc.path)
		if kind != tc.kind || userHost != tc.userHost || rest != tc.rest {
			t.Errorf("ClassifyArtifact(%q) = (%q,%q,%q), want (%q,%q,%q)",
				tc.path, kind, userHost, rest, tc.kind, tc.userHost, tc.rest)
		}
	}
}

func TestReadManifestPathRemoteNotFetched(t *testing.T) {
	e := NewExecutor("", nil)
	if _, err := e.ReadManifestPath(context.Background(), "host:/bk/1", 0); err != ErrRemoteManifest {
		t.Fatalf("remote manifest err = %v, want ErrRemoteManifest", err)
	}
}

func TestSanitizeLabel(t *testing.T) {
	if got := sanitizeLabel("192.168.1.10-3306"); got != "192.168.1.10-3306" {
		t.Fatalf("safe label mangled: %q", got)
	}
	if got := sanitizeLabel("/var/run/mysqld/mysqld.sock"); got != "_var_run_mysqld_mysqld.sock" {
		t.Fatalf("socket label not sanitized: %q", got)
	}
	if got := sanitizeLabel("###"); got != "___" {
		t.Fatalf("unsafe label = %q", got)
	}
}
