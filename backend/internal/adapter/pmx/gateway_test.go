package pmx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildStartArgs(t *testing.T) {
	spec := StartSpec{Name: "dougong-app-web", Cwd: "/srv/web", Launcher: "/var/lib/dougong/app/pm2-launcher.js",
		NodeBin: "/usr/bin/node",
		Args:    []string{"/srv/web", "/usr/bin/node", "/usr/bin/npm", "file", "server.js"}}
	got := buildStartArgs(spec)
	want := []string{"start", "/var/lib/dougong/app/pm2-launcher.js", "--name", "dougong-app-web",
		"--cwd", "/srv/web", "--interpreter", "/usr/bin/node", "--",
		"/srv/web", "/usr/bin/node", "/usr/bin/npm", "file", "server.js"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("buildStartArgs = %v, want %v", got, want)
	}
	// -- 必须位于启动器参数之前（pm2 以 -- 分隔自身选项与脚本参数）
	sep := -1
	for i, a := range got {
		if a == "--" {
			sep = i
			break
		}
	}
	if sep < 0 || sep >= len(got)-1 {
		t.Fatalf("缺少 -- 分隔或其位置错误: %v", got)
	}
	// NodeBin 为空时不带 --interpreter
	bare := buildStartArgs(StartSpec{Name: "n", Cwd: "/c", Launcher: "/l.js", Args: []string{"a"}})
	if strings.Join(bare, " ") != "start /l.js --name n --cwd /c -- a" {
		t.Fatalf("无解释器参数 = %v", bare)
	}
}

func TestParseJList(t *testing.T) {
	now := time.UnixMilli(1_700_000_100_000)
	data := []byte(`[
	  {"name":"a","pid":11,"pm2_env":{"status":"online","restart_time":2,"pm_uptime":1700000000000,"pm_out_log_path":"/h/logs/a-out.log","pm_err_log_path":"/h/logs/a-error.log"},"monit":{"memory":12345}},
	  {"name":"b","pid":0,"pm2_env":{"status":"stopped","restart_time":0,"pm_uptime":0},"monit":{"memory":0}}
	]`)
	got, err := parseJList(data, now)
	if err != nil {
		t.Fatalf("parseJList: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("条目数 = %d, want 2", len(got))
	}
	a := got[0]
	if a.Name != "a" || a.Status != "online" || a.Pid != 11 || a.Restarts != 2 || a.MemoryBytes != 12345 ||
		a.OutLogPath != "/h/logs/a-out.log" || a.ErrLogPath != "/h/logs/a-error.log" {
		t.Fatalf("online 条目收窄错误: %+v", a)
	}
	if a.UptimeMs != 100_000 {
		t.Fatalf("uptime = %d, want 100000（now - pm_uptime）", a.UptimeMs)
	}
	if got[1].UptimeMs != 0 {
		t.Fatalf("非 online 态 uptime 应为 0: %+v", got[1])
	}
	if _, err := parseJList([]byte("not json"), now); err == nil {
		t.Fatal("坏 JSON 应报错")
	}
}

func TestTailLines(t *testing.T) {
	cases := []struct {
		text string
		n    int
		want int
		last string
	}{
		{"a\nb\nc\n", 2, 2, "c"},
		{"a\r\nb\r\n", 5, 2, "b"},
		{"", 3, 0, ""},
		{"only", 3, 1, "only"},
	}
	for _, c := range cases {
		got := tailLines(c.text, c.n)
		if len(got) != c.want {
			t.Fatalf("tailLines(%q,%d) 行数 = %d, want %d", c.text, c.n, len(got), c.want)
		}
		if c.want > 0 && got[len(got)-1] != c.last {
			t.Fatalf("末行 = %q, want %q", got[len(got)-1], c.last)
		}
	}
}

func TestSavedStatus(t *testing.T) {
	home := t.TempDir()
	g := New(WithHome(home))
	// 尚未保存过：不存在=未开启，不报错
	if _, found, err := g.SavedStatus("x"); err != nil || found {
		t.Fatalf("无保存文件: found=%v err=%v", found, err)
	}
	dump := `[{"name":"dougong-app-a","status":"online"},{"name":"dougong-app-b","status":"stopped"}]`
	if err := os.WriteFile(filepath.Join(home, "dump.pm2"), []byte(dump), 0o600); err != nil {
		t.Fatal(err)
	}
	if st, found, err := g.SavedStatus("dougong-app-a"); err != nil || !found || st != "online" {
		t.Fatalf("a: st=%q found=%v err=%v", st, found, err)
	}
	if st, found, _ := g.SavedStatus("dougong-app-b"); !found || st != "stopped" {
		t.Fatalf("b: st=%q found=%v", st, found)
	}
	if _, found, _ := g.SavedStatus("dougong-app-c"); found {
		t.Fatal("不存在的进程不应 found")
	}
}

func TestTail(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "a-out.log")
	errf := filepath.Join(dir, "a-error.log")
	if err := os.WriteFile(out, []byte("o1\no2\no3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(errf, []byte("e1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := New(WithHome(dir))
	got, err := g.Tail(context.Background(), out, errf, 2)
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	want := []string{"o2", "o3", "[错误] e1"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("Tail = %v, want %v", got, want)
	}
	// 缺失的一路跳过不报错
	got, err = g.Tail(context.Background(), out, filepath.Join(dir, "nope.log"), 1)
	if err != nil || len(got) != 1 || got[0] != "o3" {
		t.Fatalf("缺失错误日志路: got=%v err=%v", got, err)
	}
}

func TestEnsureLauncher(t *testing.T) {
	dir := t.TempDir()
	p, err := EnsureLauncher(dir)
	if err != nil {
		t.Fatalf("EnsureLauncher: %v", err)
	}
	if filepath.Base(p) != LauncherName {
		t.Fatalf("文件名 = %s", filepath.Base(p))
	}
	data, err := os.ReadFile(p)
	if err != nil || !strings.Contains(string(data), "pm2-launcher") && !strings.Contains(string(data), "启动器") {
		t.Fatalf("启动器内容异常: err=%v", err)
	}
	// 幂等：再次调用不重写（mtime 不变）
	st1, _ := os.Stat(p)
	if _, err := EnsureLauncher(dir); err != nil {
		t.Fatal(err)
	}
	st2, _ := os.Stat(p)
	if !st1.ModTime().Equal(st2.ModTime()) {
		t.Fatal("内容一致时不应重写")
	}
	// 内容被改动后应覆盖回面板资产
	if err := os.WriteFile(p, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureLauncher(dir); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(p)
	if string(data) == "tampered" {
		t.Fatal("被改动的启动器应覆盖回内嵌内容")
	}
}

func TestRunUnavailable(t *testing.T) {
	g := New(WithPM2Bin(""), WithHome(t.TempDir()))
	if _, err := g.List(context.Background()); err == nil || !strings.Contains(err.Error(), "pm2 不可用") {
		t.Fatalf("pm2 缺席应明确报错: %v", err)
	}
}
