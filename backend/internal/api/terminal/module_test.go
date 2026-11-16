package terminal

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/api"
	authapi "github.com/dougong/dougong/internal/api/auth"
	auditsvc "github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	terminalsvc "github.com/dougong/dougong/internal/service/terminal"
	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	"github.com/gorilla/websocket"
)

// 测试口令占位（仅 :memory: 测试库，非真实凭据）。
const testCred = "dougong-test-only"

type envelope struct {
	Code    any             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// ---------- 网关替身（api 层自持；service 层替身在 terminal_test.go） ----------

type apiFakePty struct {
	mu        sync.Mutex
	outCh     chan []byte
	done      chan struct{}
	written   [][]byte
	killed    bool
	closeOnce sync.Once
}

func newApiFakePty() *apiFakePty {
	return &apiFakePty{outCh: make(chan []byte, 16), done: make(chan struct{})}
}

func (p *apiFakePty) Read(b []byte) (int, error) {
	select {
	case c := <-p.outCh:
		return copy(b, c), nil
	case <-p.done:
		return 0, io.EOF
	}
}

func (p *apiFakePty) Write(b []byte) error {
	p.mu.Lock()
	p.written = append(p.written, append([]byte(nil), b...))
	p.mu.Unlock()
	return nil
}

func (p *apiFakePty) Resize(cols, rows uint16) error { return nil }

func (p *apiFakePty) Kill() error {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.killed = true
		p.mu.Unlock()
		close(p.done)
	})
	return nil
}

func (p *apiFakePty) Wait() error { return nil }

type apiFakePtyGateway struct {
	mu     sync.Mutex
	handle *apiFakePty
	fail   bool
}

func (g *apiFakePtyGateway) Spawn(ctx context.Context, id terminalsvc.RunIdentity) (terminalsvc.PtyHandle, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.fail {
		return nil, errors.New("spawn boom")
	}
	g.handle = newApiFakePty()
	return g.handle, nil
}

func (g *apiFakePtyGateway) CurrentUser(ctx context.Context) (string, error) {
	return "paneluser", nil
}

func (g *apiFakePtyGateway) last() *apiFakePty {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.handle
}

type apiFakeUsers struct{}

func (apiFakeUsers) ListLoginShellUsers(ctx context.Context) ([]terminalsvc.SystemUser, error) {
	return []terminalsvc.SystemUser{
		{Name: "root", UID: 0, Shell: "/bin/bash"},
		{Name: "deploy", UID: 1000, Shell: "/bin/zsh"},
	}, nil
}

func (apiFakeUsers) Get(ctx context.Context, name string) (terminalsvc.SystemUser, bool) {
	users, _ := apiFakeUsers{}.ListLoginShellUsers(ctx)
	for _, u := range users {
		if u.Name == name {
			return u, true
		}
	}
	return terminalsvc.SystemUser{}, false
}

type apiFakeAudit struct {
	mu      sync.Mutex
	entries []auditsvc.Entry
	fail    bool
}

func (a *apiFakeAudit) Record(ctx context.Context, e auditsvc.Entry) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.fail {
		return errors.New("audit boom")
	}
	a.entries = append(a.entries, e)
	return nil
}

func (a *apiFakeAudit) byAction(action string) []auditsvc.Entry {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []auditsvc.Entry
	for _, e := range a.entries {
		if e.Action == action {
			out = append(out, e)
		}
	}
	return out
}

// ---------- 全栈装配 ----------

type testApp struct {
	ts      *httptest.Server
	termSvc *terminalsvc.Service
	pty     *apiFakePtyGateway
	audit   *apiFakeAudit
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

	ptyGw := &apiFakePtyGateway{}
	auditFake := &apiFakeAudit{}
	termSvc := terminalsvc.New(ptyGw, apiFakeUsers{}, auditFake, authSvcObj,
		NewPanelDirectory(authSvcObj), time.Now)

	server := api.NewServer()
	server.RegisterModules(
		authapi.NewModule(authSvcObj, auditSvc, false),
		NewModule(authSvcObj, termSvc),
	)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	return &testApp{ts: ts, termSvc: termSvc, pty: ptyGw, audit: auditFake}
}

