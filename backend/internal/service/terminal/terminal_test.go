package terminal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/errcode"
)

// ---------- 测试替身 ----------

// testClock 可控时钟（-race 安全）。
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTestClock() *testClock { return &testClock{now: time.Now()} }

func (c *testClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }

func (c *testClock) Advance(d time.Duration) { c.mu.Lock(); c.now = c.now.Add(d); c.mu.Unlock() }

// fakePty 记录写入/尺寸并模拟输出与退出。
type fakePty struct {
	mu        sync.Mutex
	outCh     chan []byte
	done      chan struct{}
	written   [][]byte
	resizes   [][2]uint16
	killed    bool
	waited    bool
	closeOnce sync.Once
}

func newFakePty() *fakePty {
	return &fakePty{outCh: make(chan []byte, 16), done: make(chan struct{})}
}

func (p *fakePty) Read(b []byte) (int, error) {
	select {
	case chunk := <-p.outCh:
		return copy(b, chunk), nil
	case <-p.done:
		return 0, io.EOF
	}
}

func (p *fakePty) Write(b []byte) error {
	p.mu.Lock()
	p.written = append(p.written, append([]byte(nil), b...))
	p.mu.Unlock()
	return nil
}

func (p *fakePty) Resize(cols, rows uint16) error {
	p.mu.Lock()
	p.resizes = append(p.resizes, [2]uint16{cols, rows})
	p.mu.Unlock()
	return nil
}

func (p *fakePty) Kill() error {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.killed = true
		p.mu.Unlock()
		close(p.done)
	})
	return nil
}

func (p *fakePty) Wait() error {
	p.mu.Lock()
	p.waited = true
	p.mu.Unlock()
	return nil
}

func (p *fakePty) snapshot() (killed, waited bool, written [][]byte, resizes [][2]uint16) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.killed, p.waited, p.written, p.resizes
}

// fakePtyGateway 实现 PtyGateway。
type fakePtyGateway struct {
	mu     sync.Mutex
	handle *fakePty
	fail   bool
	user   string
}

func (g *fakePtyGateway) Spawn(ctx context.Context, identity RunIdentity) (PtyHandle, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.fail {
		return nil, errors.New("spawn boom")
	}
	g.handle = newFakePty()
	return g.handle, nil
}

func (g *fakePtyGateway) CurrentUser(ctx context.Context) (string, error) {
	return g.user, nil
}

func (g *fakePtyGateway) last() *fakePty {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.handle
}

// fakeUserDir 实现 SysUserDirectory。
type fakeUserDir struct {
	users []SystemUser
	fail  bool
}

func (d *fakeUserDir) ListLoginShellUsers(ctx context.Context) ([]SystemUser, error) {
	if d.fail {
		return nil, errors.New("passwd boom")
	}
	var out []SystemUser
	for _, u := range d.users {
		if IsInteractiveShell(u.Shell) {
			out = append(out, u)
		}
	}
	return out, nil
}

func (d *fakeUserDir) Get(ctx context.Context, name string) (SystemUser, bool) {
	for _, u := range d.users {
		if u.Name == name {
			return u, true
		}
	}
	return SystemUser{}, false
}

// fakeAudit 记录写入（可注入失败）。
type fakeAudit struct {
	mu      sync.Mutex
	entries []AuditEntry
	fail    bool
}

func (a *fakeAudit) Record(ctx context.Context, e AuditEntry) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.fail {
		return errors.New("audit boom")
	}
	a.entries = append(a.entries, e)
	return nil
}

func (a *fakeAudit) snapshot() []AuditEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]AuditEntry(nil), a.entries...)
}

func (a *fakeAudit) byAction(action string) []AuditEntry {
	var out []AuditEntry
	for _, e := range a.snapshot() {
		if e.Action == action {
			out = append(out, e)
		}
	}
	return out
}

// fakeVerifier 实现 PanelSessionVerifier。
type fakeVerifier struct {
	mu        sync.Mutex
	invalid   bool // true=AUTH_001；否则原样有效
	dbErr     bool // true=存储异常（非失效）
	lastToken string
}

func (v *fakeVerifier) VerifyWs(ctx context.Context, rawToken string) (int64, string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.lastToken = rawToken
	switch {
	case v.dbErr:
		return 0, "", fmt.Errorf("verify ws: db down")
	case v.invalid:
		return 0, "", errcode.ErrUnauthenticated
	default:
		return 1, "admin", nil
	}
}

