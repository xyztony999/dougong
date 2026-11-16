package servicemgmt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/adapter/sysd"
	"github.com/dougong/dougong/internal/api"
	authapi "github.com/dougong/dougong/internal/api/auth"
	auditsvc "github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	svcsvc "github.com/dougong/dougong/internal/service/servicemgmt"
	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
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

// fakeGateway 是 SysdGateway 的进程内替身（镜像 service 层测试替身语义）。
type fakeGateway struct {
	mu        sync.Mutex
	list      []sysd.UnitSnapshot
	listErr   error
	getSeq    []getResult
	jobRes    sysd.JobResult
	jobErr    error
	starts    []string
	toggles   []string
	enableErr error
}

type getResult struct {
	d   sysd.UnitDetail
	err error
}

func (f *fakeGateway) ListUnits(context.Context) ([]sysd.UnitSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.list, f.listErr
}

func (f *fakeGateway) GetUnit(_ context.Context, name string) (sysd.UnitDetail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.getSeq) > 0 {
		r := f.getSeq[0]
		f.getSeq = f.getSeq[1:]
		return r.d, r.err
	}
	return sysd.UnitDetail{
		Name: name, UnitType: sysd.UnitTypeOf(name),
		ActiveState: "active", SubState: "running", EnabledState: "enabled",
		Description: "desc of " + name, LoadState: "loaded",
	}, nil
}

func (f *fakeGateway) job(kind, name string) (sysd.JobResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts = append(f.starts, kind+":"+name)
	return f.jobRes, f.jobErr
}

func (f *fakeGateway) StartUnit(_ context.Context, name string) (sysd.JobResult, error) {
	return f.job("StartUnit", name)
}

func (f *fakeGateway) StopUnit(_ context.Context, name string) (sysd.JobResult, error) {
	return f.job("StopUnit", name)
}

func (f *fakeGateway) RestartUnit(_ context.Context, name string) (sysd.JobResult, error) {
	return f.job("RestartUnit", name)
}

func (f *fakeGateway) EnableUnit(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.toggles = append(f.toggles, "enable:"+name)
	return f.enableErr
}

func (f *fakeGateway) DisableUnit(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.toggles = append(f.toggles, "disable:"+name)
	return nil
}

func (f *fakeGateway) submitted() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.starts...)
}

// countingRecorder 包装真实审计服务：I-06 断言用内存计数，同时保留真库写入
// 链路（HTTP 全栈不 mock 数据库）。
type countingRecorder struct {
	inner   RecorderInner
	mu      sync.Mutex
	entries []auditsvc.Entry
}

// RecorderInner 是审计写入契约（*auditsvc.Service 实现）。
type RecorderInner = interface {
	Record(ctx context.Context, e auditsvc.Entry) error
}

func (r *countingRecorder) Record(ctx context.Context, e auditsvc.Entry) error {
	if err := r.inner.Record(ctx, e); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, e)
	return nil
}

func (r *countingRecorder) snapshot() []auditsvc.Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]auditsvc.Entry(nil), r.entries...)
}

// testApp 聚合全栈测试依赖（真实 :memory: SQLite，不 mock；镜像 main 装配，
// sysd 网关替换为 fake）。
type testApp struct {
	ts  *httptest.Server
	gw  *fakeGateway
	rec *countingRecorder
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

	gw := &fakeGateway{}
	rec := &countingRecorder{inner: auditSvc}
	svc := svcsvc.New(gw, rec)
	server := api.NewServer()
	server.RegisterModules(
		authapi.NewModule(authSvcObj, auditSvc, false),
		NewModule(authSvcObj, svc),
	)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	return &testApp{ts: ts, gw: gw, rec: rec}
}

// session 持有登录会话凭据。
type session struct{ cookie, csrf string }

func decodeEnv(t *testing.T, resp *http.Response) envelope {
	t.Helper()
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, raw)
	}
	return env
}

