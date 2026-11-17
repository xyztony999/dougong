package nginxx

import (
	"os"
	"path/filepath"
	"testing"
)

// TestIsPidFileErr pid 文件类失败识别（v2.7 回退触发条件）。
func TestIsPidFileErr(t *testing.T) {
	cases := map[string]bool{
		`nginx: [error] invalid PID number "" in "/run/nginx.pid"`:                   true,
		"nginx: [error] open() /run/nginx.pid failed (2: No such file or directory)": true,
		"nginx: [emerg] bind() to 0.0.0.0:80 failed":                                 false,
		"": false,
	}
	for out, want := range cases {
		if got := isPidFileErr(out); got != want {
			t.Fatalf("isPidFileErr(%q) = %v, want %v", out, got, want)
		}
	}
}

// TestMasterPidFromProc procfs 扫描 master PID（目录注入）。
func TestMasterPidFromProc(t *testing.T) {
	dir := t.TempDir()
	for pid, cmdline := range map[string]string{
		"101": "nginx: master process /usr/sbin/nginx -c /etc/nginx/nginx.conf\x00",
		"102": "nginx: worker process\x00",
	} {
		if err := os.MkdirAll(filepath.Join(dir, pid), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, pid, "cmdline"), []byte(cmdline), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := masterPidFromProc(dir)
	if err != nil || got != 101 {
		t.Fatalf("masterPidFromProc = %d, %v; want 101", got, err)
	}
	if _, err := masterPidFromProc(t.TempDir()); err == nil {
		t.Fatal("空目录应报错")
	}
}
