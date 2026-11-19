// v1.3 配置变更钩子回归（api-db-tools D-20③⑤，S-11-01）：bindDomain
// 变更成功→观察者通知；同值零通知；钩子 panic 不阻断配置变更（recover+
// slog，panel_config 权威已成功）。
package panel

import (
	"context"
	"encoding/json"
	"testing"
)

// recordingHook 观察者桩（记录通知值；可注入 panic）。
type recordingHook struct {
	calls []string
	panic bool
}

func (h *recordingHook) OnBindDomainChanged(_ context.Context, bindDomain string) {
	if h.panic {
		panic("hook panic (test)")
	}
	h.calls = append(h.calls, bindDomain)
}

// TestUpdateConfigBindDomainHook bindDomain 变更钩子通知语义。
func TestUpdateConfigBindDomainHook(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	h := &recordingHook{}
	svc.RegisterConfigChangeHook(h)

	// 绑定 → 通知新值。
	if _, err := svc.UpdateConfig(ctx, adminActor(), UpdateConfigRequest{
		BindDomain: json.RawMessage(`"panel.example.test"`)}); err != nil {
		t.Fatal(err)
	}
	if len(h.calls) != 1 || h.calls[0] != "panel.example.test" {
		t.Fatalf("hook calls = %v, want [panel.example.test]", h.calls)
	}
	// 同值重写 → 零通知（changes.BindDomain 未置位）。
	if _, err := svc.UpdateConfig(ctx, adminActor(), UpdateConfigRequest{
		BindDomain: json.RawMessage(`"panel.example.test"`)}); err != nil {
		t.Fatal(err)
	}
	if len(h.calls) != 1 {
		t.Fatalf("同值重写不应通知: %v", h.calls)
	}
	// 解绑（null）→ 通知 ""。
	if _, err := svc.UpdateConfig(ctx, adminActor(), UpdateConfigRequest{
		BindDomain: json.RawMessage(`null`)}); err != nil {
		t.Fatal(err)
	}
	if len(h.calls) != 2 || h.calls[1] != "" {
		t.Fatalf("解绑通知 = %v, want [..., \"\"]", h.calls)
	}
	// 非 bindDomain 字段变更 → 零通知。
	if _, err := svc.UpdateConfig(ctx, adminActor(), UpdateConfigRequest{
		Lang: ptr("zh-CN")}); err != nil {
		t.Fatal(err)
	}
	if len(h.calls) != 2 {
		t.Fatalf("非 bindDomain 变更不应通知: %v", h.calls)
	}
	// 钩子 panic 不阻断配置变更（recover+slog；panel_config 权威已成功）。
	h.panic = true
	if _, err := svc.UpdateConfig(ctx, adminActor(), UpdateConfigRequest{
		BindDomain: json.RawMessage(`"other.example.test"`)}); err != nil {
		t.Fatalf("钩子 panic 不得阻断配置变更: %v", err)
	}
	got, err := svc.BindDomainState(ctx)
	if err != nil || got != "other.example.test" {
		t.Fatalf("配置权威应已落库: %q err=%v", got, err)
	}
}
