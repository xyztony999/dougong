package panel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dougong/dougong/internal/store"
	storepanel "github.com/dougong/dougong/internal/store/panel"
)

// GitHubReleaseSource 取最新 release 的 tag_name（D-P09 版本源接线）。
func TestGitHubReleaseSource(t *testing.T) {
	var gotUA, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","name":"v9.9.9"}`))
	}))
	defer srv.Close()

	src := &GitHubReleaseSource{BaseURL: srv.URL, Repo: "owner/name"}
	got, err := src.LatestVersion(context.Background())
	if err != nil || got != "v9.9.9" {
		t.Fatalf("LatestVersion = %q err=%v, want v9.9.9", got, err)
	}
	if gotPath != "/repos/owner/name/releases/latest" {
		t.Fatalf("request path = %s", gotPath)
	}
	if gotUA == "" {
		t.Fatal("GitHub API requires a User-Agent header")
	}

	// 非 200 → 错误（PANEL_005 上游）。
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer bad.Close()
	if _, err := (&GitHubReleaseSource{BaseURL: bad.URL}).LatestVersion(context.Background()); err == nil {
		t.Fatal("non-200 must error")
	}
}

// 装配版本源后检查更新可用（此前 s.source=nil 恒 PANEL_005「版本源不可达」）。
func TestUpdateCheckWiredSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9"}`))
	}))
	defer srv.Close()

	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := New(storepanel.New(db), WithVersionSource(&GitHubReleaseSource{BaseURL: srv.URL}))
	ctx := context.Background()
	if _, err := svc.Bootstrap(ctx); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	// 默认关 → PANEL_004（I-P05）。
	if _, err := svc.UpdateCheck(ctx); err == nil {
		t.Fatal("update check must be gated when disabled")
	}
	// 开启后触发 → 拿到最新版本（开发构建当前版本视作 0.0.0，恒有更新）。
	if _, err := svc.UpdateConfig(ctx, adminActor(), UpdateConfigRequest{UpdateCheck: ptr(true)}); err != nil {
		t.Fatalf("enable update check: %v", err)
	}
	res, err := svc.UpdateCheck(ctx)
	if err != nil {
		t.Fatalf("update check with wired source: %v", err)
	}
	if res.LatestVersion == nil || *res.LatestVersion != "v9.9.9" || !res.UpdateAvailable {
		t.Fatalf("result = %+v, want latest v9.9.9 + update available", res)
	}
}