// fakeConn 实现 WSConn（帧收发经通道；clientClosed 模拟客户端断开）。
type fakeConn struct {
	mu           sync.Mutex
	frames       [][]byte
	readCh       chan []byte
	clientClosed chan struct{}
	srvClosed    chan struct{}
	closeOnce    sync.Once
	pings        int
	pongs        int
	deadline     time.Time
	writeErr     error
	readErr      error
}

func newFakeConn() *fakeConn {
	return &fakeConn{
		readCh:       make(chan []byte, 16),
		clientClosed: make(chan struct{}),
		srvClosed:    make(chan struct{}),
	}
}

func (c *fakeConn) ReadFrame() ([]byte, error) {
	c.mu.Lock()
	readErr := c.readErr
	c.mu.Unlock()
	if readErr != nil {
		return nil, readErr
	}
	select {
	case f := <-c.readCh:
		return f, nil
	case <-c.clientClosed:
		return nil, errors.New("client closed connection")
	case <-c.srvClosed:
		return nil, errors.New("server closed connection")
	}
}

// injectReadErr 注入读侧错误（-race 安全）。
func (c *fakeConn) injectReadErr(err error) {
	c.mu.Lock()
	c.readErr = err
	c.mu.Unlock()
}

// osDeadline 构造包装 os.ErrDeadlineExceeded 的读超时错误（模拟读截止到点）。
func osDeadline() error {
	return fmt.Errorf("read tcp: %w", os.ErrDeadlineExceeded)
}

func (c *fakeConn) WriteFrame(payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.writeErr != nil {
		return c.writeErr
	}
	c.frames = append(c.frames, append([]byte(nil), payload...))
	return nil
}

func (c *fakeConn) Ping() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pings++
	return nil
}

func (c *fakeConn) SetPongHandler(h func()) {
	c.mu.Lock()
	c.pongs++
	c.mu.Unlock()
}

func (c *fakeConn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deadline = t
	return nil
}

func (c *fakeConn) Close() error {
	c.closeOnce.Do(func() { close(c.srvClosed) })
	return nil
}

// clientClose 模拟客户端侧断开（标签页关闭/close 帧）。
func (c *fakeConn) clientClose() { close(c.clientClosed) }

// framesSnapshot 读取已写帧快照。
func (c *fakeConn) framesSnapshot() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([][]byte(nil), c.frames...)
}

// frame 是测试侧服务端帧解析形态。
type frame struct {
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
	Truncated bool            `json:"truncated"`
}

func parseFrames(t *testing.T, raw [][]byte) []frame {
	t.Helper()
	out := make([]frame, 0, len(raw))
	for _, b := range raw {
		var f frame
		if err := json.Unmarshal(b, &f); err != nil {
			t.Fatalf("frame decode %s: %v", b, err)
		}
		out = append(out, f)
	}
	return out
}

// waitFor 轮询等待条件成立（2s 截止）。
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

// testEnv 聚合服务层测试依赖。
type testEnv struct {
	svc      *Service
	clock    *testClock
	pty      *fakePtyGateway
	users    *fakeUserDir
	audit    *fakeAudit
	verifier *fakeVerifier
}

// newTestEnv 构造小限额服务（MaxConcurrent=2；限时段参数见用例内覆写）。
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	clock := newTestClock()
	env := &testEnv{
		clock: clock,
		pty:   &fakePtyGateway{user: "paneluser"},
		users: &fakeUserDir{users: []SystemUser{
			{Name: "root", UID: 0, Shell: "/bin/bash"},
			{Name: "deploy", UID: 1000, Shell: "/bin/zsh"},
			{Name: "svc", UID: 998, Shell: "/sbin/nologin"},
		}},
		audit:    &fakeAudit{},
		verifier: &fakeVerifier{},
	}
	env.svc = New(env.pty, env.users, env.audit, env.verifier, nil, clock.Now)
	env.svc.limits.MaxConcurrent = 2
	env.svc.limits.IdleTimeout = 30 * time.Minute
	env.svc.limits.HardLimit = 8 * time.Hour
	env.svc.limits.RecheckInterval = 30 * time.Second
	env.svc.limits.PingInterval = 30 * time.Second
	env.svc.limits.PongWait = 75 * time.Second
	// Registry 以默认 MaxConcurrent 构建——重建成小限额版本。
	env.svc.registry = NewRegistry(2)
	return env
}

