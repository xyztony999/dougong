// db-tools api 全栈 httptest（env-mgmt module_test 同法）：权限矩阵（两角色
// 查看/admin 变更+CSRF/⚠️A10 工具访问 admin 专属）+ 内部端点防护链回归
// （loopback/秘密头裸 404、gating 无会话/非 admin 裸 401、signon 防伪包络）
// + reopen 302 载体。网关替身带锁（worker goroutine 与请求并发安全）。
package dbtools

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/adapter/nginxx"
	"github.com/dougong/dougong/internal/adapter/phpfpm"
	"github.com/dougong/dougong/internal/adapter/sysd"
	"github.com/dougong/dougong/internal/api"
	authapi "github.com/dougong/dougong/internal/api/auth"
	"github.com/dougong/dougong/internal/api/middleware"
	auditsvc "github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	dbmgmtsvc "github.com/dougong/dougong/internal/service/dbmgmt"
	dbtoolssvc "github.com/dougong/dougong/internal/service/dbtools"
	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	storedbt "github.com/dougong/dougong/internal/store/dbtools"
	"github.com/gin-gonic/gin"
)

const testCred = "dougong-dbtools-test-only"

// ---- 并发安全替身（单锁覆盖全部状态）----

type apiFakes struct {
	mu        sync.Mutex
	files     map[string][]byte
	confFiles map[string]string
	units     map[string]sysd.UnitDetail
	ready     map[int64]dbmgmtsvc.ReadyInfo
	creds     map[int64]dbmgmtsvc.ToolCreds
}

func newAPIFakes() *apiFakes {
	conf := `server {
    listen 80;
    server_name panel.example.test;
    location / {
        proxy_pass http://127.0.0.1:8687;
    }
}
`
	return &apiFakes{
		files:     map[string][]byte{},
		confFiles: map[string]string{"/etc/nginx/conf.d/panel.conf": conf},
		units:     map[string]sysd.UnitDetail{},
		ready: map[int64]dbmgmtsvc.ReadyInfo{
			9: {Ready: true, Engine: "mysql"}, 7: {Ready: true, Engine: "mongodb"}},
		creds: map[int64]dbmgmtsvc.ToolCreds{
			9: {Username: "root", Password: "mysql-pass", Host: "127.0.0.1", Port: 3306, Engine: "mysql"},
			7: {Username: "root", Password: "mongo-pass", Host: "127.0.0.1", Port: 27017, Engine: "mongodb"},
		},
	}
}

func (f *apiFakes) DownloadTarball(_ context.Context, _, _ string, _ time.Duration) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files["/tmp/dl.tar.gz"] = []byte("tarball")
	return "/tmp/dl.tar.gz", nil
}

func (f *apiFakes) VerifySha256(_, _ string) (bool, error) { return true, nil }

func (f *apiFakes) ExtractTar(_, dest string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[filepath.Join(dest, "index.php")] = []byte("<?php")
	return nil
}

func (f *apiFakes) NpmInstall(_ context.Context, _, _, _ string, _ time.Duration) (string, error) {
	return "ok", nil
}

func (f *apiFakes) NpmRemove(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "ok", nil
}

func (f *apiFakes) NpmGlobalRoot(_ context.Context) (string, error) {
	return "/usr/lib/node_modules", nil
}

func (f *apiFakes) PhpExtensions(_ context.Context) ([]string, error) {
	return []string{"mysqli", "mbstring", "zip", "gd", "json", "curl", "openssl"}, nil
}

func (f *apiFakes) WriteSecretFile(path string, content []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[path] = content
	return nil
}

func (f *apiFakes) WriteFile(path string, content []byte, _ fs.FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[path] = content
	return nil
}

func (f *apiFakes) ReadFile(path string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if b, ok := f.files[path]; ok {
		return b, nil
	}
	return nil, &os.PathError{Op: "read", Path: path, Err: os.ErrNotExist}
}

func (f *apiFakes) MkdirAll(_ string, _ fs.FileMode) error { return nil }

func (f *apiFakes) Remove(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.files, path)
	return nil
}

