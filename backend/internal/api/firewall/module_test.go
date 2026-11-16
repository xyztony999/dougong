package firewall

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

	"github.com/dougong/dougong/internal/adapter/firewallx"
	"github.com/dougong/dougong/internal/adapter/netlisten"
	"github.com/dougong/dougong/internal/api"
	authapi "github.com/dougong/dougong/internal/api/auth"
	auditsvc "github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	fwsvc "github.com/dougong/dougong/internal/service/firewall"
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

// fakeGateway 是 FirewallGateway 的进程内替身（镜像 service 层测试替身语义）。
type fakeGateway struct {
	mu       sync.Mutex
	detect   firewallx.BackendInfo
	list     []firewallx.BackendRule
	listErr  error
	addRes   firewallx.AddResult
	addErr   error
	addCalls []firewallx.RuleSpec
	delCalls []string
}

func (f *fakeGateway) Detect(context.Context) (firewallx.BackendInfo, error) {
	return f.detect, nil
}

func (f *fakeGateway) ListRules(context.Context) ([]firewallx.BackendRule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.list, f.listErr
}

func (f *fakeGateway) AddRule(_ context.Context, spec firewallx.RuleSpec) (firewallx.AddResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addCalls = append(f.addCalls, spec)
	return f.addRes, f.addErr
}

func (f *fakeGateway) DeleteRule(_ context.Context, ruleKey string) (firewallx.DelResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.delCalls = append(f.delCalls, ruleKey)
	return firewallx.DelResult{PermanentOK: true, RuntimeOK: true}, nil
}

func (f *fakeGateway) writes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.addCalls) + len(f.delCalls)
}