// admit 以标准 admin 操作者执行 Admit。
func (e *testEnv) admit(t *testing.T, identitySource, username string) (*Session, error) {
	t.Helper()
	return e.svc.Admit(context.Background(), WsActor{
		UserID: 1, Username: "admin", Role: "admin", RawToken: "tok-1",
	}, identitySource, username, "127.0.0.1", "unit-test")
}

// attach 后台附着（serve 阻塞于空读）。
func attach(t *testing.T, svc *Service, pending *Session, conn *fakeConn) {
	t.Helper()
	go func() { _ = svc.Attach(pending, conn) }()
	waitFor(t, "ready frame", func() bool {
		fs := parseFramesQuiet(conn.framesSnapshot())
		return len(fs) > 0 && fs[0].Type == FrameReady
	})
}

func parseFramesQuiet(raw [][]byte) []frame {
	out := make([]frame, 0, len(raw))
	for _, b := range raw {
		var f frame
		if err := json.Unmarshal(b, &f); err == nil {
			out = append(out, f)
		}
	}
	return out
}

// wantSymbol 断言错误符号。
func wantSymbol(t *testing.T, err error, want string) {
	t.Helper()
	var ec *errcode.Error
	if !errors.As(err, &ec) {
		t.Fatalf("err = %v, want errcode %s", err, want)
	}
	if ec.Symbol() != want {
		t.Fatalf("symbol = %s, want %s (message=%s)", ec.Symbol(), want, ec.Message())
	}
}

// ---------- 用例 ----------

func TestIsInteractiveShell(t *testing.T) {
	tests := []struct {
		shell string
		want  bool
	}{
		{"/bin/bash", true},
		{"/bin/zsh", true},
		{"/sbin/nologin", false},
		{"/bin/false", false},
		{"/sbin/halt", false},
		{"/sbin/shutdown", false},
		{"", false},
		{"  ", false},
		{"/usr/sbin/NOLOGIN", false}, // 大小写归一
	}
	for _, tt := range tests {
		if got := IsInteractiveShell(tt.shell); got != tt.want {
			t.Errorf("IsInteractiveShell(%q) = %v, want %v", tt.shell, got, tt.want)
		}
	}
}

func TestListUserOptions(t *testing.T) {
	env := newTestEnv(t)
	data, err := env.svc.ListUserOptions(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// svc 网关过滤 nologin（svc 条目）+ 排序由实现保证（fake 未排序——
	// 生产实现在 adapter；此处校验过滤语义与载荷形状）。
	if len(data.Items) != 2 {
		t.Fatalf("items = %d, want 2（nologin 已过滤）", len(data.Items))
	}
	if data.GeneratedAt == "" {
		t.Fatal("generatedAt 为空")
	}
	if data.Items[0].Name != "root" || data.Items[0].UID != 0 || data.Items[0].Shell != "/bin/bash" {
		t.Fatalf("item0 = %+v", data.Items[0])
	}

	env.users.fail = true
	if _, err := env.svc.ListUserOptions(context.Background()); err == nil {
		t.Fatal("实况读取失败应报 TERM_007")
	} else {
		wantSymbol(t, err, "TERM_007")
	}
}

func TestValidateConnect(t *testing.T) {
	tests := []struct {
		name           string
		source, user   string
		wantErr        bool
		wantFieldCount int
	}{
		{"panel ok", IdentitySourcePanel, "", false, 0},
		{"specified ok", IdentitySourceSpecified, "root", false, 0},
		{"未知来源", "who", "", true, 1},
		{"specified 缺 username", IdentitySourceSpecified, "", true, 1},
		{"specified 非法形态", IdentitySourceSpecified, "Bad;Name", true, 1},
		{"panel 携带 username", IdentitySourcePanel, "root", true, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConnect(tt.source, tt.user)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				var ec *errcode.Error
				if !errors.As(err, &ec) || ec.Symbol() != "TERM_005" {
					t.Fatalf("err = %v, want TERM_005", err)
				}
				vd, ok := ec.Data().(errcode.ValidationData)
				if !ok || len(vd.Fields) != tt.wantFieldCount {
					t.Fatalf("fields = %+v, want %d 项", ec.Data(), tt.wantFieldCount)
				}
			}
		})
	}
}

