package appmgmt

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/adapter/sysd"
	"github.com/dougong/dougong/internal/api"
	authapi "github.com/dougong/dougong/internal/api/auth"
	appsvc "github.com/dougong/dougong/internal/service/appmgmt"
	auditsvc "github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	envsvc "github.com/dougong/dougong/internal/service/envmgmt"
	"github.com/dougong/dougong/internal/store"
	storeapp "github.com/dougong/dougong/internal/store/appmgmt"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	"github.com/gin-gonic/gin"
)

// 全栈 httptest（env-mgmt module_test 同法）：权限矩阵（两角色查看/admin
// 变更+CSRF/日志与参数 admin 专属读取 ⚠️A8）+ 生命周期链路（创建→档案登记→
// 删除→档案撤除）。venv/pip/systemd/journalctl 实联归 Linux 冒烟（替身注入）。

const testCred = "dougong-app-test-only"

type testApp struct{ ts *httptest.Server }

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
	appStore := storeapp.New(db)
	svc := appsvc.New(&stubPy{}, &stubSysd{}, appStore, appsvc.Options{
		EnvReady:     stubReady{},
		SyncDispatch: true,
	})
	server := api.NewServer()
	server.RegisterModules(
		authapi.NewModule(authSvcObj, auditSvc, false),
		NewModule(authSvcObj, svc),
	)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	return &testApp{ts: ts}
}

// ---- 替身（api 层同构缝；实联归 Linux 冒烟）----

type stubPy struct{ written map[string]string }

func (s *stubPy) CreateVenv(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", nil
}
func (s *stubPy) PipInstall(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", nil
}
func (s *stubPy) VenvExists(_ string) bool { return true }
func (s *stubPy) DepsProbe(_ context.Context, _ string) (int, error) {
	return 3, nil
}
func (s *stubPy) RemoveDir(_ string) error { return nil }
func (s *stubPy) WriteUnit(_ context.Context, u, c string) (func(), error) {
	if s.written == nil {
		s.written = map[string]string{}
	}
	s.written[u] = c
	return func() {}, nil
}
func (s *stubPy) RemoveUnit(_ context.Context, _ string) error { return nil }
func (s *stubPy) Tail(_ context.Context, _ string, _ int) (string, error) {
	return "line1\nline2", nil
}

type stubSysd struct{}

func (s *stubSysd) GetUnit(_ context.Context, _ string) (sysd.UnitDetail, error) {
	return sysd.UnitDetail{LoadState: "loaded", ActiveState: "active", EnabledState: "enabled"}, nil
}
func (s *stubSysd) StartUnit(_ context.Context, _ string) (sysd.JobResult, error) {
	return sysd.JobResult{}, nil
}
func (s *stubSysd) StopUnit(_ context.Context, _ string) (sysd.JobResult, error) {
	return sysd.JobResult{}, nil
}
func (s *stubSysd) RestartUnit(_ context.Context, _ string) (sysd.JobResult, error) {
	return sysd.JobResult{}, nil
}
func (s *stubSysd) EnableUnit(_ context.Context, _ string) error  { return nil }
func (s *stubSysd) DisableUnit(_ context.Context, _ string) error { return nil }

type stubReady struct{}

func (stubReady) PythonReadiness(_ context.Context) (envsvc.PythonReadiness, error) {
	return envsvc.PythonReadiness{Installed: true, Version: "3.11.2", VenvSupport: true, PipAvailable: true}, nil
}

func (stubReady) NodeReadiness(_ context.Context) (envsvc.NodeReadiness, error) {
	return envsvc.NodeReadiness{NodeAvailable: true, NpmAvailable: true, Pm2Available: true, NodeVersion: "22.14.0"}, nil
}

// ---- 会话与请求辅助（env-mgmt module_test 同法）----

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

