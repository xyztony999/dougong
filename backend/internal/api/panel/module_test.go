package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/api"
	authapi "github.com/dougong/dougong/internal/api/auth"
	"github.com/dougong/dougong/internal/api/middleware"
	auditsvc "github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	panelsvc "github.com/dougong/dougong/internal/service/panel"
	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	storepanel "github.com/dougong/dougong/internal/store/panel"
)

// 测试口令占位（程序内生成语义的固定串，非真实凭据；仅 :memory: 测试库）。
const testCred = "dougong-test-only"

type envelope struct {
	Code    any             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func codeStr(env envelope) string { return fmt.Sprint(env.Code) }

func wantCode(t *testing.T, env envelope, want string) {
	t.Helper()
	if got := codeStr(env); got != want {
		t.Fatalf("code = %s, want %s (message=%s)", got, want, env.Message)
	}
}

// testApp 聚合全栈测试依赖（真实 :memory: SQLite，不 mock）。
type testApp struct {
	ts   *httptest.Server
	svc  *panelsvc.Service
	auth *authsvc.Service
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if cerr := db.Close(); cerr != nil {
			t.Errorf("close db: %v", cerr)
		}
	})
	ctx := context.Background()
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := storeauth.New(db)
	if err := repo.Init().EnsureRow(ctx); err != nil {
		t.Fatalf("ensure init row: %v", err)
	}
	hasher, err := authsvc.NewBcryptHasher(10)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	authSvcObj := authsvc.New(repo, hasher, authsvc.NewTotpService(nil), authsvc.DefaultSessionPolicy())
	auditSvc := auditsvc.New(repo.Audit(), time.Now)

	panelSvc := panelsvc.New(storepanel.New(db))
	if _, err := panelSvc.Bootstrap(ctx); err != nil {
		t.Fatalf("panel bootstrap: %v", err)
	}
	// 镜像 main 的构建期菜单登记（D-P07：登记发生在装配期）。
	panelSvc.Menus().Register(
		panelsvc.MenuItem{Key: "panel-settings", Title: "面板设置", Path: "/settings",
			Icon: "settings", Order: 900, Visibility: []string{storeauth.RoleAdmin, storeauth.RoleReadonly}},
		panelsvc.MenuItem{Key: "panel-about", Title: "关于", Path: "/about",
			Icon: "info", Order: 910, Visibility: []string{storeauth.RoleAdmin, storeauth.RoleReadonly}},
	)

	server := api.NewServer()
	server.RegisterModules(
		authapi.NewModule(authSvcObj, auditSvc, false),
		NewModule(authSvcObj, panelSvc),
	)
	// D-P03：EntryPathHandler 包裹在 gin engine 之外（全局第一环）。
	ts := httptest.NewServer(middleware.EntryPathHandler(panelSvc, server.Handler()))
	t.Cleanup(ts.Close)
	return &testApp{ts: ts, svc: panelSvc, auth: authSvcObj}
}

// session 持有登录会话凭据。
type session struct {
	cookie string
	csrf   string
}

// initWizardAndLogin 完成初始化向导（admin）。
func initWizardAndLogin(t *testing.T, app *testApp) session {
	t.Helper()
	body := map[string]string{"username": "admin", "password": testCred}
	b, _ := json.Marshal(body)
	resp, err := http.Post(app.ts.URL+"/auth/init/wizard", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("wizard: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("wizard decode: %v (body=%s)", err, raw)
	}
	wantCode(t, env, "0")
	var data struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("wizard data: %v", err)
	}
	for _, c := range resp.Cookies() {
		if c.Name == authsvc.SessionCookieName {
			return session{cookie: c.Value, csrf: data.CSRFToken}
		}
	}
	t.Fatal("wizard response missing session cookie")
	return session{}
}