func (f *apiFakes) RemoveAll(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for p := range f.files {
		if strings.HasPrefix(p, path) {
			delete(f.files, p)
		}
	}
	return nil
}

func (f *apiFakes) Rename(_, _ string) error { return nil }

func (f *apiFakes) ProbePortAvailable(_ int) (bool, error) { return true, nil }

func (f *apiFakes) ConfigApply(_ context.Context, file, content string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.confFiles[file] = content
	return nil
}

func (f *apiFakes) ConfigRemove(_ context.Context, _ string) error { return nil }

func (f *apiFakes) EnvStatus(_ context.Context, _ string) (bool, string, bool) {
	return true, "1.0", true
}

func (f *apiFakes) GetUnit(_ context.Context, name string) (sysd.UnitDetail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if d, ok := f.units[name]; ok {
		return d, nil
	}
	return sysd.UnitDetail{LoadState: "not-found"}, nil
}

func (f *apiFakes) StartUnit(_ context.Context, name string) (sysd.JobResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.units[name] = sysd.UnitDetail{LoadState: "loaded", ActiveState: "active"}
	return sysd.JobResult{Succeeded: true}, nil
}

func (f *apiFakes) StopUnit(_ context.Context, name string) (sysd.JobResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.units[name] = sysd.UnitDetail{LoadState: "loaded", ActiveState: "inactive"}
	return sysd.JobResult{Succeeded: true}, nil
}

func (f *apiFakes) RestartUnit(_ context.Context, _ string) (sysd.JobResult, error) {
	return sysd.JobResult{Succeeded: true}, nil
}

func (f *apiFakes) EnableUnit(_ context.Context, _ string) error  { return nil }
func (f *apiFakes) DisableUnit(_ context.Context, _ string) error { return nil }
func (f *apiFakes) DaemonReload(_ context.Context) error          { return nil }

func (f *apiFakes) ListPools(_ context.Context) ([]phpfpm.Pool, error) {
	return []phpfpm.Pool{{Name: "www", Socket: "/run/php/php8.1-fpm.sock"}}, nil
}

func (f *apiFakes) ScanConfDirs() ([]nginxx.FileSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []nginxx.FileSnapshot
	for p, c := range f.confFiles {
		out = append(out, nginxx.FileSnapshot{Path: p, Dir: filepath.Dir(p), Name: filepath.Base(p), Content: c})
	}
	return out, nil
}

func (f *apiFakes) ReadFileConf(path string) (string, time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.confFiles[path]; ok {
		return c, time.Now(), nil
	}
	return "", time.Time{}, &os.PathError{Op: "read", Path: path, Err: os.ErrNotExist}
}

func (f *apiFakes) PanelAccess(_ context.Context) (dbtoolssvc.PanelAccess, error) {
	return dbtoolssvc.PanelAccess{BindDomain: "panel.example.test", ListenHost: "127.0.0.1", ListenPort: 8687}, nil
}

func (f *apiFakes) InstanceReadyForTool(_ context.Context, id int64) (dbmgmtsvc.ReadyInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.ready[id]; ok {
		return r, nil
	}
	return dbmgmtsvc.ReadyInfo{Ready: false, Reason: "未纳管"}, nil
}

func (f *apiFakes) InstanceCredentialsForTool(_ context.Context, id int64) (dbmgmtsvc.ToolCreds, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.creds[id]; ok {
		return c, nil
	}
	return dbmgmtsvc.ToolCreds{}, os.ErrNotExist
}

// nginxAdapter 把 ReadFileConf 适配为 NginxConfReader.ReadFile 签名。
type nginxAdapter struct{ f *apiFakes }

func (a nginxAdapter) ScanConfDirs() ([]nginxx.FileSnapshot, error) { return a.f.ScanConfDirs() }
func (a nginxAdapter) ReadFile(p string) (string, time.Time, error) { return a.f.ReadFileConf(p) }
func (a nginxAdapter) FirstWritableDir() (string, error)            { return "/etc/nginx/conf.d", nil }

// ---- 测试装配 ----