func TestAdmitRejections(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		user       string
		prepare    func(env *testEnv)
		wantSymbol string
		wantKill   bool
	}{
		{"参数-未知来源", "who", "", nil, "TERM_005", false},
		{"参数-缺 username", IdentitySourceSpecified, "", nil, "TERM_005", false},
		{"用户不存在", IdentitySourceSpecified, "ghost", nil, "TERM_002", false},
		{"非交互 shell", IdentitySourceSpecified, "svc", nil, "TERM_003", false},
		{"PTY 失败", IdentitySourcePanel, "", func(env *testEnv) { env.pty.fail = true },
			"TERM_004", false},
		{"审计失败拒建", IdentitySourcePanel, "", func(env *testEnv) { env.audit.fail = true },
			"TERM_006", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv(t)
			if tt.prepare != nil {
				tt.prepare(env)
			}
			_, err := env.admit(t, tt.source, tt.user)
			wantSymbol(t, err, tt.wantSymbol)
			if tt.wantKill {
				h := env.pty.last()
				if h == nil {
					t.Fatal("PTY 未 spawn")
				}
				killed, waited, _, _ := h.snapshot()
				if !killed || !waited {
					t.Fatalf("TERM_006 路径应回收 PTY：killed=%v waited=%v", killed, waited)
				}
			}
			if got := len(env.audit.byAction(ActionSessionOpened)); tt.wantSymbol == "TERM_006" && got != 0 {
				t.Fatalf("opened 审计应未落库，got %d", got)
			}
			if env.svc.registry.Len() != 0 {
				t.Fatalf("拒绝路径不应登记会话，registry=%d", env.svc.registry.Len())
			}
		})
	}
}

func TestAdmitSuccess(t *testing.T) {
	env := newTestEnv(t)
	sess, err := env.admit(t, IdentitySourceSpecified, "deploy")
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if sess.RunUser != "deploy" {
		t.Fatalf("runUser = %s, want deploy", sess.RunUser)
	}
	if sess.Identity.SystemUsername != "deploy" || sess.Identity.Source != IdentitySourceSpecified {
		t.Fatalf("identity = %+v", sess.Identity)
	}
	if sess.PanelSessionRef != "tok-1" {
		t.Fatalf("panelSessionRef = %s", sess.PanelSessionRef)
	}
	opened := env.audit.byAction(ActionSessionOpened)
	if len(opened) != 1 {
		t.Fatalf("opened 审计 = %d, want 1", len(opened))
	}
	e := opened[0]
	if e.Category != CategorySecurity || e.Source != SourceWeb || e.ObjectID != sess.ID ||
		e.ObjectType != ObjectTypeTerminalSession || e.ActorName != "admin" {
		t.Fatalf("opened 审计载荷异常： %+v", e)
	}
	detail, _ := e.Detail.(map[string]string)
	if detail["identitySource"] != IdentitySourceSpecified || detail["runUser"] != "deploy" ||
		detail["sourceIp"] != "127.0.0.1" {
		t.Fatalf("opened detail = %+v", e.Detail)
	}
}

func TestAdmitConcurrentLimit(t *testing.T) {
	env := newTestEnv(t) // MaxConcurrent=2
	conns := make([]*fakeConn, 0, 2)
	for i := 0; i < 2; i++ {
		sess, err := env.admit(t, IdentitySourcePanel, "")
		if err != nil {
			t.Fatalf("admit #%d: %v", i+1, err)
		}
		conn := newFakeConn()
		attach(t, env.svc, sess, conn)
		conns = append(conns, conn)
	}
	if got := env.svc.registry.CountByUser(1); got != 2 {
		t.Fatalf("count = %d, want 2", got)
	}
	_, err := env.admit(t, IdentitySourcePanel, "")
	if err == nil {
		t.Fatal("第 3 个连接应被拒")
	}
	wantSymbol(t, err, "TERM_001")
	var ec *errcode.Error
	if errors.As(err, &ec) {
		data, ok := ec.Data().(map[string]int)
		if !ok || data["limit"] != 2 || data["current"] != 2 {
			t.Fatalf("TERM_001 data = %+v, want {limit:2,current:2}", ec.Data())
		}
	}
	for _, c := range conns {
		c.clientClose()
	}
	waitFor(t, "registry 清空", func() bool { return env.svc.registry.Len() == 0 })
}