// initWizardAndLogin 完成初始化向导（admin）并返回会话。
func initWizardAndLogin(t *testing.T, app *testApp) session {
	t.Helper()
	body := map[string]string{"username": "admin", "password": testCred}
	b, _ := json.Marshal(body)
	resp, err := http.Post(app.ts.URL+"/auth/init/wizard", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("wizard: %v", err)
	}
	env := decodeEnv(t, resp)
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

// loginAndCreateReadonly 登录 admin 并创建 readonly（返回两者会话）。
func loginAndCreateReadonly(t *testing.T, app *testApp, admin session) session {
	t.Helper()
	// admin 登出态直接建 readonly 经 /auth/user/create（admin 会话来自向导）。
	body := map[string]any{"username": "watcher", "password": testCred, "role": "readonly"}
	b, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", app.ts.URL+"/auth/user/create", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", authsvc.SessionCookieName+"="+admin.cookie)
	req.Header.Set(authsvc.CSRFHeader, admin.csrf)
	req.Header.Set("Origin", app.ts.URL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create readonly: %v", err)
	}
	wantCode(t, decodeEnv(t, resp), "0")

	// readonly 登录（登录响应携带 csrfToken）。
	lb, _ := json.Marshal(map[string]string{"username": "watcher", "password": testCred})
	lreq, err := http.NewRequest("POST", app.ts.URL+"/auth/session/login", bytes.NewReader(lb))
	if err != nil {
		t.Fatalf("new login: %v", err)
	}
	lreq.Header.Set("Content-Type", "application/json")
	lresp, err := http.DefaultClient.Do(lreq)
	if err != nil {
		t.Fatalf("login readonly: %v", err)
	}
	lenv := decodeEnv(t, lresp)
	wantCode(t, lenv, "0")
	var ldata struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.Unmarshal(lenv.Data, &ldata); err != nil {
		t.Fatalf("login data: %v", err)
	}
	for _, c := range lresp.Cookies() {
		if c.Name == authsvc.SessionCookieName {
			return session{cookie: c.Value, csrf: ldata.CSRFToken}
		}
	}
	t.Fatal("login response missing session cookie")
	return session{}
}

