package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/api"
	authapi "github.com/dougong/dougong/internal/api/auth"
	auditsvc "github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	monsvc "github.com/dougong/dougong/internal/service/monitoring"
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

// testApp 聚合全栈测试依赖（真实 :memory: SQLite，不 mock；镜像 main 装配）。
type testApp struct {
	ts   *httptest.Server
	svc  *monsvc.Service
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

	monSvc := monsvc.NewService()
	server := api.NewServer()
	server.RegisterModules(
		authapi.NewModule(authSvcObj, auditSvc, false),
		NewModule(authSvcObj, monSvc),
	)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	return &testApp{ts: ts, svc: monSvc, auth: authSvcObj}
}

// session 持有登录会话凭据。
type session struct{ cookie, csrf string }

// initWizardAndLogin 完成初始化向导（admin）并返回会话。
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
		t.Fatalf("login decode: %v (body=%s)", err, raw)
	}
	wantCode(t, env, "0")
	for _, c := range resp.Cookies() {
		if c.Name == authsvc.SessionCookieName {
			return session{cookie: c.Value}
		}
	}
	t.Fatal("login response missing session cookie")
	return session{}
}

// createReadonly 由 admin 建 readonly 用户（ADR-018 全可看验证用）。
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
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("create readonly decode: %v", err)
	}
	wantCode(t, env, "0")
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
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%s status = %d（统一 200 语义）", path, resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode %s: %v (body=%s)", path, err, raw)
	}
	return env
}

// fillHistory 注入两分钟历史条目（cpu/网络/负载维度，含 v=null 缺口）。
func fillHistory(t *testing.T, svc *monsvc.Service) {
	t.Helper()
	now := time.Now()
	f := func(v float64) *float64 { return &v }
	svc.Buffer().AppendMinute(monsvc.MinuteEntry{TS: now.Truncate(time.Minute).Add(-2 * time.Minute),
		CPUTotal: f(10), NetUpBps: newInt64(100)})
	svc.Buffer().AppendMinute(monsvc.MinuteEntry{TS: now.Truncate(time.Minute).Add(-1 * time.Minute),
		NetUpBps: newInt64(200), NetDownBps: newInt64(300)})
}

func newInt64(v int64) *int64 { return &v }

// fillLatest 注入实时层最新采样（进程采集成功 + 完整指标）。
func fillLatest(t *testing.T, svc *monsvc.Service, processesOK bool) {
	t.Helper()
	f := func(v float64) *float64 { return &v }
	u := func(v uint64) *uint64 { return &v }
	smp := monsvc.Sample{
		TS:             time.Now().UTC().Truncate(time.Second),
		CPUTotal:       f(42.5),
		CPUPerCore:     []float64{40.1, 44.9},
		MemUsedBytes:   u(8 * 1024),
		MemTotalBytes:  u(16 * 1024),
		MemUsedPercent: f(50),
		DiskCapacity:   []monsvc.DiskCapacityItem{{Mount: "/", TotalBytes: 100, UsedBytes: 40}},
		DiskIOReadBps:  newInt64(1000),
		DiskIOWriteBps: newInt64(2000),
		NetUpBps:       newInt64(10),
		NetDownBps:     newInt64(20),
		Load1:          f(0.11), Load5: f(0.22), Load15: f(0.33),
		ProcessesOK: processesOK,
	}
	if processesOK {
		p1, p2 := 8.5, 3.2
		smp.Processes = []monsvc.ProcessInfo{
			{PID: 10, Name: "nginx", CPUPercent: &p1, MemBytes: u(2048)},
			{PID: 20, Name: "mysqld", CPUPercent: &p2, MemBytes: u(4096)},
		}
	}
	svc.Buffer().SetLatest(smp)
}

// ---- 会话与权限（§3.3 权限矩阵：admin+readonly 全可看，访客不可达）----

func TestMonitoringEndpointsRequireSession(t *testing.T) {
	app := newTestApp(t)
	_ = initWizardAndLogin(t, app)
	for _, path := range []string{
		"/monitoring/metrics/snapshot", "/monitoring/metrics/history?window=last_1h&dimension=cpu",
		"/monitoring/processes/top", "/monitoring/system/info",
	} {
		env := get(t, app, path, nil)
		wantCode(t, env, "AUTH_001")
	}
}

