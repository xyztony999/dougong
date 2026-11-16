// Package cronback 测试：httptest 全栈（真实 :memory: SQLite + 真实审计服务，
// cronx/backupx 网关替换 fake——servicemgmt module_test 同法）。
package cronback

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/adapter/backupx"
	"github.com/dougong/dougong/internal/adapter/cronx"
	"github.com/dougong/dougong/internal/api"
	authapi "github.com/dougong/dougong/internal/api/auth"
	auditsvc "github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	cronbacksvc "github.com/dougong/dougong/internal/service/cronback"
	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	storecron "github.com/dougong/dougong/internal/store/cronback"
)

const testCred = "dougong-test-only" // 测试口令（非真实凭据，仅 :memory: 库）

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

// ---- fake 网关（镜像 service 层测试替身语义，经 Module 注入真实装配链）----

type fakeTab struct {
	mu      sync.Mutex
	content string
	sys     cronx.SystemEntries
}

func (f *fakeTab) List(context.Context) (*cronx.TabProjection, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &cronx.TabProjection{
		Entries:       cronx.ParseTabLines(strings.Split(strings.TrimSuffix(f.content, "\n"), "\n")),
		SystemEntries: f.sys,
	}, nil
}
func (f *fakeTab) ReadRaw(context.Context) (string, error) { return f.content, nil }
func (f *fakeTab) Upsert(_ context.Context, oldKey string, job cronx.JobInput) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	lines := strings.Split(strings.TrimSuffix(f.content, "\n"), "\n")
	if f.content == "" {
		lines = nil
	}
	out, key, err := cronx.RenderTabJob(lines, oldKey, job)
	if err != nil {
		return "", err
	}
	f.content = strings.Join(out, "\n") + "\n"
	return key, nil
}
func (f *fakeTab) Delete(_ context.Context, jobKey string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	lines := strings.Split(strings.TrimSuffix(f.content, "\n"), "\n")
	out, err := cronx.RemoveTabJob(lines, jobKey)
	if err != nil {
		return err
	}
	f.content = strings.Join(out, "\n") + "\n"
	return nil
}

type fakeLogs struct{ text string }

func (f fakeLogs) Tail(context.Context, int) (string, error) { return f.text, nil }

type fakeRunner struct{ res cronx.TriggerResult }

func (f *fakeRunner) Run(context.Context, string, time.Duration) cronx.TriggerResult {
	return f.res
}

type fakeExec struct {
	mu        sync.Mutex
	execRes   backupx.ExecResult
	pullLocal string
	manifest  *backupx.Manifest
}

func (f *fakeExec) Execute(context.Context, backupx.JobSpec, time.Time) (backupx.ExecResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.execRes, nil
}
func (f *fakeExec) PullFile(_ context.Context, _, _ string, _ int) (string, func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pullLocal, func() {}, nil
}
func (f *fakeExec) ReadManifestPath(_ context.Context, _ string, _ int) (*backupx.Manifest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.manifest, nil
}
func (f *fakeExec) RemoveArtifactPath(context.Context, string, int) error { return nil }

type fakeInst struct{}

func (fakeInst) InstanceArchive(_ context.Context, id int64) (bool, string, string, error) {
	return id == 7, "mysql", "local", nil
}

// ---- 装置 ----