// get 执行带会话 GET 并解析包络。
func get(t *testing.T, app *testApp, path string, sess *session) envelope {
	t.Helper()
	req, err := http.NewRequest("GET", app.ts.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if sess != nil {
		req.Header.Set("Cookie", authsvc.SessionCookieName+"="+sess.cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get %s: %v", path, err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("%s status = %d（统一 200 语义）", path, resp.StatusCode)
	}
	return decodeEnv(t, resp)
}

// post 执行带会话/CSRF/同源头的 POST 并解析包络；csrf/origin 为空即省略。
func post(t *testing.T, app *testApp, path string, sess *session, body map[string]any, csrf, origin string) envelope {
	t.Helper()
	b, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", app.ts.URL+path, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if sess != nil {
		req.Header.Set("Cookie", authsvc.SessionCookieName+"="+sess.cookie)
	}
	if csrf != "" {
		req.Header.Set(authsvc.CSRFHeader, csrf)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post %s: %v", path, err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("%s status = %d（统一 200 语义）", path, resp.StatusCode)
	}
	return decodeEnv(t, resp)
}

func svcDetail(name, enabled string) getResult {
	return getResult{d: sysd.UnitDetail{Name: name, UnitType: "service",
		ActiveState: "active", SubState: "running", EnabledState: enabled,
		Description: "desc", LoadState: "loaded"}}
}

// ---- 用例 ----

func TestUnauthenticated(t *testing.T) {
	app := newTestApp(t)
	_ = initWizardAndLogin(t, app) // 系统先初始化（InitGuard 先于会话校验）
	if env := get(t, app, "/services/unit/list", nil); codeStr(env) != "AUTH_001" {
		t.Fatalf("list code = %s, want AUTH_001", codeStr(env))
	}
	if env := get(t, app, "/services/unit/detail?name=nginx.service", nil); codeStr(env) != "AUTH_001" {
		t.Fatalf("detail code = %s, want AUTH_001", codeStr(env))
	}
	if env := post(t, app, "/services/unit/start", nil, map[string]any{"name": "nginx.service"}, "x", app.ts.URL); codeStr(env) != "AUTH_001" {
		t.Fatalf("start code = %s, want AUTH_001", codeStr(env))
	}
}

func TestListAndDetailAuthorized(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	ro := loginAndCreateReadonly(t, app, admin)

	app.gw.list = []sysd.UnitSnapshot{
		{Name: "zulu.service", UnitType: "service", ActiveState: "failed", SubState: "failed", EnabledState: "enabled"},
		{Name: "nginx.service", UnitType: "service", ActiveState: "active", SubState: "running", EnabledState: "static"},
		{Name: "backup.timer", UnitType: "timer", ActiveState: "inactive", SubState: "waiting", EnabledState: "enabled"},
	}

	for name, sess := range map[string]session{"admin": admin, "readonly": ro} {
		env := get(t, app, "/services/unit/list?state=active&search=NGINX", &sess)
		wantCode(t, env, "0")
		var data struct {
			Items       []map[string]any `json:"items"`
			Total       int              `json:"total"`
			GeneratedAt string           `json:"generatedAt"`
		}
		if err := json.Unmarshal(env.Data, &data); err != nil {
			t.Fatalf("%s list data: %v", name, err)
		}
		if data.Total != 1 || len(data.Items) != 1 {
			t.Fatalf("%s: total=%d（过滤 active+搜索 nginx 应 1 条：%+v）", name, data.Total, data.Items)
		}
		it := data.Items[0]
		if it["name"] != "nginx.service" || it["unitType"] != "service" ||
			it["activeState"] != "active" || it["subState"] != "running" || it["enabledState"] != "static" {
			t.Fatalf("%s: item 契约字段错位: %+v", name, it)
		}
		if it["canOperate"] != true || it["canToggleAutostart"] != false {
			t.Fatalf("%s: 权威计算字段错位（service+static）: %+v", name, it)
		}
		if _, err := time.Parse(time.RFC3339, data.GeneratedAt); err != nil {
			t.Fatalf("%s: generatedAt 非 RFC3339: %q", name, data.GeneratedAt)
		}

		denv := get(t, app, "/services/unit/detail?name=nginx.service", &sess)
		wantCode(t, denv, "0")
		var dd struct {
			Name        string  `json:"name"`
			Description *string `json:"description"`
			LoadState   *string `json:"loadState"`
		}
		if err := json.Unmarshal(denv.Data, &dd); err != nil {
			t.Fatalf("%s detail data: %v", name, err)
		}
		if dd.Name != "nginx.service" || dd.Description == nil || *dd.Description != "desc of nginx.service" || dd.LoadState == nil || *dd.LoadState != "loaded" {
			t.Fatalf("%s: detail 字段错位: %+v", name, dd)
		}
	}
}

func TestListParamAndChannelErrors(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)

	if env := get(t, app, "/services/unit/list?state=running", &admin); codeStr(env) != "SVC_002" {
		t.Fatalf("bad state code = %s, want SVC_002", codeStr(env))
	}
	if env := get(t, app, "/services/unit/detail?name=nginx", &admin); codeStr(env) != "SVC_001" {
		t.Fatalf("bad name code = %s, want SVC_001", codeStr(env))
	}
	app.gw.mu.Lock()
	app.gw.getSeq = []getResult{{err: errors.Join(sysd.ErrUnitNotFound)}}
	app.gw.mu.Unlock()
	if env := get(t, app, "/services/unit/detail?name=ghost.service", &admin); codeStr(env) != "SVC_003" {
		t.Fatalf("ghost code = %s, want SVC_003", codeStr(env))
	}

	app.gw.mu.Lock()
	app.gw.listErr = errors.New("boom") // 非 sentinel → INTERNAL 脱敏
	app.gw.mu.Unlock()
	env := get(t, app, "/services/unit/list", &admin)
	if codeStr(env) != "INTERNAL" {
		t.Fatalf("unknown gateway error code = %s, want INTERNAL", codeStr(env))
	}
}

func TestReadonlyBoundary(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	ro := loginAndCreateReadonly(t, app, admin)

	// readonly 动作越界：AUTH_008（ADR-018 边界拒绝；带合法 CSRF 证明到达角色层）。
	env := post(t, app, "/services/unit/start", &ro,
		map[string]any{"name": "nginx.service"}, ro.csrf, app.ts.URL)
	if codeStr(env) != "AUTH_008" {
		t.Fatalf("readonly start code = %s, want AUTH_008", codeStr(env))
	}
	if subs := app.gw.submitted(); len(subs) != 0 {
		t.Fatalf("越界后不应触 systemd: %v", subs)
	}
	if entries := app.rec.snapshot(); len(entries) != 0 {
		t.Fatalf("越界拦截（中间件层）不落业务审计: %+v", entries)
	}
}

