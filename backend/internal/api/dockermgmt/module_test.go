// module_test.go 是 HTTP/WS 全栈测试（真实 :memory: SQLite + 真实 auth/audit
// 服务，dockerx 网关/CLI 替身注入——daemon 交互不出单测边界；firewall
// module_test 同构基座）。
package dockermgmt

import (
	"bytes"
	"context"
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

	"github.com/dougong/dougong/internal/adapter/dockerx"
	"github.com/dougong/dougong/internal/api"
	authapi "github.com/dougong/dougong/internal/api/auth"
	auditsvc "github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	dockersvc "github.com/dougong/dougong/internal/service/dockermgmt"
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

func wantCode(t *testing.T, env envelope, want string) {
	t.Helper()
	if got := fmt.Sprint(env.Code); got != want {
		t.Fatalf("code = %s, want %s (message=%s)", got, want, env.Message)
	}
}

// ---------- 替身 ----------

// fakeGateway 实现 dockersvc.DockerGateway（daemon 通道替身）。
type fakeGateway struct {
	mu           sync.Mutex
	probe        dockerx.DaemonInfo
	list         []dockerx.ContainerFact
	exists       map[string]bool
	state        map[string]string
	followStream *followStub
	images       []dockerx.ImageFact
	volumes      []dockerx.VolumeFact
	networks     []dockerx.NetworkFact
	actionCalls  []string
	deleteCalls  []string
}

type followStub struct {
	send   chan string
	end    chan string
	cancel context.CancelFunc
}

func (f *fakeGateway) newFollowStub() *followStub {
	send := make(chan string, 8)
	end := make(chan string, 1)
	_, cancel := context.WithCancel(context.Background())
	f.followStream = &followStub{send: send, end: end, cancel: cancel}
	return f.followStream
}

func (f *fakeGateway) stream() *dockerx.LogStream {
	st := f.followStream
	return &dockerx.LogStream{Lines: st.send, End: st.end, Cancel: st.cancel}
}

func (f *fakeGateway) ProbeDaemon(context.Context) dockerx.DaemonInfo { return f.probe }

func (f *fakeGateway) ListContainers(context.Context) ([]dockerx.ContainerFact, error) {
	return f.list, nil
}

func (f *fakeGateway) ContainerState(_ context.Context, id string) (string, error) {
	if s, ok := f.state[id]; ok {
		return s, nil
	}
	return "", errors.New("dockerx: resource not found")
}

func (f *fakeGateway) ContainerExists(_ context.Context, id string) (bool, error) {
	return f.exists[id], nil
}

func (f *fakeGateway) GetContainerLogs(_ context.Context, id string, tail int) ([]string, error) {
	return []string{"c-log-line"}, nil
}

func (f *fakeGateway) FollowContainerLogs(context.Context, string, int) (*dockerx.LogStream, error) {
	if f.followStream != nil {
		return f.stream(), nil
	}
	return nil, &dockerx.ExecError{Verb: "logs follow", Reason: "no stub"}
}

func (f *fakeGateway) GetContainerStats(_ context.Context, id string) (dockerx.StatsFact, error) {
	return dockerx.StatsFact{CPUPercent: 1.5, MemUsageByte: 10, MemLimitByte: 100, MemPercent: 10}, nil
}

func (f *fakeGateway) ContainerAction(_ context.Context, id, action string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.actionCalls = append(f.actionCalls, action+":"+id)
	return nil
}

func (f *fakeGateway) DeleteContainer(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteCalls = append(f.deleteCalls, "container:"+id)
	return nil
}

func (f *fakeGateway) ListImages(context.Context) ([]dockerx.ImageFact, error) { return f.images, nil }
func (f *fakeGateway) PullImage(_ context.Context, ref string) (dockerx.ImageFact, error) {
	return dockerx.ImageFact{ImageID: "sha256:new", Ref: ref}, nil
}
func (f *fakeGateway) DeleteImage(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteCalls = append(f.deleteCalls, "image:"+id)
	return nil
}
func (f *fakeGateway) ListVolumes(context.Context) ([]dockerx.VolumeFact, error) {
	return f.volumes, nil
}
func (f *fakeGateway) DeleteVolume(ctx context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteCalls = append(f.deleteCalls, "volume:"+name)
	return nil
}
func (f *fakeGateway) ListNetworks(context.Context) ([]dockerx.NetworkFact, error) {
	return f.networks, nil
}
func (f *fakeGateway) DeleteNetwork(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteCalls = append(f.deleteCalls, "network:"+id)
	return nil
}

func (f *fakeGateway) writes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.deleteCalls...)
}

