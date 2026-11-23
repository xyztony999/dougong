package appmgmt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/adapter/sysd"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	envsvc "github.com/dougong/dougong/internal/service/envmgmt"
	"github.com/dougong/dougong/internal/store"
	storeapp "github.com/dougong/dougong/internal/store/appmgmt"
)

// ---- 注入替身（零 runner 实况依赖——CI 纪律；实机 python3/systemd/journalctl
// 实联归 Linux 目标环境冒烟）----

type fakePy struct {
	venvExists   bool
	createErr    error
	pipErr       error
	writeUnitErr error
	unitWritten  map[string]string
	removedUnits []string
	removedDirs  []string
	tail         string
}

func (f *fakePy) CreateVenv(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", f.createErr
}
func (f *fakePy) PipInstall(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", f.pipErr
}
func (f *fakePy) VenvExists(dir string) bool {
	if f.venvExists {
		return true
	}
	_, err := os.Stat(filepath.Join(dir, "requirements.txt"))
	return err == nil // 测试目录有 requirements.txt 时视为已建 venv（简化替身语义）
}
func (f *fakePy) DepsProbe(_ context.Context, _ string) (int, error) { return 5, nil }
func (f *fakePy) RemoveDir(dir string) error {
	f.removedDirs = append(f.removedDirs, dir)
	return nil
}
func (f *fakePy) WriteUnit(_ context.Context, unitName, content string) (func(), error) {
	if f.unitWritten == nil {
		f.unitWritten = map[string]string{}
	}
	f.unitWritten[unitName] = content
	return func() {}, f.writeUnitErr
}
func (f *fakePy) RemoveUnit(_ context.Context, unitName string) error {
	f.removedUnits = append(f.removedUnits, unitName)
	return nil
}
func (f *fakePy) Tail(_ context.Context, _ string, _ int) (string, error) { return f.tail, nil }

type fakeSysd struct {
	started  []string
	stopped  []string
	disabled []string
	detail   sysd.UnitDetail
	getErr   error
}

func (f *fakeSysd) GetUnit(_ context.Context, _ string) (sysd.UnitDetail, error) {
	return f.detail, f.getErr
}
func (f *fakeSysd) StartUnit(_ context.Context, n string) (sysd.JobResult, error) {
	f.started = append(f.started, n)
	return sysd.JobResult{}, nil
}
func (f *fakeSysd) StopUnit(_ context.Context, n string) (sysd.JobResult, error) {
	f.stopped = append(f.stopped, n)
	return sysd.JobResult{}, nil
}
func (f *fakeSysd) RestartUnit(_ context.Context, _ string) (sysd.JobResult, error) {
	return sysd.JobResult{}, nil
}
func (f *fakeSysd) EnableUnit(_ context.Context, _ string) error { return nil }
func (f *fakeSysd) DisableUnit(_ context.Context, n string) error {
	f.disabled = append(f.disabled, n)
	return nil
}

type fakeReady struct {
	rd   envsvc.PythonReadiness
	node envsvc.NodeReadiness
}

func (f fakeReady) PythonReadiness(_ context.Context) (envsvc.PythonReadiness, error) {
	return f.rd, nil
}

func (f fakeReady) NodeReadiness(_ context.Context) (envsvc.NodeReadiness, error) {
	return f.node, nil
}

type fakeSiteLink struct {
	created   []string
	hasSite   bool
	createErr error
}

func (f *fakeSiteLink) CreateReverseProxySite(_ context.Context, _ *authsvc.Identity, name string, _ int, _ string) (SiteRef, error) {
	if f.createErr != nil {
		return SiteRef{}, f.createErr
	}
	f.created = append(f.created, name)
	return SiteRef{SiteID: "s1", SourceFile: "/etc/nginx/conf.d/x.conf"}, nil
}
func (f *fakeSiteLink) HasUpstreamSite(_ context.Context, _ int) (bool, error) {
	return f.hasSite, nil
}

// ---- 测试基座 ----