// login 登录既有用户。
func login(t *testing.T, app *testApp, username string) session {
	t.Helper()
	body := map[string]string{"username": username, "password": testCred}
	b, _ := json.Marshal(body)
	resp, err := http.Post(app.ts.URL+"/auth/session/login", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("login %s: %v", username, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("login decode: %v", err)
	}
	wantCode(t, env, "0")
	var data struct {
		CSRFToken string `json:"csrfToken"`
	}
	_ = json.Unmarshal(env.Data, &data)
	for _, c := range resp.Cookies() {
		if c.Name == authsvc.SessionCookieName {
			return session{cookie: c.Value, csrf: data.CSRFToken}
		}
	}
	t.Fatal("login response missing session cookie")
	return session{}
}

// do 执行 JSON 请求并解析包络（headers 可为 nil）。
func do(t *testing.T, ts *httptest.Server, method, path string, body any, sess *session, withCSRF bool) (envelope, *http.Response) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, ts.URL+path, rd)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if sess != nil {
		req.Header.Set("Cookie", authsvc.SessionCookieName+"="+sess.cookie)
		if withCSRF {
			req.Header.Set(authsvc.CSRFHeader, sess.csrf)
			req.Header.Set("Origin", ts.URL)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode envelope %s %s: %v (body=%s)", method, path, err, raw)
	}
	return env, resp
}

// doRaw 执行请求并返回原始响应（404 无包络断言用）。
func doRaw(t *testing.T, ts *httptest.Server, method, path string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, ts.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do raw %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp, string(raw)
}

// ---- 会话与权限（D-P11 / ADR-018）----

func TestPanelEndpointsRequireSession(t *testing.T) {
	app := newTestApp(t)
	_ = initWizardAndLogin(t, app) // 初始化后才有可达语义

	for _, path := range []string{"/panel/config/detail", "/panel/menu/list", "/panel/system/version"} {
		env, resp := do(t, app.ts, "GET", path, nil, nil, false)
		if resp.StatusCode != 200 {
			t.Fatalf("%s status = %d", path, resp.StatusCode)
		}
		wantCode(t, env, "AUTH_001")
	}
}