type session struct{ cookie, csrf string }

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
	if fmt.Sprint(env.Code) != "0" {
		t.Fatalf("wizard code = %v (body=%s)", env.Code, raw)
	}
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
	if err := json.Unmarshal(raw, &env); err != nil || fmt.Sprint(env.Code) != "0" {
		t.Fatalf("login %s: env=%v body=%s", username, env.Code, raw)
	}
	for _, c := range resp.Cookies() {
		if c.Name == authsvc.SessionCookieName {
			return session{cookie: c.Value}
		}
	}
	t.Fatal("login response missing session cookie")
	return session{}
}

func createReadonly(t *testing.T, app *testApp, admin session) {
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
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil || fmt.Sprint(env.Code) != "0" {
		t.Fatalf("create readonly: code=%v body=%s", env.Code, raw)
	}
}

// dialWS 以会话 Cookie + 同源 Origin 拨号（失败时返回握手 HTTP 响应供断言）。
func dialWS(t *testing.T, app *testApp, sess *session, query string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	header := http.Header{}
	if sess != nil {
		header.Set("Cookie", authsvc.SessionCookieName+"="+sess.cookie)
	}
	header.Set("Origin", app.ts.URL)
	u := "ws" + strings.TrimPrefix(app.ts.URL, "http") + "/terminal/session/connect" + query
	conn, resp, err := websocket.DefaultDialer.Dial(u, header)
	return conn, resp, err
}

// wantReject 断言握手拒绝：HTTP 状态 + 包络错误码。
func wantReject(t *testing.T, resp *http.Response, status int, code string) {
	t.Helper()
	if resp == nil {
		t.Fatal("握手拒绝应携带 HTTP 响应")
	}
	defer resp.Body.Close()
	if resp.StatusCode != status {
		t.Fatalf("status = %d, want %d", resp.StatusCode, status)
	}
	raw, _ := io.ReadAll(resp.Body)
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("envelope decode: %v (body=%s)", err, raw)
	}
	if fmt.Sprint(env.Code) != code {
		t.Fatalf("code = %s, want %s (body=%s)", fmt.Sprint(env.Code), code, raw)
	}
}

// wsFrame 测试侧帧形态。
type wsFrame struct {
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
	Truncated bool            `json:"truncated"`
}

func readFrame(t *testing.T, conn *websocket.Conn) wsFrame {
	t.Helper()
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	var f wsFrame
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("frame decode %s: %v", raw, err)
	}
	return f
}

// ---------- 用例 ----------

