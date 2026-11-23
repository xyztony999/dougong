// create_test.go 是 v1.1 创建类服务测试（容器/卷/网络/Compose 项目创建：校验链、
// 缺失镜像自动拉取、参数规范化、落盘前处理、up 失败的落盘处置）。
//
// 背景（2026-11-23 入口可达性复核）：v1.1 创建类代码在 service/adapter 两层此前
// 零测试覆盖，下列三处必现缺陷因此漏过门控——
//   - 新建编排项目向 ComposeRunner 传字面量 "up -d"，被动词白名单拒绝；
//   - 归属标签写在 YAML 顶层（compose 规范无此键，CLI 拒绝整个文件）；
//   - composeArgs 把 -d 放在 up 之前（adapter 层 TestComposeArgs 已锁）。
//
// 本文件中对应用例在旧实现上必失败。
package dockermgmt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/dougong/dougong/internal/adapter/dockerx"
	"github.com/dougong/dougong/internal/config"
	"github.com/dougong/dougong/internal/service/audit"
	"gopkg.in/yaml.v3"
)

// ---------- 创建类替身 ----------

// createGateway 在 fakeGateway 之上补齐创建类可观测行为。
type createGateway struct {
	*fakeGateway
	mu         sync.Mutex
	images     map[string]bool // ref → 本地存在
	pulled     []string
	spec       dockerx.CreateContainerSpec
	createID   string
	createErr  error
	createPull error
	volumes    []string // name:driver
	volErr     error
	netMade    []string // name:driver:subnet:gateway
	netErr     error
}

func (g *createGateway) ImageExists(_ context.Context, ref string) bool {
	return g.images[ref]
}

func (g *createGateway) PullImage(_ context.Context, ref string) (dockerx.ImageFact, error) {
	g.mu.Lock()
	g.pulled = append(g.pulled, ref)
	g.mu.Unlock()
	if g.createPull != nil {
		return dockerx.ImageFact{}, g.createPull
	}
	return dockerx.ImageFact{}, nil
}

func (g *createGateway) CreateContainer(_ context.Context, spec dockerx.CreateContainerSpec) (string, error) {
	g.mu.Lock()
	g.spec = spec
	g.mu.Unlock()
	if g.createErr != nil {
		return "", g.createErr
	}
	if g.createID == "" {
		return "cid-new", nil
	}
	return g.createID, nil
}

func (g *createGateway) CreateVolume(_ context.Context, name, driver string) error {
	g.mu.Lock()
	g.volumes = append(g.volumes, name+":"+driver)
	g.mu.Unlock()
	return g.volErr
}

func (g *createGateway) CreateNetwork(_ context.Context, name, driver, subnet, gateway string) error {
	g.mu.Lock()
	g.netMade = append(g.netMade, strings.Join([]string{name, driver, subnet, gateway}, ":"))
	g.mu.Unlock()
	return g.netErr
}

func (g *createGateway) pullCalls() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.pulled...)
}

// composeRunner 在 fakeRunner 之上补齐清单读取失败注入。
type composeRunner struct {
	*fakeRunner
	listErr error
}

func (r *composeRunner) ListProjectFacts(context.Context) ([]dockerx.ComposeProjectFact, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return r.fakeRunner.ListProjectFacts(context.Background())
}

func newCreateService(t *testing.T, opts ...Option) (*Service, *createGateway, *composeRunner, *fakeAudit) {
	t.Helper()
	gw := &createGateway{fakeGateway: &fakeGateway{}, images: map[string]bool{}}
	runner := &composeRunner{fakeRunner: &fakeRunner{}}
	au := &fakeAudit{}
	return New(gw, runner, au, opts...), gw, runner, au
}

func results(au *fakeAudit) []string {
	out := make([]string, 0)
	for _, e := range au.snapshot() {
		out = append(out, e.Action+"/"+e.Result)
	}
	return out
}

// ---------- 容器创建 ----------

func validContainerReq() ContainerCreateRequest {
	return ContainerCreateRequest{
		Name: "web", Image: "nginx:alpine",
		PortMappings:  []PortMappingDTO{{HostPort: "8080", ContainerPort: "80"}},
		RestartPolicy: "unless-stopped",
	}
}

