// ConfigWritePipeline OHS 测试（api-site-mgmt v2.8 D-21 / api-db-tools D-03
// 消费契约）：ConfigApply 四步管线+重载；ConfigRemove 最近快照恢复。
package sitemgmt

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestConfigApplyOHS 管线泛化提供：写入+nginx -t+reload；失败回滚原字节。
func TestConfigApplyOHS(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	file := env.writeConf(t, "sites-enabled", "panel.conf", "server { listen 80; }\n")
	// 成功路径：内容落盘 + reload 调用。
	if err := env.svc.ConfigApply(ctx, file, "server { listen 8080; }\n", false); err != nil {
		t.Fatalf("apply: %v", err)
	}
	b, err := os.ReadFile(file)
	if err != nil || string(b) != "server { listen 8080; }\n" {
		t.Fatalf("content = %q err=%v", b, err)
	}
	if env.gw.reloadCalls == 0 {
		t.Fatal("reload should be called after apply")
	}
	// nginx -t 失败 → 回滚原字节（R-06）。
	env.gw.testOK = false
	if err := env.svc.ConfigApply(ctx, file, "server { broken\n", false); err == nil {
		t.Fatal("broken config should fail")
	}
	b, _ = os.ReadFile(file)
	if string(b) != "server { listen 8080; }\n" {
		t.Fatalf("rollback failed: %q", b)
	}
}

// TestConfigRemoveOHS 撤除恢复最近写前快照（整文件级恢复语义）。
func TestConfigRemoveOHS(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	file := env.writeConf(t, "sites-enabled", "panel.conf", "original\n")
	if err := env.svc.ConfigApply(ctx, file, "first-edit\n", false); err != nil {
		t.Fatal(err)
	}
	env.now = env.now.Add(time.Second) // 快照时间戳区分（定宽 unixnano）
	env.svc.clock = func() time.Time { return env.now }
	if err := env.svc.ConfigApply(ctx, file, "second-edit\n", false); err != nil {
		t.Fatal(err)
	}
	// Remove → 恢复最近写前快照（first-edit）。
	if err := env.svc.ConfigRemove(ctx, file); err != nil {
		t.Fatalf("remove: %v", err)
	}
	b, _ := os.ReadFile(file)
	if string(b) != "first-edit\n" {
		t.Fatalf("restore = %q, want first-edit", b)
	}
	// 无快照文件 → 如实报错。
	other := env.writeConf(t, "sites-enabled", "nosnap.conf", "x\n")
	if err := env.svc.ConfigRemove(ctx, other); err == nil {
		t.Fatal("no snapshot should error")
	}
}

// TestConfigRemoveOriginAbsent v1.3（api-db-tools D-19⑥）：管线 allowNew
// 创建的新文件原态=不存在→ConfigRemove 恢复原字节=删除文件+reload（自动
// 落地块清块语义；后续写前快照含块内容，快照恢复无法表达「文件不存在」，
// origin-absent 标记优先）；文件已不存在=幂等成功。
func TestConfigRemoveOriginAbsent(t *testing.T) {
	env := newEnv(t)
	ctx := context.Background()
	dir := filepath.Join(env.gw.root, "conf.d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "dougong-tools.conf")
	// allowNew 创建（无写前快照——原态=不存在）。
	if err := env.svc.ConfigApply(ctx, file, "server { listen 18090; }\n", true); err != nil {
		t.Fatalf("create: %v", err)
	}
	env.now = env.now.Add(time.Second)
	env.svc.clock = func() time.Time { return env.now }
	// 后续写入产生快照（内容含自动块）。
	if err := env.svc.ConfigApply(ctx, file, "server { listen 18090; server_name x; }\n", false); err != nil {
		t.Fatal(err)
	}
	before := env.gw.reloadCalls
	if err := env.svc.ConfigRemove(ctx, file); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("原态=不存在应删除文件, stat err=%v", err)
	}
	if env.gw.reloadCalls != before+1 {
		t.Fatal("清块后应 reload")
	}
	// 幂等：文件已不存在=清块成功。
	if err := env.svc.ConfigRemove(ctx, file); err != nil {
		t.Fatalf("idempotent remove: %v", err)
	}
}
