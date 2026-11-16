package envmgmt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/adapter/envpack"
	"github.com/dougong/dougong/internal/adapter/sysd"
	"github.com/dougong/dougong/internal/api"
	authapi "github.com/dougong/dougong/internal/api/auth"
	auditsvc "github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	envsvc "github.com/dougong/dougong/internal/service/envmgmt"
	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	storeenv "github.com/dougong/dougong/internal/store/envmgmt"
	"github.com/gin-gonic/gin"
)

// 全栈 httptest（site-mgmt module_test 同法）：权限矩阵（两角色查看/admin
// 变更+CSRF）、通道不可用语义（Windows 开发机 envpack 退化如实 ENV_001——
// 包操作实联归 Linux 冒烟，与 sysd/dockerx 环境绑定口径一致）。

const testCred = "dougong-env-test-only"

type testApp struct {
	ts *httptest.Server
}

type session struct{ cookie, csrf string }

func newTestApp(t *testing.T) *testApp {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := storeauth.New(db)
	if err := repo.Init().EnsureRow(ctx); err != nil {
		t.Fatalf("ensure init: %v", err)
	}
	hasher, err := authsvc.NewBcryptHasher(10)
	if err != nil {
		t.Fatal(err)
	}
	authSvcObj := authsvc.New(repo, hasher, authsvc.NewTotpService(nil), authsvc.DefaultSessionPolicy())
	auditSvc := auditsvc.New(repo.Audit(), time.Now)
	// 通道探测注入恒失败：CI Linux runner 预装 apt，真实探测会击穿
	// TestInstallChannelUnavailable 的「通道不可用」假设（Windows 开发机
	// 恰好成立属环境巧合）；注入后断言环境无关，实联归 Linux 冒烟。
	envSvc := envsvc.New(
		envpack.NewGatewayWithProbe(func(string) (string, error) {
			return "", errors.New("probe disabled in tests")
		}),
		sysd.NewGateway(0), storeenv.New(db), envsvc.Options{})
	server := api.NewServer()
	server.RegisterModules(
		authapi.NewModule(authSvcObj, auditSvc, false),
		NewModule(authSvcObj, envSvc),
	)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	return &testApp{ts: ts}
}

func loginSession(t *testing.T, app *testApp, username string) session {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": testCred})
	resp, err := http.Post(app.ts.URL+"/auth/init/wizard", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env struct {
		Code int `json:"code"`
		Data struct {
			CSRFToken string `json:"csrfToken"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil || env.Code != 0 {
		t.Fatalf("init wizard: %v (body=%s)", err, raw)
	}
	for _, c := range resp.Cookies() {
		if c.Name == authsvc.SessionCookieName {
			return session{cookie: c.Value, csrf: env.Data.CSRFToken}
		}
	}
	t.Fatal("wizard missing session cookie")
	return session{}
}

func post(t *testing.T, app *testApp, s session, path, body string) string {
	t.Helper()
	req, _ := http.NewRequest("POST", app.ts.URL+path, bytes.NewReader([]byte(body)))
	req.Header.Set("Cookie", authsvc.SessionCookieName+"="+s.cookie)
	req.Header.Set("X-CSRF-Token", s.csrf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", app.ts.URL) // CsrfGuard 同源校验
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return string(raw)
}

func get(t *testing.T, app *testApp, s session, path string) string {
	t.Helper()
	req, _ := http.NewRequest("GET", app.ts.URL+path, nil)
	req.Header.Set("Cookie", authsvc.SessionCookieName+"="+s.cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return string(raw)
}

func createReadonly(t *testing.T, app *testApp, admin session) session {
	t.Helper()
	out := post(t, app, admin, "/auth/user/create",
		`{"username":"watcher","password":"`+testCred+`","role":"readonly"}`)
	if !bytes.Contains([]byte(out), []byte(`"code":0`)) {
		t.Fatalf("create readonly: %s", out)
	}
	body, _ := json.Marshal(map[string]string{"username": "watcher", "password": testCred})
	resp, err := http.Post(app.ts.URL+"/auth/session/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env struct {
		Data struct {
			CSRFToken string `json:"csrfToken"`
		} `json:"data"`
	}
	json.Unmarshal(raw, &env)
	for _, c := range resp.Cookies() {
		if c.Name == authsvc.SessionCookieName {
			return session{cookie: c.Value, csrf: env.Data.CSRFToken}
		}
	}
	t.Fatal("readonly login missing cookie")
	return session{}
}

// TestEnvListBothRoles 两角色可看 + 通道不可用如实呈现（channel.available=false，
// 列表观测不报错——D-01/D-03）。
func TestEnvListBothRoles(t *testing.T) {
	app := newTestApp(t)
	admin := loginSession(t, app, "admin")
	ro := createReadonly(t, app, admin)
	for _, s := range []session{admin, ro} {
		out := get(t, app, s, "/env/env/list")
		if !bytes.Contains([]byte(out), []byte(`"channel"`)) ||
			!bytes.Contains([]byte(out), []byte(`"displayName":"nginx"`)) {
			t.Fatalf("env list: %s", out)
		}
	}
}

// TestInstallChannelUnavailable 通道不可用 → ENV_001（Windows 退化如实；
// Linux 实联归目标环境冒烟）。
func TestInstallChannelUnavailable(t *testing.T) {
	app := newTestApp(t)
	admin := loginSession(t, app, "admin")
	out := post(t, app, admin, "/env/env/install", `{"envType":"nginx"}`)
	if !bytes.Contains([]byte(out), []byte("ENV_001")) {
		t.Fatalf("install on unavailable channel: %s", out)
	}
}

// TestReadonlyDeniedAndInvalidType readonly 越界 AUTH_008；envType 非法 ENV_002。
func TestReadonlyDeniedAndInvalidType(t *testing.T) {
	app := newTestApp(t)
	admin := loginSession(t, app, "admin")
	ro := createReadonly(t, app, admin)
	if out := post(t, app, ro, "/env/env/install", `{"envType":"nginx"}`); !bytes.Contains([]byte(out), []byte("AUTH_008")) {
		t.Fatalf("readonly install: %s", out)
	}
	if out := post(t, app, admin, "/env/env/install", `{"envType":"postgres"}`); !bytes.Contains([]byte(out), []byte("ENV_002")) {
		t.Fatalf("invalid envType: %s", out)
	}
	if out := get(t, app, ro, "/env/task/list?page=1&pageSize=10"); !bytes.Contains([]byte(out), []byte(`"code":0`)) {
		t.Fatalf("readonly task list: %s", out)
	}
	if out := get(t, app, admin, "/env/task/detail?taskId=999"); !bytes.Contains([]byte(out), []byte("ENV_005")) {
		t.Fatalf("task not found: %s", out)
	}
}