// TestCreateContainerValidation 校验链逐项归码；校验失败不得触达创建。
func TestCreateContainerValidation(t *testing.T) {
	cases := []struct {
		name string
		mut  func(r *ContainerCreateRequest)
		want string
	}{
		{"容器名非法（-开头）", func(r *ContainerCreateRequest) { r.Name = "-bad" }, "DK_019"},
		{"容器名非法（含空格）", func(r *ContainerCreateRequest) { r.Name = "a b" }, "DK_019"},
		{"镜像必填", func(r *ContainerCreateRequest) { r.Image = "  " }, "DK_001"},
		{"宿主端口越界", func(r *ContainerCreateRequest) {
			r.PortMappings = []PortMappingDTO{{HostPort: "70000", ContainerPort: "80"}}
		}, "DK_020"},
		{"容器端口非数字", func(r *ContainerCreateRequest) {
			r.PortMappings = []PortMappingDTO{{HostPort: "8080", ContainerPort: "http"}}
		}, "DK_020"},
		{"协议非法", func(r *ContainerCreateRequest) {
			r.PortMappings = []PortMappingDTO{{HostPort: "8080", ContainerPort: "80", Protocol: "sctp"}}
		}, "DK_020"},
		{"环境变量名含空格", func(r *ContainerCreateRequest) {
			r.Env = []EnvKVDTO{{Key: "A B", Value: "1"}}
		}, "DK_021"},
		{"环境变量名为空", func(r *ContainerCreateRequest) {
			r.Env = []EnvKVDTO{{Key: "", Value: "1"}}
		}, "DK_021"},
		{"挂载来源含 ..", func(r *ContainerCreateRequest) {
			r.Mounts = []MountDTO{{Source: "../etc", Target: "/etc/x"}}
		}, "DK_022"},
		{"挂载目标为空", func(r *ContainerCreateRequest) {
			r.Mounts = []MountDTO{{Source: "/data", Target: ""}}
		}, "DK_022"},
		// 历史缺陷：重启策略非法被误归 DK_019（向用户呈现「容器名格式非法」）。
		{"重启策略取值非法", func(r *ContainerCreateRequest) { r.RestartPolicy = "always!" }, "DK_001"},
		{"网络不存在", func(r *ContainerCreateRequest) { r.Network = "no-such-net" }, "DK_009"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, gw, _, au := newCreateService(t)
			gw.networks = []dockerx.NetworkFact{{NetworkID: "n1", Name: "bridge", Driver: "bridge"}}
			req := validContainerReq()
			tc.mut(&req)
			_, err := svc.CreateContainer(context.Background(), "admin", req)
			wantSymbol(t, err, tc.want)
			if gw.spec.Name != "" {
				t.Fatalf("校验失败不应触达创建，实际 spec=%+v", gw.spec)
			}
			if got := results(au); len(got) != 1 || got[0] != AuditContainerCreated+"/"+audit.ResultFailure {
				t.Fatalf("审计 = %v, want 一条 %s/failure", got, AuditContainerCreated)
			}
		})
	}
}