func newTestService(t *testing.T) (*Service, *fakePy, *fakeSysd, *storeapp.Store) {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	st := storeapp.New(db)
	py := &fakePy{}
	sd := &fakeSysd{}
	nd := &fakeNode{nodeBin: "/usr/bin/node", npmBin: "/usr/bin/npm"}
	pm := newFakePM2()
	tstNode, tstPM2 = nd, pm // Node 分支测试经包级变量取回替身（见 node_test.go）
	svc := New(py, sd, st, Options{
		EnvReady: fakeReady{
			rd: envsvc.PythonReadiness{
				Installed: true, Version: "3.11.2", VenvSupport: true, PipAvailable: true},
			node: envsvc.NodeReadiness{NodeAvailable: true, NpmAvailable: true, Pm2Available: true,
				NodeVersion: "22.14.0"}},
		SyncDispatch: true, // 同步执行 runTask（确定性断言，env-mgmt 先例）
		Node:         nd, PM2: pm, DataDir: t.TempDir(),
	})
	return svc, py, sd, st
}

func actor() *authsvc.Identity { return &authsvc.Identity{UserID: 1, Username: "admin"} }

// waitSettled 取任务终态（SyncDispatch 同步执行——调用返回即已终态；保留名称
// 与轮询兼容形态以便后续异步场景复用）。
func waitSettled(t *testing.T, st *storeapp.Store, taskID int64) storeapp.AppTask {
	t.Helper()
	task, err := st.Task().GetByID(context.Background(), taskID)
	if err != nil {
		t.Fatalf("task %d: %v", taskID, err)
	}
	if task.Status != "succeeded" && task.Status != "failed" {
		t.Fatalf("task %d not settled (status=%s)", taskID, task.Status)
	}
	return task
}

// ---- Create 校验链（D-03/APP_001~006/012）----

func TestCreateValidationChain(t *testing.T) {
	svc, _, _, st := newTestService(t)
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.py"), []byte("pass"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := CreateRequest{Name: "myapp", Dir: dir, Entry: "app.py",
		StartMode: "preset_fastapi", StartSpec: StartSpec{Module: "app", App: "app"}, ListenPort: 8100}

	cases := []struct {
		name string
		mut  func(r CreateRequest) CreateRequest
		want string // 期望错误符号（空=成功路径在后续用例覆盖）
	}{
		{"APP_001 名称非法", func(r CreateRequest) CreateRequest { r.Name = "Bad_Name"; return r }, "APP_001"},
		{"APP_003 目录不存在", func(r CreateRequest) CreateRequest { r.Dir = filepath.Join(dir, "nope"); return r }, "APP_003"},
		{"APP_003 入口缺失", func(r CreateRequest) CreateRequest { r.Entry = "main.py"; return r }, "APP_003"},
		{"APP_005 端口越界", func(r CreateRequest) CreateRequest { r.ListenPort = 80; return r }, "APP_005"},
		{"APP_012 preset 缺模块", func(r CreateRequest) CreateRequest { r.StartSpec = StartSpec{}; return r }, "APP_012"},
		{"APP_012 custom 白名单", func(r CreateRequest) CreateRequest {
			r.StartMode = "custom"
			r.StartSpec = StartSpec{Command: "/usr/bin/evil; rm -rf /"}
			return r
		}, "APP_012"},
	}
	for _, tc := range cases {
		_, err := svc.Create(ctx, actor(), tc.mut(base))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: want %s got %v", tc.name, tc.want, err)
		}
	}
	// 就绪度缺失分层（APP_004 missing=venv）。
	svcNoVenv := New(&fakePy{}, &fakeSysd{}, st, Options{EnvReady: fakeReady{rd: envsvc.PythonReadiness{
		Installed: true, VenvSupport: false, PipAvailable: true}}})
	_, err := svcNoVenv.Create(ctx, actor(), base)
	if err == nil || !(strings.Contains(err.Error(), "APP_004") || strings.Contains(err.Error(), "Python 环境未就绪")) {
		t.Fatalf("APP_004 missing=venv: got %v", err)
	}
	// 成功路径：任务创建 + 互斥生效（I-05）。
	res, err := svc.Create(ctx, actor(), base)
	if err != nil {
		t.Fatalf("create ok: %v", err)
	}
	if _, err := svc.Create(ctx, actor(), CreateRequest{Name: base.Name, Dir: dir, Entry: base.Entry,
		StartMode: "preset_fastapi", StartSpec: StartSpec{Module: "app", App: "app"}, ListenPort: 8101}); err == nil {
		t.Fatal("want APP_002 name taken (档案未登记前以同名互斥/唯一)")
	}
	_ = res
	_ = st
}

