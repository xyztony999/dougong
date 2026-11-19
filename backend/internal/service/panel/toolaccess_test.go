// PanelAccess 读契约测试（db-tools D-03 落点选择/内部端点 URL 构造输入）。
package panel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	storepanel "github.com/dougong/dougong/internal/store/panel"
)

// TestPanelAccessProjection 绑定域名/监听形态/入口前缀/HTTPS 投影。
func TestPanelAccessProjection(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	info, err := svc.PanelAccess(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// 默认：未绑定域名，listen_scope=all → 0.0.0.0（D-P02 枚举映射）。
	if info.BindDomain != "" || info.ListenHost != "0.0.0.0" || info.ListenPort == 0 {
		t.Fatalf("default access = %+v", info)
	}
	// 绑定域名 + localhost 监听 + HTTPS。
	domain := json.RawMessage(`"panel.example.test"`)
	scope := storepanel.ListenScopeLocalhost
	https := storepanel.HTTPSModeSelfSigned
	if _, err := svc.UpdateConfig(ctx, adminActor(), UpdateConfigRequest{
		BindDomain: domain, ListenScope: &scope, HTTPSMode: &https,
	}); err != nil {
		t.Fatalf("update config: %v", err)
	}
	info, err = svc.PanelAccess(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.BindDomain != "panel.example.test" || info.ListenHost != "127.0.0.1" || !info.HTTPSEnabled {
		t.Fatalf("bound access = %+v", info)
	}
	// 安全入口开启 → EntryPath 含前导 /（重生成后）。
	if _, err := svc.RegenerateEntryPath(ctx, adminActor()); err != nil {
		t.Fatalf("regen entry path: %v", err)
	}
	info, _ = svc.PanelAccess(ctx)
	if info.EntryPath != "" && !strings.HasPrefix(info.EntryPath, "/") {
		t.Fatalf("entry path = %q, want leading /", info.EntryPath)
	}
}