// countingRecorder 包装真实审计服务：I-07 断言用内存计数，同时保留真库写入
// 链路（HTTP 全栈不 mock 数据库）。
type countingRecorder struct {
	inner interface {
		Record(ctx context.Context, e auditsvc.Entry) error
	}
	mu      sync.Mutex
	entries []auditsvc.Entry
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

// fakeListen 是监听端口取数替身。
type fakeListen struct{ ports []netlisten.ListenPort }

func (f *fakeListen) List(context.Context) ([]netlisten.ListenPort, error) {
	return f.ports, nil
}

// testApp 聚合全栈测试依赖（真实 :memory: SQLite，不 mock；镜像 main 装配，
// firewallx/netlisten 网关替换为 fake；面板端口固定 8443——A-W9 静态注入）。
type testApp struct {
	ts     *httptest.Server
	gw     *fakeGateway
	listen *fakeListen
	rec    *countingRecorder
}

func activeUFWInfo() firewallx.BackendInfo {
	v := "0.36.1"
	return firewallx.BackendInfo{Type: firewallx.BackendUFW, Active: true, Version: &v}
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

	gw := &fakeGateway{detect: activeUFWInfo()}
	listen := &fakeListen{}
	rec := &countingRecorder{inner: auditSvc}
	svc := fwsvc.New(gw, listen, fwsvc.StaticPanelListen(8443), rec)
	server := api.NewServer()
	server.RegisterModules(
		authapi.NewModule(authSvcObj, auditSvc, false),
		NewModule(authSvcObj, svc),
	)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	return &testApp{ts: ts, gw: gw, listen: listen, rec: rec}
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

func fwRule(ruleKey, port, proto, action string) firewallx.BackendRule {
	return firewallx.BackendRule{RuleKey: ruleKey, Port: port, Protocol: proto, Action: action, RawSpec: ruleKey}
}

// ---- 用例 ----

func TestUnauthenticated(t *testing.T) {
	app := newTestApp(t)
	_ = initWizardAndLogin(t, app) // 系统先初始化（InitGuard 先于会话校验）
	for _, path := range []string{"/firewall/backend/status", "/firewall/rule/list", "/firewall/port/list"} {
		if env := get(t, app, path, nil); codeStr(env) != "AUTH_001" {
			t.Fatalf("%s code = %s, want AUTH_001", path, codeStr(env))
		}
	}
	if env := post(t, app, "/firewall/rule/create", nil,
		map[string]any{"port": "80", "protocol": "tcp", "action": "allow"}, "x", app.ts.URL); codeStr(env) != "AUTH_001" {
		t.Fatalf("create code = %s, want AUTH_001", codeStr(env))
	}
	if env := post(t, app, "/firewall/rule/delete", nil,
		map[string]any{"ruleKey": "allow 80/tcp"}, "x", app.ts.URL); codeStr(env) != "AUTH_001" {
		t.Fatalf("delete code = %s, want AUTH_001", codeStr(env))
	}
}

func TestReadEndpointsAuthorized(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	ro := loginAndCreateReadonly(t, app, admin)

	app.gw.list = []firewallx.BackendRule{
		fwRule("allow 8443/tcp", "8443", "tcp", "allow"),
		fwRule("allow 22/tcp", "22", "tcp", "allow"),
	}
	app.listen.ports = []netlisten.ListenPort{
		{Proto: "tcp", Addr: "0.0.0.0", Port: 8443, Pid: 1, Name: "dougong"},
		{Proto: "tcp", Addr: "127.0.0.1", Port: 22},
	}

	for name, sess := range map[string]session{"admin": admin, "readonly": ro} {
		// status：即时探测结论 + 权威动作面（D-01）。
		env := get(t, app, "/firewall/backend/status", &sess)
		wantCode(t, env, "0")
		var st struct {
			Backend          string   `json:"backend"`
			Active           bool     `json:"active"`
			Version          *string  `json:"version"`
			SupportedActions []string `json:"supportedActions"`
			GeneratedAt      string   `json:"generatedAt"`
		}
		if err := json.Unmarshal(env.Data, &st); err != nil {
			t.Fatalf("%s status data: %v", name, err)
		}
		if st.Backend != "ufw" || !st.Active || st.Version == nil || *st.Version != "0.36.1" {
			t.Fatalf("%s status = %+v", name, st)
		}
		if len(st.SupportedActions) != 4 {
			t.Fatalf("%s supportedActions = %v（ufw 全四值）", name, st.SupportedActions)
		}

		// rule/list：全量投影 + isPanelPort 标注 + backend 摘要 + 排序。
		env = get(t, app, "/firewall/rule/list?action=allow", &sess)
		wantCode(t, env, "0")
		var rl struct {
			Items []struct {
				RuleKey     string  `json:"ruleKey"`
				Port        string  `json:"port"`
				Protocol    string  `json:"protocol"`
				Direction   string  `json:"direction"`
				Source      *string `json:"source"`
				Action      string  `json:"action"`
				RawSpec     string  `json:"rawSpec"`
				IsPanelPort bool    `json:"isPanelPort"`
			} `json:"items"`
			Total   int `json:"total"`
			Backend struct {
				Backend string `json:"backend"`
				Active  bool   `json:"active"`
			} `json:"backend"`
			GeneratedAt string `json:"generatedAt"`
		}
		if err := json.Unmarshal(env.Data, &rl); err != nil {
			t.Fatalf("%s list data: %v", name, err)
		}
		if rl.Total != 2 || rl.Items[0].RuleKey != "allow 22/tcp" || rl.Items[1].RuleKey != "allow 8443/tcp" {
			t.Fatalf("%s list 排序/数量错位: %+v", name, rl)
		}
		if !rl.Items[1].IsPanelPort || rl.Items[0].IsPanelPort {
			t.Fatalf("%s isPanelPort 标注错位（面板端口 8443）: %+v", name, rl.Items)
		}
		if rl.Items[0].Direction != "ingress" || rl.Items[0].Source != nil {
			t.Fatalf("%s 契约字段错位: %+v", name, rl.Items[0])
		}
		if rl.Backend.Backend != "ufw" || !rl.Backend.Active {
			t.Fatalf("%s backend 摘要错位: %+v", name, rl.Backend)
		}

		// port/list：对照 + external + process。
		env = get(t, app, "/firewall/port/list", &sess)
		wantCode(t, env, "0")
		var pl struct {
			Items []struct {
				Proto   string `json:"proto"`
				Addr    string `json:"addr"`
				Port    int    `json:"port"`
				Process *struct {
					Pid  *int    `json:"pid"`
					Name *string `json:"name"`
				} `json:"process"`
				External  bool   `json:"external"`
				OpenState string `json:"openState"`
			} `json:"items"`
			Total int `json:"total"`
		}
		if err := json.Unmarshal(env.Data, &pl); err != nil {
			t.Fatalf("%s port data: %v", name, err)
		}
		if pl.Total != 2 {
			t.Fatalf("%s port total = %d: %+v", name, pl.Total, pl)
		}
		p8443, p22 := pl.Items[0], pl.Items[1]
		if p8443.OpenState != "allowed_by_rule" || !p8443.External || p8443.Process == nil ||
			p8443.Process.Pid == nil || *p8443.Process.Pid != 1 {
			t.Fatalf("%s 8443 对照错位: %+v", name, p8443)
		}
		if p22.OpenState != "allowed_by_rule" || p22.External || p22.Process != nil {
			t.Fatalf("%s 22 对照错位（allow 22/tcp 命中+loopback 非对外+process null）: %+v", name, p22)
		}
	}
}

func TestListGatesAndFilters(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)

	if env := get(t, app, "/firewall/rule/list?action=bogus", &admin); codeStr(env) != "FW_001" {
		t.Fatalf("bad filter code = %s, want FW_001", codeStr(env))
	}
	app.gw.mu.Lock()
	app.gw.detect = firewallx.BackendInfo{Type: firewallx.BackendNone}
	app.gw.mu.Unlock()
	if env := get(t, app, "/firewall/rule/list", &admin); codeStr(env) != "FW_009" {
		t.Fatalf("none code = %s, want FW_009（不返回空规则列表，I-01）", codeStr(env))
	}
	app.gw.mu.Lock()
	app.gw.detect = firewallx.BackendInfo{Type: firewallx.BackendFirewalld, Active: false}
	app.gw.mu.Unlock()
	if env := get(t, app, "/firewall/port/list", &admin); codeStr(env) != "FW_010" {
		t.Fatalf("inactive code = %s, want FW_010", codeStr(env))
	}
	app.gw.mu.Lock()
	app.gw.detect = activeUFWInfo()
	app.gw.listErr = errors.New("ufw status: exit 1: You need to be root")
	app.gw.mu.Unlock()
	if env := get(t, app, "/firewall/rule/list", &admin); codeStr(env) != "FW_008" {
		t.Fatalf("exec fail code = %s, want FW_008", codeStr(env))
	}
}

func TestReadonlyBoundary(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	ro := loginAndCreateReadonly(t, app, admin)

	// readonly 写越界：AUTH_008（ADR-018 边界拒绝；带合法 CSRF 证明到达角色层）。
	if env := post(t, app, "/firewall/rule/create", &ro,
		map[string]any{"port": "80", "protocol": "tcp", "action": "allow"}, ro.csrf, app.ts.URL); codeStr(env) != "AUTH_008" {
		t.Fatalf("readonly create code = %s, want AUTH_008", codeStr(env))
	}
	if env := post(t, app, "/firewall/rule/delete", &ro,
		map[string]any{"ruleKey": "allow 80/tcp"}, ro.csrf, app.ts.URL); codeStr(env) != "AUTH_008" {
		t.Fatalf("readonly delete code = %s, want AUTH_008", codeStr(env))
	}
	if w := app.gw.writes(); w != 0 {
		t.Fatalf("越界后不应触后端命令: %d", w)
	}
	if entries := app.rec.snapshot(); len(entries) != 0 {
		t.Fatalf("越界拦截（中间件层）不落业务审计: %+v", entries)
	}
}

func TestCsrfGuardOnWrites(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)

	// 缺 CSRF 头 → AUTH_003。
	if env := post(t, app, "/firewall/rule/create", &admin,
		map[string]any{"port": "80", "protocol": "tcp", "action": "allow"}, "", app.ts.URL); codeStr(env) != "AUTH_003" {
		t.Fatalf("no csrf code = %s, want AUTH_003", codeStr(env))
	}
	// 错误 token → AUTH_003。
	if env := post(t, app, "/firewall/rule/create", &admin,
		map[string]any{"port": "80", "protocol": "tcp", "action": "allow"}, "wrong", app.ts.URL); codeStr(env) != "AUTH_003" {
		t.Fatalf("bad csrf code = %s, want AUTH_003", codeStr(env))
	}
	// 缺 Origin → AUTH_003。
	if env := post(t, app, "/firewall/rule/create", &admin,
		map[string]any{"port": "80", "protocol": "tcp", "action": "allow"}, admin.csrf, ""); codeStr(env) != "AUTH_003" {
		t.Fatalf("no origin code = %s, want AUTH_003", codeStr(env))
	}
	if w := app.gw.writes(); w != 0 {
		t.Fatalf("CSRF 拦截后不应触后端命令: %d", w)
	}
}