// fakeRunner 实现 dockersvc.ComposeRunner。
type fakeRunner struct {
	mu    sync.Mutex
	facts []dockerx.ComposeProjectFact
	runs  []string
}

func (f *fakeRunner) ListProjectFacts(context.Context) ([]dockerx.ComposeProjectFact, error) {
	return f.facts, nil
}
func (f *fakeRunner) Run(_ context.Context, p dockerx.ComposeProjectFact, verb string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runs = append(f.runs, p.Project+":"+verb)
	return nil
}
func (f *fakeRunner) Logs(context.Context, dockerx.ComposeProjectFact, int) ([]string, error) {
	return []string{"p-1", "p-2"}, nil
}
func (f *fakeRunner) FollowLogs(context.Context, dockerx.ComposeProjectFact, int) (*dockerx.LogStream, error) {
	return nil, errors.New("compose follow 不在本用例")
}

// countingRecorder 计数审计（保留真库写入链路）。
type countingRecorder struct {
	inner *auditsvc.Service
	mu    sync.Mutex
	n     int
}

func (r *countingRecorder) Record(ctx context.Context, e auditsvc.Entry) error {
	if err := r.inner.Record(ctx, e); err != nil {
		return err
	}
	r.mu.Lock()
	r.n++
	r.mu.Unlock()
	return nil
}
func (r *countingRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.n
}

// ---------- 基座 ----------

type testApp struct {
	ts   *httptest.Server
	gw   *fakeGateway
	comp *fakeRunner
	rec  *countingRecorder
}

func availableProbe() dockerx.DaemonInfo {
	v := "27.5.1"
	return dockerx.DaemonInfo{State: dockerx.StateAvailable, Version: &v}
}

func sampleContainers() []dockerx.ContainerFact {
	return []dockerx.ContainerFact{
		{ContainerID: "c1", Name: "alpha", Image: "nginx:1", State: "running",
			ComposeProject: "web", Labels: map[string]string{
				"com.docker.compose.project":              "web",
				"com.docker.compose.project.working_dir":  "/srv/web",
				"com.docker.compose.project.config_files": "/srv/web/a.yml",
			}},
		{ContainerID: "c2", Name: "beta", Image: "redis:7", State: "stopped"},
	}
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

	gw := &fakeGateway{probe: availableProbe(), list: sampleContainers(),
		state:    map[string]string{"c1": "running", "c2": "running"},
		images:   []dockerx.ImageFact{{ImageID: "i1", Ref: "nginx:1", SizeBytes: 5}},
		volumes:  []dockerx.VolumeFact{{VolumeName: "v1", Mountpoint: "/m", SizeBytes: 1}},
		networks: []dockerx.NetworkFact{{NetworkID: "n1", Name: "bridge", Driver: "bridge", Subnet: "172.17.0.0/16"}}}
	comp := &fakeRunner{facts: []dockerx.ComposeProjectFact{{
		Project: "web", WorkingDir: "/srv/web", ConfigFile: "/srv/web/a.yml", Locatable: true,
		Containers: []dockerx.ComposeContainerRef{{ContainerID: "c1", Name: "alpha", State: "running"}},
	}}}
	rec := &countingRecorder{inner: auditSvc}
	dockerSvc := dockersvc.New(gw, comp, rec)
	server := api.NewServer()
	server.RegisterModules(
		authapi.NewModule(authSvcObj, auditSvc, false),
		NewModule(authSvcObj, dockerSvc),
	)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	return &testApp{ts: ts, gw: gw, comp: comp, rec: rec}
}

// ---------- 会话助手（firewall 同法） ----------

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

func initWizardAndLogin(t *testing.T, app *testApp) session {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"username": "admin", "password": testCred})
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

func loginReadonly(t *testing.T, app *testApp, admin session) session {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"username": "watcher", "password": testCred, "role": "readonly"})
	req, _ := http.NewRequest("POST", app.ts.URL+"/auth/user/create", bytes.NewReader(b))
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
	lreq, _ := http.NewRequest("POST", app.ts.URL+"/auth/session/login", bytes.NewReader(lb))
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
	_ = json.Unmarshal(lenv.Data, &ldata)
	for _, c := range lresp.Cookies() {
		if c.Name == authsvc.SessionCookieName {
			return session{cookie: c.Value, csrf: ldata.CSRFToken}
		}
	}
	t.Fatal("login response missing session cookie")
	return session{}
}