type testApp struct {
	ts   *httptest.Server
	tab  *fakeTab
	exec *fakeExec
	svc  *cronbacksvc.Service
	st   *storecron.Store
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()
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
		t.Fatalf("ensure init row: %v", err)
	}
	hasher, err := authsvc.NewBcryptHasher(10)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	authSvcObj := authsvc.New(repo, hasher, authsvc.NewTotpService(nil), authsvc.DefaultSessionPolicy())
	auditSvc := auditsvc.New(repo.Audit(), time.Now)

	sysDirs := make([]cronx.CronDir, 0, 4)
	for _, d := range []string{"/etc/cron.hourly", "/etc/cron.daily", "/etc/cron.weekly", "/etc/cron.monthly"} {
		sysDirs = append(sysDirs, cronx.CronDir{Dir: d, Scripts: []string{}})
	}
	tab := &fakeTab{
		content: "0 2 * * * /legacy/nightly.sh\n",
		sys:     cronx.SystemEntries{CronDFiles: []cronx.CronDFile{}, CronDirs: sysDirs},
	}
	exec := &fakeExec{}
	st := storecron.New(db)
	svc := cronbacksvc.New(tab, fakeLogs{text: "CRON[1]: (root) CMD (/legacy/nightly.sh)"},
		&fakeRunner{res: cronx.TriggerResult{}}, exec,
		cronbacksvc.AdaptStore(st), auditSvc, fakeInst{}, "/var/lib/dougong")

	server := api.NewServer()
	server.RegisterModules(
		authapi.NewModule(authSvcObj, auditSvc, false),
		NewModule(authSvcObj, svc),
	)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	return &testApp{ts: ts, tab: tab, exec: exec, svc: svc, st: st}
}

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

func loginAs(t *testing.T, app *testApp, username string) session {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"username": username, "password": testCred})
	resp, err := http.Post(app.ts.URL+"/auth/session/login", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("login %s: %v", username, err)
	}
	env := decodeEnv(t, resp)
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

func doReq(t *testing.T, app *testApp, method, path string, sess *session, body map[string]any, csrf bool) envelope {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, app.ts.URL+path, rd)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if sess != nil {
		req.Header.Set("Cookie", authsvc.SessionCookieName+"="+sess.cookie)
	}
	if csrf && sess != nil {
		req.Header.Set(authsvc.CSRFHeader, sess.csrf)
		req.Header.Set("Origin", app.ts.URL)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return decodeEnv(t, resp)
}

func TestUnauthenticatedAndRoles(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	// readonly 创建+登录。
	doReq(t, app, "POST", "/auth/user/create", &admin,
		map[string]any{"username": "watcher", "password": testCred, "role": "readonly"}, true)
	ro := loginAs(t, app, "watcher")

	// 未登录 → AUTH_001（InitGuard 已过：向导完成）。
	env := doReq(t, app, "GET", "/cron/job/list", nil, nil, false)
	wantCode(t, env, "AUTH_001")

	// readonly：GET 两角色可看（ADR-018 全可看；log/tail 为纯文本通道单独验证；
	// detail 对不存在 run 回 CB_007 业务码——非 AUTH_008 即证明可访问）。
	for _, tc := range []struct{ path, want string }{
		{"/cron/job/list", "0"},
		{"/cron/backup-job/list", "0"},
		{"/cron/backup-run/list", "0"},
		{"/cron/backup-run/detail?runId=1", "CB_007"},
	} {
		env := doReq(t, app, "GET", tc.path, &ro, nil, false)
		wantCode(t, env, tc.want)
	}
	// readonly：log/tail 纯文本 200。
	req, _ := http.NewRequest("GET", app.ts.URL+"/cron/log/tail", nil)
	req.Header.Set("Cookie", authsvc.SessionCookieName+"="+ro.cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("readonly tail: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("readonly tail status = %d", resp.StatusCode)
	}
	// readonly：download 越界 → AUTH_008（ADR-018 禁上传下载）。
	env = doReq(t, app, "GET", "/cron/backup-run/download?runId=1&file=x", &ro, nil, false)
	wantCode(t, env, "AUTH_008")
	// readonly：POST 越界 → AUTH_008。
	env = doReq(t, app, "POST", "/cron/job/validate", &ro, map[string]any{"expr": "* * * * *"}, true)
	wantCode(t, env, "AUTH_008")
}

func TestCsrfGuardOnWrites(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)
	env := doReq(t, app, "POST", "/cron/job/validate", &admin, map[string]any{"expr": "* * * * *"}, false)
	wantCode(t, env, "AUTH_003")
}