func TestUserOptionsPermissions(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	createReadonly(t, app, admin)
	readonly := login(t, app, "watcher")

	get := func(sess *session) envelope {
		req, _ := http.NewRequest("GET", app.ts.URL+"/terminal/user/options", nil)
		if sess != nil {
			req.Header.Set("Cookie", authsvc.SessionCookieName+"="+sess.cookie)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("decode: %v (body=%s)", err, raw)
		}
		return env
	}

	env := get(&admin)
	if fmt.Sprint(env.Code) != "0" {
		t.Fatalf("admin code = %v", env.Code)
	}
	var data struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
		GeneratedAt string `json:"generatedAt"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("data decode: %v", err)
	}
	if len(data.Items) != 2 || data.Items[0].Name != "root" || data.GeneratedAt == "" {
		t.Fatalf("items = %+v", data)
	}

	if env := get(&readonly); fmt.Sprint(env.Code) != "AUTH_008" {
		t.Fatalf("readonly code = %v, want AUTH_008（整域禁用 ADR-018）", env.Code)
	}
	if env := get(nil); fmt.Sprint(env.Code) != "AUTH_001" {
		t.Fatalf("未登录 code = %v, want AUTH_001", env.Code)
	}
}

func TestWSHandshakeGuard(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	createReadonly(t, app, admin)
	readonly := login(t, app, "watcher")

	// ① Origin 缺失 → AUTH_003（403）。
	_, resp, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(app.ts.URL, "http")+"/terminal/session/connect",
		http.Header{"Cookie": []string{authsvc.SessionCookieName + "=" + admin.cookie}})
	if err == nil {
		t.Fatal("无 Origin 应拒绝")
	}
	wantReject(t, resp, 403, "AUTH_003")

	// ① Origin 跨站 → AUTH_003（403）。
	_, resp, err = websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(app.ts.URL, "http")+"/terminal/session/connect",
		http.Header{
			"Cookie": []string{authsvc.SessionCookieName + "=" + admin.cookie},
			"Origin": []string{"https://evil.example"},
		})
	if err == nil {
		t.Fatal("跨站 Origin 应拒绝")
	}
	wantReject(t, resp, 403, "AUTH_003")

	// ② 无会话 Cookie → AUTH_001（401）。
	_, resp, err = dialWS(t, app, nil, "")
	if err == nil {
		t.Fatal("无 Cookie 应拒绝")
	}
	wantReject(t, resp, 401, "AUTH_001")

	// ③ readonly → AUTH_008（403，附边界原因 ADR-018）。
	ro := readonly
	_, resp, err = dialWS(t, app, &ro, "")
	if err == nil {
		t.Fatal("readonly 应拒绝")
	}
	wantReject(t, resp, 403, "AUTH_008")
}

func TestWSAdmissionRejections(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)

	tests := []struct {
		name   string
		query  string
		status int
		code   string
	}{
		{"未知身份来源", "?identitySource=who", 400, "TERM_005"},
		{"specified 缺 username", "?identitySource=specified_system_user", 400, "TERM_005"},
		{"系统用户不存在", "?identitySource=specified_system_user&username=ghost", 400, "TERM_002"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := admin
			_, resp, err := dialWS(t, app, &s, tt.query)
			if err == nil {
				t.Fatal("应拒绝握手")
			}
			wantReject(t, resp, tt.status, tt.code)
		})
	}

	// 审计不可用 → TERM_006 从严拒建（D-06）。
	app.audit.mu.Lock()
	app.audit.fail = true
	app.audit.mu.Unlock()
	s := admin
	_, resp, err := dialWS(t, app, &s, "?identitySource=panel_run_user")
	if err == nil {
		t.Fatal("审计失败应拒建")
	}
	wantReject(t, resp, 400, "TERM_006")
}

func TestWSSessionLifecycle(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)

	conn, resp, err := dialWS(t, app, &admin, "?identitySource=panel_run_user")
	if err != nil {
		t.Fatalf("dial: %v（resp=%v）", err, resp)
	}
	defer conn.Close() //nolint:errcheck

	// ready 首帧（权威值：limits=ADR-023 标准档）。
	f := readFrame(t, conn)
	if f.Type != "ready" {
		t.Fatalf("首帧 = %s, want ready", f.Type)
	}
	var ready struct {
		SessionID string `json:"sessionId"`
		Identity  struct {
			Source  string `json:"source"`
			RunUser string `json:"runUser"`
		} `json:"identity"`
		Limits struct {
			MaxConcurrent      int `json:"maxConcurrent"`
			IdleTimeoutSeconds int `json:"idleTimeoutSeconds"`
			HardLimitSeconds   int `json:"hardLimitSeconds"`
		} `json:"limits"`
	}
	if err := json.Unmarshal(f.Data, &ready); err != nil {
		t.Fatalf("ready decode: %v", err)
	}
	if ready.SessionID == "" || ready.Identity.RunUser != "paneluser" ||
		ready.Limits.MaxConcurrent != 5 || ready.Limits.IdleTimeoutSeconds != 1800 ||
		ready.Limits.HardLimitSeconds != 28800 {
		t.Fatalf("ready = %+v", ready)
	}

	// input → PTY 写入。
	input := base64.StdEncoding.EncodeToString([]byte("whoami"))
	if err := conn.WriteMessage(websocket.TextMessage,
		[]byte(`{"type":"input","data":"`+input+`"}`)); err != nil {
		t.Fatalf("write input: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if h := app.pty.last(); h != nil {
			h.mu.Lock()
			n := len(h.written)
			h.mu.Unlock()
			if n == 1 {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	h := app.pty.last()
	if h == nil {
		t.Fatal("PTY 未 spawn")
	}
	h.mu.Lock()
	written := append([][]byte(nil), h.written...)
	h.mu.Unlock()
	if len(written) != 1 || string(written[0]) != "whoami" {
		t.Fatalf("pty written = %q, want whoami", written)
	}

	// PTY 输出 → output 帧（Base64 往返）。
	h.outCh <- []byte("paneluser\n")
	f = readFrame(t, conn)
	if f.Type != "output" {
		t.Fatalf("帧 = %s, want output", f.Type)
	}
	var dataStr string
	if err := json.Unmarshal(f.Data, &dataStr); err != nil {
		t.Fatalf("output data decode: %v", err)
	}
	decoded, _ := base64.StdEncoding.DecodeString(dataStr)
	if string(decoded) != "paneluser\n" {
		t.Fatalf("output = %q", decoded)
	}

	// 客户端 close → closed 帧（reason=self_close）+ 统一清理 + 审计对。
	//（gorilla 连接读失败后不可重复读——以收帧 goroutine 递送。）
	if err := conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second)); err != nil {
		t.Fatalf("write close: %v", err)
	}
	frames := make(chan wsFrame, 8)
	go func() {
		defer close(frames)
		for {
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			f, err := readFrameSoft(conn)
			if err != nil {
				return
			}
			frames <- f
		}
	}()
	var closed *wsFrame
	for f := range frames {
		if f.Type == "closed" {
			closed = &f
			break
		}
	}
	if closed == nil {
		t.Fatal("未收到 closed 帧")
	}
	var cp struct {
		Reason          string `json:"reason"`
		ClosedAt        string `json:"closedAt"`
		DurationSeconds int64  `json:"durationSeconds"`
	}
	if err := json.Unmarshal(closed.Data, &cp); err != nil {
		t.Fatalf("closed decode: %v", err)
	}
	if cp.Reason != "self_close" || cp.ClosedAt == "" {
		t.Fatalf("closed = %+v", cp)
	}
	waitForCond(t, func() bool { return len(app.audit.byAction("terminal_session_closed")) == 1 })
	waitForCond(t, func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.killed
	})
	opened := app.audit.byAction("terminal_session_opened")
	if len(opened) != 1 {
		t.Fatalf("opened 审计 = %d, want 1（I-07）", len(opened))
	}
}

func readFrameSoft(conn *websocket.Conn) (wsFrame, error) {
	_, raw, err := conn.ReadMessage()
	if err != nil {
		return wsFrame{}, err
	}
	var f wsFrame
	if err := json.Unmarshal(raw, &f); err != nil {
		return wsFrame{}, err
	}
	return f, nil
}

func waitForCond(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timeout waiting for condition")
}

func TestWSConcurrentLimitEndToEnd(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)

	// 占满 5 个并发（保持打开）。
	conns := make([]*websocket.Conn, 0, 5)
	for i := 0; i < 5; i++ {
		conn, resp, err := dialWS(t, app, &admin, "?identitySource=panel_run_user")
		if err != nil {
			t.Fatalf("dial #%d: %v（resp=%v）", i+1, err, resp)
		}
		if f := readFrame(t, conn); f.Type != "ready" {
			t.Fatalf("dial #%d ready 缺失", i+1)
		}
		conns = append(conns, conn)
	}
	for _, c := range conns {
		defer c.Close() //nolint:errcheck
	}

	// 第 6 个 → TERM_001（400，data={limit:5,current:5}）。
	s := admin
	_, resp, err := dialWS(t, app, &s, "?identitySource=panel_run_user")
	if err == nil {
		t.Fatal("第 6 个连接应被拒")
	}
	if resp == nil {
		t.Fatal("拒绝应携带 HTTP 响应")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("envelope decode: %v (body=%s)", err, raw)
	}
	if fmt.Sprint(env.Code) != "TERM_001" {
		t.Fatalf("code = %s, want TERM_001 (body=%s)", fmt.Sprint(env.Code), raw)
	}
	var data struct {
		Limit   int `json:"limit"`
		Current int `json:"current"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil || data.Limit != 5 || data.Current != 5 {
		t.Fatalf("TERM_001 data = %s (err=%v)", env.Data, err)
	}
}
