package appmgmt

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/dougong/dougong/internal/adapter/pmx"
	"github.com/dougong/dougong/internal/errcode"
	envsvc "github.com/dougong/dougong/internal/service/envmgmt"
	storeapp "github.com/dougong/dougong/internal/store/appmgmt"
)

// ---- Node 运行时替身（ADR-033：npm 通道与 PM2 通道；零实机依赖）----

var (
	tstNode *fakeNode // newTestService 写入：最近一次夹具的 Node 通道替身
	tstPM2  *fakePM2  // 最近一次夹具的 PM2 通道替身
)

type fakeNode struct {
	installDirs []string
	installErr  error
	nodeBin     string
	npmBin      string
	resolveErr  error
}

func (f *fakeNode) NpmInstall(_ context.Context, dir string, _ time.Duration) (string, error) {
	f.installDirs = append(f.installDirs, dir)
	if f.installErr != nil {
		return "", f.installErr
	}
	return "added 1 package", nil
}

func (f *fakeNode) ResolveBins(context.Context) (string, string, error) {
	return f.nodeBin, f.npmBin, f.resolveErr
}

// fakePM2 带状态的 PM2 替身：status 记录已注册进程及其运行态；Save 按实测
// 语义记录「保存时的状态」，SavedStatus 据此回答——这样开机自启的持久语义
// （手动启停不保存、仅三处保存）在测试里是被真实模拟的，而不是手填结果。
type fakePM2 struct {
	started                             []pmx.StartSpec
	startedByName, stopped, restarted   []string
	deleted                             []string
	saves                               int
	saveErr, stopErr, startErr, listErr error
	status                              map[string]bool // 进程名 → 是否运行
	saved                               map[string]string
	tailLines                           []string
	listHook                            func() ([]pmx.Proc, bool) // 非空时 List 直接采用其结果
}

func newFakePM2() *fakePM2 {
	return &fakePM2{status: map[string]bool{}, saved: map[string]string{}}
}

func (f *fakePM2) Start(_ context.Context, spec pmx.StartSpec) (string, error) {
	if f.startErr != nil {
		return "", f.startErr
	}
	f.started = append(f.started, spec)
	f.status[spec.Name] = true
	return "", nil
}
func (f *fakePM2) StartByName(_ context.Context, name string) (string, error) {
	if f.startErr != nil {
		return "", f.startErr
	}
	f.startedByName = append(f.startedByName, name)
	f.status[name] = true
	return "", nil
}
func (f *fakePM2) Stop(_ context.Context, name string) (string, error) {
	if f.stopErr != nil {
		return "", f.stopErr
	}
	f.stopped = append(f.stopped, name)
	if _, ok := f.status[name]; ok {
		f.status[name] = false
	}
	return "", nil
}
func (f *fakePM2) Restart(_ context.Context, name string) (string, error) {
	f.restarted = append(f.restarted, name)
	return "", nil
}
func (f *fakePM2) Delete(_ context.Context, name string) (string, error) {
	f.deleted = append(f.deleted, name)
	delete(f.status, name)
	return "", nil
}
func (f *fakePM2) Save(_ context.Context) (string, error) {
	if f.saveErr != nil {
		return "", f.saveErr
	}
	f.saves++
	for n, online := range f.status {
		f.saved[n] = map[bool]string{true: "online", false: "stopped"}[online]
	}
	return "", nil
}
func (f *fakePM2) List(_ context.Context) ([]pmx.Proc, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if f.listHook != nil {
		if procs, ok := f.listHook(); ok {
			return procs, nil
		}
	}
	var procs []pmx.Proc
	for n, online := range f.status {
		st := "stopped"
		if online {
			st = "online"
		}
		procs = append(procs, pmx.Proc{Name: n, Status: st, Restarts: 2, MemoryBytes: 123,
			OutLogPath: "/h/logs/" + n + "-out.log", ErrLogPath: "/h/logs/" + n + "-error.log"})
	}
	sort.Slice(procs, func(i, j int) bool { return procs[i].Name < procs[j].Name })
	return procs, nil
}
func (f *fakePM2) SavedStatus(name string) (string, bool, error) {
	st, ok := f.saved[name]
	return st, ok, nil
}
func (f *fakePM2) Tail(_ context.Context, _, _ string, _ int) ([]string, error) {
	return f.tailLines, nil
}