// TestCreateContainerSpec 规范化出口：多组端口/环境变量/挂载/重启策略/网络逐项
// 映射到适配器参数（协议缺省 tcp）；镜像已在本地时不拉取。
func TestCreateContainerSpec(t *testing.T) {
	svc, gw, _, au := newCreateService(t)
	gw.images["nginx:alpine"] = true
	gw.networks = []dockerx.NetworkFact{{NetworkID: "n1", Name: "app-net", Driver: "bridge"}}
	_, err := svc.CreateContainer(context.Background(), "admin", ContainerCreateRequest{
		Name:  "web",
		Image: " nginx:alpine ",
		PortMappings: []PortMappingDTO{
			{HostPort: "8080", ContainerPort: "80"},
			{HostPort: "5353", ContainerPort: "53", Protocol: "udp"},
		},
		Env:           []EnvKVDTO{{Key: "TZ", Value: "Asia/Shanghai"}, {Key: "MODE", Value: "dev"}},
		Mounts:        []MountDTO{{Source: "web-data", Target: "/data", Mode: "ro"}},
		RestartPolicy: "on-failure", Network: "app-net",
	})
	if err != nil {
		t.Fatalf("CreateContainer: %v", err)
	}
	spec := gw.spec
	if spec.Name != "web" || spec.Image != "nginx:alpine" {
		t.Fatalf("name/image = %q/%q（镜像应去首尾空白）", spec.Name, spec.Image)
	}
	if len(spec.PortMappings) != 2 || spec.PortMappings[0].Protocol != "tcp" ||
		spec.PortMappings[0].HostPort != "8080" || spec.PortMappings[1].Protocol != "udp" {
		t.Fatalf("ports = %+v", spec.PortMappings)
	}
	if len(spec.Env) != 2 || spec.Env[0] != "TZ=Asia/Shanghai" || spec.Env[1] != "MODE=dev" {
		t.Fatalf("env = %v", spec.Env)
	}
	if len(spec.Mounts) != 1 || spec.Mounts[0].Source != "web-data" ||
		spec.Mounts[0].Target != "/data" || spec.Mounts[0].Mode != "ro" {
		t.Fatalf("mounts = %+v", spec.Mounts)
	}
	if spec.RestartPolicy != "on-failure" || spec.NetworkName != "app-net" {
		t.Fatalf("restart/network = %q/%q", spec.RestartPolicy, spec.NetworkName)
	}
	if len(gw.pullCalls()) != 0 {
		t.Fatalf("镜像已在本地不应拉取，pulled=%v", gw.pullCalls())
	}
	if got := results(au); len(got) != 1 || got[0] != AuditContainerCreated+"/"+audit.ResultSuccess {
		t.Fatalf("审计 = %v, want 一条 %s/success", got, AuditContainerCreated)
	}
}

// TestCreateContainerMissingImagePullsFirst 镜像本地缺失 → 先拉取再创建；拉取失败
// DK_013 且不创建；创建失败 DK_023 透传原因。
func TestCreateContainerMissingImagePullsFirst(t *testing.T) {
	ctx := context.Background()

	svc, gw, _, au := newCreateService(t)
	if _, err := svc.CreateContainer(ctx, "admin", validContainerReq()); err != nil {
		t.Fatalf("缺失镜像自动拉取后创建: %v", err)
	}
	if calls := gw.pullCalls(); len(calls) != 1 || calls[0] != "nginx:alpine" {
		t.Fatalf("pulled = %v, want [nginx:alpine]", calls)
	}
	if gw.spec.Image != "nginx:alpine" {
		t.Fatalf("拉取后应创建，spec=%+v", gw.spec)
	}

	svc, gw, _, au = newCreateService(t)
	gw.createPull = errors.New("pull access denied")
	_, err := svc.CreateContainer(ctx, "admin", validContainerReq())
	wantSymbol(t, err, "DK_013")
	if !strings.Contains(errData(t, err)["reason"].(string), "pull access denied") {
		t.Fatalf("DK_013 data.reason 应透传拉取原因，data=%v", errData(t, err))
	}
	if gw.spec.Name != "" {
		t.Fatalf("拉取失败不应创建，spec=%+v", gw.spec)
	}
	if got := results(au); len(got) != 1 || got[0] != AuditContainerCreated+"/"+audit.ResultFailure {
		t.Fatalf("审计 = %v, want 一条失败记录", got)
	}

	svc, gw, _, _ = newCreateService(t)
	gw.createErr = errors.New("port is already allocated")
	_, err = svc.CreateContainer(ctx, "admin", validContainerReq())
	wantSymbol(t, err, "DK_023")
	if !strings.Contains(errData(t, err)["reason"].(string), "port is already allocated") {
		t.Fatalf("DK_023 data.reason 应透传 daemon 原因，data=%v", errData(t, err))
	}
}

// ---------- 卷 / 网络 ----------