func TestCsrfGuardOnActions(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	app.gw.getSeq = []getResult{svcDetail("nginx.service", "enabled")}

	// 缺 CSRF 头 → AUTH_003
	if env := post(t, app, "/services/unit/start", &admin,
		map[string]any{"name": "nginx.service"}, "", app.ts.URL); codeStr(env) != "AUTH_003" {
		t.Fatalf("no csrf code = %s, want AUTH_003", codeStr(env))
	}
	// 错误 token → AUTH_003
	if env := post(t, app, "/services/unit/start", &admin,
		map[string]any{"name": "nginx.service"}, "wrong-token", app.ts.URL); codeStr(env) != "AUTH_003" {
		t.Fatalf("bad csrf code = %s, want AUTH_003", codeStr(env))
	}
	// 缺 Origin → AUTH_003
	if env := post(t, app, "/services/unit/start", &admin,
		map[string]any{"name": "nginx.service"}, admin.csrf, ""); codeStr(env) != "AUTH_003" {
		t.Fatalf("no origin code = %s, want AUTH_003", codeStr(env))
	}
	if subs := app.gw.submitted(); len(subs) != 0 {
		t.Fatalf("CSRF 拦截后不应触 systemd: %v", subs)
	}
}

func TestAdminActionFlow(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)

	// start 成功（操作后快照）
	app.gw.getSeq = []getResult{
		svcDetail("nginx.service", "enabled"),
		{d: sysd.UnitDetail{Name: "nginx.service", UnitType: "service",
			ActiveState: "active", SubState: "running", EnabledState: "enabled"}},
	}
	app.gw.jobRes = sysd.JobResult{Succeeded: true, Reason: "done", ActiveState: "active", SubState: "running"}
	env := post(t, app, "/services/unit/start", &admin,
		map[string]any{"name": "nginx.service"}, admin.csrf, app.ts.URL)
	wantCode(t, env, "0")
	var ad struct {
		Name, ActiveState, SubState, EnabledState string
	}
	if err := json.Unmarshal(env.Data, &ad); err != nil {
		t.Fatalf("start data: %v", err)
	}
	if ad.Name != "nginx.service" || ad.ActiveState != "active" || ad.SubState != "running" || ad.EnabledState != "enabled" {
		t.Fatalf("start data 错位: %+v", ad)
	}

	// 超时 → SVC_006 + note
	app.gw.mu.Lock()
	app.gw.getSeq = []getResult{svcDetail("slow.service", "enabled")}
	app.gw.jobErr = svcsvcWrapTimeout()
	app.gw.mu.Unlock()
	env = post(t, app, "/services/unit/restart", &admin,
		map[string]any{"name": "slow.service"}, admin.csrf, app.ts.URL)
	if codeStr(env) != "SVC_006" {
		t.Fatalf("timeout code = %s, want SVC_006", codeStr(env))
	}
	var td struct {
		Name, Note string
	}
	if err := json.Unmarshal(env.Data, &td); err != nil || td.Note != svcsvc.TimeoutNote {
		t.Fatalf("timeout data = %+v (%v)", td, err)
	}

	// Job 失败 → SVC_007（reason+实际终态）
	app.gw.mu.Lock()
	app.gw.getSeq = []getResult{
		svcDetail("broken.service", "disabled"),
		{d: sysd.UnitDetail{Name: "broken.service", ActiveState: "failed", SubState: "failed", EnabledState: "disabled"}},
	}
	app.gw.jobErr = nil
	app.gw.jobRes = sysd.JobResult{Succeeded: false, Reason: "failed", ActiveState: "failed"}
	app.gw.mu.Unlock()
	env = post(t, app, "/services/unit/stop", &admin,
		map[string]any{"name": "broken.service"}, admin.csrf, app.ts.URL)
	if codeStr(env) != "SVC_007" {
		t.Fatalf("job fail code = %s, want SVC_007", codeStr(env))
	}
	var fd struct {
		Name, Reason string
		ActiveState  *string
	}
	if err := json.Unmarshal(env.Data, &fd); err != nil || fd.Reason != "failed" ||
		fd.ActiveState == nil || *fd.ActiveState != "failed" {
		t.Fatalf("job fail data = %+v (%v)", fd, err)
	}

	// 类型边界 → SVC_004（timer 存在）
	app.gw.mu.Lock()
	app.gw.getSeq = []getResult{{d: sysd.UnitDetail{Name: "backup.timer", UnitType: "timer",
		ActiveState: "inactive", EnabledState: "enabled"}}}
	app.gw.mu.Unlock()
	if env := post(t, app, "/services/unit/start", &admin,
		map[string]any{"name": "backup.timer"}, admin.csrf, app.ts.URL); codeStr(env) != "SVC_004" {
		t.Fatalf("timer code = %s, want SVC_004", codeStr(env))
	}

	// 自启非两态 → SVC_005（static）
	app.gw.mu.Lock()
	app.gw.getSeq = []getResult{svcDetail("static.service", "static")}
	app.gw.mu.Unlock()
	if env := post(t, app, "/services/unit/enable", &admin,
		map[string]any{"name": "static.service"}, admin.csrf, app.ts.URL); codeStr(env) != "SVC_005" {
		t.Fatalf("static enable code = %s, want SVC_005", codeStr(env))
	}

	// enable 成功（刷新值）
	app.gw.mu.Lock()
	app.gw.getSeq = []getResult{svcDetail("sshd.service", "disabled"),
		{d: sysd.UnitDetail{Name: "sshd.service", EnabledState: "enabled"}}}
	app.gw.mu.Unlock()
	env = post(t, app, "/services/unit/enable", &admin,
		map[string]any{"name": "sshd.service"}, admin.csrf, app.ts.URL)
	wantCode(t, env, "0")
	var ed struct {
		Name, EnabledState string
	}
	if err := json.Unmarshal(env.Data, &ed); err != nil || ed.Name != "sshd.service" || ed.EnabledState != "enabled" {
		t.Fatalf("enable data = %+v (%v)", ed, err)
	}

	// 畸形 JSON body → AUTH_012（A-S9）
	req, err := http.NewRequest("POST", app.ts.URL+"/services/unit/start", bytes.NewReader([]byte("{broken")))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", authsvc.SessionCookieName+"="+admin.cookie)
	req.Header.Set(authsvc.CSRFHeader, admin.csrf)
	req.Header.Set("Origin", app.ts.URL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("broken body post: %v", err)
	}
	if env := decodeEnv(t, resp); codeStr(env) != "AUTH_012" {
		t.Fatalf("broken body code = %s, want AUTH_012", codeStr(env))
	}

	// 空 name（合法 JSON）→ SVC_001（服务端形态校验唯一权威）
	if env := post(t, app, "/services/unit/start", &admin,
		map[string]any{"name": ""}, admin.csrf, app.ts.URL); codeStr(env) != "SVC_001" {
		t.Fatalf("empty name code = %s, want SVC_001", codeStr(env))
	}
}