func TestCreateFlowOverHTTP(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	app.gw.mu.Lock()
	app.gw.addRes = firewallx.AddResult{RuleKey: "deny 8443/tcp", PermanentOK: true, RuntimeOK: true}
	app.gw.mu.Unlock()

	// 涉险未确认 → FW_005 + data 命中详情；零审计（I-05/I-07）。
	env := post(t, app, "/firewall/rule/create", &admin,
		map[string]any{"port": "8443", "protocol": "tcp", "action": "deny"}, admin.csrf, app.ts.URL)
	wantCode(t, env, "FW_005")
	var dd struct {
		PanelPort int    `json:"panelPort"`
		RulePort  string `json:"rulePort"`
		Action    string `json:"action"`
		Hint      string `json:"hint"`
	}
	if err := json.Unmarshal(env.Data, &dd); err != nil || dd.PanelPort != 8443 || dd.RulePort != "8443" || dd.Action != "deny" || dd.Hint == "" {
		t.Fatalf("FW_005 data = %+v (%v)", dd, err)
	}
	if w := app.gw.writes(); w != 0 {
		t.Fatalf("FW_005 拒绝不应触后端命令")
	}
	if entries := app.rec.snapshot(); len(entries) != 0 {
		t.Fatalf("FW_005 拒绝未发生变更不记审计（I-07）: %+v", entries)
	}

	// 确认后重提交 → 成功（写入后定位引用）。
	env = post(t, app, "/firewall/rule/create", &admin,
		map[string]any{"port": "8443", "protocol": "tcp", "action": "deny", "confirmDanger": true}, admin.csrf, app.ts.URL)
	wantCode(t, env, "0")
	var cd struct {
		RuleKey  string `json:"ruleKey"`
		Port     string `json:"port"`
		Protocol string `json:"protocol"`
		Action   string `json:"action"`
	}
	if err := json.Unmarshal(env.Data, &cd); err != nil || cd.RuleKey != "deny 8443/tcp" ||
		cd.Port != "8443" || cd.Protocol != "tcp" || cd.Action != "deny" {
		t.Fatalf("create data = %+v (%v)", cd, err)
	}

	// 参数校验失败 → FW_001 + 一条 failure 审计（A-W11）。
	env = post(t, app, "/firewall/rule/create", &admin,
		map[string]any{"port": "70000", "protocol": "tcp", "action": "allow"}, admin.csrf, app.ts.URL)
	wantCode(t, env, "FW_001")

	entries := app.rec.snapshot()
	if len(entries) != 2 {
		t.Fatalf("audit entries = %d, want 2（FW_005 零条 + success 1 条 + FW_001 failure 1 条）", len(entries))
	}
	if entries[0].Action != fwsvc.AuditActionRuleCreated || entries[0].Result != auditsvc.ResultSuccess ||
		entries[0].ObjectType != fwsvc.ObjectTypeFirewallRule || entries[0].ActorName != "admin" {
		t.Fatalf("success 审计错位: %+v", entries[0])
	}
	if entries[1].Result != auditsvc.ResultFailure {
		t.Fatalf("failure 审计错位: %+v", entries[1])
	}
	if d, ok := entries[0].Detail.(map[string]any); !ok || d["port"] != "8443" || d["confirmDanger"] != true {
		t.Fatalf("create detail 契约字段（port/protocol/source/action/confirmDanger）: %+v", entries[0].Detail)
	}

	// 畸形 JSON body → AUTH_012（A-S9 同法）。
	req, err := http.NewRequest("POST", app.ts.URL+"/firewall/rule/create", bytes.NewReader([]byte("{broken")))
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
}