func TestMonitoringReadEndpointsBothRoles(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	createReadonly(t, app, admin)
	ro := login(t, app, "watcher")
	fillLatest(t, app.svc, true)
	fillHistory(t, app.svc)

	for _, sess := range []session{admin, ro} {
		for _, path := range []string{
			"/monitoring/metrics/snapshot",
			"/monitoring/metrics/history?window=last_1h&dimension=cpu",
			"/monitoring/processes/top",
			"/monitoring/system/info",
		} {
			env := get(t, app, path, &sess)
			wantCode(t, env, "0")
		}
	}
}

// ---- 数据就绪性（MON_004 / D-07 空序列非错误流）----

func TestMonitoringNoDataYet(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)

	// 快照/TOP：启动后首轮采样未完成 → MON_004。
	wantCode(t, get(t, app, "/monitoring/metrics/snapshot", &admin), "MON_004")
	wantCode(t, get(t, app, "/monitoring/processes/top", &admin), "MON_004")

	// 趋势：空缓冲 → code=0 + series=[] + startTime=null（D-07）。
	env := get(t, app, "/monitoring/metrics/history?window=last_1h&dimension=cpu", &admin)
	wantCode(t, env, "0")
	var hist struct {
		Series    []any   `json:"series"`
		StartTime *string `json:"startTime"`
		Partial   bool    `json:"partial"`
	}
	if err := json.Unmarshal(env.Data, &hist); err != nil {
		t.Fatalf("history data decode: %v", err)
	}
	if hist.Series == nil || len(hist.Series) != 0 || hist.StartTime != nil {
		t.Fatalf("empty history should be series=[] + startTime=null: %+v", hist)
	}

	// 最新轮进程采集失败：TOP 仍 MON_004（进程维度无部分成功语义）。
	fillLatest(t, app.svc, false)
	wantCode(t, get(t, app, "/monitoring/processes/top", &admin), "MON_004")
	// 快照正常返回：processes null + degraded=true（D-08）。
	env = get(t, app, "/monitoring/metrics/snapshot", &admin)
	wantCode(t, env, "0")
	var snap struct {
		Degraded  bool             `json:"degraded"`
		Processes []map[string]any `json:"processes"`
	}
	if err := json.Unmarshal(env.Data, &snap); err != nil {
		t.Fatalf("snapshot decode: %v", err)
	}
	if !snap.Degraded || snap.Processes != nil {
		t.Fatalf("processes failed round: degraded=%v processes=%v", snap.Degraded, snap.Processes)
	}
}

// ---- 参数校验（MON_001~003，ParamsGuard 映射）----

func TestMonitoringHistoryValidation(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	cases := []struct{ path, want string }{
		{"/monitoring/metrics/history?window=last_1d&dimension=cpu", "MON_001"},
		{"/monitoring/metrics/history?dimension=cpu", "MON_001"}, // window 缺省 = 非法
		{"/monitoring/metrics/history?window=last_1h&dimension=gpu", "MON_002"},
		{"/monitoring/metrics/history?window=last_1h", "MON_002"}, // dimension 缺省 = 非法
		{"/monitoring/metrics/history?window=last_1h&dimension=cpu&scope=all", "MON_003"},
		{"/monitoring/metrics/history?window=last_1h&dimension=memory&scope=total", "MON_003"},
	}
	for _, tc := range cases {
		env := get(t, app, tc.path, &admin)
		if got := codeStr(env); got != tc.want {
			t.Errorf("%s code = %s want %s (message=%s)", tc.path, got, tc.want, env.Message)
		}
	}
	// 合法参数组合过校验（空数据 code=0）。
	for _, path := range []string{
		"/monitoring/metrics/history?window=last_1h&dimension=cpu",
		"/monitoring/metrics/history?window=last_24h&dimension=cpu&scope=per_core",
		"/monitoring/metrics/history?window=last_1h&dimension=memory",
	} {
		wantCode(t, get(t, app, path, &admin), "0")
	}
}

// ---- 契约形状（§4.1.4 字段逐一核对）----