func TestSessionEchoFlow(t *testing.T) {
	env := newTestEnv(t)
	sess, err := env.admit(t, IdentitySourcePanel, "")
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if sess.RunUser != "paneluser" {
		t.Fatalf("runUser = %s, want paneluser（CurrentUser 解析）", sess.RunUser)
	}
	conn := newFakeConn()
	attach(t, env.svc, sess, conn)

	// ready 首帧：sessionId/identity/limits 权威值。
	fs := parseFrames(t, conn.framesSnapshot())
	if fs[0].Type != FrameReady {
		t.Fatalf("首帧 = %s, want ready", fs[0].Type)
	}
	var ready ReadyPayload
	if err := json.Unmarshal(fs[0].Data, &ready); err != nil {
		t.Fatalf("ready decode: %v", err)
	}
	if ready.SessionID != sess.ID || ready.Identity.RunUser != "paneluser" ||
		ready.Identity.Source != IdentitySourcePanel {
		t.Fatalf("ready = %+v", ready)
	}
	if ready.Limits.MaxConcurrent != 2 || ready.Limits.IdleTimeoutSeconds != 1800 ||
		ready.Limits.HardLimitSeconds != 28800 {
		t.Fatalf("ready limits = %+v", ready.Limits)
	}

	// input 帧 → PTY 写入 + lastActive 刷新。
	before := sess.activityAt()
	env.clock.Advance(time.Second)
	conn.readCh <- []byte(`{"type":"input","data":"` + base64.StdEncoding.EncodeToString([]byte("ls -l")) + `"}`)
	h := env.pty.last()
	waitFor(t, "pty 写入", func() bool {
		_, _, w, _ := h.snapshot()
		return len(w) == 1
	})
	_, _, w, _ := h.snapshot()
	if string(w[0]) != "ls -l" {
		t.Fatalf("pty written = %q", w[0])
	}
	waitFor(t, "lastActive 刷新", func() bool { return sess.activityAt().After(before) })

	// resize 帧 → PTY 尺寸同步。
	conn.readCh <- []byte(`{"type":"resize","cols":120,"rows":40}`)
	waitFor(t, "pty resize", func() bool {
		_, _, _, rs := h.snapshot()
		return len(rs) == 1 && rs[0] == [2]uint16{120, 40}
	})

	// PTY 输出 → output 帧（Base64）。
	h.outCh <- []byte("hello world")
	waitFor(t, "output 帧", func() bool {
		fs := parseFramesQuiet(conn.framesSnapshot())
		for _, f := range fs {
			if f.Type == FrameOutput {
				return true
			}
		}
		return false
	})
	fs = parseFrames(t, conn.framesSnapshot())
	var got string
	for _, f := range fs {
		if f.Type == FrameOutput {
			var data string
			if err := json.Unmarshal(f.Data, &data); err != nil {
				t.Fatalf("output data decode: %v", err)
			}
			decoded, _ := base64.StdEncoding.DecodeString(data)
			got = string(decoded)
		}
	}
	if got != "hello world" {
		t.Fatalf("output = %q, want hello world", got)
	}

	// 客户端断开 → closed 帧 + Kill + 注销 + closed 审计（I-02/I-07）。
	conn.clientClose()
	waitFor(t, "registry 注销", func() bool { return env.svc.registry.Len() == 0 })
	fs = parseFrames(t, conn.framesSnapshot())
	var closed *frame
	for i := range fs {
		if fs[i].Type == FrameClosed {
			closed = &fs[i]
		}
	}
	if closed == nil {
		t.Fatalf("缺 closed 帧： %+v", fs)
	}
	var cp ClosedPayload
	if err := json.Unmarshal(closed.Data, &cp); err != nil {
		t.Fatalf("closed decode: %v", err)
	}
	if cp.Reason != ReasonSelfClose || cp.DurationSeconds < 0 {
		t.Fatalf("closed = %+v", cp)
	}
	if _, err := time.Parse(time.RFC3339, cp.ClosedAt); err != nil {
		t.Fatalf("closedAt 非 RFC3339: %s", cp.ClosedAt)
	}
	killed, waited, _, _ := h.snapshot()
	if !killed || !waited {
		t.Fatalf("PTY 应回收：killed=%v waited=%v", killed, waited)
	}
	closedAudit := env.audit.byAction(ActionSessionClosed)
	if len(closedAudit) != 1 {
		t.Fatalf("closed 审计 = %d, want 1", len(closedAudit))
	}
	if closedAudit[0].Source != SourceWeb { // self_close → web（I-07 映射）
		t.Fatalf("closed source = %s, want web", closedAudit[0].Source)
	}
}

