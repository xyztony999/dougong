package dockermgmt

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/dougong/dougong/internal/adapter/dockerx"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	"gopkg.in/yaml.v3"
)

// ---- 夹具 ----

// newPanelService 注入临时落盘根的服务夹具（落盘扫描测试必须隔离在临时目录）。
func newPanelService(t *testing.T, opts ...Option) (*Service, *composeRunner, *fakeAudit, string) {
	t.Helper()
	root := t.TempDir()
	svc, _, runner, au := newCreateService(t, append([]Option{WithComposeRootDir(root)}, opts...)...)
	return svc, runner, au, root
}

// writeProject 在落盘根下放一个项目目录与编排文件，返回目录路径。
func writeProject(t *testing.T, root, project, text string) string {
	t.Helper()
	dir := filepath.Join(root, project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ComposeFileName), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// writeAppDir 放一个面板应用的编排文件（带归属标签、带端口映射），返回目录路径。
func writeAppDir(t *testing.T, root, name string, port int) string {
	t.Helper()
	text := fmt.Sprintf("services:\n  app:\n    image: nginx:alpine\n    ports:\n      - \"127.0.0.1:%d:80\"\n    labels:\n      dougong.app: %s\n", port, name)
	return writeProject(t, root, name, text)
}

func runningCtr(name string) dockerx.ComposeContainerRef {
	return dockerx.ComposeContainerRef{ContainerID: "c-" + name, Name: name, State: dockerx.StatusRunning}
}

type fakeSiteLink struct {
	ports    []int
	domains  [][]string
	hasSite  bool
	createEr error
}

func (f *fakeSiteLink) CreateReverseProxySite(_ context.Context, _ *authsvc.Identity, _ string, port int, domains []string) (AppSiteRef, error) {
	if f.createEr != nil {
		return AppSiteRef{}, f.createEr
	}
	f.ports = append(f.ports, port)
	f.domains = append(f.domains, domains)
	return AppSiteRef{SiteID: "s-1", SourceFile: "/etc/nginx/conf.d/x.conf"}, nil
}

func (f *fakeSiteLink) HasUpstreamSite(_ context.Context, _ int) (bool, error) { return f.hasSite, nil }

func sym(err error) string {
	var e *errcode.Error
	if errors.As(err, &e) {
		return e.Symbol()
	}
	return ""
}

func reasonOf(err error) string {
	var e *errcode.Error
	if errors.As(err, &e) {
		if m, ok := e.Data().(map[string]any); ok {
			if r, ok := m["reason"].(string); ok {
				return r
			}
		}
	}
	return ""
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func validAppReq(name string, port int) AppCreateRequest {
	return AppCreateRequest{Name: name, Image: "nginx:alpine", HostPort: port, ContainerPort: 80}
}

func dirExists(p string) bool { _, err := os.Stat(p); return err == nil }

// ---- 落盘扫描 ----

func TestScanPanelRoot(t *testing.T) {
	if out, err := scanPanelRoot(filepath.Join(t.TempDir(), "missing")); err != nil || len(out) != 0 {
		t.Fatalf("根不存在应为空清单: %v %v", out, err)
	}
	root := t.TempDir()
	writeProject(t, root, "plain", "services:\n  web:\n    image: busybox\n")
	// 映射形式标签（面板应用编排的写法）与端口
	writeProject(t, root, "app-map", "services:\n  app:\n    image: nginx:1.27\n    ports:\n      - \"127.0.0.1:18080:80\"\n    labels:\n      dougong.app: myapp\n")
	// 列表形式标签（手写常见）
	writeProject(t, root, "app-list", "services:\n  web:\n    image: redis:7\n    ports:\n    - 6379:6379\n    labels:\n      - dougong.app=cache\n      - team=ops\n")
	// 无编排文件的目录、不合命名规则的目录、解析不了的文件
	_ = os.MkdirAll(filepath.Join(root, "empty"), 0o755)
	writeProject(t, root, "Bad_Name", "services:\n  web:\n    image: busybox\n")
	writeProject(t, root, "broken", "services:\n  web:\n    image: [unclosed\n")

	out, err := scanPanelRoot(root)
	if err != nil {
		t.Fatalf("scanPanelRoot: %v", err)
	}
	got := map[string]panelCompose{}
	for _, pc := range out {
		got[pc.project] = pc
	}
	if len(out) != 4 {
		t.Fatalf("项目数 = %d（无编排文件与不合命名的目录不应被认领）: %+v", len(out), out)
	}
	if p := got["plain"]; p.isApp || !p.parsed {
		t.Fatalf("普通项目: %+v", p)
	}
	if a := got["app-map"]; !a.isApp || a.appName != "myapp" || a.image != "nginx:1.27" || a.hostPort != 18080 || a.containerPort != 80 {
		t.Fatalf("映射形式标签的应用解析错误: %+v", a)
	}
	if a := got["app-list"]; !a.isApp || a.appName != "cache" || a.image != "redis:7" || a.hostPort != 6379 {
		t.Fatalf("列表形式标签的应用解析错误: %+v", a)
	}
	if b := got["broken"]; b.isApp || b.parsed || b.configFile == "" {
		t.Fatalf("解析失败的目录应仍作为项目呈现（可删除）且不当作应用: %+v", b)
	}
	for i := 1; i < len(out); i++ { // 遍历顺序稳定（按项目名排序）
		if out[i-1].project > out[i].project {
			t.Fatalf("结果应按项目名排序: %v", out)
		}
	}
}

func TestParseComposePort(t *testing.T) {
	cases := map[string][2]int{
		"127.0.0.1:18080:80": {18080, 80},
		"8080:80":            {8080, 80},
		"80":                 {0, 0},
		"a:b":                {0, 0},
	}
	for in, want := range cases {
		if h, c := parseComposePort(in); h != want[0] || c != want[1] {
			t.Errorf("parseComposePort(%q) = %d,%d want %v", in, h, c, want)
		}
	}
}

// ---- Compose 列表：零容器项目可见；应用不进 Compose 列表 ----

func TestComposeListIncludesZeroContainerPanelProjects(t *testing.T) {
	svc, runner, _, root := newPanelService(t)
	outsideDir := filepath.Join(t.TempDir(), "outside")
	writeProject(t, root, "ghost", "services:\n  web:\n    image: busybox\n") // 零容器：label 聚合看不到
	writeProject(t, root, "live", "services:\n  web:\n    image: busybox\n")  // 面板项目且容器在跑
	writeProject(t, root, "myapp", "services:\n  app:\n    image: nginx\n    labels:\n      dougong.app: myapp\n")
	runner.facts = []dockerx.ComposeProjectFact{
		{Project: "outside", WorkingDir: outsideDir, Containers: []dockerx.ComposeContainerRef{runningCtr("o")}},
		{Project: "live", WorkingDir: filepath.Join(root, "live"), Containers: []dockerx.ComposeContainerRef{runningCtr("l")}},
	}

	data, err := svc.ComposeList(context.Background())
	if err != nil {
		t.Fatalf("ComposeList: %v", err)
	}
	byName := map[string]ComposeItem{}
	for _, it := range data.Items {
		byName[it.Project] = it
	}
	if len(data.Items) != 3 || data.Total != 3 {
		t.Fatalf("项目清单 = %+v（应用 myapp 不应出现在 Compose 列表）", data.Items)
	}
	ghost := byName["ghost"]
	if len(ghost.Containers) != 0 || !ghost.Locatable || !ghost.PanelManaged ||
		ghost.WorkingDir == nil || *ghost.WorkingDir != filepath.Join(root, "ghost") ||
		ghost.ConfigFile == nil || *ghost.ConfigFile != filepath.Join(root, "ghost", ComposeFileName) {
		t.Fatalf("零容器的面板项目应可见、可定位、标记为面板创建: %+v", ghost)
	}
	live := byName["live"]
	if len(live.Containers) != 1 || !live.PanelManaged {
		t.Fatalf("有容器的面板项目应保留容器实况并标记为面板创建: %+v", live)
	}
	if o := byName["outside"]; o.PanelManaged || len(o.Containers) != 1 {
		t.Fatalf("面板外项目不应被标记为面板创建: %+v", o)
	}
}

func TestComposeActionOnZeroContainerProject(t *testing.T) {
	svc, runner, _, root := newPanelService(t)
	writeProject(t, root, "ghost", "services:\n  web:\n    image: busybox\n")
	// 历史缺陷：down 之后零容器，项目在 label 聚合里消失，再启动报 DK_014。
	if _, err := svc.ComposeAction(context.Background(), "admin", "ghost", dockerx.VerbUp); err != nil {
		t.Fatalf("零容器项目应可启动: %v", err)
	}
	if runs := runner.runs(); len(runs) != 1 || runs[0] != "ghost:up" {
		t.Fatalf("runner 调用 = %v, want [ghost:up]", runs)
	}
	if _, err := svc.ComposeAction(context.Background(), "admin", "nope", dockerx.VerbUp); sym(err) != "DK_014" {
		t.Fatalf("不存在的项目应 DK_014: %v", err)
	}
}

// ---- Compose 项目删除 ----

func TestComposeDelete(t *testing.T) {
	svc, runner, au, root := newPanelService(t)
	ctx := context.Background()
	ghostDir := writeProject(t, root, "ghost", "services:\n  web:\n    image: busybox\n")

	data, err := svc.ComposeDelete(ctx, "admin", "ghost")
	if err != nil {
		t.Fatalf("ComposeDelete: %v", err)
	}
	if runs := runner.runs(); len(runs) != 1 || runs[0] != "ghost:down" {
		t.Fatalf("删除前应先 down: %v", runs)
	}
	if !data.DeletedDir || data.Project != "ghost" {
		t.Fatalf("data = %+v", data)
	}
	if dirExists(ghostDir) {
		t.Fatal("面板项目的目录应被删除")
	}
	if got := results(au); len(got) != 1 || got[0] != AuditComposeProjectDeleted+"/"+audit.ResultSuccess {
		t.Fatalf("审计 = %v", got)
	}

	// 面板外项目（仅容器 label 发现，目录与编排文件都在面板落盘根之外）：只停止，
	// 不删目录。
	outsideParent := t.TempDir()
	outsideDir := filepath.Join(outsideParent, "outside")
	keepFile := filepath.Join(outsideParent, "keep.txt")
	_ = os.WriteFile(keepFile, []byte("x"), 0o644)
	outsideCompose := filepath.Join(outsideDir, ComposeFileName)
	_ = os.MkdirAll(outsideDir, 0o755)
	_ = os.WriteFile(outsideCompose, []byte("services:\n  web:\n    image: busybox\n"), 0o644)
	runner.facts = []dockerx.ComposeProjectFact{{Project: "outside", WorkingDir: outsideDir,
		ConfigFile: outsideCompose, Locatable: true,
		Containers: []dockerx.ComposeContainerRef{runningCtr("o")}}}
	data, err = svc.ComposeDelete(ctx, "admin", "outside")
	if err != nil {
		t.Fatalf("ComposeDelete outside: %v", err)
	}
	if runs := runner.runs(); runs[len(runs)-1] != "outside:down" {
		t.Fatalf("面板外项目也应 down: %v", runs)
	}
	if data.DeletedDir {
		t.Fatal("面板外项目不应删除目录（deletedDir 应为 false）")
	}
	if !dirExists(outsideCompose) || !dirExists(keepFile) {
		t.Fatal("面板外目录不得被触碰")
	}

	// 不存在的项目；down 失败时目录保留；进行中互斥。
	if _, err := svc.ComposeDelete(ctx, "admin", "nope"); sym(err) != "DK_014" {
		t.Fatalf("不存在应 DK_014: %v", err)
	}
	// 面板外且目录/文件已不在（无法执行 down、也没有可删的目录）：如实报定位失败，
	// 不能假装删除成功。
	runner.facts = []dockerx.ComposeProjectFact{{Project: "vanished", WorkingDir: "/gone/x", ConfigFile: "/gone/x/a.yml",
		Containers: []dockerx.ComposeContainerRef{runningCtr("v")}}}
	if _, err := svc.ComposeDelete(ctx, "admin", "vanished"); sym(err) != "DK_015" {
		t.Fatalf("面板外且已失联的项目应 DK_015: %v", err)
	}
	keep := writeProject(t, root, "keep", "services:\n  web:\n    image: busybox\n")
	runner.runErr = map[string]error{"keep": &dockerx.ExecError{Verb: "compose down", Reason: "daemon down"}}
	if _, err := svc.ComposeDelete(ctx, "admin", "keep"); sym(err) != "DK_032" || !strings.Contains(reasonOf(err), "daemon down") {
		t.Fatalf("down 失败应 DK_032 并带原因: %v", err)
	}
	if !dirExists(keep) {
		t.Fatal("down 失败时目录必须保留（否则项目失联）")
	}
	runner.runErr = map[string]error{"keep": dockerx.ErrBusy}
	if _, err := svc.ComposeDelete(ctx, "admin", "keep"); sym(err) != "DK_018" {
		t.Fatalf("互斥应 DK_018: %v", err)
	}
}

// ---- 应用：创建 ----

func TestAppCreate(t *testing.T) {
	svc, runner, au, root := newPanelService(t)
	port := freePort(t)
	req := validAppReq("myapp", port)
	req.ContainerPort = 8080
	req.Env = []EnvKVDTO{{Key: "GREETING", Value: "hello world"}, {Key: "PORT", Value: "8080"}}
	req.Mounts = []MountDTO{{Source: "data", Target: "/data"}, {Source: "/srv/x", Target: "/x", Mode: "ro"}}

	data, err := svc.AppCreate(context.Background(), "admin", req)
	if err != nil {
		t.Fatalf("AppCreate: %v", err)
	}
	if runs := runner.runs(); len(runs) != 1 || runs[0] != "myapp:"+dockerx.VerbUp {
		t.Fatalf("创建后应 up: %v", runs)
	}
	if data.State != AppStateRunning || data.HostPort != port || data.WorkingDir != filepath.Join(root, "myapp") {
		t.Fatalf("data = %+v", data)
	}
	raw, err := os.ReadFile(filepath.Join(root, "myapp", ComposeFileName))
	if err != nil {
		t.Fatalf("编排文件未落盘: %v", err)
	}
	var doc struct {
		Services map[string]struct {
			Image       string            `yaml:"image"`
			Ports       []string          `yaml:"ports"`
			Environment map[string]string `yaml:"environment"`
			Volumes     []string          `yaml:"volumes"`
			Restart     string            `yaml:"restart"`
			Labels      map[string]string `yaml:"labels"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("写出的编排不是合法 YAML: %v\n%s", err, raw)
	}
	svcDef, ok := doc.Services[appServiceName]
	if !ok || len(doc.Services) != 1 {
		t.Fatalf("应用编排应恰好一个服务 app: %v", doc.Services)
	}
	if svcDef.Image != "nginx:alpine" || len(svcDef.Ports) != 1 ||
		svcDef.Ports[0] != "127.0.0.1:"+strconv.Itoa(port)+":8080" {
		t.Fatalf("镜像或端口映射错误（主机端口只绑 127.0.0.1）: %+v", svcDef)
	}
	if svcDef.Labels[AppOwnershipLabel] != "myapp" {
		t.Fatalf("应写入归属标签: %+v", svcDef.Labels)
	}
	if svcDef.Restart != "unless-stopped" {
		t.Fatalf("重启策略缺省应为 unless-stopped: %q", svcDef.Restart)
	}
	if svcDef.Environment["GREETING"] != "hello world" || svcDef.Environment["PORT"] != "8080" {
		t.Fatalf("环境变量错误: %+v", svcDef.Environment)
	}
	if len(svcDef.Volumes) != 2 || svcDef.Volumes[0] != "data:/data" || svcDef.Volumes[1] != "/srv/x:/x:ro" {
		t.Fatalf("挂载错误: %+v", svcDef.Volumes)
	}
	if got := results(au); len(got) != 1 || got[0] != AuditAppCreated+"/"+audit.ResultSuccess {
		t.Fatalf("审计 = %v", got)
	}
	if d := fmt.Sprint(au.snapshot()[0].Detail); strings.Contains(d, "hello world") {
		t.Fatalf("审计 detail 不得包含环境变量值: %s", d)
	}
	// 面板自己写出的编排必须能被落盘扫描识别为应用（写与读同一口径）。
	panel, err := scanPanelRoot(root)
	if err != nil || len(panel) != 1 || !panel[0].isApp || panel[0].appName != "myapp" ||
		panel[0].image != "nginx:alpine" || panel[0].hostPort != port || panel[0].containerPort != 8080 {
		t.Fatalf("创建的应用应被扫描识别: %+v err=%v", panel, err)
	}
}

func TestAppCreateRejections(t *testing.T) {
	svc, runner, au, root := newPanelService(t)
	ctx := context.Background()
	base := validAppReq("ok", freePort(t))

	cases := []struct {
		name string
		mut  func(r *AppCreateRequest)
		want string
	}{
		{"应用名非法（大写）", func(r *AppCreateRequest) { r.Name = "MyApp" }, "DK_029"},
		{"应用名非法（含路径）", func(r *AppCreateRequest) { r.Name = "a/b" }, "DK_029"},
		{"镜像为空", func(r *AppCreateRequest) { r.Image = "" }, "DK_001"},
		{"镜像引用格式非法", func(r *AppCreateRequest) { r.Image = "Bad Image!" }, "DK_012"},
		{"主机端口越界", func(r *AppCreateRequest) { r.HostPort = 70000 }, "DK_001"},
		{"容器端口为零", func(r *AppCreateRequest) { r.ContainerPort = 0 }, "DK_001"},
		{"重启策略非法", func(r *AppCreateRequest) { r.RestartPolicy = "forever" }, "DK_001"},
		{"环境变量键含空格", func(r *AppCreateRequest) { r.Env = []EnvKVDTO{{Key: "BAD KEY", Value: "x"}} }, "DK_021"},
		{"挂载目标不是绝对路径", func(r *AppCreateRequest) { r.Mounts = []MountDTO{{Source: "a", Target: "data"}} }, "DK_022"},
		{"挂载来源含 ..", func(r *AppCreateRequest) { r.Mounts = []MountDTO{{Source: "../etc", Target: "/x"}} }, "DK_022"},
	}
	for _, tc := range cases {
		r := base
		tc.mut(&r)
		if _, err := svc.AppCreate(ctx, "admin", r); sym(err) != tc.want {
			t.Errorf("%s: want %s got %v", tc.name, tc.want, err)
		}
	}
	if runs := runner.runs(); len(runs) != 0 {
		t.Fatalf("校验失败不得触达 up: %v", runs)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatalf("校验失败不得留任何落盘产物: %v", entries)
	}
	for _, e := range au.snapshot() {
		if e.Result != audit.ResultFailure {
			t.Fatalf("被拒的创建审计应为 failure: %+v", e)
		}
	}

	// 主机端口已被占用 → DK_031（真实占用一个端口来验证）。
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = l.Close() }()
	busy := base
	busy.HostPort = l.Addr().(*net.TCPAddr).Port
	if _, err := svc.AppCreate(ctx, "admin", busy); sym(err) != "DK_031" {
		t.Fatalf("端口被占用应 DK_031: %v", err)
	}

	// 目录已存在（同名应用或同名 Compose 项目）→ DK_027，不动既有文件。
	dup := base
	dup.Name = "dup"
	dupFile := filepath.Join(writeProject(t, root, "dup", "services:\n  x:\n    image: busybox\n"), ComposeFileName)
	if _, err := svc.AppCreate(ctx, "admin", dup); sym(err) != "DK_027" {
		t.Fatalf("重名应 DK_027: %v", err)
	}
	if raw, _ := os.ReadFile(dupFile); !strings.Contains(string(raw), "x:") {
		t.Fatal("重名被拒时不得改写既有编排文件")
	}

	// 启动失败且项目零容器 → DK_030 带原因，清理落盘（不占住应用名）。
	boom := base
	boom.Name = "boom"
	runner.runErr = map[string]error{"boom": &dockerx.ExecError{Verb: "compose up", Reason: "image pull denied"}}
	if _, err := svc.AppCreate(ctx, "admin", boom); sym(err) != "DK_030" || !strings.Contains(reasonOf(err), "image pull denied") {
		t.Fatalf("启动失败应 DK_030 并带原因: %v", err)
	}
	if dirExists(filepath.Join(root, "boom")) {
		t.Fatal("启动失败且零容器时应清理落盘")
	}
	// 启动失败但项目已有容器（部分起来了）→ 保留目录，应用可见。
	half := base
	half.Name = "half"
	runner.facts = []dockerx.ComposeProjectFact{{Project: "half", Containers: []dockerx.ComposeContainerRef{runningCtr("h")}}}
	runner.runErr = map[string]error{"half": &dockerx.ExecError{Verb: "compose up", Reason: "one service failed"}}
	if _, err := svc.AppCreate(ctx, "admin", half); sym(err) != "DK_030" {
		t.Fatalf("启动失败应 DK_030: %v", err)
	}
	if !dirExists(filepath.Join(root, "half")) {
		t.Fatal("项目已有容器时应保留目录（应用仍可见、可处置）")
	}
}

// ---- 应用：清单 ----

func TestAppList(t *testing.T) {
	svc, runner, _, root := newPanelService(t)
	writeAppDir(t, root, "run", 18091)
	writeAppDir(t, root, "idle", 18092)
	writeAppDir(t, root, "part", 18093)
	writeProject(t, root, "plain", "services:\n  web:\n    image: busybox\n") // 普通项目：不属于应用
	runner.facts = []dockerx.ComposeProjectFact{
		{Project: "run", Containers: []dockerx.ComposeContainerRef{runningCtr("r")}},
		{Project: "part", Containers: []dockerx.ComposeContainerRef{runningCtr("p1"),
			{ContainerID: "c-p2", Name: "p2", State: dockerx.StatusStopped}}},
	}

	data, err := svc.AppList(context.Background())
	if err != nil {
		t.Fatalf("AppList: %v", err)
	}
	if len(data.Items) != 3 || data.Total != 3 {
		t.Fatalf("应用清单 = %+v（普通项目 plain 不应出现）", data.Items)
	}
	byName := map[string]AppItem{}
	for _, it := range data.Items {
		byName[it.Name] = it
	}
	if r := byName["run"]; r.State != AppStateRunning || len(r.Containers) != 1 || r.HostPort != 18091 ||
		r.ContainerPort != 80 || r.Image != "nginx:alpine" {
		t.Fatalf("运行中的应用: %+v", r)
	}
	if i := byName["idle"]; i.State != AppStateStopped || len(i.Containers) != 0 {
		t.Fatalf("零容器应用应为已停止: %+v", i)
	}
	if p := byName["part"]; p.State != AppStatePartial || len(p.Containers) != 2 {
		t.Fatalf("部分运行的应用: %+v", p)
	}
	if byName["run"].SiteLinked != nil {
		t.Fatalf("建站通道未装配时 siteLinked 应为 null: %v", *byName["run"].SiteLinked)
	}

	// 落盘根不存在（从未创建过应用）→ 空清单而非报错。
	empty, _, _, _ := newPanelService(t)
	if d, err := empty.AppList(context.Background()); err != nil || len(d.Items) != 0 {
		t.Fatalf("空根: %+v %v", d, err)
	}
}

func TestAppListSiteLinked(t *testing.T) {
	sl := &fakeSiteLink{}
	svc, _, _, root := newPanelService(t, WithSiteLinker(sl))
	writeAppDir(t, root, "linked", 18094)
	sl.hasSite = true
	data, err := svc.AppList(context.Background())
	if err != nil || len(data.Items) != 1 {
		t.Fatalf("AppList: %+v %v", data, err)
	}
	if it := data.Items[0]; it.SiteLinked == nil || !*it.SiteLinked {
		t.Fatalf("已有上游站点时 siteLinked 应为 true: %+v", it)
	}
	sl.hasSite = false
	data, _ = svc.AppList(context.Background())
	if it := data.Items[0]; it.SiteLinked == nil || *it.SiteLinked {
		t.Fatalf("无上游站点时 siteLinked 应为 false: %+v", it)
	}
}

// ---- 应用：启停重启 / 日志 / 删除 / 建站 ----

func TestAppServiceOps(t *testing.T) {
	svc, runner, au, root := newPanelService(t)
	writeAppDir(t, root, "svc", 18095)
	ctx := context.Background()

	for op, verb := range map[string]string{"start": dockerx.VerbUp, "stop": dockerx.VerbStop, "restart": dockerx.VerbRestart} {
		data, err := svc.AppService(ctx, "admin", "svc", op)
		if err != nil || data.Op != op || data.Name != "svc" {
			t.Fatalf("%s: %+v %v", op, data, err)
		}
		if runs := runner.runs(); runs[len(runs)-1] != "svc:"+verb {
			t.Fatalf("%s 应执行 %s: %v", op, verb, runs)
		}
	}
	if _, err := svc.AppService(ctx, "admin", "svc", "pause"); sym(err) != "DK_001" {
		t.Fatalf("未知操作应 DK_001: %v", err)
	}
	if _, err := svc.AppService(ctx, "admin", "nope", "start"); sym(err) != "DK_028" {
		t.Fatalf("不存在的应用应 DK_028: %v", err)
	}
	runner.runErr = map[string]error{"svc": &dockerx.ExecError{Verb: "compose stop", Reason: "daemon down"}}
	if _, err := svc.AppService(ctx, "admin", "svc", "stop"); sym(err) != "DK_030" || !strings.Contains(reasonOf(err), "daemon down") {
		t.Fatalf("执行失败应 DK_030 并带原因: %v", err)
	}
	runner.runErr = map[string]error{"svc": dockerx.ErrBusy}
	if _, err := svc.AppService(ctx, "admin", "svc", "stop"); sym(err) != "DK_018" {
		t.Fatalf("互斥应 DK_018: %v", err)
	}
	successes := 0
	for _, e := range au.snapshot() {
		if e.Action == AuditAppService && e.Result == audit.ResultSuccess {
			successes++
		}
	}
	if successes != 3 {
		t.Fatalf("成功的启停重启应各记一条审计，实际 %d", successes)
	}
}

func TestAppLogs(t *testing.T) {
	svc, _, _, root := newPanelService(t)
	writeAppDir(t, root, "lg", 18096)
	data, err := svc.AppLogs(context.Background(), "lg", "200")
	if err != nil || len(data.Lines) != 2 {
		t.Fatalf("AppLogs: %+v %v", data, err)
	}
	if _, err := svc.AppLogs(context.Background(), "nope", "200"); sym(err) != "DK_028" {
		t.Fatalf("不存在的应用应 DK_028: %v", err)
	}
	if _, err := svc.AppLogs(context.Background(), "lg", "99999"); sym(err) != "DK_001" {
		t.Fatalf("档位越界应 DK_001: %v", err)
	}
}

func TestAppDelete(t *testing.T) {
	svc, runner, au, root := newPanelService(t)
	ctx := context.Background()
	dir := writeAppDir(t, root, "gone", 18097)

	data, err := svc.AppDelete(ctx, "admin", "gone")
	if err != nil {
		t.Fatalf("AppDelete: %v", err)
	}
	if runs := runner.runs(); len(runs) != 1 || runs[0] != "gone:"+dockerx.VerbDown {
		t.Fatalf("删除前应先 down: %v", runs)
	}
	if !data.DeletedDir || dirExists(dir) {
		t.Fatalf("应用目录应被删除: %+v exists=%v", data, dirExists(dir))
	}
	if got := results(au); got[0] != AuditAppDeleted+"/"+audit.ResultSuccess {
		t.Fatalf("审计 = %v", got)
	}
	// 删除后从清单消失
	if list, _ := svc.AppList(ctx); len(list.Items) != 0 {
		t.Fatalf("删除后不应再出现在应用清单: %+v", list.Items)
	}
	if _, err := svc.AppDelete(ctx, "admin", "gone"); sym(err) != "DK_028" {
		t.Fatalf("不存在的应用应 DK_028: %v", err)
	}
	// down 失败：目录必须保留（应用不失联），DK_030 带原因
	keep := writeAppDir(t, root, "keep", 18098)
	runner.runErr = map[string]error{"keep": &dockerx.ExecError{Verb: "compose down", Reason: "daemon down"}}
	if _, err := svc.AppDelete(ctx, "admin", "keep"); sym(err) != "DK_030" || !dirExists(keep) {
		t.Fatalf("down 失败应 DK_030 且保留目录: %v exists=%v", err, dirExists(keep))
	}
}

func TestAppSite(t *testing.T) {
	sl := &fakeSiteLink{}
	svc, _, au, root := newPanelService(t, WithSiteLinker(sl))
	writeAppDir(t, root, "myapp", 18099)
	actor := &authsvc.Identity{UserID: 1, Username: "admin"}

	data, err := svc.AppSite(context.Background(), actor, AppSiteRequest{Name: "myapp",
		Domains: []string{"a.example.com", " b.example.com ", ""}})
	if err != nil {
		t.Fatalf("AppSite: %v", err)
	}
	if data.SiteID != "s-1" || data.Name != "myapp" {
		t.Fatalf("data = %+v", data)
	}
	// 反代目标为应用的本机端口；域名去空白、去空项。
	if len(sl.ports) != 1 || sl.ports[0] != 18099 {
		t.Fatalf("建站端口 = %v, want [18099]", sl.ports)
	}
	if d := sl.domains[0]; len(d) != 2 || d[0] != "a.example.com" || d[1] != "b.example.com" {
		t.Fatalf("域名 = %v", d)
	}
	if got := results(au); got[0] != AuditAppSiteLinked+"/"+audit.ResultSuccess {
		t.Fatalf("审计 = %v", got)
	}

	if _, err := svc.AppSite(context.Background(), actor, AppSiteRequest{Name: "nope"}); sym(err) != "DK_028" {
		t.Fatalf("不存在的应用应 DK_028: %v", err)
	}
	// site-mgmt 的拒绝原因原样透传（如域名不合法）。
	sl.createEr = errors.New("域名格式不合法")
	if _, err := svc.AppSite(context.Background(), actor, AppSiteRequest{Name: "myapp", Domains: []string{"bad domain"}}); sym(err) != "DK_030" || !strings.Contains(reasonOf(err), "域名格式不合法") {
		t.Fatalf("建站被拒应 DK_030 并透传原因: %v", err)
	}
	// 编排里没有端口映射 → 无法建站
	writeProject(t, root, "noport", "services:\n  app:\n    image: nginx\n    labels:\n      dougong.app: noport\n")
	if _, err := svc.AppSite(context.Background(), actor, AppSiteRequest{Name: "noport"}); sym(err) != "DK_030" {
		t.Fatalf("无端口映射应 DK_030: %v", err)
	}
	// 建站通道未装配
	bare, _, _, root2 := newPanelService(t)
	writeAppDir(t, root2, "a", 18100)
	if _, err := bare.AppSite(context.Background(), actor, AppSiteRequest{Name: "a"}); sym(err) != "DK_030" {
		t.Fatalf("通道未装配应 DK_030: %v", err)
	}
}