func TestAuditExactlyOnceOverHTTP(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)

	// 成功动作 → 恰好 1 条 success 审计（含四要素；R-03）
	app.gw.getSeq = []getResult{svcDetail("nginx.service", "enabled")}
	app.gw.jobRes = sysd.JobResult{Succeeded: true, Reason: "done", ActiveState: "active", SubState: "running"}
	if env := post(t, app, "/services/unit/start", &admin,
		map[string]any{"name": "nginx.service"}, admin.csrf, app.ts.URL); codeStr(env) != "0" {
		t.Fatalf("start code = %s", codeStr(env))
	}
	// 拒绝动作 → 再 1 条 failure 审计（A-S5：guard 拒绝也记）
	if env := post(t, app, "/services/unit/start", &admin,
		map[string]any{"name": "nginx"}, admin.csrf, app.ts.URL); codeStr(env) != "SVC_001" {
		t.Fatalf("start reject code = %s", codeStr(env))
	}

	entries := app.rec.snapshot()
	if len(entries) != 2 {
		t.Fatalf("audit entries = %d, want 2（每动作接口调用恰好一条）", len(entries))
	}
	ok, fail := entries[0], entries[1]
	if ok.Action != svcsvc.AuditActionServiceStarted || ok.Result != auditsvc.ResultSuccess ||
		ok.ObjectType != svcsvc.ObjectTypeSystemdService || ok.ObjectID != "nginx.service" || ok.ActorName != "admin" {
		t.Fatalf("success 审计错位: %+v", ok)
	}
	if fail.Action != svcsvc.AuditActionServiceStarted || fail.Result != auditsvc.ResultFailure {
		t.Fatalf("failure 审计错位: %+v", fail)
	}
	if d, okk := fail.Detail.(map[string]any); !okk || d["error"] != "SVC_001" {
		t.Fatalf("failure 审计 detail = %+v, want error=SVC_001", fail.Detail)
	}
}

// svcsvcWrapTimeout 构造超时哨兵（独立函数避免与 fake 字段竞态）。
func svcsvcWrapTimeout() error { return errors.Join(sysd.ErrJobTimeout) }