func TestSessionProtocolViolations(t *testing.T) {
	tests := []struct {
		name  string
		frame string
	}{
		{"未知帧类型", `{"type":"output","data":"aGk="}`},
		{"超限帧", `{"type":"input","data":"` + bigBase64(t, FrameDataLimit+1) + `"}`},
		{"resize 越界", `{"type":"resize","cols":0,"rows":40}`},
		{"非法 base64", `{"type":"input","data":"@@@"}`},
		{"非法 JSON", `{"type":`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv(t)
			sess, err := env.admit(t, IdentitySourcePanel, "")
			if err != nil {
				t.Fatalf("admit: %v", err)
			}
			conn := newFakeConn()
			attach(t, env.svc, sess, conn)
			conn.readCh <- []byte(tt.frame)
			waitFor(t, "protocol_error 断开", func() bool {
				for _, f := range parseFramesQuiet(conn.framesSnapshot()) {
					if f.Type == FrameClosed {
						var cp ClosedPayload
						if json.Unmarshal(f.Data, &cp) == nil {
							return cp.Reason == ReasonProtocolError
						}
					}
				}
				return false
			})
			waitFor(t, "registry 注销", func() bool { return env.svc.registry.Len() == 0 })
			// 审计是清理链第 6 步（registry.Remove 之后，session.go 清理序）——
			// 注册表清零与审计落库之间存在调度窗口，必须等待而非立即断言。
			waitFor(t, "closed 审计恰好一条", func() bool {
				return len(env.audit.byAction(ActionSessionClosed)) == 1
			})
		})
	}
}

func bigBase64(t *testing.T, n int) string {
	t.Helper()
	b := make([]byte, n)
	return base64.StdEncoding.EncodeToString(b)
}

func TestSessionCloseIdempotent(t *testing.T) {
	env := newTestEnv(t)
	sess, err := env.admit(t, IdentitySourcePanel, "")
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	conn := newFakeConn()
	attach(t, env.svc, sess, conn)
	conn.clientClose()
	waitFor(t, "首次清理完成", func() bool { return env.svc.registry.Len() == 0 })
	sess.close(ReasonIdleTimeout, "double close") // 重复触发（I-02 幂等）
	sess.close(ReasonHardLimit, "triple close")
	if got := len(env.audit.byAction(ActionSessionClosed)); got != 1 {
		t.Fatalf("closed 审计 = %d, want 1（幂等）", got)
	}
}

func TestLifecycleIdleAndHardLimit(t *testing.T) {
	env := newTestEnv(t)
	env.svc.limits.IdleTimeout = 100 * time.Millisecond
	sess, err := env.admit(t, IdentitySourcePanel, "")
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	conn := newFakeConn()
	attach(t, env.svc, sess, conn)

	// 未到阈值：不关闭（活跃刷新判定锚点=lastActiveAt）。
	env.clock.Advance(50 * time.Millisecond)
	env.svc.scanOnce(context.Background())
	if env.svc.registry.Len() != 1 {
		t.Fatalf("未到期不应关闭")
	}

	// 空闲超时 → idle_timeout。
	env.clock.Advance(60 * time.Millisecond)
	env.svc.scanOnce(context.Background())
	waitFor(t, "idle 断开", func() bool {
		for _, f := range parseFramesQuiet(conn.framesSnapshot()) {
			if f.Type == FrameClosed {
				var cp ClosedPayload
				if json.Unmarshal(f.Data, &cp) == nil {
					return cp.Reason == ReasonIdleTimeout
				}
			}
		}
		return false
	})
	if env.svc.registry.Len() != 0 {
		t.Fatal("idle 断开后应注销")
	}
	// closed 审计 source=system（非 self_close）。
	if got := env.audit.byAction(ActionSessionClosed); len(got) != 1 || got[0].Source != SourceSystem {
		t.Fatalf("closed 审计映射异常： %+v", got)
	}

	// 硬上限：createdAt+HardLimit 与活跃无关（空闲阈值拉长以隔离变量）。
	env2 := newTestEnv(t)
	env2.svc.limits.HardLimit = time.Hour
	env2.svc.limits.IdleTimeout = 10 * time.Hour
	sess2, err := env2.admit(t, IdentitySourcePanel, "")
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	conn2 := newFakeConn()
	attach(t, env2.svc, sess2, conn2)
	env2.clock.Advance(time.Hour + time.Second)
	env2.svc.scanOnce(context.Background())
	waitFor(t, "hard 断开", func() bool {
		for _, f := range parseFramesQuiet(conn2.framesSnapshot()) {
			if f.Type == FrameClosed {
				var cp ClosedPayload
				if json.Unmarshal(f.Data, &cp) == nil {
					return cp.Reason == ReasonHardLimit
				}
			}
		}
		return false
	})
}