func TestCreateVolumeAndNetwork(t *testing.T) {
	ctx := context.Background()
	svc, gw, _, au := newCreateService(t)

	if _, err := svc.CreateVolume(ctx, "admin", "data", ""); err != nil {
		t.Fatalf("CreateVolume: %v", err)
	}
	if _, err := svc.CreateNetwork(ctx, "admin", "app-net", "", "172.30.0.0/16", "172.30.0.1"); err != nil {
		t.Fatalf("CreateNetwork: %v", err)
	}
	if len(gw.volumes) != 1 || gw.volumes[0] != "data:local" {
		t.Fatalf("volumes = %v, want 驱动缺省 local", gw.volumes)
	}
	if len(gw.netMade) != 1 || gw.netMade[0] != "app-net:bridge:172.30.0.0/16:172.30.0.1" {
		t.Fatalf("networks = %v, want 驱动缺省 bridge", gw.netMade)
	}
	if got := results(au); len(got) != 2 ||
		got[0] != AuditVolumeCreated+"/"+audit.ResultSuccess ||
		got[1] != AuditNetworkCreated+"/"+audit.ResultSuccess {
		t.Fatalf("审计 = %v", got)
	}

	gw.volErr, gw.netErr = errors.New("volume exists"), errors.New("Pool overlaps")
	_, err := svc.CreateVolume(ctx, "admin", "data", "local")
	wantSymbol(t, err, "DK_024")
	_, err = svc.CreateNetwork(ctx, "admin", "app-net", "bridge", "", "")
	wantSymbol(t, err, "DK_025")
	_, err = svc.CreateVolume(ctx, "admin", "../escape", "")
	wantSymbol(t, err, "DK_024")
	if len(gw.volumes) != 2 {
		t.Fatalf("卷名非法不应触达创建，volumes=%v", gw.volumes)
	}
}

// ---------- Compose：落盘前处理 ----------

const composeSample = `# 我的服务
version: "3"
services:
  web:
    image: nginx:alpine   # 行内注释
    ports:
      - "8080:80"
  api:
    image: busybox
    command: sleep 3600
`

// parseDoc 解析为通用结构供断言。
func parseDoc(t *testing.T, text string) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		t.Fatalf("输出不是合法 YAML: %v\n%s", err, text)
	}
	return doc
}

// TestValidateComposeYAML 粘贴创建的落盘前校验（ADR-033 D-app-05：只校验、
// 不改写——归属标签仅由「应用」通道注入，粘贴的项目原样落盘）。结构问题全部在
// 落盘前拒绝；用户自带的 labels（列表或键值对）、别名共享的 labels 均合法。
func TestValidateComposeYAML(t *testing.T) {
	if err := validateComposeYAML(composeSample); err != nil {
		t.Fatalf("合法样例应通过: %v", err)
	}
	for name, text := range map[string]string{
		"列表形式 labels":  "services:\n  a:\n    image: busybox\n    labels:\n      - keep=1\n",
		"键值对形式 labels": "services:\n  b:\n    image: busybox\n    labels:\n      keep: \"2\"\n",
		"自带归属标签":       "services:\n  a:\n    image: busybox\n    labels:\n      dougong.app: mine\n",
		"别名共享 labels":  "x-common: &common\n  - team=ops\nservices:\n  a:\n    image: busybox\n    labels: *common\n",
	} {
		if err := validateComposeYAML(text); err != nil {
			t.Errorf("%s: 应通过，实际 %v", name, err)
		}
	}
	bad := []struct{ name, text, want string }{
		{"空内容", "", "内容为空"},
		{"顶层不是映射", "- a\n- b\n", "顶层"},
		{"缺少 services", "version: '3'\n", "缺少 services"},
		{"services 为空", "services: {}\n", "至少一个服务"},
		{"服务不是映射", "services:\n  web: nginx\n", "必须是键值对"},
		{"语法错误", "services:\n  web:\n    image: [unclosed\n", "YAML 语法错误"},
	}
	for _, tc := range bad {
		if err := validateComposeYAML(tc.text); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want 含 %q", tc.name, err, tc.want)
		}
	}
}

// ---------- Compose：项目创建 ----------

// useTempComposeRoot 返回「注入临时落盘根」的 Option 与该根路径（测试隔离：
// 服务实例级注入，不动包级状态——生产默认 DefaultComposeRootDir 不变）。
func useTempComposeRoot(t *testing.T) (Option, string) {
	t.Helper()
	root := t.TempDir()
	return WithComposeRootDir(root), root
}