func TestDeleteFlowOverHTTP(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	app.gw.mu.Lock()
	app.gw.list = []firewallx.BackendRule{fwRule("allow 8443/tcp", "8443", "tcp", "allow")}
	app.gw.mu.Unlock()

	// 空 ruleKey → FW_001。
	if env := post(t, app, "/firewall/rule/delete", &admin,
		map[string]any{"ruleKey": ""}, admin.csrf, app.ts.URL); codeStr(env) != "FW_001" {
		t.Fatalf("empty ruleKey code = %s, want FW_001", codeStr(env))
	}
	// 不存在 → FW_003（实况复核，不静默成功）。
	if env := post(t, app, "/firewall/rule/delete", &admin,
		map[string]any{"ruleKey": "allow 9999/tcp"}, admin.csrf, app.ts.URL); codeStr(env) != "FW_003" {
		t.Fatalf("missing rule code = %s, want FW_003", codeStr(env))
	}
	// allow 族命中面板端口未确认 → FW_005。
	if env := post(t, app, "/firewall/rule/delete", &admin,
		map[string]any{"ruleKey": "allow 8443/tcp"}, admin.csrf, app.ts.URL); codeStr(env) != "FW_005" {
		t.Fatalf("panel port delete code = %s, want FW_005", codeStr(env))
	}
	// 确认后成功。
	env := post(t, app, "/firewall/rule/delete", &admin,
		map[string]any{"ruleKey": "allow 8443/tcp", "confirmDanger": true}, admin.csrf, app.ts.URL)
	wantCode(t, env, "0")
	var dd struct {
		Deleted bool   `json:"deleted"`
		RuleKey string `json:"ruleKey"`
	}
	if err := json.Unmarshal(env.Data, &dd); err != nil || !dd.Deleted || dd.RuleKey != "allow 8443/tcp" {
		t.Fatalf("delete data = %+v (%v)", dd, err)
	}

	// 审计：FW_001/FW_003 各一条 failure + FW_005 零条 + 成功一条（I-07）。
	entries := app.rec.snapshot()
	if len(entries) != 3 {
		t.Fatalf("audit entries = %d, want 3: %+v", len(entries), entries)
	}
	last := entries[len(entries)-1]
	if last.Action != fwsvc.AuditActionRuleDeleted || last.Result != auditsvc.ResultSuccess ||
		last.ObjectID != "allow 8443/tcp" {
		t.Fatalf("delete success 审计错位: %+v", last)
	}
	if d, ok := last.Detail.(map[string]any); !ok || d["ruleKey"] != "allow 8443/tcp" {
		t.Fatalf("delete detail 契约字段（ruleKey）: %+v", last.Detail)
	}
}