func TestCronJobFlowOverHTTP(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)

	// job/list：legacy 任务投影 + 系统预置位结构。
	env := doReq(t, app, "GET", "/cron/job/list", &admin, nil, false)
	wantCode(t, env, "0")
	var list struct {
		Items []struct {
			JobKey string  `json:"jobKey"`
			Expr   string  `json:"expr"`
			Source string  `json:"source"`
			Remark *string `json:"remark"`
		} `json:"items"`
		SystemEntries struct {
			CronDFiles []json.RawMessage `json:"cronDFiles"`
			CronDirs   []struct {
				Dir     string   `json:"dir"`
				Scripts []string `json:"scripts"`
			} `json:"cronDirs"`
		} `json:"systemEntries"`
	}
	if err := json.Unmarshal(env.Data, &list); err != nil {
		t.Fatalf("list data: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Source != "existing" || list.Items[0].Remark != nil {
		t.Fatalf("legacy projection wrong: %+v", list.Items)
	}
	if len(list.SystemEntries.CronDirs) != 4 {
		t.Fatalf("cronDirs = %d, want 4", len(list.SystemEntries.CronDirs))
	}

	// validate：成功预览 + CB_002 段位错误。
	env = doReq(t, app, "POST", "/cron/job/validate", &admin, map[string]any{"expr": "@daily"}, true)
	wantCode(t, env, "0")
	var prev struct {
		NextFires []string `json:"nextFires"`
	}
	_ = json.Unmarshal(env.Data, &prev)
	if len(prev.NextFires) != 5 {
		t.Fatalf("nextFires = %d", len(prev.NextFires))
	}
	env = doReq(t, app, "POST", "/cron/job/validate", &admin, map[string]any{"expr": "61 * * *"}, true)
	wantCode(t, env, "CB_002")

	// create：标记注释写入 + jobKey 返回。
	remark := "面板托管"
	env = doReq(t, app, "POST", "/cron/job/create", &admin, map[string]any{
		"expr": "*/10 * * * *", "command": "/scripts/x.sh", "remark": remark}, true)
	wantCode(t, env, "0")
	var created struct {
		JobKey string `json:"jobKey"`
	}
	_ = json.Unmarshal(env.Data, &created)
	if created.JobKey == "" {
		t.Fatal("jobKey missing")
	}
	// 列表呈现 panel 来源与备注（D-02）。
	env = doReq(t, app, "GET", "/cron/job/list", &admin, nil, false)
	_ = json.Unmarshal(env.Data, &list)
	var panelItem, legacyItem bool
	for _, it := range list.Items {
		if it.JobKey == created.JobKey && it.Source == "panel" && it.Remark != nil && *it.Remark == remark {
			panelItem = true
		}
		if it.Expr == "0 2 * * *" && it.Source == "existing" {
			legacyItem = true
		}
	}
	if !panelItem || !legacyItem {
		t.Fatalf("panel/legacy projection: panel=%v legacy=%v", panelItem, legacyItem)
	}

	// update → 新 jobKey；旧 key delete → CB_003。
	env = doReq(t, app, "POST", "/cron/job/update", &admin, map[string]any{
		"jobKey": created.JobKey, "expr": "*/5 * * * *", "command": "/scripts/x.sh"}, true)
	wantCode(t, env, "0")
	var updated struct {
		JobKey string `json:"jobKey"`
	}
	_ = json.Unmarshal(env.Data, &updated)
	if updated.JobKey == created.JobKey {
		t.Fatal("jobKey must change on update")
	}
	env = doReq(t, app, "POST", "/cron/job/delete", &admin, map[string]any{"jobKey": created.JobKey}, true)
	wantCode(t, env, "CB_003")
	env = doReq(t, app, "POST", "/cron/job/delete", &admin, map[string]any{"jobKey": updated.JobKey}, true)
	wantCode(t, env, "0")

	// trigger：同步业务结果（D-05）——exitCode null + output。
	env = doReq(t, app, "POST", "/cron/job/trigger", &admin,
		map[string]any{"jobKey": cronx.MakeJobKey("0 2 * * *", "/legacy/nightly.sh")}, true)
	wantCode(t, env, "0")
	var trig struct {
		ExitCode   *int   `json:"exitCode"`
		Output     string `json:"output"`
		TimedOut   bool   `json:"timedOut"`
		DurationMs int64  `json:"durationMs"`
	}
	_ = json.Unmarshal(env.Data, &trig)
	if trig.TimedOut {
		t.Fatalf("trigger result wrong: %+v", trig)
	}
	// 面板外已删 → CB_003。
	env = doReq(t, app, "POST", "/cron/job/trigger", &admin, map[string]any{"jobKey": "ghost\tc"}, true)
	wantCode(t, env, "CB_003")

	// log/tail：200 纯文本。
	req, _ := http.NewRequest("GET", app.ts.URL+"/cron/log/tail?lines=50", nil)
	req.Header.Set("Cookie", authsvc.SessionCookieName+"="+admin.cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), "text/plain") ||
		!strings.Contains(string(body), "CRON[1]") {
		t.Fatalf("tail: status=%d type=%s body=%s", resp.StatusCode, resp.Header.Get("Content-Type"), body)
	}
}