func doPostRaw(t *testing.T, app *testApp, path string, sess *session, raw string) envelope {
	t.Helper()
	req, _ := http.NewRequest("POST", app.ts.URL+path, strings.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if sess != nil {
		req.Header.Set("Cookie", authsvc.SessionCookieName+"="+sess.cookie)
		req.Header.Set(authsvc.CSRFHeader, sess.csrf)
	}
	req.Header.Set("Origin", app.ts.URL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post %s: %v", path, err)
	}
	return decodeEnv(t, resp)
}

func doGet(t *testing.T, app *testApp, path string, sess *session) envelope {
	t.Helper()
	req, _ := http.NewRequest("GET", app.ts.URL+path, nil)
	if sess != nil {
		req.Header.Set("Cookie", authsvc.SessionCookieName+"="+sess.cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get %s: %v", path, err)
	}
	return decodeEnv(t, resp)
}

func doPost(t *testing.T, app *testApp, path string, sess *session, body any) envelope {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", app.ts.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if sess != nil {
		req.Header.Set("Cookie", authsvc.SessionCookieName+"="+sess.cookie)
		req.Header.Set(authsvc.CSRFHeader, sess.csrf)
	}
	req.Header.Set("Origin", app.ts.URL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post %s: %v", path, err)
	}
	return decodeEnv(t, resp)
}

// ---------- 用例 ----------

func TestUnauthenticatedAllEndpoints(t *testing.T) {
	app := newTestApp(t)
	_ = initWizardAndLogin(t, app) // 先过 InitGuard（未初始化时 AUTH_009 先于鉴权）
	paths := []string{
		"/docker/daemon/status", "/docker/container/list", "/docker/container/logs?containerId=c1",
		"/docker/container/stats?containerId=c1", "/docker/image/list", "/docker/volume/list",
		"/docker/network/list", "/docker/compose/list", "/docker/compose/logs?project=web",
	}
	for _, p := range paths {
		env := doGet(t, app, p, nil)
		wantCode(t, env, "AUTH_001")
	}
	env := doPost(t, app, "/docker/container/start", nil, map[string]string{"containerId": "c1"})
	wantCode(t, env, "AUTH_001")
}

func TestDaemonUnavailableGate(t *testing.T) {
	app := newTestApp(t)
	detail := dockerx.DetailNotInstalled
	app.gw.probe = dockerx.DaemonInfo{State: dockerx.StateUnavailable, Detail: &detail}
	admin := initWizardAndLogin(t, app)

	// status 是探测面本身：如实返回 unavailable + detail（引导页依据）。
	env := doGet(t, app, "/docker/daemon/status", &admin)
	wantCode(t, env, "0")
	var st struct {
		State  string  `json:"state"`
		Detail *string `json:"detail"`
	}
	if err := json.Unmarshal(env.Data, &st); err != nil {
		t.Fatalf("status data: %v", err)
	}
	if st.State != "unavailable" || st.Detail == nil || *st.Detail != "not_installed" {
		t.Fatalf("status = %+v", st)
	}

	// 业务面全部 DK_002（不返回空清单——I-01）。
	for _, p := range []string{"/docker/container/list", "/docker/image/list",
		"/docker/volume/list", "/docker/network/list", "/docker/compose/list"} {
		wantCode(t, doGet(t, app, p, &admin), "DK_002")
	}
	wantCode(t, doPost(t, app, "/docker/container/start", &admin,
		map[string]string{"containerId": "c1"}), "DK_002")
	if len(app.gw.writes()) != 0 || app.rec.count() != 0 {
		t.Fatalf("DK_002 拒绝零写侧触达零审计（w=%d a=%d）", len(app.gw.writes()), app.rec.count())
	}
}

func TestReadEndpointsBothRoles(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	readonly := loginReadonly(t, app, admin)

	for _, sess := range []session{admin, readonly} {
		env := doGet(t, app, "/docker/container/list", &sess)
		wantCode(t, env, "0")
		var list struct {
			Items []struct {
				ContainerID      string   `json:"containerId"`
				Name             string   `json:"name"`
				State            string   `json:"state"`
				ComposeProject   *string  `json:"composeProject"`
				SupportedActions []string `json:"supportedActions"`
			} `json:"items"`
			Total  int `json:"total"`
			Daemon struct {
				State   string  `json:"state"`
				Version *string `json:"version"`
			} `json:"daemon"`
		}
		if err := json.Unmarshal(env.Data, &list); err != nil {
			t.Fatalf("list data: %v", err)
		}
		if list.Total != 2 || list.Items[0].Name != "alpha" || list.Items[1].Name != "beta" {
			t.Fatalf("items = %+v", list.Items)
		}
		if list.Items[0].ComposeProject == nil || *list.Items[0].ComposeProject != "web" {
			t.Fatalf("composeProject = %+v", list.Items[0].ComposeProject)
		}
		if len(list.Items[0].SupportedActions) != 3 {
			t.Fatalf("supportedActions = %v", list.Items[0].SupportedActions)
		}
		if list.Daemon.State != "available" || list.Daemon.Version == nil {
			t.Fatalf("daemon 摘要 = %+v", list.Daemon)
		}

		for path, want := range map[string]string{
			"/docker/container/logs?containerId=c1":   "0",
			"/docker/container/stats?containerId=c1":  "0",
			"/docker/image/list":                      "0",
			"/docker/volume/list":                     "0",
			"/docker/network/list":                    "0",
			"/docker/compose/list":                    "0",
			"/docker/compose/logs?project=web":        "0",
			"/docker/container/logs?containerId=":     "DK_001",
			"/docker/container/logs?containerId=gone": "DK_003",
			"/docker/container/list?state=bogus":      "DK_001",
			"/docker/compose/logs?project=ghost":      "DK_014",
		} {
			wantCode(t, doGet(t, app, path, &sess), want)
		}
	}
}

func TestWriteEndpointsAdminOnly(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	readonly := loginReadonly(t, app, admin)

	// readonly 越界：AUTH_008 + 零写触达。
	for _, p := range []string{"/docker/container/start", "/docker/container/delete",
		"/docker/image/pull", "/docker/image/delete", "/docker/volume/delete",
		"/docker/network/delete", "/docker/compose/up"} {
		env := doPost(t, app, p, &readonly, map[string]string{"containerId": "c1"})
		wantCode(t, env, "AUTH_008")
	}
	if len(app.gw.writes()) != 0 || len(app.gw.actionCalls) != 0 || app.rec.count() != 0 {
		t.Fatal("readonly 越界零触达零审计")
	}

	// 缺 CSRF：AUTH_003。
	b, _ := json.Marshal(map[string]string{"containerId": "c2"})
	req, _ := http.NewRequest("POST", app.ts.URL+"/docker/container/start", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", authsvc.SessionCookieName+"="+admin.cookie)
	req.Header.Set("Origin", app.ts.URL)
	resp, _ := http.DefaultClient.Do(req)
	wantCode(t, decodeEnv(t, resp), "AUTH_003")

	// admin 全链路：start(stopped c2) 成功 → DK_004（stop on stopped）→ delete。
	env := doPost(t, app, "/docker/container/start", &admin, map[string]string{"containerId": "c2"})
	wantCode(t, env, "0")
	var data struct {
		ContainerID  string `json:"containerId"`
		CurrentState string `json:"currentState"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil || data.CurrentState != "running" {
		t.Fatalf("start data = %+v", data)
	}
	env = doPost(t, app, "/docker/container/stop", &admin, map[string]string{"containerId": "gone"})
	wantCode(t, env, "DK_003")
	env = doPost(t, app, "/docker/image/pull", &admin, map[string]string{"ref": "nginx:1.25"})
	wantCode(t, env, "0")
	env = doPost(t, app, "/docker/image/pull", &admin, map[string]string{"ref": "BAD REF"})
	wantCode(t, env, "DK_012")
	env = doPost(t, app, "/docker/image/delete", &admin, map[string]string{"imageId": "i1"})
	wantCode(t, env, "0")
	env = doPost(t, app, "/docker/volume/delete", &admin, map[string]string{"volumeName": "v1"})
	wantCode(t, env, "0")
	env = doPost(t, app, "/docker/network/delete", &admin, map[string]string{"networkId": "n1"})
	wantCode(t, env, "0")
	env = doPost(t, app, "/docker/compose/up", &admin, map[string]string{"project": "web"})
	wantCode(t, env, "0")
	env = doPost(t, app, "/docker/compose/up", &admin, map[string]string{"project": "ghost"})
	wantCode(t, env, "DK_014")
	// 畸形 body（非法 JSON）→ AUTH_012；字段缺失 → DK_001（requireRef 唯一
	// 校验通道，R-14）。
	env = doPostRaw(t, app, "/docker/container/start", &admin, "{not-json")
	wantCode(t, env, "AUTH_012")
	env = doPost(t, app, "/docker/container/start", &admin, map[string]any{"x": 1})
	wantCode(t, env, "DK_001")

	// 审计恰 9 条（start ok/stop 003 fail/pull ok/pull 012 fail/image del/vol del/
	// net del/compose up ok/compose 014 fail）。
	if n := app.rec.count(); n != 9 {
		t.Fatalf("审计 = %d, want 9", n)
	}
}

func TestContainerLogsFollowWS(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	app.gw.exists = map[string]bool{"c1": true}
	stub := app.gw.newFollowStub()
	wsURL := "ws" + strings.TrimPrefix(app.ts.URL, "http") + "/docker/container/logs-follow?containerId=c1&tail=100"

	// 坏 Origin → 403（升级前拒绝）。
	d := &websocket.Dialer{}
	h := http.Header{"Origin": []string{"http://evil.example"}}
	_, resp, err := d.Dial(wsURL, h)
	if err == nil {
		t.Fatal("坏 Origin 应拒绝握手")
	}
	if resp == nil || resp.StatusCode != 403 {
		t.Fatalf("坏 Origin status = %v", resp)
	}

	// 无 Cookie → 401。
	h = http.Header{"Origin": []string{app.ts.URL}}
	_, resp, err = d.Dial(wsURL, h)
	if err == nil || resp == nil || resp.StatusCode != 401 {
		t.Fatalf("无会话握手: err=%v resp=%v", err, resp)
	}

	// daemon 不可用 → 503 DK_002（A-D13）。
	app.gw.probe = dockerx.DaemonInfo{State: dockerx.StateUnavailable}
	h = http.Header{"Origin": []string{app.ts.URL},
		"Cookie": []string{authsvc.SessionCookieName + "=" + admin.cookie}}
	_, resp, err = d.Dial(wsURL, h)
	if err == nil || resp == nil || resp.StatusCode != 503 {
		t.Fatalf("daemon 不可用握手: err=%v resp=%v", err, resp)
	}

	// 正常流：log×2 + end(container_stopped) 后服务端关闭（D-03）。
	app.gw.probe = availableProbe()
	conn, resp, err := d.Dial(wsURL, h)
	if err != nil {
		t.Fatalf("dial: %v（resp=%v）", err, resp)
	}
	defer conn.Close()
	stub.send <- "line-1"
	stub.send <- "line-2"
	stub.end <- dockerx.ReasonContainerStopped
	close(stub.send)

	var frames []wsFrame
	deadline := time.Now().Add(3 * time.Second)
	conn.SetReadDeadline(deadline)
	for len(frames) < 3 {
		var f wsFrame
		if err := conn.ReadJSON(&f); err != nil {
			t.Fatalf("read frame #%d: %v（got=%+v）", len(frames), err, frames)
		}
		frames = append(frames, f)
	}
	if frames[0].Type != "log" || frames[0].Line != "line-1" ||
		frames[1].Type != "log" || frames[1].Line != "line-2" ||
		frames[2].Type != "end" || frames[2].Reason != dockerx.ReasonContainerStopped {
		t.Fatalf("frames = %+v", frames)
	}
}

func TestComposeLogsFollowWSReject(t *testing.T) {
	app := newTestApp(t)
	readonly := loginReadonly(t, app, initWizardAndLogin(t, app))
	wsURL := "ws" + strings.TrimPrefix(app.ts.URL, "http") + "/docker/compose/logs-follow?project=web"

	// readonly 可订阅（查看类流，ADR-018）：项目存在且 locatable —— compose
	// 跟随经 ComposeRunner.FollowLogs 替身（本用例仅验证握手角色面，不流帧）。
	d := &websocket.Dialer{}
	h := http.Header{"Origin": []string{app.ts.URL},
		"Cookie": []string{authsvc.SessionCookieName + "=" + readonly.cookie}}
	// project=ghost → DK_014 拒绝握手（400）。
	_, resp, err := d.Dial(
		"ws"+strings.TrimPrefix(app.ts.URL, "http")+"/docker/compose/logs-follow?project=ghost", h)
	if err == nil || resp == nil || resp.StatusCode != 400 {
		t.Fatalf("ghost 项目握手: err=%v resp=%v", err, resp)
	}
	_ = wsURL
}