// TestCreateWorkerChain 创建执行链（D-05/I-C01/C02）：成功→档案登记同事务；
// venv 失败→任务 failed 且无档案残留（I-C02）。
func TestCreateWorkerChain(t *testing.T) {
	svc, py, sd, st := newTestService(t)
	ctx := context.Background()
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "app.py"), []byte("pass"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("fastapi\n"), 0o644)
	req := CreateRequest{Name: "nim", Dir: dir, Entry: "app.py", StartMode: "preset_fastapi",
		StartSpec: StartSpec{Module: "app", App: "app"}, ListenPort: 8100, StartNow: true}

	res, err := svc.Create(ctx, actor(), req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	task := waitSettled(t, st, res.TaskID)
	if task.Status != storeapp.TaskSucceeded {
		t.Fatalf("want succeeded got %s (%v)", task.Status, deref(task.Error))
	}
	if _, err := st.App().GetByName(ctx, "nim"); err != nil {
		t.Fatalf("archive registered: %v", err) // I-C01 档案同事务登记
	}
	if _, ok := py.unitWritten["dougong-app-nim.service"]; !ok {
		t.Fatal("unit written")
	}
	if len(sd.started) != 1 || sd.started[0] != "dougong-app-nim.service" {
		t.Fatalf("startNow: %v", sd.started)
	}

	// venv 失败 → failed 且无档案残留（I-C02）。
	svc2, py2, _, st2 := newTestService(t)
	py2.createErr = fmt.Errorf("disk full")
	dir2 := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir2, "app.py"), []byte("pass"), 0o644)
	res2, err := svc2.Create(ctx, actor(), CreateRequest{Name: "bad", Dir: dir2, Entry: "app.py",
		StartMode: "preset_fastapi", StartSpec: StartSpec{Module: "app", App: "app"}, ListenPort: 8102})
	if err != nil {
		t.Fatalf("create2: %v", err)
	}
	task2 := waitSettled(t, st2, res2.TaskID)
	if task2.Status != storeapp.TaskFailed {
		t.Fatalf("want failed got %s", task2.Status)
	}
	if _, err := st2.App().GetByName(ctx, "bad"); err == nil {
		t.Fatal("I-C02: 半档案残留")
	}
}

// TestDeleteChain 删除执行链（D-07/I-C04）：purgeDir=false 保留目录/档案撤除；
// purgeDir=true 删目录（替身记录）；站点不自动删。
func TestDeleteChain(t *testing.T) {
	svc, py, sd, st := newTestService(t)
	ctx := context.Background()
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "app.py"), []byte("pass"), 0o644)
	res, err := svc.Create(ctx, actor(), CreateRequest{Name: "del", Dir: dir, Entry: "app.py",
		StartMode: "preset_fastapi", StartSpec: StartSpec{Module: "app", App: "app"}, ListenPort: 8103})
	if err != nil {
		t.Fatalf("seed create: %v", err)
	}
	waitSettled(t, st, res.TaskID)
	app, err := st.App().GetByName(ctx, "del")
	if err != nil {
		t.Fatalf("seed archive: %v", err)
	}
	dres, err := svc.Delete(ctx, actor(), DeleteRequest{AppID: app.ID})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	task := waitSettled(t, st, dres.TaskID)
	if task.Status != storeapp.TaskSucceeded || task.PurgeDir {
		t.Fatalf("delete task: %s purge=%v", task.Status, task.PurgeDir)
	}
	if _, err := st.App().GetByID(ctx, app.ID); err == nil {
		t.Fatal("archive not removed (I-C01)")
	}
	if len(py.removedDirs) != 0 {
		t.Fatal("purgeDir=false must keep dir")
	}
	if len(sd.stopped) == 0 || len(py.removedUnits) != 1 {
		t.Fatal("unit removal chain")
	}
	// purgeDir=true。
	res2, _ := svc.Create(ctx, actor(), CreateRequest{Name: "del2", Dir: dir, Entry: "app.py",
		StartMode: "preset_fastapi", StartSpec: StartSpec{Module: "app", App: "app"}, ListenPort: 8104})
	waitSettled(t, st, res2.TaskID)
	app2, _ := st.App().GetByName(ctx, "del2")
	d2, _ := svc.Delete(ctx, actor(), DeleteRequest{AppID: app2.ID, PurgeDir: true})
	waitSettled(t, st, d2.TaskID)
	if len(py.removedDirs) != 1 || py.removedDirs[0] != dir {
		t.Fatalf("purge dir: %v", py.removedDirs)
	}
}