// nodeTestService 返回夹具与两个替身。
func nodeTestService(t *testing.T) (*Service, *fakeNode, *fakePM2, *storeapp.Store) {
	t.Helper()
	svc, _, _, st := newTestService(t)
	return svc, tstNode, tstPM2, st
}

func mkNodeDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func errCode(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func missingOf(err error) []string {
	var e *errcode.Error
	if errors.As(err, &e) {
		if m, ok := e.Data().(map[string]any)["missing"].([]string); ok {
			return m
		}
	}
	return nil
}

func createNodeApp(t *testing.T, svc *Service, st *storeapp.Store, name string, port int, startNow bool) storeapp.App {
	t.Helper()
	dir := mkNodeDir(t, map[string]string{"server.js": "require('http')"})
	res, err := svc.Create(context.Background(), actor(), CreateRequest{
		Name: name, Dir: dir, Entry: "server.js", Runtime: "node",
		StartMode: "node_file", ListenPort: port, StartNow: startNow})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	if task := waitSettled(t, st, res.TaskID); task.Status != "succeeded" {
		t.Fatalf("create %s task = %s: %s", name, task.Status, task.OutputTail)
	}
	app, err := st.App().GetByName(context.Background(), name)
	if err != nil {
		t.Fatalf("档案未登记: %v", err)
	}
	return app
}

// ---- 创建 ----

func TestNodeCreateFileMode(t *testing.T) {
	svc, nd, pm, st := nodeTestService(t)
	ctx := context.Background()
	dir := mkNodeDir(t, map[string]string{"server.js": "require('http')"})
	res, err := svc.Create(ctx, actor(), CreateRequest{
		Name: "webapp", Dir: dir, Entry: "server.js", Runtime: "node",
		StartMode: "node_file", StartSpec: StartSpec{File: "server.js"}, ListenPort: 18700, StartNow: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	task := waitSettled(t, st, res.TaskID)
	if task.Status != "succeeded" {
		t.Fatalf("task = %s: %s", task.Status, task.OutputTail)
	}
	if len(pm.started) != 1 {
		t.Fatalf("PM2 注册次数 = %d, want 1", len(pm.started))
	}
	spec := pm.started[0]
	if spec.Name != "dougong-app-webapp" || spec.Cwd != dir {
		t.Fatalf("注册参数错误: %+v", spec)
	}
	if spec.NodeBin != "/usr/bin/node" {
		t.Fatalf("解释器未用 node 绝对路径: %q", spec.NodeBin)
	}
	want := []string{dir, "/usr/bin/node", "/usr/bin/npm", "file", "server.js"}
	if strings.Join(spec.Args, " ") != strings.Join(want, " ") {
		t.Fatalf("启动器参数 = %v, want %v", spec.Args, want)
	}
	if _, err := os.Stat(spec.Launcher); err != nil {
		t.Fatalf("启动器未落位: %v", err)
	}
	if pm.saves != 1 {
		t.Fatalf("创建应恰好保存一次进程列表，实际 %d", pm.saves)
	}
	// 创建即运行 + 保存 → 开机自启为开启
	if pm.saved["dougong-app-webapp"] != "online" {
		t.Fatalf("创建并启动后保存状态应为 online: %v", pm.saved)
	}
	// 无 package.json → 不装依赖
	if len(nd.installDirs) != 0 {
		t.Fatalf("无 package.json 不应执行 npm install: %v", nd.installDirs)
	}
	app, err := st.App().GetByName(ctx, "webapp")
	if err != nil {
		t.Fatalf("档案未登记: %v", err)
	}
	if app.Runtime != "node" || app.StartMode != "node_file" || app.UnitName != "dougong-app-webapp" ||
		app.Entry != "server.js" || app.ListenPort != 18700 {
		t.Fatalf("档案字段错误: %+v", app)
	}
	// 清单下发应用实际保存的启动参数（参数编辑预填的数据源）
	list, _ := svc.AppList(ctx, "node")
	if sp := list.Items[0].StartSpec; sp == nil || sp.File != "server.js" {
		t.Fatalf("清单应带出实际保存的启动参数: %+v", sp)
	}
}

// TestParamsPrefillFromStoredSpec 参数编辑以应用实际保存的启动参数为准：此前
// 前端只能拿创建表单的上次输入顶替，对早先创建的应用会把启动参数悄悄改成
// 默认值。清单下发 startSpec 后，按它原样提交不改变任何参数。
func TestParamsPrefillFromStoredSpec(t *testing.T) {
	svc, _, _, st := newTestService(t)
	ctx := context.Background()
	dir := mkNodeDir(t, map[string]string{"app.py": "pass"})
	res, err := svc.Create(ctx, actor(), CreateRequest{Name: "real", Dir: dir, Entry: "app.py",
		StartMode: "preset_fastapi", StartSpec: StartSpec{Module: "mymod", App: "myapp", Args: "--workers 2"},
		ListenPort: 18711})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	waitSettled(t, st, res.TaskID)
	list, _ := svc.AppList(ctx, "python")
	sp := list.Items[0].StartSpec
	if sp == nil || sp.Module != "mymod" || sp.App != "myapp" || sp.Args != "--workers 2" {
		t.Fatalf("清单应带出实际保存的启动参数: %+v", sp)
	}
	// 以清单下发的值原样提交 → 保存成功且档案不变
	if _, err := svc.ParamsSave(ctx, actor(), ParamsSaveRequest{AppID: list.Items[0].ID, StartSpec: sp}); err != nil {
		t.Fatalf("按实际参数保存应成功: %v", err)
	}
	after, _ := st.App().GetByName(ctx, "real")
	if !strings.Contains(after.StartSpec, `"module":"mymod"`) || !strings.Contains(after.StartSpec, `"args":"--workers 2"`) {
		t.Fatalf("原样提交不应改变启动参数: %s", after.StartSpec)
	}
}

func TestNodeCreateNpmScriptMode(t *testing.T) {
	svc, nd, pm, st := nodeTestService(t)
	ctx := context.Background()
	dir := mkNodeDir(t, map[string]string{"package.json": `{"scripts":{"start":"node server.js"}}`})
	res, err := svc.Create(ctx, actor(), CreateRequest{
		Name: "svr", Dir: dir, Entry: "", Runtime: "node",
		StartMode: "npm_script", ListenPort: 18701, StartNow: true}) // 脚本名缺省 start
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	task := waitSettled(t, st, res.TaskID)
	if task.Status != "succeeded" {
		t.Fatalf("task = %s: %s", task.Status, task.OutputTail)
	}
	if len(nd.installDirs) != 1 || nd.installDirs[0] != dir {
		t.Fatalf("有 package.json 应在项目目录执行 npm install: %v", nd.installDirs)
	}
	args := pm.started[0].Args
	if strings.Join(args[len(args)-2:], " ") != "script start" {
		t.Fatalf("npm 脚本参数 = %v，脚本名应缺省为 start", args)
	}
	app, _ := st.App().GetByName(ctx, "svr")
	if app.StartMode != "npm_script" || !strings.Contains(app.StartSpec, `"script":"start"`) {
		t.Fatalf("档案启动方式错误: mode=%s spec=%s", app.StartMode, app.StartSpec)
	}
}

func TestNodeCreateNotStartNow(t *testing.T) {
	svc, _, pm, st := nodeTestService(t)
	app := createNodeApp(t, svc, st, "cold", 18702, false)
	// 未选立即启动：注册后停止，再保存——保存时为停止态 → 下次开机不自启
	if len(pm.stopped) != 1 || pm.stopped[0] != app.UnitName {
		t.Fatalf("未选立即启动应停止进程: %v", pm.stopped)
	}
	if pm.saves != 1 || pm.saved[app.UnitName] != "stopped" {
		t.Fatalf("保存语义错误: saves=%d saved=%v", pm.saves, pm.saved)
	}
}

func TestNodeCreateRejections(t *testing.T) {
	svc, _, pm, _ := nodeTestService(t)
	ctx := context.Background()
	dir := mkNodeDir(t, map[string]string{"server.js": "x"})
	base := CreateRequest{Name: "n1", Dir: dir, Entry: "server.js", Runtime: "node",
		StartMode: "node_file", ListenPort: 18703}

	// 容器运行形态已下线（ADR-033）
	r := base
	r.StartMode = "container"
	if _, err := svc.Create(ctx, actor(), r); !strings.Contains(errCode(err), "APP_014") {
		t.Fatalf("container 应 APP_014: %v", err)
	}
	// APP_016：入口越出项目目录 / 脚本名非法 / 启动方式未知
	for name, mut := range map[string]func(*CreateRequest){
		"入口含 ..":         func(r *CreateRequest) { r.StartSpec = StartSpec{File: "../etc/x.js"} },
		"入口为 POSIX 绝对路径": func(r *CreateRequest) { r.StartSpec = StartSpec{File: "/etc/x.js"} },
		"入口为盘符绝对路径":      func(r *CreateRequest) { r.StartSpec = StartSpec{File: `C:\etc\x.js`} },
		"脚本名含空格":         func(r *CreateRequest) { r.StartMode = "npm_script"; r.StartSpec = StartSpec{Script: "my script"} },
		"未知 Node 启动方式":   func(r *CreateRequest) { r.StartMode = "preset_fastapi"; r.StartSpec = StartSpec{Module: "a", App: "b"} },
	} {
		rr := base
		mut(&rr)
		if _, err := svc.Create(ctx, actor(), rr); !strings.Contains(errCode(err), "APP_016") {
			t.Fatalf("%s: 应 APP_016，实际 %v", name, err)
		}
	}
	// 上述全部拒绝都不应触达 PM2（前置校验拦在前）
	if len(pm.started) != 0 || pm.saves != 0 {
		t.Fatalf("被拒的创建不应注册进程: started=%d saves=%d", len(pm.started), pm.saves)
	}
	// APP_015：Node 环境缺 PM2（data.missing 逐项列出）
	svc.envReady = fakeReady{rd: envsvc.PythonReadiness{Installed: true, VenvSupport: true, PipAvailable: true},
		node: envsvc.NodeReadiness{NodeAvailable: true, NpmAvailable: true, Pm2Available: false}}
	_, err := svc.Create(ctx, actor(), base)
	if !strings.Contains(errCode(err), "APP_015") {
		t.Fatalf("缺 PM2 应 APP_015: %v", err)
	}
	if m := missingOf(err); len(m) != 1 || m[0] != "pm2" {
		t.Fatalf("缺失项应逐项列出 [pm2]: %v", m)
	}
	// runtime 取值域
	r = base
	r.Runtime = "ruby"
	if _, err := svc.Create(ctx, actor(), r); !strings.Contains(errCode(err), "APP_012") {
		t.Fatalf("未知运行时应被拒: %v", err)
	}
}

// ---- 服务操作与开机自启（D-17：手动启停不保存，仅三处保存）----

func TestNodeServiceOpAutostartSemantics(t *testing.T) {
	svc, _, pm, st := nodeTestService(t)
	ctx := context.Background()
	app := createNodeApp(t, svc, st, "ops", 18704, true)
	before := pm.saves

	// 手动停掉：不保存 → 开机自启设置保持「开启」（下次开机仍会启动）
	if _, err := svc.ServiceOp(ctx, actor(), app.ID, "stop"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if pm.saves != before {
		t.Fatalf("手动停止不应保存进程列表: %d → %d", before, pm.saves)
	}
	if pm.status[app.UnitName] {
		t.Fatal("停止后进程应不再运行")
	}
	data, _ := svc.AppList(ctx, "node")
	if it := data.Items[0]; it.ServiceRunning == nil || *it.ServiceRunning || it.ServiceEnabled == nil || !*it.ServiceEnabled {
		t.Fatalf("手动停止后应为「已停止、开机自启仍开启」: running=%v enabled=%v", it.ServiceRunning, it.ServiceEnabled)
	}
	// 手动启动 / 重启：同样不保存
	for _, op := range []string{"start", "restart"} {
		if _, err := svc.ServiceOp(ctx, actor(), app.ID, op); err != nil {
			t.Fatalf("%s: %v", op, err)
		}
	}
	if pm.saves != before {
		t.Fatalf("手动启停重启不应保存进程列表: %d", pm.saves)
	}
	// 关闭开机自启=停止+保存
	if _, err := svc.ServiceOp(ctx, actor(), app.ID, "disable"); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if pm.saves != before+1 || pm.saved[app.UnitName] != "stopped" || pm.status[app.UnitName] {
		t.Fatalf("disable 应停止并保存为 stopped: saves=%d saved=%v running=%v", pm.saves, pm.saved[app.UnitName], pm.status[app.UnitName])
	}
	// 已关闭后手动启动：仍不保存 → 下次开机仍不会启动
	if _, err := svc.ServiceOp(ctx, actor(), app.ID, "start"); err != nil {
		t.Fatalf("start: %v", err)
	}
	data, _ = svc.AppList(ctx, "node")
	if it := data.Items[0]; !*it.ServiceRunning || *it.ServiceEnabled {
		t.Fatalf("关闭后手动启动应为「运行中、开机自启仍关闭」: running=%v enabled=%v", *it.ServiceRunning, *it.ServiceEnabled)
	}
	// 开启开机自启=启动+保存
	if _, err := svc.ServiceOp(ctx, actor(), app.ID, "enable"); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if pm.saves != before+2 || pm.saved[app.UnitName] != "online" || !pm.status[app.UnitName] {
		t.Fatalf("enable 应启动并保存为 online: saves=%d saved=%v", pm.saves, pm.saved[app.UnitName])
	}
	// 保存失败：开机自启设置未生效应如实报错（不能静默成功）
	pm.saveErr = errors.New("disk full")
	if _, err := svc.ServiceOp(ctx, actor(), app.ID, "disable"); err == nil || !strings.Contains(errCode(err), "APP_010") {
		t.Fatalf("保存失败应 APP_010: %v", err)
	}
}

// ---- 依赖重装 / 日志 / 删除 ----

func TestNodeDepsDeleteAndLogs(t *testing.T) {
	svc, nd, pm, st := nodeTestService(t)
	ctx := context.Background()
	dir := mkNodeDir(t, map[string]string{"server.js": "x", "package.json": "{}"})
	res, err := svc.Create(ctx, actor(), CreateRequest{
		Name: "full", Dir: dir, Entry: "server.js", Runtime: "node",
		StartMode: "node_file", ListenPort: 18705, StartNow: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	waitSettled(t, st, res.TaskID)
	app, _ := st.App().GetByName(ctx, "full")

	// 依赖重装=npm install（Node 不要求 venv；不自动重启）
	d, err := svc.Deps(ctx, actor(), app.ID)
	if err != nil {
		t.Fatalf("deps: %v", err)
	}
	if task := waitSettled(t, st, d.TaskID); task.Status != "succeeded" || len(nd.installDirs) != 2 {
		t.Fatalf("依赖重装应执行 npm install: status=%s installs=%v", task.Status, nd.installDirs)
	}
	if len(pm.restarted) != 0 {
		t.Fatal("依赖重装不应自动重启")
	}
	nd.installErr = errors.New("E404 not found")
	d, _ = svc.Deps(ctx, actor(), app.ID)
	if task := waitSettled(t, st, d.TaskID); task.Status != "failed" || !strings.Contains(*task.Error, "依赖安装失败") {
		t.Fatalf("安装失败应任务失败并带原因: %+v", task)
	}
	// 无 package.json 的项目如实拒绝（不虚构任务）
	bare := createNodeApp(t, svc, st, "bare", 18708, true)
	if _, err := svc.Deps(ctx, actor(), bare.ID); err == nil || !strings.Contains(errCode(err), "APP_012") {
		t.Fatalf("无 package.json 应被拒: %v", err)
	}

	// 日志：取 PM2 日志文件尾部
	pm.tailLines = []string{"out line", "[错误] boom"}
	log, err := svc.LogTail(ctx, app.ID, 200)
	if err != nil || log.Content != "out line\n[错误] boom" {
		t.Fatalf("Node 日志: %+v err=%v", log, err)
	}
	pm.listErr = errors.New("pm2 down")
	if _, err := svc.LogTail(ctx, app.ID, 200); err == nil {
		t.Fatal("PM2 不可用时日志查询应报错")
	}
	pm.listErr = nil

	// 删除：PM2 进程注销 + 保存 + 档案撤除
	saves := pm.saves
	d, err = svc.Delete(ctx, actor(), DeleteRequest{AppID: app.ID})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if task := waitSettled(t, st, d.TaskID); task.Status != "succeeded" {
		t.Fatalf("delete task = %s: %s", task.Status, task.OutputTail)
	}
	if len(pm.deleted) != 1 || pm.deleted[0] != app.UnitName || pm.saves != saves+1 {
		t.Fatalf("删除应注销进程并保存: deleted=%v saves=%d", pm.deleted, pm.saves)
	}
	if _, ok := pm.status[app.UnitName]; ok {
		t.Fatal("删除后进程应已注销")
	}
	if _, err := st.App().GetByID(ctx, app.ID); err == nil {
		t.Fatal("档案应已撤除")
	}
}

// ---- 参数保存：Node 改启动方式须重新注册进程，且保留运行态与开机自启 ----

func TestNodeParamsSaveReRegisters(t *testing.T) {
	svc, _, pm, st := nodeTestService(t)
	ctx := context.Background()
	app := createNodeApp(t, svc, st, "reg", 18709, true) // 运行中 + 开机自启开启

	// 只改 .env：不动进程（启动器每次启动时读取 .env）
	envLines := []string{"PORT=18709", "GREETING=hi"}
	if _, err := svc.ParamsSave(ctx, actor(), ParamsSaveRequest{AppID: app.ID, EnvLines: envLines}); err != nil {
		t.Fatalf("save env: %v", err)
	}
	if len(pm.deleted) != 0 || len(pm.started) != 1 {
		t.Fatal("只改 .env 不应重新注册进程")
	}
	raw, _ := os.ReadFile(filepath.Join(app.Dir, ".env"))
	if strings.TrimSpace(string(raw)) != "PORT=18709\nGREETING=hi" {
		t.Fatalf(".env 未按预期写入: %q", raw)
	}

	// 改启动文件：重新注册（注销 + 注册），运行中保持运行中，开机自启保持开启
	saves := pm.saves
	out, err := svc.ParamsSave(ctx, actor(), ParamsSaveRequest{AppID: app.ID,
		StartSpec: &StartSpec{File: "bin/www.js"}})
	if err != nil {
		t.Fatalf("save start spec: %v", err)
	}
	if len(pm.deleted) != 1 || len(pm.started) != 2 {
		t.Fatalf("改启动方式应重新注册: deleted=%d started=%d", len(pm.deleted), len(pm.started))
	}
	if got := pm.started[1].Args; got[len(got)-1] != "bin/www.js" {
		t.Fatalf("新注册应使用新启动文件: %v", got)
	}
	if !pm.status[app.UnitName] || pm.saved[app.UnitName] != "online" || pm.saves != saves+1 {
		t.Fatalf("重新注册应保持运行中并重新保存: running=%v saved=%v saves=%d→%d",
			pm.status[app.UnitName], pm.saved[app.UnitName], saves, pm.saves)
	}
	if out.NeedRestart {
		t.Fatal("进程已按新参数重新注册，不应再提示需重启")
	}
	updated, _ := st.App().GetByName(ctx, "reg")
	if !strings.Contains(updated.StartSpec, `"file":"bin/www.js"`) {
		t.Fatalf("档案启动参数未更新: %s", updated.StartSpec)
	}

	// 非法的启动参数被拒，且不触碰进程
	n := len(pm.deleted)
	if _, err := svc.ParamsSave(ctx, actor(), ParamsSaveRequest{AppID: app.ID,
		StartSpec: &StartSpec{File: "../../etc/passwd"}}); !strings.Contains(errCode(err), "APP_016") {
		t.Fatalf("越界入口应 APP_016: %v", err)
	}
	if len(pm.deleted) != n {
		t.Fatal("被拒的参数保存不应注销进程")
	}
}

// ---- 列表：运行时过滤与 PM2 实况字段 ----

func TestAppListRuntimeFilterAndPM2Fields(t *testing.T) {
	svc, _, pm, st := nodeTestService(t)
	ctx := context.Background()
	app := createNodeApp(t, svc, st, "listed", 18706, true)
	dir := app.Dir
	if _, err := st.App().Create(ctx, storeapp.App{Name: "pyapp", Dir: dir, Entry: "server.js",
		Runtime: storeapp.RuntimePython, StartMode: "preset_fastapi", ListenPort: 18707, UnitName: "dougong-app-pyapp.service"}); err != nil {
		t.Fatalf("seed python app: %v", err)
	}

	all, err := svc.AppList(ctx, "")
	if err != nil || len(all.Items) != 2 {
		t.Fatalf("全部清单: n=%d err=%v", len(all.Items), err)
	}
	if !all.NodeReadiness.Pm2Available || all.NodeReadiness.NodeVersion == nil || *all.NodeReadiness.NodeVersion != "22.14.0" {
		t.Fatalf("Node 就绪度响应错误: %+v", all.NodeReadiness)
	}
	nodeOnly, _ := svc.AppList(ctx, "node")
	if len(nodeOnly.Items) != 1 || nodeOnly.Items[0].Runtime != "node" {
		t.Fatalf("runtime=node 过滤结果错误: %+v", nodeOnly.Items)
	}
	item := nodeOnly.Items[0]
	if item.Pm2Status == nil || *item.Pm2Status != "online" || item.ServiceRunning == nil || !*item.ServiceRunning ||
		item.Restarts == nil || *item.Restarts != 2 || item.MemoryBytes == nil {
		t.Fatalf("PM2 实况字段错误: %+v", item)
	}
	if item.ServiceEnabled == nil || !*item.ServiceEnabled {
		t.Fatalf("创建即运行并保存的项目开机自启应为 true: %v", item.ServiceEnabled)
	}
	if item.VenvExists {
		t.Fatal("Node 项目不应探测 venv")
	}
	// 进程未注册 → 实况字段全部 null（区分「未注册」与「已停止」）
	delete(pm.status, app.UnitName)
	nodeOnly, _ = svc.AppList(ctx, "node")
	if it := nodeOnly.Items[0]; it.Pm2Status != nil || it.ServiceRunning != nil || it.ServiceEnabled != nil {
		t.Fatalf("未注册进程的实况字段应为 null: %+v", it)
	}
	// 探测失败 → null（不虚构为 false）
	pm.listErr = errors.New("pm2 down")
	nodeOnly, _ = svc.AppList(ctx, "node")
	if it := nodeOnly.Items[0]; it.ServiceRunning != nil {
		t.Fatalf("探测失败应为 null 而非 false: %+v", it)
	}
	pm.listErr = nil
	pyOnly, _ := svc.AppList(ctx, "python")
	if len(pyOnly.Items) != 1 || pyOnly.Items[0].Name != "pyapp" || pyOnly.Items[0].Pm2Status != nil {
		t.Fatalf("runtime=python 过滤结果错误: %+v", pyOnly.Items)
	}
}

func TestNormalizeNodeStartSpec(t *testing.T) {
	if sp, err := normalizeNodeStartSpec("node_file", StartSpec{File: "bin/www"}); err != nil || sp.File != "bin/www" {
		t.Fatalf("bin/www: %+v %v", sp, err)
	}
	if sp, err := normalizeNodeStartSpec("npm_script", StartSpec{}); err != nil || sp.Script != "start" {
		t.Fatalf("脚本名缺省 start: %+v %v", sp, err)
	}
	if _, err := normalizeNodeStartSpec("node_file", StartSpec{}); err == nil {
		t.Fatal("空入口文件应被拒")
	}
}

// TestIsAbsAny 绝对路径的判定不依赖操作系统：同一份输入在 Windows（开发机）与
// Linux（服务器、CI）上必须得到相同结果。v0.4.0 发版时 CI 在 Linux 上挂过一次——
// 判定用了 filepath.IsAbs，盘符路径在 Linux 上不算绝对路径，被当成项目内的相对路径放行。
func TestIsAbsAny(t *testing.T) {
	absolute := []string{
		"/etc/x.js",           // POSIX
		`\etc\x.js`,           // 根相对（Windows）
		`C:\etc\x.js`,         // 盘符 + 反斜杠
		"C:/etc/x.js",         // 盘符 + 斜杠
		"c:/x.js",             // 小写盘符
		`Z:\`,                 // 只有盘符根
		"1:/x",                // 冒号前不是字母也算（与 Windows 一致）
		"//server/share/x.js", // UNC 路径
	}
	for _, p := range absolute {
		if !isAbsAny(p) {
			t.Errorf("isAbsAny(%q) = false, want true", p)
		}
	}
	relative := []string{
		"server.js",
		"bin/www",
		"./server.js",
		"C:",     // 只有盘符、没有根
		"C:x.js", // 盘符相对路径（没有紧跟的分隔符）
		"ab:/x",  // 第二个字符不是冒号
		"a.js",
	}
	for _, p := range relative {
		if isAbsAny(p) {
			t.Errorf("isAbsAny(%q) = true, want false", p)
		}
	}
}