func TestBackupFlowOverHTTP(t *testing.T) {
	app := newTestApp(t)
	admin := initWizardAndLogin(t, app)

	jobBody := map[string]any{
		"name": "nightly",
		"sourceItems": []map[string]any{
			{"type": "site_dir", "path": "/var/www/site"},
		},
		"targetType":     "local",
		"targetConfig":   map[string]any{"path": "/var/backups"},
		"retentionCount": 3,
		"scheduleCron":   "0 3 * * *",
	}
	// CB_001：目标=面板数据目录（A-C8）。
	bad := map[string]any{}
	for k, v := range jobBody {
		bad[k] = v
	}
	bad["targetConfig"] = map[string]any{"path": "/var/lib/dougong"}
	env := doReq(t, app, "POST", "/cron/backup-job/create", &admin, bad, true)
	wantCode(t, env, "CB_001")
	// 创建成功。
	env = doReq(t, app, "POST", "/cron/backup-job/create", &admin, jobBody, true)
	wantCode(t, env, "0")
	var created struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(env.Data, &created)
	if created.ID <= 0 {
		t.Fatalf("created id = %d", created.ID)
	}
	// CB_005 重名。
	env = doReq(t, app, "POST", "/cron/backup-job/create", &admin, jobBody, true)
	wantCode(t, env, "CB_005")

	// job/list：nextFireAt 推算。
	env = doReq(t, app, "GET", "/cron/backup-job/list", &admin, nil, false)
	wantCode(t, env, "0")
	var jobs struct {
		Items []struct {
			ID         int64   `json:"id"`
			NextFireAt *string `json:"nextFireAt"`
			LastRun    *struct {
				ID int64 `json:"id"`
			} `json:"lastRun"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &jobs)
	if len(jobs.Items) != 1 || jobs.Items[0].NextFireAt == nil || jobs.Items[0].LastRun != nil {
		t.Fatalf("job list wrong: %+v", jobs.Items)
	}

	// run：异步返回 runId（D-07）；worker 消费后终态 success。
	app.exec.execRes = backupx.ExecResult{
		ArtifactPath: "/var/backups/nightly/x",
		ArtifactSize: 10,
		Files:        []backupx.FileEntry{{Name: "site-1.tar.gz", Size: 10}},
	}
	env = doReq(t, app, "POST", "/cron/backup-job/run", &admin, map[string]any{"jobId": created.ID}, true)
	wantCode(t, env, "0")
	var ran struct {
		RunID int64 `json:"runId"`
	}
	_ = json.Unmarshal(env.Data, &ran)
	if ran.RunID <= 0 {
		t.Fatalf("runId = %d", ran.RunID)
	}
	// 消费队列执行（等价 main 的 worker 编排）。
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = app.svc.RunExecutor(ctx)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		env = doReq(t, app, "GET",
			fmt.Sprintf("/cron/backup-run/detail?runId=%d", ran.RunID), &admin, nil, false)
		var d struct {
			Status string `json:"status"`
		}
		_ = json.Unmarshal(env.Data, &d)
		if d.Status == "success" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	wg.Wait()
	app.exec.manifest = &backupx.Manifest{ // detail sources 来自 manifest 语义（A-C5）
		Sources: []backupx.SourceSpec{
			{Kind: "site_dir", Ref: "/var/www/site"},
			{Kind: "database", InstanceID: 7, Database: "shop", Ref: "local·shop"},
		},
	}
	env = doReq(t, app, "GET",
		fmt.Sprintf("/cron/backup-run/detail?runId=%d", ran.RunID), &admin, nil, false)
	wantCode(t, env, "0")
	var detail struct {
		Status string `json:"status"`
		Files  []struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
		} `json:"files"`
		Sources []struct {
			Type string `json:"type"`
			Ref  string `json:"ref"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(env.Data, &detail); err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Status != "success" || len(detail.Files) != 1 || detail.Files[0].Name != "site-1.tar.gz" {
		t.Fatalf("detail wrong: %+v", detail)
	}
	if len(detail.Sources) != 2 || detail.Sources[0].Ref != "/var/www/site" || detail.Sources[1].Ref != "local·shop" {
		t.Fatalf("detail sources ref 形状断裂（E1 回归防线）: %+v", detail.Sources)
	}

	// list：筛选 status=success 命中 + 分页形状。
	env = doReq(t, app, "GET", "/cron/backup-run/list?status=success&page=1&pageSize=20", &admin, nil, false)
	wantCode(t, env, "0")
	var runs struct {
		Items []struct {
			ID       int64  `json:"id"`
			JobName  string `json:"jobName"`
			Duration *int64 `json:"durationMs"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	_ = json.Unmarshal(env.Data, &runs)
	if runs.Total != 1 || len(runs.Items) != 1 || runs.Items[0].JobName != "nightly" ||
		runs.Items[0].Duration == nil {
		t.Fatalf("run list wrong: %+v", runs)
	}

	// download：成功流（attachment 原名，D-10/D-16）。
	pulled := filepath.Join(t.TempDir(), "site-1.tar.gz")
	if err := os.WriteFile(pulled, []byte("012345678"), 0o644); err != nil {
		t.Fatalf("seed pulled: %v", err)
	}
	app.exec.pullLocal = pulled
	req, _ := http.NewRequest("GET",
		app.ts.URL+fmt.Sprintf("/cron/backup-run/download?runId=%d&file=site-1.tar.gz", ran.RunID), nil)
	req.Header.Set("Cookie", authsvc.SessionCookieName+"="+admin.cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Disposition") == "" ||
		!strings.Contains(resp.Header.Get("Content-Disposition"), "site-1.tar.gz") ||
		len(body) != 9 {
		t.Fatalf("download: status=%d cd=%q len=%d", resp.StatusCode,
			resp.Header.Get("Content-Disposition"), len(body))
	}
	// CB_001：非法文件名。
	env = doReq(t, app, "GET",
		fmt.Sprintf("/cron/backup-run/download?runId=%d&file=../x", ran.RunID), &admin, nil, false)
	wantCode(t, env, "CB_001")
	// CB_007：run 不存在。
	env = doReq(t, app, "GET", "/cron/backup-run/download?runId=999&file=a", &admin, nil, false)
	wantCode(t, env, "CB_007")
	// runId 形态非法 → CB_001。
	env = doReq(t, app, "GET", "/cron/backup-run/detail?runId=abc", &admin, nil, false)
	wantCode(t, env, "CB_001")

	// delete job：run 不级联（FR-09），历史仍可查。
	env = doReq(t, app, "POST", "/cron/backup-job/delete", &admin, map[string]any{"jobId": created.ID}, true)
	wantCode(t, env, "0")
	env = doReq(t, app, "GET", "/cron/backup-run/list?jobId="+fmt.Sprint(created.ID), &admin, nil, false)
	wantCode(t, env, "0")
	_ = json.Unmarshal(env.Data, &runs)
	if runs.Total != 1 {
		t.Fatalf("runs after job delete = %d, want 1（弱引用归组 D-12）", runs.Total)
	}
	// CB_004。
	env = doReq(t, app, "POST", "/cron/backup-job/run", &admin, map[string]any{"jobId": created.ID}, true)
	wantCode(t, env, "CB_004")
}