func TestMonitoringSnapshotShape(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	fillLatest(t, app.svc, true)

	env := get(t, app, "/monitoring/metrics/snapshot", &admin)
	wantCode(t, env, "0")
	var snap struct {
		TS       string `json:"ts"`
		Degraded bool   `json:"degraded"`
		CPU      struct {
			TotalPercent *float64  `json:"totalPercent"`
			PerCore      []float64 `json:"perCore"`
		} `json:"cpu"`
		Memory struct {
			UsedBytes   *uint64  `json:"usedBytes"`
			TotalBytes  *uint64  `json:"totalBytes"`
			UsedPercent *float64 `json:"usedPercent"`
		} `json:"memory"`
		Disk struct {
			Capacity []struct {
				Mount      string `json:"mount"`
				TotalBytes uint64 `json:"totalBytes"`
				UsedBytes  uint64 `json:"usedBytes"`
			} `json:"capacity"`
			IOReadBps  *int64 `json:"ioReadBps"`
			IOWriteBps *int64 `json:"ioWriteBps"`
		} `json:"disk"`
		Net struct {
			UpBps   *int64 `json:"upBps"`
			DownBps *int64 `json:"downBps"`
		} `json:"net"`
		Load struct {
			L1  *float64 `json:"l1"`
			L5  *float64 `json:"l5"`
			L15 *float64 `json:"l15"`
		} `json:"load"`
		Processes []struct {
			PID        int32    `json:"pid"`
			Name       string   `json:"name"`
			CPUPercent *float64 `json:"cpuPercent"`
			MemBytes   *uint64  `json:"memBytes"`
		} `json:"processes"`
	}
	if err := json.Unmarshal(env.Data, &snap); err != nil {
		t.Fatalf("snapshot decode: %v (data=%s)", err, env.Data)
	}
	if snap.TS == "" || snap.Degraded {
		t.Fatalf("ts/degraded wrong: %q %v", snap.TS, snap.Degraded)
	}
	if snap.CPU.TotalPercent == nil || *snap.CPU.TotalPercent != 42.5 || len(snap.CPU.PerCore) != 2 {
		t.Fatalf("cpu wrong: %+v", snap.CPU)
	}
	if *snap.Memory.UsedBytes != 8*1024 || *snap.Memory.TotalBytes != 16*1024 || *snap.Memory.UsedPercent != 50 {
		t.Fatalf("memory wrong: %+v", snap.Memory)
	}
	if len(snap.Disk.Capacity) != 1 || snap.Disk.Capacity[0].Mount != "/" ||
		snap.Disk.Capacity[0].UsedBytes != 40 || *snap.Disk.IOReadBps != 1000 || *snap.Disk.IOWriteBps != 2000 {
		t.Fatalf("disk wrong: %+v", snap.Disk)
	}
	if *snap.Net.UpBps != 10 || *snap.Net.DownBps != 20 {
		t.Fatalf("net wrong: %+v", snap.Net)
	}
	if *snap.Load.L1 != 0.11 || *snap.Load.L5 != 0.22 || *snap.Load.L15 != 0.33 {
		t.Fatalf("load wrong: %+v", snap.Load)
	}
	if len(snap.Processes) != 2 || snap.Processes[0].PID != 10 || snap.Processes[0].Name != "nginx" ||
		*snap.Processes[0].CPUPercent != 8.5 || *snap.Processes[0].MemBytes != 2048 {
		t.Fatalf("processes wrong: %+v", snap.Processes)
	}
}