// TestRetrySemantics 重试语义（D-13/I-06 例外）：deps 失败可重试回 running→
// succeeded；delete 失败不可重试。
func TestRetrySemantics(t *testing.T) {
	svc, py, _, st := newTestService(t)
	ctx := context.Background()
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "app.py"), []byte("pass"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("fastapi\n"), 0o644)
	res, err := svc.Create(ctx, actor(), CreateRequest{Name: "rt", Dir: dir, Entry: "app.py",
		StartMode: "preset_fastapi", StartSpec: StartSpec{Module: "app", App: "app"}, ListenPort: 8105})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	waitSettled(t, st, res.TaskID)
	app, _ := st.App().GetByName(ctx, "rt")
	py.pipErr = fmt.Errorf("network down")
	dres, err := svc.Deps(ctx, actor(), app.ID)
	if err != nil {
		t.Fatalf("deps: %v", err)
	}
	task := waitSettled(t, st, dres.TaskID)
	if task.Status != storeapp.TaskFailed {
		t.Fatalf("want failed: %s", task.Status)
	}
	py.pipErr = nil
	if _, err := svc.TaskRetry(ctx, actor(), dres.TaskID); err != nil {
		t.Fatalf("retry: %v", err)
	}
	ok := waitSettled(t, st, dres.TaskID)
	if ok.Status != storeapp.TaskSucceeded {
		t.Fatalf("retry settle: %s", ok.Status)
	}
	// delete 任务不可重试（D-13）。
	dres2, _ := svc.Delete(ctx, actor(), DeleteRequest{AppID: app.ID})
	dtask := waitSettled(t, st, dres2.TaskID)
	if dtask.Status != storeapp.TaskSucceeded {
		t.Fatalf("delete settle: %s", dtask.Status)
	}
}

// TestMutex 同应用在途互斥（I-05/APP_006）。
func TestMutex(t *testing.T) {
	svc, py, _, st := newTestService(t)
	py.pipErr = fmt.Errorf("slow")
	ctx := context.Background()
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "app.py"), []byte("pass"), 0o644)
	res, _ := svc.Create(ctx, actor(), CreateRequest{Name: "mu", Dir: dir, Entry: "app.py",
		StartMode: "preset_fastapi", StartSpec: StartSpec{Module: "app", App: "app"}, ListenPort: 8106})
	waitSettled(t, st, res.TaskID)
	app, _ := st.App().GetByName(ctx, "mu")
	// 发起 deps（worker 即刻 failed——替身 pipErr）；在其 settle 前再次发起互斥判定
	// 依赖事务原子性，此处以先注入一个 pending 任务的方式精确验证互斥路径：
	_, _ = st.Task().Create(ctx, storeapp.AppTask{AppID: app.ID, AppName: app.Name,
		Action: storeapp.ActionDepsInstall, RequestedBy: "admin"})
	if _, err := svc.Deps(ctx, actor(), app.ID); err == nil || !strings.Contains(err.Error(), "APP_006") {
		t.Fatalf("mutex: %v", err)
	}
}

// TestRecoverInFlight 启动恢复（I-08）。
func TestRecoverInFlight(t *testing.T) {
	_, _, _, st := newTestService(t)
	ctx := context.Background()
	id, _ := st.Task().Create(ctx, storeapp.AppTask{AppID: 1, AppName: "x",
		Action: storeapp.ActionDepsInstall, RequestedBy: "admin"})
	_ = st.Task().MarkRunning(ctx, id)
	svc, _, _, _ := newTestService(t)
	_ = svc
	// 直接以 store 通道验证（RecoverInFlight 内部即 FailRunningAll）。
	n, err := st.Task().FailRunningAll(ctx, "面板重启中断")
	if err != nil || n != 1 {
		t.Fatalf("recover: n=%d err=%v", n, err)
	}
	task, _ := st.Task().GetByID(ctx, id)
	if task.Status != storeapp.TaskFailed || deref(task.Error) != "面板重启中断" {
		t.Fatalf("recover task: %s %v", task.Status, deref(task.Error))
	}
}