type testApp struct {
	ts      *httptest.Server
	handler http.Handler // gin 引擎裸 handler（全局包装器回归用）
	svc     *dbtoolssvc.Service
	store   *storedbt.Store
	fakes   *apiFakes
}

type session struct{ cookie, csrf string }

func newTestApp(t *testing.T) *testApp {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	repo := storeauth.New(db)
	if err := repo.Init().EnsureRow(ctx); err != nil {
		t.Fatal(err)
	}
	hasher, err := authsvc.NewBcryptHasher(10)
	if err != nil {
		t.Fatal(err)
	}
	authSvcObj := authsvc.New(repo, hasher, authsvc.NewTotpService(nil), authsvc.DefaultSessionPolicy())
	auditSvc := auditsvc.New(repo.Audit(), time.Now)
	f := newAPIFakes()
	dataDir := t.TempDir()
	vault, err := dbtoolssvc.OpenToolVault(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	st := storedbt.New(db)
	svc := dbtoolssvc.New(st, vault, dbtoolssvc.Deps{
		Deploy: f, Creds: f, Pipeline: f, Env: f, Sysd: f, PhpFpm: f,
		Nginx: nginxAdapter{f}, Panel: f, Session: authSvcObj, Audit: auditSvc,
	}, dbtoolssvc.Options{DataDir: dataDir})
	server := api.NewServer()
	server.RegisterModules(
		authapi.NewModule(authSvcObj, auditSvc, false),
		NewModule(authSvcObj, svc, false),
	)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	return &testApp{ts: ts, handler: server.Handler(), svc: svc, store: st, fakes: f}
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

func post(t *testing.T, app *testApp, s session, path, body string) string {
	t.Helper()
	req, _ := http.NewRequest("POST", app.ts.URL+path, bytes.NewReader([]byte(body)))
	req.Header.Set("Cookie", authsvc.SessionCookieName+"="+s.cookie)
	req.Header.Set("X-CSRF-Token", s.csrf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", app.ts.URL)
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

// ---- 权限矩阵（§3.3）----

// TestReadEndpointsBothRoles 查看类两角色同等（ADR-018）。
func TestReadEndpointsBothRoles(t *testing.T) {
	app := newTestApp(t)
	admin := loginSession(t, app, "admin")
	ro := createReadonly(t, app, admin)
	for _, s := range []session{admin, ro} {
		for _, path := range []string{"/dbtool/tool/list", "/dbtool/task/list"} {
			out := get(t, app, s, path)
			if !strings.Contains(out, `"code":0`) {
				t.Fatalf("%s as %s: %s", path, s.cookie[:8], out)
			}
		}
	}
	// 未登录 → AUTH_001。
	req, _ := http.NewRequest("GET", app.ts.URL+"/dbtool/tool/list", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(raw), "AUTH_001") {
		t.Fatalf("unauthenticated: %s", raw)
	}
}

// TestWriteEndpointsAdminOnly readonly 越界 AUTH_008；缺 CSRF AUTH_003。
func TestWriteEndpointsAdminOnly(t *testing.T) {
	app := newTestApp(t)
	admin := loginSession(t, app, "admin")
	ro := createReadonly(t, app, admin)
	writes := []struct{ path, body string }{
		{"/dbtool/tool/install", `{"toolType":"phpmyadmin"}`},
		{"/dbtool/tool/upgrade", `{"toolType":"phpmyadmin"}`},
		{"/dbtool/tool/uninstall", `{"toolType":"phpmyadmin"}`},
		{"/dbtool/access/open", `{"toolType":"phpmyadmin","instanceId":9}`},
		{"/dbtool/access/rotate", `{"toolType":"mongo_express"}`},
	}
	for _, w := range writes {
		out := post(t, app, ro, w.path, w.body)
		if !strings.Contains(out, "AUTH_008") {
			t.Fatalf("readonly %s: %s", w.path, out)
		}
	}
	// admin 缺 CSRF 头 → AUTH_003。
	req, _ := http.NewRequest("POST", app.ts.URL+"/dbtool/tool/install",
		bytes.NewReader([]byte(`{"toolType":"phpmyadmin"}`)))
	req.Header.Set("Cookie", authsvc.SessionCookieName+"="+admin.cookie)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", app.ts.URL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(raw), "AUTH_003") {
		t.Fatalf("missing csrf: %s", raw)
	}
	// admin 非法 toolType → DBT_002（不触发 worker）。
	out := post(t, app, admin, "/dbtool/tool/install", `{"toolType":"nope"}`)
	if !strings.Contains(out, "DBT_002") {
		t.Fatalf("invalid type: %s", out)
	}
	// 未装升级/卸载/打开/轮换 → DBT_004/009。
	if out := post(t, app, admin, "/dbtool/tool/upgrade", `{"toolType":"phpmyadmin"}`); !strings.Contains(out, "DBT_004") {
		t.Fatalf("upgrade not installed: %s", out)
	}
	if out := post(t, app, admin, "/dbtool/access/rotate", `{"toolType":"phpmyadmin"}`); !strings.Contains(out, "DBT_009") {
		t.Fatalf("rotate phpmyadmin: %s", out)
	}
}

// ---- 内部端点防护链（D-11 安全底线回归）----

// seedInternalSecret 预置内部秘密（安装前测试防护链用）。
func seedInternalSecret(t *testing.T, app *testApp) string {
	t.Helper()
	const secret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	app.fakes.mu.Lock()
	app.fakes.files[filepath.Join(app.svc.ToolDir(), dbtoolssvc.InternalSecretName)] = []byte(secret)
	app.fakes.mu.Unlock()
	return secret
}

func internalGet(t *testing.T, app *testApp, path, secret, cookie string) int {
	t.Helper()
	req, _ := http.NewRequest("GET", app.ts.URL+path, nil)
	if secret != "" {
		req.Header.Set(dbtoolssvc.InternalSecretHeader, secret)
	}
	if cookie != "" {
		req.Header.Set("Cookie", authsvc.SessionCookieName+"="+cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

// TestGateVerifyGuard gating 回归：秘密头缺失/错误→裸 404；无会话/readonly
// →裸 401；admin→裸 200（auth_request 语义，无包络）。
func TestGateVerifyGuard(t *testing.T) {
	app := newTestApp(t)
	admin := loginSession(t, app, "admin")
	ro := createReadonly(t, app, admin)
	secret := seedInternalSecret(t, app)

	if code := internalGet(t, app, "/dbtool/gate/verify", "", admin.cookie); code != 404 {
		t.Fatalf("missing secret header: %d, want 404", code)
	}
	if code := internalGet(t, app, "/dbtool/gate/verify", "wrong-secret", admin.cookie); code != 404 {
		t.Fatalf("wrong secret: %d, want 404", code)
	}
	if code := internalGet(t, app, "/dbtool/gate/verify", secret, ""); code != 401 {
		t.Fatalf("no session: %d, want 401", code)
	}
	if code := internalGet(t, app, "/dbtool/gate/verify", secret, ro.cookie); code != 401 {
		t.Fatalf("readonly session: %d, want 401（⚠️A10 admin 专属）", code)
	}
	if code := internalGet(t, app, "/dbtool/gate/verify", secret, "invalid-cookie"); code != 401 {
		t.Fatalf("invalid session: %d, want 401", code)
	}
	if code := internalGet(t, app, "/dbtool/gate/verify", secret, admin.cookie); code != 200 {
		t.Fatalf("admin session: %d, want 200", code)
	}
}

// TestSignonVerifyGuard signon 防护链：秘密头缺失→裸 404；伪造令牌→包络
// DBT_007（HTTP 200）；凭据永不进浏览器（响应仅 loopback 通道）。
func TestSignonVerifyGuard(t *testing.T) {
	app := newTestApp(t)
	secret := seedInternalSecret(t, app)
	body := `{"token":"forged.token","session":"x","instanceCookie":"9"}`
	// 缺秘密头 → 裸 404。
	req, _ := http.NewRequest("POST", app.ts.URL+"/dbtool/signon/verify", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("missing secret: %d want 404 (body=%s)", resp.StatusCode, raw)
	}
	// 携秘密头 + 伪造令牌 → DBT_007 包络。
	req, _ = http.NewRequest("POST", app.ts.URL+"/dbtool/signon/verify", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(dbtoolssvc.InternalSecretHeader, secret)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(raw), "DBT_007") {
		t.Fatalf("forged token: %d %s", resp.StatusCode, raw)
	}
	if strings.Contains(string(raw), "password") && strings.Contains(string(raw), "mysql-pass") {
		t.Fatalf("credentials leaked: %s", raw)
	}
}

// TestReopenRedirects reopen 302 载体：无会话→登录页；readonly→工具页；
// admin 未装→工具页（浏览器导航形态，不返回 JSON）。
func TestReopenRedirects(t *testing.T) {
	app := newTestApp(t)
	admin := loginSession(t, app, "admin")
	ro := createReadonly(t, app, admin)
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse // 不跟随重定向
	}}
	cases := []struct {
		name, cookie, wantLocation string
	}{
		{"no-session", "", "/login"},
		{"readonly", ro.cookie, "/db-tools"},
		{"admin-not-installed", admin.cookie, "/db-tools"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", app.ts.URL+"/dbtool/access/reopen?instanceId=9", nil)
			if tc.cookie != "" {
				req.Header.Set("Cookie", authsvc.SessionCookieName+"="+tc.cookie)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			io.Copy(io.Discard, resp.Body)
			if resp.StatusCode != 302 {
				t.Fatalf("status = %d, want 302", resp.StatusCode)
			}
			if loc := resp.Header.Get("Location"); loc != tc.wantLocation {
				t.Fatalf("Location = %q, want %q", loc, tc.wantLocation)
			}
		})
	}
}

// TestInstallOpenFlow 安装→任务终态→打开全链（worker 异步创建即触发；
// 任务台账轮询同步）：open 响应 URL 附令牌 + dg_tool_instance cookie 下发。
func TestInstallOpenFlow(t *testing.T) {
	app := newTestApp(t)
	admin := loginSession(t, app, "admin")
	out := post(t, app, admin, "/dbtool/tool/install", `{"toolType":"phpmyadmin"}`)
	if !strings.Contains(out, `"code":0`) || !strings.Contains(out, `"taskId"`) {
		t.Fatalf("install: %s", out)
	}
	var created struct {
		Data struct {
			TaskID int64 `json:"taskId"`
		} `json:"data"`
	}
	json.Unmarshal([]byte(out), &created)
	// 轮询任务终态（fake 链即时完成）。
	deadline := time.Now().Add(10 * time.Second)
	for {
		detail := get(t, app, admin, "/dbtool/task/detail?taskId="+itoa(created.Data.TaskID))
		if strings.Contains(detail, `"status":"succeeded"`) {
			break
		}
		if strings.Contains(detail, `"status":"failed"`) {
			t.Fatalf("install task failed: %s", detail)
		}
		if time.Now().After(deadline) {
			t.Fatalf("task not terminal: %s", detail)
		}
		time.Sleep(20 * time.Millisecond)
	}
	// 打开：URL 附 dg_signon + tokenExpiresAt + 实例 cookie。
	req, _ := http.NewRequest("POST", app.ts.URL+"/dbtool/access/open",
		bytes.NewReader([]byte(`{"toolType":"phpmyadmin","instanceId":9}`)))
	req.Header.Set("Cookie", authsvc.SessionCookieName+"="+admin.cookie)
	req.Header.Set("X-CSRF-Token", admin.csrf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", app.ts.URL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(raw), "dg_signon=") || !strings.Contains(string(raw), "tokenExpiresAt") {
		t.Fatalf("open response: %s", raw)
	}
	var foundCookie bool
	for _, c := range resp.Cookies() {
		if c.Name == dbtoolssvc.ToolInstanceCookie && c.Value == "9" && c.HttpOnly &&
			c.Path == dbtoolssvc.ToolPathPrefixPHPMyAdmin {
			foundCookie = true
		}
	}
	if !foundCookie {
		t.Fatalf("dg_tool_instance cookie missing: %v", resp.Cookies())
	}
	// 工具列表投影已装。
	list := get(t, app, admin, "/dbtool/tool/list")
	if !strings.Contains(list, `"installed":true`) || !strings.Contains(list, `"installedVersion":"5.2.2"`) {
		t.Fatalf("tool list: %s", list)
	}
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

// guardStub 全局网关数据源桩（HostGuard+EntryPathSource 双契约）。
type guardStub struct{ domain string }

func (g guardStub) BindDomainState(context.Context) (string, error) { return g.domain, nil }
func (g guardStub) EntryPathState(context.Context) (bool, string, error) {
	return false, "", nil
}

// TestHostGuardInternalExemptFullStack v1.3 D-21 全栈回归：绑域名后全局
// Entry→Host 包装——内部 gate/signon（loopback+秘密头+会话/令牌）仍可达
// 不被误拦 404；豁免不放宽鉴权（无秘密头仍裸 404/无会话仍 401）；非内部
// 路径 Host 不匹配仍标准 404（强校验不变）。
func TestHostGuardInternalExemptFullStack(t *testing.T) {
	app := newTestApp(t)
	admin := loginSession(t, app, "admin")
	secret := seedInternalSecret(t, app)
	src := guardStub{domain: "panel.example.test"}
	wrapped := httptest.NewServer(middleware.EntryPathHandler(src, middleware.HostGuard(src, app.handler)))
	t.Cleanup(wrapped.Close)

	do := func(method, path, host, secretHdr, cookie string, body []byte) (int, string) {
		var rdr io.Reader
		if body != nil {
			rdr = bytes.NewReader(body)
		}
		req, _ := http.NewRequest(method, wrapped.URL+path, rdr)
		req.Host = host // Host≠绑定域名（IP 直连形态——子请求/回调误拦根因）
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if secretHdr != "" {
			req.Header.Set(dbtoolssvc.InternalSecretHeader, secretHdr)
		}
		if cookie != "" {
			req.Header.Set("Cookie", authsvc.SessionCookieName+"="+cookie)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(raw)
	}
	// 内部 gate：Host≠域名 + loopback + 秘密头 + admin 会话 → 200 放行。
	if code, _ := do("GET", "/dbtool/gate/verify", "127.0.0.1:8687", secret, admin.cookie, nil); code != 200 {
		t.Fatalf("绑域名后内部 gate 应豁免可达: %d, want 200", code)
	}
	// 豁免不放宽鉴权：无秘密头 → 裸 404；无会话 → 401。
	if code, _ := do("GET", "/dbtool/gate/verify", "127.0.0.1:8687", "", admin.cookie, nil); code != 404 {
		t.Fatalf("无秘密头仍裸 404（防护链不变）: %d", code)
	}
	if code, _ := do("GET", "/dbtool/gate/verify", "127.0.0.1:8687", secret, "", nil); code != 401 {
		t.Fatalf("无会话仍 401（gating 不变）: %d", code)
	}
	// 内部 signon：Host≠域名 + 秘密头 + 伪造令牌 → 包络 DBT_007（HTTP 200
	// 可达非 404——令牌四重校验不放宽）。
	code, raw := do("POST", "/dbtool/signon/verify", "127.0.0.1:8687", secret, "",
		[]byte(`{"token":"forged.token","session":"x","instanceCookie":"9"}`))
	if code != 200 || !strings.Contains(raw, "DBT_007") {
		t.Fatalf("绑域名后内部 signon 应豁免可达（防伪不变）: %d %s", code, raw)
	}
	// 非内部路径 Host 不匹配仍标准 404（强校验不变，含 IP 直连面板）。
	if code, _ := do("GET", "/dbtool/tool/list", "127.0.0.1:8687", "", admin.cookie, nil); code != 404 {
		t.Fatalf("非内部路径 HostGuard 强校验不变: %d, want 404", code)
	}
	// 非内部匹配 Host 照常放行。
	if code, _ := do("GET", "/dbtool/tool/list", "panel.example.test", "", admin.cookie, nil); code != 200 {
		t.Fatalf("匹配 Host 非内部照常放行: %d", code)
	}
}