func TestPanelReadEndpointsBothRoles(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	// 建 readonly 用户并登录。
	env, _ := do(t, app.ts, "POST", "/auth/user/create", map[string]any{
		"username": "watcher", "password": testCred, "role": "readonly",
	}, &admin, true)
	wantCode(t, env, "0")
	ro := login(t, app, "watcher")

	// detail：结构完整（readonly 全可看，含安全入口路径值——FL 附录B）。
	env, _ = do(t, app.ts, "GET", "/panel/config/detail", nil, &ro, false)
	wantCode(t, env, "0")
	var detail struct {
		Listen struct {
			Scope      string  `json:"scope"`
			Address    *string `json:"address"`
			Port       int     `json:"port"`
			EffectMode string  `json:"effectMode"`
		} `json:"listen"`
		EntryPath struct {
			Enabled    bool    `json:"enabled"`
			Path       *string `json:"path"`
			EffectMode string  `json:"effectMode"`
		} `json:"entryPath"`
		HTTPS struct {
			Mode string `json:"mode"`
			Cert *struct {
				Source string `json:"source"`
			} `json:"cert"`
		} `json:"https"`
		UpdateCheck struct {
			Enabled bool `json:"enabled"`
		} `json:"updateCheck"`
		Lang struct {
			Value     string   `json:"value"`
			Supported []string `json:"supported"`
		} `json:"lang"`
		Session struct {
			AbsoluteTimeout int64 `json:"absoluteTimeout"`
			IdleTimeout     int64 `json:"idleTimeout"`
		} `json:"session"`
		PendingRestart bool `json:"pendingRestart"`
	}
	if err := json.Unmarshal(env.Data, &detail); err != nil {
		t.Fatalf("detail data: %v (%s)", err, env.Data)
	}
	if detail.Listen.EffectMode != "restart_required" || detail.EntryPath.EffectMode != "immediate" {
		t.Fatalf("effect mode classification: %+v", detail)
	}
	if detail.HTTPS.Cert != nil || detail.PendingRestart {
		t.Fatalf("fresh state mismatch: %+v", detail)
	}
	if detail.Session.AbsoluteTimeout <= 0 || detail.Session.IdleTimeout <= 0 {
		t.Fatalf("session passthrough missing: %+v", detail.Session)
	}

	// menu/list：两角色均可达；admin 全量。
	env, _ = do(t, app.ts, "GET", "/panel/menu/list", nil, &admin, false)
	wantCode(t, env, "0")
	var menus struct {
		Items []struct {
			Key      string `json:"key"`
			Children []struct {
				Key string `json:"key"`
			} `json:"children"`
		} `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &menus); err != nil {
		t.Fatalf("menu data: %v", err)
	}
	if len(menus.Items) == 0 {
		t.Fatal("menu items empty (main registers panel menus)")
	}

	// version：三键齐备。
	env, _ = do(t, app.ts, "GET", "/panel/system/version", nil, &ro, false)
	wantCode(t, env, "0")
	var ver map[string]any
	if err := json.Unmarshal(env.Data, &ver); err != nil {
		t.Fatalf("version data: %v", err)
	}
	for _, k := range []string{"version", "gitCommit", "buildTime"} {
		if _, ok := ver[k]; !ok {
			t.Fatalf("version missing %q: %s", k, env.Data)
		}
	}
}

func TestPanelWriteGuards(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	env, _ := do(t, app.ts, "POST", "/auth/user/create", map[string]any{
		"username": "watcher", "password": testCred, "role": "readonly",
	}, &admin, true)
	wantCode(t, env, "0")
	ro := login(t, app, "watcher")

	// readonly 越界 → AUTH_008（ADR-018 边界）。
	env, _ = do(t, app.ts, "POST", "/panel/config/update", map[string]any{"lang": "zh-CN"}, &ro, true)
	wantCode(t, env, "AUTH_008")

	// 缺 CSRF → AUTH_003（R-02）。
	env, _ = do(t, app.ts, "POST", "/panel/config/update", map[string]any{"updateCheck": true}, &admin, false)
	wantCode(t, env, "AUTH_003")

	// admin + CSRF：合法更新（响应为更新后视图，结构同 detail）。
	env, _ = do(t, app.ts, "POST", "/panel/config/update", map[string]any{"updateCheck": true}, &admin, true)
	wantCode(t, env, "0")
	var updated struct {
		UpdateCheck struct {
			Enabled bool `json:"enabled"`
		} `json:"updateCheck"`
	}
	if err := json.Unmarshal(env.Data, &updated); err != nil || !updated.UpdateCheck.Enabled {
		t.Fatalf("update response wrong: %s", env.Data)
	}

	// 更新检查默认语义已切换：开关开启但版本源未装配 → PANEL_005（A-P8）。
	env, _ = do(t, app.ts, "POST", "/panel/system/update-check", nil, &admin, true)
	wantCode(t, env, "PANEL_005")

	// 关回开关 → PANEL_004（I-P05 双重门控）。
	env, _ = do(t, app.ts, "POST", "/panel/config/update", map[string]any{"updateCheck": false}, &admin, true)
	wantCode(t, env, "0")
	env, _ = do(t, app.ts, "POST", "/panel/system/update-check", nil, &admin, true)
	wantCode(t, env, "PANEL_004")
}

// ---- EntryPathGuard（D-P03：标准 404 无业务码；前缀改写路由）----

func TestEntryPathGuardOverHTTP(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)

	// 关闭态：无前缀直通。
	resp, body := doRaw(t, app.ts, "GET", "/auth/init/check")
	if resp.StatusCode != 200 || !strings.Contains(body, `"code"`) {
		t.Fatalf("disabled guard must pass through: status=%d body=%s", resp.StatusCode, body)
	}

	// 开启安全入口（自动生成路径——api 4.1.4）。
	env, _ := do(t, app.ts, "POST", "/panel/config/update", map[string]any{"entryPathEnabled": true}, &admin, true)
	wantCode(t, env, "0")
	var updated struct {
		EntryPath struct {
			Enabled bool    `json:"enabled"`
			Path    *string `json:"path"`
		} `json:"entryPath"`
	}
	if err := json.Unmarshal(env.Data, &updated); err != nil || !updated.EntryPath.Enabled || updated.EntryPath.Path == nil {
		t.Fatalf("entry path not enabled: %s", env.Data)
	}
	prefix := "/" + *updated.EntryPath.Path

	// 未携带前缀 → 标准 404（无业务码包裹，防探测）。
	resp, body = doRaw(t, app.ts, "GET", "/auth/init/check")
	if resp.StatusCode != 404 {
		t.Fatalf("no-prefix want 404, got %d", resp.StatusCode)
	}
	if strings.Contains(body, `"code"`) || strings.Contains(body, "AUTH_") {
		t.Fatalf("404 body must not carry envelope: %s", body)
	}

	// 错误前缀 → 404。
	resp, _ = doRaw(t, app.ts, "GET", "/totally-wrong/auth/init/check")
	if resp.StatusCode != 404 {
		t.Fatalf("wrong prefix want 404, got %d", resp.StatusCode)
	}

	// 正确前缀：认证接口与业务接口均按规范路径路由。
	resp, body = doRaw(t, app.ts, "GET", prefix+"/auth/init/check")
	if resp.StatusCode != 200 || !strings.Contains(body, `"code"`) {
		t.Fatalf("prefixed auth route broken: status=%d body=%s", resp.StatusCode, body)
	}
	env, _ = do(t, app.ts, "GET", prefix+"/panel/menu/list", nil, &admin, false)
	wantCode(t, env, "0")

	// 会话/Cookie/CSRF 在前缀路径下照常工作（Cookie Path=/）。
	env, _ = do(t, app.ts, "POST", prefix+"/panel/config/regenerate-entry-path", nil, &admin, true)
	wantCode(t, env, "0")
	var regen struct {
		EntryPath string `json:"entryPath"`
	}
	if err := json.Unmarshal(env.Data, &regen); err != nil || regen.EntryPath == "" {
		t.Fatalf("regenerate result wrong: %s", env.Data)
	}
	if "/"+regen.EntryPath == prefix {
		t.Fatal("regenerate must change the path")
	}

	// 旧前缀随重新生成即时失效（EntryPathGuard 下一请求即用新值）。
	resp, _ = doRaw(t, app.ts, "GET", prefix+"/auth/init/check")
	if resp.StatusCode != 404 {
		t.Fatalf("old prefix must be invalidated immediately, got %d", resp.StatusCode)
	}
	resp, body = doRaw(t, app.ts, "GET", "/"+regen.EntryPath+"/auth/init/check")
	if resp.StatusCode != 200 {
		t.Fatalf("new prefix must work, got %d", resp.StatusCode)
	}

	// 关闭开关 → 恢复直通。
	env, _ = do(t, app.ts, "POST", "/"+regen.EntryPath+"/panel/config/update",
		map[string]any{"entryPathEnabled": false}, &admin, true)
	wantCode(t, env, "0")
	resp, _ = doRaw(t, app.ts, "GET", "/auth/init/check")
	if resp.StatusCode != 200 {
		t.Fatalf("disable must restore direct access, got %d", resp.StatusCode)
	}
}

// 校验错误经 HTTP 通道保持契约符号（R-14 字段提示 + PANEL_001）。
func TestPanelValidationOverHTTP(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)

	env, _ := do(t, app.ts, "POST", "/panel/config/update", map[string]any{"listenScope": "custom"}, &admin, true)
	wantCode(t, env, "PANEL_001")

	env, _ = do(t, app.ts, "POST", "/panel/config/update", map[string]any{"lang": "en-US"}, &admin, true)
	wantCode(t, env, "PANEL_006")

	env, _ = do(t, app.ts, "POST", "/panel/config/update", map[string]any{"listenPort": 80}, &admin, true)
	wantCode(t, env, "AUTH_012")
	var data struct {
		Fields []struct {
			Field string `json:"field"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil || len(data.Fields) == 0 || data.Fields[0].Field != "listenPort" {
		t.Fatalf("AUTH_012 fields payload wrong: %s", env.Data)
	}

	// 证书上传 PANEL_002（R-14）。
	env, _ = do(t, app.ts, "POST", "/panel/config/upload-certificate",
		map[string]string{"certPem": "garbage", "keyPem": "garbage"}, &admin, true)
	wantCode(t, env, "PANEL_002")

	// 切 imported 无证书 → PANEL_003。
	env, _ = do(t, app.ts, "POST", "/panel/config/update", map[string]any{"httpsMode": "imported"}, &admin, true)
	wantCode(t, env, "PANEL_003")

	// 切 self_signed → 即时生成证书（detail 摘要可见，D-P06）。
	env, _ = do(t, app.ts, "POST", "/panel/config/update", map[string]any{"httpsMode": "self_signed"}, &admin, true)
	wantCode(t, env, "0")
	var detail struct {
		HTTPS struct {
			Mode string `json:"mode"`
			Cert *struct {
				Source string `json:"source"`
			} `json:"cert"`
		} `json:"https"`
	}
	if err := json.Unmarshal(env.Data, &detail); err != nil || detail.HTTPS.Mode != "self_signed" ||
		detail.HTTPS.Cert == nil || detail.HTTPS.Cert.Source != "self_signed" {
		t.Fatalf("self-signed over http wrong: %s", env.Data)
	}
}