func TestLifecyclePanelSessionRecheck(t *testing.T) {
	env := newTestEnv(t)
	env.svc.limits.RecheckInterval = 0 // 每轮扫描均复核
	sess, err := env.admit(t, IdentitySourcePanel, "")
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	conn := newFakeConn()
	attach(t, env.svc, sess, conn)

	// 存储异常非失效：不联动断开（D-07 判别）。
	env.verifier.dbErr = true
	env.svc.scanOnce(context.Background())
	waitFor(t, "复核已触达", func() bool {
		env.verifier.mu.Lock()
		defer env.verifier.mu.Unlock()
		return env.verifier.lastToken == "tok-1"
	})
	if env.svc.registry.Len() != 1 {
		t.Fatal("存储异常不应联动断开")
	}

	// 会话失效 → panel_session_invalid 联动断开。
	env.verifier.dbErr = false
	env.verifier.invalid = true
	env.svc.scanOnce(context.Background())
	waitFor(t, "联动断开", func() bool {
		for _, f := range parseFramesQuiet(conn.framesSnapshot()) {
			if f.Type == FrameClosed {
				var cp ClosedPayload
				if json.Unmarshal(f.Data, &cp) == nil {
					return cp.Reason == ReasonPanelSessionInvalid
				}
			}
		}
		return false
	})
	if env.svc.registry.Len() != 0 {
		t.Fatal("联动断开后应注销")
	}
}

func TestLifecyclePing(t *testing.T) {
	env := newTestEnv(t)
	env.svc.limits.PingInterval = 0 // 每轮扫描均 ping
	sess, err := env.admit(t, IdentitySourcePanel, "")
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	conn := newFakeConn()
	attach(t, env.svc, sess, conn)
	env.svc.scanOnce(context.Background())
	conn.mu.Lock()
	pings := conn.pings
	deadline := conn.deadline
	conn.mu.Unlock()
	if pings == 0 {
		t.Fatal("扫描应发送心跳 ping")
	}
	if deadline.IsZero() {
		t.Fatal("serve 应设置读截止")
	}
	// 读截止超时 → network_error（A-T8 等价实现）：注入超时哨兵后以一帧
	// 触发读侧重入（ReadFrame 于入口检查注入值）。
	conn.injectReadErr(osDeadline())
	conn.readCh <- []byte(`{"type":"input","data":""}`)
	waitFor(t, "network_error 断开", func() bool {
		for _, f := range parseFramesQuiet(conn.framesSnapshot()) {
			if f.Type == FrameClosed {
				var cp ClosedPayload
				if json.Unmarshal(f.Data, &cp) == nil {
					return cp.Reason == ReasonNetworkError
				}
			}
		}
		return false
	})
}

func TestAbortPendingSession(t *testing.T) {
	env := newTestEnv(t)
	sess, err := env.admit(t, IdentitySourcePanel, "")
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	env.svc.Abort(sess, ReasonNetworkError)
	if env.svc.registry.Len() != 0 {
		t.Fatal("abort 不应登记会话")
	}
	if got := len(env.audit.byAction(ActionSessionClosed)); got != 1 {
		t.Fatalf("closed 审计 = %d, want 1（I-07 开合成对）", got)
	}
	h := env.pty.last()
	killed, waited, _, _ := h.snapshot()
	if !killed || !waited {
		t.Fatalf("abort 应回收 PTY：killed=%v waited=%v", killed, waited)
	}
}