// TestComposeRootOption 落盘根默认 /opt/dougong-compose（D-11 防逃逸前提）；仅
// 经 WithComposeRootDir 注入才改变。
func TestComposeRootOption(t *testing.T) {
	svc, _, _, _ := newCreateService(t)
	if svc.composeRoot != DefaultComposeRootDir || DefaultComposeRootDir != "/opt/dougong-compose" {
		t.Fatalf("默认落盘根 = %q, want /opt/dougong-compose", svc.composeRoot)
	}
	// 配置默认值与服务默认值两处同值（main 装配以配置值注入）。
	if got := config.Defaults().Docker.ComposeRoot; got != DefaultComposeRootDir {
		t.Fatalf("config 默认 compose_root = %q, 服务默认 = %q（两处应同值）", got, DefaultComposeRootDir)
	}
	svc, _, _, _ = newCreateService(t, WithComposeRootDir("E:/tmp/root"))
	if svc.composeRoot != "E:/tmp/root" {
		t.Fatalf("注入后落盘根 = %q", svc.composeRoot)
	}
}

// TestComposeCreateUpsWithVerbConstant 新建项目落盘后以动词常量 up 启动（历史
// 缺陷：传字面量 "up -d" 被白名单拒绝，自动启动必失败——旧实现上本用例失败）。
func TestComposeCreateUpsWithVerbConstant(t *testing.T) {
	opt, root := useTempComposeRoot(t)
	svc, _, runner, au := newCreateService(t, opt)

	data, err := svc.CreateComposeProject(context.Background(), "admin", "demo", composeSample, nil)
	if err != nil {
		t.Fatalf("CreateComposeProject: %v", err)
	}
	if runs := runner.runs(); len(runs) != 1 || runs[0] != "demo:"+dockerx.VerbUp {
		t.Fatalf("runner 调用 = %v, want [demo:up]", runs)
	}
	file := filepath.Join(root, "demo", ComposeFileName)
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("compose.yml 未落盘: %v", err)
	}
	// ADR-033 D-app-05：粘贴的项目原样落盘——不注入归属标签，注释与排版保持原样
	// （归属标签仅由「应用」通道写入）。
	if strings.Contains(string(raw), AppOwnershipLabel) {
		t.Fatalf("粘贴创建的项目不应被注入归属标签:\n%s", raw)
	}
	if norm := strings.ReplaceAll(string(raw), "\r\n", "\n"); norm != strings.ReplaceAll(composeSample, "\r\n", "\n") {
		t.Fatalf("落盘内容应与用户原文逐字一致:\n%s", raw)
	}
	if data.WorkingDir != filepath.Join(root, "demo") || data.ConfigFile != file || !data.Started {
		t.Fatalf("data = %+v", data)
	}
	if got := results(au); len(got) != 1 ||
		got[0] != AuditComposeProjectCreated+"/"+audit.ResultSuccess {
		t.Fatalf("审计 = %v", got)
	}
	if strings.Contains(fmt.Sprint(au.snapshot()[0].Detail), "nginx:alpine") {
		t.Fatalf("审计 detail 不得包含 compose 全文: %v", au.snapshot()[0].Detail)
	}
}

// TestComposeCreateNoStart autoUp=false：只落盘不启动。
func TestComposeCreateNoStart(t *testing.T) {
	opt, root := useTempComposeRoot(t)
	svc, _, runner, _ := newCreateService(t, opt)
	no := false
	data, err := svc.CreateComposeProject(context.Background(), "admin", "demo", composeSample, &no)
	if err != nil {
		t.Fatalf("CreateComposeProject(autoUp=false): %v", err)
	}
	if len(runner.runs()) != 0 || data.Started {
		t.Fatalf("autoUp=false 不应启动，runs=%v started=%v", runner.runs(), data.Started)
	}
	if _, err := os.Stat(filepath.Join(root, "demo", ComposeFileName)); err != nil {
		t.Fatalf("应已落盘: %v", err)
	}
}