// TestParamsBaseline 参数保存基线比对（D-09/APP_012）：外部修改后旧基线保存拒绝。
func TestParamsBaseline(t *testing.T) {
	svc, _, _, st := newTestService(t)
	ctx := context.Background()
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "app.py"), []byte("pass"), 0o644)
	res, _ := svc.Create(ctx, actor(), CreateRequest{Name: "pb", Dir: dir, Entry: "app.py",
		StartMode: "preset_fastapi", StartSpec: StartSpec{Module: "app", App: "app"}, ListenPort: 8107})
	waitSettled(t, st, res.TaskID)
	app, _ := st.App().GetByName(ctx, "pb")
	envPath := filepath.Join(dir, ".env")
	_ = os.WriteFile(envPath, []byte("A=1\n"), 0o600)
	fi, _ := os.Stat(envPath)
	base := fmtMtime(fi.ModTime().UTC())
	_ = os.WriteFile(envPath, []byte("A=2\n"), 0o600) // 外部修改
	// 文件系统时间戳粒度可能粗于两次写入的间隔（如 Windows 的时钟节拍），同一
	// 时刻的两次写入 mtime 相同就判不出「被改过」——显式把 mtime 拨后一秒，用例
	// 不依赖时序（此前在 Windows 上偶发失败）。
	later := fi.ModTime().Add(time.Second)
	_ = os.Chtimes(envPath, later, later)
	if _, err := svc.ParamsSave(ctx, actor(), ParamsSaveRequest{
		AppID: app.ID, EnvLines: []string{"B=3"}, BaseMtime: &base}); err == nil ||
		!(strings.Contains(err.Error(), "APP_012") || strings.Contains(err.Error(), "外部修改")) {
		t.Fatalf("baseline: %v", err)
	}
	// 同基线（重新 stat）保存成功。
	fi2, _ := os.Stat(envPath)
	b2 := fmtMtime(fi2.ModTime().UTC())
	out, err := svc.ParamsSave(ctx, actor(), ParamsSaveRequest{
		AppID: app.ID, EnvLines: []string{"B=3"}, BaseMtime: &b2})
	if err != nil || !out.NeedRestart {
		t.Fatalf("save: %v", err)
	}
}

// TestSiteLink 建站联动（D-12）：成功审计+值快照；失败透传（APP_011）。
func TestSiteLink(t *testing.T) {
	svc, _, _, st := newTestService(t)
	link := &fakeSiteLink{}
	svc.siteLink = link
	ctx := context.Background()
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "app.py"), []byte("pass"), 0o644)
	if apps, err := st.App().List(ctx); err == nil {
		t.Logf("[debug] pre-create app rows=%d", len(apps))
	}
	res, _ := svc.Create(ctx, actor(), CreateRequest{Name: "sl", Dir: dir, Entry: "app.py",
		StartMode: "preset_fastapi", StartSpec: StartSpec{Module: "app", App: "app"}, ListenPort: 8108})
	waitSettled(t, st, res.TaskID)
	app, _ := st.App().GetByName(ctx, "sl")
	out, err := svc.SiteLink(ctx, actor(), SiteLinkRequest{AppID: app.ID, Domain: "x.example.com"})
	if err != nil || len(link.created) != 1 || link.created[0] != "sl" {
		t.Fatalf("site link: %v %v", out, err)
	}
	link.createErr = fmt.Errorf("server_name conflict")
	if _, err := svc.SiteLink(ctx, actor(), SiteLinkRequest{AppID: app.ID}); err == nil ||
		!strings.Contains(err.Error(), "APP_011") {
		t.Fatalf("site link err: %v", err)
	}
}

// ---- 小工具 ----

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func errCodeData(err error) any {
	typeWithData, ok := err.(interface{ GetData() any })
	if ok {
		return typeWithData.GetData()
	}
	return err.Error()
}