func TestAttachQuotaRace(t *testing.T) {
	env := newTestEnv(t) // MaxConcurrent=2
	// 直接占满配额（不经 Attach 的读循环）。
	for i := 0; i < 2; i++ {
		s, err := env.admit(t, IdentitySourcePanel, "")
		if err != nil {
			t.Fatalf("admit: %v", err)
		}
		if !env.svc.registry.Create(s) {
			t.Fatalf("登记 #%d 应成功", i+1)
		}
	}
	// 模拟「Admit 预检通过后、登记前」的并发窗口：手工构造第三个已 spawn
	// 会话（预检无法再走——registry 已满会被 ⑤ 拦截，恰证明双闸语义），
	// Attach 时由原子 CAS 拒绝 → TERM_001 + 统一清理。
	now := env.clock.Now()
	race := &Session{
		ID: "race0003", UserID: 1, Username: "admin", RunUser: "paneluser",
		CreatedAt: now, LastActiveAt: now, PTY: newFakePty(),
		ring: newOutputRing(OutputRingSize), svc: env.svc,
		HardDeadlineAt: hardDeadline(now, env.svc.limits.HardLimit),
	}
	conn := newFakeConn()
	if err := env.svc.Attach(race, conn); err == nil {
		t.Fatal("配额竞态应返回 TERM_001")
	} else {
		wantSymbol(t, err, "TERM_001")
	}
	if got := len(env.audit.byAction(ActionSessionClosed)); got != 1 {
		t.Fatalf("竞态清理 closed 审计 = %d, want 1", got)
	}
	if env.svc.registry.Len() != 2 {
		t.Fatalf("registry = %d, want 2（既有会话不受影响）", env.svc.registry.Len())
	}
}

func TestRegistryAtomicLimit(t *testing.T) {
	r := NewRegistry(2)
	mk := func(id string) *Session { return &Session{ID: id, UserID: 7} }
	if !r.Create(mk("a")) || !r.Create(mk("b")) {
		t.Fatal("前两个登记应成功")
	}
	if r.Create(mk("c")) {
		t.Fatal("超限登记应被拒（原子 CAS）")
	}
	r.Remove("a")
	if !r.Create(mk("d")) {
		t.Fatal("释放配额后应可登记")
	}
	if r.CountByUser(7) != 2 || len(r.ListByUser(7)) != 2 {
		t.Fatalf("count/list 异常")
	}
	if _, ok := r.Get("d"); !ok {
		t.Fatal("Get(d) 未命中")
	}
	r.Remove("d") // 幂等重复删除
	r.Remove("d")
	if r.Len() != 1 {
		t.Fatalf("len = %d, want 1", r.Len())
	}
}

func TestOutputRingBackpressure(t *testing.T) {
	r := newOutputRing(8)
	r.write([]byte("abcdef"))
	chunk, trunc := r.take()
	if string(chunk) != "abcdef" || trunc {
		t.Fatalf("take1 = %q trunc=%v", chunk, trunc)
	}
	// 溢出丢最旧 + truncated 标记（D-04）。
	r.write([]byte("12345678"))
	r.write([]byte("XYZ"))
	chunk, trunc = r.take()
	if string(chunk) != "45678XYZ" || !trunc {
		t.Fatalf("take2 = %q trunc=%v, want 45678XYZ+truncated", chunk, trunc)
	}
	// 标记消费后恢复：后续批次不再误报。
	r.write([]byte("ok"))
	chunk, trunc = r.take()
	if string(chunk) != "ok" || trunc {
		t.Fatalf("take3 = %q trunc=%v", chunk, trunc)
	}
	// 超容量整写只留最新。
	r.write(make([]byte, 16))
	chunk, trunc = r.take()
	if len(chunk) != 8 || !trunc {
		t.Fatalf("take4 len=%d trunc=%v, want 8+truncated", len(chunk), trunc)
	}
	// 环形回绕正确性。
	r2 := newOutputRing(5)
	r2.write([]byte("ab"))    // ab---
	r2.take()                 // 清空
	r2.write([]byte("12345")) // 满环
	chunk, _ = r2.take()
	if string(chunk) != "12345" {
		t.Fatalf("回绕读取 = %q", chunk)
	}
	// stop 唤醒退出。
	r.stop()
	if c, _ := r.take(); c != nil {
		t.Fatal("stop 后 take 应返回 nil")
	}
}

func TestRunLifecycleExits(t *testing.T) {
	env := newTestEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = env.svc.RunLifecycle(ctx, 5*time.Millisecond)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunLifecycle 应随 ctx 取消退出（G-06）")
	}
}