// TestComposeCreateValidationLeavesNothing 校验失败不留任何落盘产物。
func TestComposeCreateValidationLeavesNothing(t *testing.T) {
	opt, root := useTempComposeRoot(t)
	cases := []struct {
		name, project, yaml, want string
	}{
		{"项目名非法", "Bad_Name", composeSample, "DK_001"},
		{"项目名含路径", "../escape", composeSample, "DK_001"},
		{"YAML 语法错误", "demo", "services:\n  web:\n    image: [x\n", "DK_026"},
		{"缺少 services", "demo", "version: '3'\n", "DK_026"},
		{"services 为空", "demo", "services: {}\n", "DK_026"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, runner, _ := newCreateService(t, opt)
			_, err := svc.CreateComposeProject(context.Background(), "admin", tc.project, tc.yaml, nil)
			wantSymbol(t, err, tc.want)
			if len(runner.runs()) != 0 {
				t.Fatalf("校验失败不应启动，runs=%v", runner.runs())
			}
			entries, _ := os.ReadDir(root)
			if len(entries) != 0 {
				t.Fatalf("校验失败不应留目录，root=%v", entries)
			}
		})
	}
}

// TestComposeCreateExistingDirectory 目录已存在 → DK_027，不动既有文件、不启动。
func TestComposeCreateExistingDirectory(t *testing.T) {
	opt, root := useTempComposeRoot(t)
	dir := filepath.Join(root, "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(dir, ComposeFileName)
	if err := os.WriteFile(existing, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc, _, runner, _ := newCreateService(t, opt)
	_, err := svc.CreateComposeProject(context.Background(), "admin", "demo", composeSample, nil)
	wantSymbol(t, err, "DK_027")
	if len(runner.runs()) != 0 {
		t.Fatalf("重名不应启动，runs=%v", runner.runs())
	}
	raw, _ := os.ReadFile(existing)
	if string(raw) != "services: {}\n" {
		t.Fatalf("既有文件不得被改写: %q", raw)
	}
}

// TestComposeCreateUpFailureDisposal up 失败的落盘处置：项目零容器 → 清理本次
// 落盘（否则清单不可见且占住项目名）；已有容器或无法判定 → 保留。
func TestComposeCreateUpFailureDisposal(t *testing.T) {
	upErr := &dockerx.ExecError{Verb: "compose up", Reason: "service \"web\" refers to undefined network x"}

	newBroken := func(t *testing.T) (*Service, *composeRunner, string) {
		opt, root := useTempComposeRoot(t)
		svc, _, runner, _ := newCreateService(t, opt)
		runner.runErr = map[string]error{"demo": upErr}
		return svc, runner, filepath.Join(root, "demo")
	}

	t.Run("零容器：清理落盘", func(t *testing.T) {
		svc, _, dir := newBroken(t)
		_, err := svc.CreateComposeProject(context.Background(), "admin", "demo", composeSample, nil)
		wantSymbol(t, err, "DK_016")
		if !strings.Contains(errData(t, err)["reason"].(string), "undefined network") {
			t.Fatalf("DK_016 data.reason 应透传 CLI 原因: %v", errData(t, err))
		}
		if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
			t.Fatalf("零容器失败应清理落盘目录，stat err=%v", statErr)
		}
	})

	t.Run("已有容器：保留落盘", func(t *testing.T) {
		svc, runner, dir := newBroken(t)
		runner.facts = []dockerx.ComposeProjectFact{{Project: "demo", Containers: []dockerx.ComposeContainerRef{
			{ContainerID: "c1", Name: "demo-api-1", State: "running"}}}}
		_, err := svc.CreateComposeProject(context.Background(), "admin", "demo", composeSample, nil)
		wantSymbol(t, err, "DK_016")
		if _, statErr := os.Stat(filepath.Join(dir, ComposeFileName)); statErr != nil {
			t.Fatalf("已有容器应保留落盘: %v", statErr)
		}
	})

	t.Run("清单读取失败：无法判定则保留", func(t *testing.T) {
		svc, runner, dir := newBroken(t)
		runner.listErr = errors.New("daemon down")
		_, err := svc.CreateComposeProject(context.Background(), "admin", "demo", composeSample, nil)
		wantSymbol(t, err, "DK_016")
		if _, statErr := os.Stat(filepath.Join(dir, ComposeFileName)); statErr != nil {
			t.Fatalf("无法判定时不应删除: %v", statErr)
		}
	})
}