func TestMonitoringHistoryShapeAndDimensions(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	fillHistory(t, app.svc)

	type point struct {
		TS string   `json:"ts"`
		V  *float64 `json:"v"`
	}
	type series struct {
		Key    string  `json:"key"`
		Label  string  `json:"label"`
		Unit   string  `json:"unit"`
		Points []point `json:"points"`
	}
	parse := func(env envelope) struct {
		Window    string   `json:"window"`
		Dimension string   `json:"dimension"`
		Scope     string   `json:"scope"`
		StartTime *string  `json:"startTime"`
		Partial   bool     `json:"partial"`
		Series    []series `json:"series"`
	} {
		t.Helper()
		var d struct {
			Window    string   `json:"window"`
			Dimension string   `json:"dimension"`
			Scope     string   `json:"scope"`
			StartTime *string  `json:"startTime"`
			Partial   bool     `json:"partial"`
			Series    []series `json:"series"`
		}
		if err := json.Unmarshal(env.Data, &d); err != nil {
			t.Fatalf("history decode: %v (data=%s)", err, env.Data)
		}
		return d
	}

	// cpu 默认 total：1 条 series，60 点，-2 分钟有值、-1 分钟 v=null（cpu 缺）。
	env := get(t, app, "/monitoring/metrics/history?window=last_1h&dimension=cpu", &admin)
	wantCode(t, env, "0")
	d := parse(env)
	if d.Window != "last_1h" || d.Dimension != "cpu" || d.Scope != "total" {
		t.Fatalf("meta wrong: %+v", d)
	}
	if !d.Partial || d.StartTime == nil {
		t.Fatalf("partial/startTime wrong: %+v", d)
	}
	if len(d.Series) != 1 || d.Series[0].Key != "cpu_total" || d.Series[0].Unit != "percent" ||
		d.Series[0].Label == "" || len(d.Series[0].Points) != 60 {
		t.Fatalf("cpu series wrong: %+v", d.Series)
	}
	nonNull := 0
	for _, p := range d.Series[0].Points {
		if p.V != nil {
			nonNull++
		}
	}
	if nonNull != 1 || d.Series[0].Points[59].V != nil {
		t.Fatalf("cpu points: nonNull=%d（注入仅 1 个 cpu 有效点）", nonNull)
	}

	// network：双序列，-1 分钟双值齐。
	env = get(t, app, "/monitoring/metrics/history?window=last_1h&dimension=network", &admin)
	wantCode(t, env, "0")
	d = parse(env)
	if len(d.Series) != 2 || d.Series[0].Key != "network_up" || d.Series[1].Key != "network_down" ||
		d.Series[0].Unit != "bps" {
		t.Fatalf("network series wrong: %+v", d.Series)
	}
	last := d.Series[0].Points[59]
	if last.V == nil || *last.V != 200 || *d.Series[1].Points[59].V != 300 {
		t.Fatalf("network last values wrong: %+v", last)
	}

	// cpu + per_core（注入条目无分核数据 → 空序列但 code=0，A-M10 边界）。
	env = get(t, app, "/monitoring/metrics/history?window=last_1h&dimension=cpu&scope=per_core", &admin)
	wantCode(t, env, "0")
	if d = parse(env); len(d.Series) != 0 {
		t.Fatalf("per_core without core data should be empty: %+v", d.Series)
	}

	// last_24h 点数上限。
	env = get(t, app, "/monitoring/metrics/history?window=last_24h&dimension=cpu", &admin)
	wantCode(t, env, "0")
	if d = parse(env); len(d.Series) != 1 || len(d.Series[0].Points) != 1440 {
		t.Fatalf("24h points = %d want 1440", len(d.Series[0].Points))
	}
}

func TestMonitoringTopShape(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	fillLatest(t, app.svc, true)

	env := get(t, app, "/monitoring/processes/top", &admin)
	wantCode(t, env, "0")
	var top struct {
		TS        string `json:"ts"`
		Processes []struct {
			PID        int32    `json:"pid"`
			Name       string   `json:"name"`
			CPUPercent *float64 `json:"cpuPercent"`
			MemBytes   *uint64  `json:"memBytes"`
		} `json:"processes"`
	}
	if err := json.Unmarshal(env.Data, &top); err != nil {
		t.Fatalf("top decode: %v", err)
	}
	if top.TS == "" || len(top.Processes) != 2 || top.Processes[0].PID != 10 ||
		*top.Processes[0].CPUPercent != 8.5 {
		t.Fatalf("top wrong: %+v", top)
	}
}

func TestMonitoringSystemInfoShape(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)

	env := get(t, app, "/monitoring/system/info", &admin)
	wantCode(t, env, "0")
	var info struct {
		Distro        *string `json:"distro"`
		Kernel        *string `json:"kernel"`
		Hostname      *string `json:"hostname"`
		UptimeSeconds *uint64 `json:"uptimeSeconds"`
		Errors        []struct {
			Field  string `json:"field"`
			Reason string `json:"reason"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(env.Data, &info); err != nil {
		t.Fatalf("system info decode: %v", err)
	}
	// 本机实时读取（I-08：成功字段非空或失败字段配 errors，二者必居其一）。
	if info.Hostname == nil && info.Errors == nil {
		t.Fatal("hostname null without errors entry (I-08)")
	}
}