func createReadonly(t *testing.T, app *testApp, admin session) session {
	t.Helper()
	out := post(t, app, admin, "/auth/user/create",
		`{"username":"watcher","password":"`+testCred+`","role":"readonly"}`)
	if !strings.Contains(out, `"code":0`) {
		t.Fatalf("create readonly: %s", out)
	}
	body, _ := json.Marshal(map[string]string{"username": "watcher", "password": testCred})
	resp, err := http.Post(app.ts.URL+"/auth/session/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	for _, c := range resp.Cookies() {
		if c.Name == authsvc.SessionCookieName {
			raw, _ := io.ReadAll(resp.Body)
			var env struct {
				Data struct {
					CSRFToken string `json:"csrfToken"`
				} `json:"data"`
			}
			_ = json.Unmarshal(raw, &env)
			return session{cookie: c.Value, csrf: env.Data.CSRFToken}
		}
	}
	t.Fatal("readonly login missing cookie")
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

// TestAppPermissionMatrix 权限矩阵与生命周期链路（ADR-018/⚠️A8）：两角色
// 查看；readonly 日志/参数读取与全部写越界 AUTH_008；admin 创建→档案登记→
// 日志/参数/建站通道可达（stub 语义）→删除→档案撤除。
func TestAppPermissionMatrix(t *testing.T) {
	app := newTestApp(t)
	admin := loginSession(t, app, "admin")
	readonly := createReadonly(t, app, admin)

	// 创建应用（admin；stub 全绿链路——SyncDispatch 同步落档）。
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.py"), []byte("pass"), 0o644); err != nil {
		t.Fatal(err)
	}
	createBody := `{"name":"nim-proxy","dir":"` + filepath.ToSlash(dir) + `","entry":"app.py",
		"startMode":"preset_fastapi","startSpec":{"module":"app","app":"app"},"listenPort":8100,"startNow":true}`
	out := post(t, app, admin, "/app/app/create", createBody)
	if !strings.Contains(out, `"code":0`) || !strings.Contains(out, "taskId") {
		t.Fatalf("create: %s", out)
	}

	// 档案登记（I-C01 同事务）；两角色 list 可看。
	lo := get(t, app, admin, "/app/app/list")
	if !strings.Contains(lo, `"name":"nim-proxy"`) || !strings.Contains(lo, `"venvSupport":true`) {
		t.Fatalf("admin list: %s", lo)
	}
	lr := get(t, app, readonly, "/app/app/list")
	if !strings.Contains(lr, `"name":"nim-proxy"`) {
		t.Fatalf("readonly list: %s", lr)
	}

	// ⚠️A8：日志/参数 admin 专属——readonly 越界 AUTH_008；admin 可达。
	if r := get(t, app, readonly, "/app/log?appId=1"); !strings.Contains(r, "AUTH_008") {
		t.Fatalf("readonly log want AUTH_008: %s", r)
	}
	if r := get(t, app, readonly, "/app/app/env?appId=1"); !strings.Contains(r, "AUTH_008") {
		t.Fatalf("readonly env want AUTH_008: %s", r)
	}
	if r := get(t, app, admin, "/app/log?appId=1&lines=500"); !strings.Contains(r, `"content":"line1\nline2"`) {
		t.Fatalf("admin log: %s", r)
	}
	if r := get(t, app, admin, "/app/app/env?appId=1"); !strings.Contains(r, `"appId":1`) {
		t.Fatalf("admin env: %s", r)
	}

	// readonly 全部写越界 AUTH_008（创建/服务操作/建站/参数）。
	if r := post(t, app, readonly, "/app/app/create", createBody); !strings.Contains(r, "AUTH_008") {
		t.Fatalf("readonly create: %s", r)
	}
	if r := post(t, app, readonly, "/app/service/stop", `{"appId":1}`); !strings.Contains(r, "AUTH_008") {
		t.Fatalf("readonly service: %s", r)
	}
	if r := post(t, app, readonly, "/app/site/link", `{"appId":1}`); !strings.Contains(r, "AUTH_008") {
		t.Fatalf("readonly site: %s", r)
	}
	if r := post(t, app, readonly, "/app/app/params", `{"appId":1,"envLines":[]}`); !strings.Contains(r, "AUTH_008") {
		t.Fatalf("readonly params: %s", r)
	}

	// CSRF 越界（无 token → AUTH_003 语义）。
	req, _ := http.NewRequest("POST", app.ts.URL+"/app/app/create", strings.NewReader(createBody))
	req.Header.Set("Cookie", authsvc.SessionCookieName+"="+admin.cookie)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", app.ts.URL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(raw), "AUTH_003") {
		t.Fatalf("csrf: %s", raw)
	}

	// 任务台账两角色可看（任务 1 已 succeeded）。
	tl := get(t, app, readonly, "/app/task/list")
	if !strings.Contains(tl, `"action":"create"`) || !strings.Contains(tl, `"succeeded"`) {
		t.Fatalf("readonly task list: %s", tl)
	}

	// 删除（purgeDir=false）→ 档案撤除；list 回空。
	out = post(t, app, admin, "/app/app/delete", `{"appId":1}`)
	if !strings.Contains(out, `"code":0`) {
		t.Fatalf("delete: %s", out)
	}
	lo = get(t, app, admin, "/app/app/list")
	if strings.Contains(lo, `"name":"nim-proxy"`) {
		t.Fatalf("post-delete list: %s", lo)
	}
}
