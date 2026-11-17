package main

import (
	"path/filepath"
	"testing"
)

// TestResolveConfigPath 配置缺省路径解析（v0.1.3 CLI bug 修正的回归锁）：
// 显式 -config 优先；已安装布局（/var/lib/dougong 存在）指向服务数据目录——
// CLI 在任意 CWD 执行都读到 systemd 服务那套配置；未安装回退 CWD（开发）。
func TestResolveConfigPath(t *testing.T) {
	orig := serviceDataDirExists
	defer func() { serviceDataDirExists = orig }()

	t.Run("显式 -config 优先", func(t *testing.T) {
		serviceDataDirExists = func() bool { return true }
		if got := resolveConfigPath("/srv/panel/prod.json"); got != "/srv/panel/prod.json" {
			t.Fatalf("显式配置被覆盖: %q", got)
		}
	})
	t.Run("已安装布局指向服务数据目录", func(t *testing.T) {
		serviceDataDirExists = func() bool { return true }
		want := filepath.Join(serviceDataDir, "dougong.json")
		if got := resolveConfigPath(""); got != want {
			t.Fatalf("缺省 = %q, want %q", got, want)
		}
	})
	t.Run("未安装回退 CWD", func(t *testing.T) {
		serviceDataDirExists = func() bool { return false }
		if got := resolveConfigPath(""); got != "dougong.json" {
			t.Fatalf("开发回退 = %q, want dougong.json", got)
		}
	})
}
