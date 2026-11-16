// service_test.go 是应用服务表驱动测试（fake 网关/CLI 替身注入——daemon
// 交互不出单测边界；矩阵/校验/编排/审计口径/错误归一全覆盖）。
package dockermgmt

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/dougong/dougong/internal/adapter/dockerx"
	"github.com/dougong/dougong/internal/errcode"
	"github.com/dougong/dougong/internal/service/audit"
)

// ---------- fake 网关 ----------

// fakeGateway 是 DockerGateway 替身。
type fakeGateway struct {
	mu sync.Mutex

	probe        dockerx.DaemonInfo
	list         []dockerx.ContainerFact
	inspect      map[string]string // id → raw state（ContainerState）
	exists       map[string]bool
	logs         map[string][]string
	stats        map[string]dockerx.StatsFact
	followStream *dockerx.LogStream
	images       []dockerx.ImageFact
	pullFact     dockerx.ImageFact
	pullErr      error
	volumes      []dockerx.VolumeFact
	networks     []dockerx.NetworkFact

	actionErr map[string]error // containerID → 动作错误
	delErr    map[string]error // 资源键错误（image:/volume:/network:）

	actionCalls []string
	deleteCalls []string
}

func (f *fakeGateway) ProbeDaemon(context.Context) dockerx.DaemonInfo { return f.probe }

func (f *fakeGateway) ListContainers(context.Context) ([]dockerx.ContainerFact, error) {
	return f.list, nil
}

func (f *fakeGateway) ContainerState(_ context.Context, id string) (string, error) {
	if state, ok := f.inspect[id]; ok {
		return state, nil // 已映射态（ContainerFact.State 同形）
	}
	return "", fmt.Errorf("dockerx: resource not found: inspect %s", id)
}

func (f *fakeGateway) ContainerExists(_ context.Context, id string) (bool, error) {
	return f.exists[id], nil
}

func (f *fakeGateway) GetContainerLogs(_ context.Context, id string, tail int) ([]string, error) {
	lines := f.logs[id]
	if len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}
	return lines, nil
}

func (f *fakeGateway) FollowContainerLogs(_ context.Context, id string, tail int) (*dockerx.LogStream, error) {
	if f.followStream != nil {
		return f.followStream, nil
	}
	return nil, &dockerx.ExecError{Verb: "container logs follow", Reason: "no stream"}
}

// setFollowStream 是容器跟随订阅替身句柄（用例按需注入；nil=通道失败）。
func (f *fakeGateway) setFollowStream() *dockerx.LogStream {
	lines := make(chan string, 4)
	end := make(chan string, 1)
	_, cancel := context.WithCancel(context.Background())
	f.followStream = &dockerx.LogStream{Lines: lines, End: end, Cancel: cancel}
	return f.followStream
}

func (f *fakeGateway) GetContainerStats(_ context.Context, id string) (dockerx.StatsFact, error) {
	return f.stats[id], nil
}

func (f *fakeGateway) ContainerAction(_ context.Context, id, action string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.actionCalls = append(f.actionCalls, action+":"+id)
	if err, ok := f.actionErr[id]; ok {
		return err
	}
	return nil
}

func (f *fakeGateway) DeleteContainer(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteCalls = append(f.deleteCalls, "container:"+id)
	if err, ok := f.delErr["container:"+id]; ok {
		return err
	}
	return nil
}

func (f *fakeGateway) ListImages(context.Context) ([]dockerx.ImageFact, error) { return f.images, nil }

func (f *fakeGateway) PullImage(context.Context, string) (dockerx.ImageFact, error) {
	if f.pullErr != nil {
		return dockerx.ImageFact{}, f.pullErr
	}
	return f.pullFact, nil
}

func (f *fakeGateway) DeleteImage(ctx context.Context, id string) error {
	return f.deleteResource(ctx, "image", id)
}

func (f *fakeGateway) deleteResource(_ context.Context, kind, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteCalls = append(f.deleteCalls, kind+":"+id)
	if err, ok := f.delErr[kind+":"+id]; ok {
		return err
	}
	return nil
}

func (f *fakeGateway) ListVolumes(context.Context) ([]dockerx.VolumeFact, error) {
	return f.volumes, nil
}

func (f *fakeGateway) DeleteVolume(ctx context.Context, name string) error {
	return f.deleteResource(ctx, "volume", name)
}

func (f *fakeGateway) ListNetworks(context.Context) ([]dockerx.NetworkFact, error) {
	return f.networks, nil
}

func (f *fakeGateway) DeleteNetwork(ctx context.Context, id string) error {
	return f.deleteResource(ctx, "network", id)
}

func (f *fakeGateway) writes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.deleteCalls...)
}

func (f *fakeGateway) actions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.actionCalls...)
}

// ---------- fake ComposeRunner ----------

// fakeRunner 是 ComposeRunner 替身。
type fakeRunner struct {
	mu     sync.Mutex
	facts  []dockerx.ComposeProjectFact
	runErr map[string]error // project → 错误
	runLog []string
}

func (f *fakeRunner) ListProjectFacts(context.Context) ([]dockerx.ComposeProjectFact, error) {
	return f.facts, nil
}

func (f *fakeRunner) Run(_ context.Context, p dockerx.ComposeProjectFact, verb string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.runErr[p.Project]; ok {
		if errors.Is(err, dockerx.ErrBusy) {
			return err // 互斥拒绝：不触达 CLI（真实 ComposeRunner 先拒绝后执行）
		}
		f.runLog = append(f.runLog, p.Project+":"+verb)
		return err
	}
	f.runLog = append(f.runLog, p.Project+":"+verb)
	return nil
}

func (f *fakeRunner) Logs(context.Context, dockerx.ComposeProjectFact, int) ([]string, error) {
	return []string{"proj-line-1", "proj-line-2"}, nil
}

func (f *fakeRunner) FollowLogs(context.Context, dockerx.ComposeProjectFact, int) (*dockerx.LogStream, error) {
	return nil, errors.New("follow in module_test")
}

func (f *fakeRunner) runs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.runLog...)
}

// ---------- fake 审计 ----------

type fakeAudit struct {
	mu      sync.Mutex
	entries []audit.Entry
	failN   int // 前 N 次失败（D-09 重试验证）
}

func (f *fakeAudit) Record(_ context.Context, e audit.Entry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failN > 0 {
		f.failN--
		return errors.New("audit down")
	}
	f.entries = append(f.entries, e)
	return nil
}

func (f *fakeAudit) snapshot() []audit.Entry {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]audit.Entry(nil), f.entries...)
}

// ---------- 断言助手 ----------

func wantSymbol(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("err = nil, want %s", want)
	}
	var ec *errcode.Error
	if !errors.As(err, &ec) {
		t.Fatalf("err = %v（非 errcode），want %s", err, want)
	}
	if ec.Symbol() != want {
		t.Fatalf("err = %s, want %s（message=%s）", ec.Symbol(), want, ec.Message())
	}
}

func errData(t *testing.T, err error) map[string]any {
	t.Helper()
	var ec *errcode.Error
	if !errors.As(err, &ec) {
		t.Fatalf("err = %v 非 errcode", err)
	}
	m, ok := ec.Data().(map[string]any)
	if !ok {
		t.Fatalf("data = %#v, want map", ec.Data())
	}
	return m
}

// ---------- 矩阵（BA 附录B.2/D-02） ----------

func TestSupportedActions(t *testing.T) {
	tests := []struct {
		state string
		want  []string
	}{
		{"running", []string{"stop", "restart", "pause"}},
		{"paused", []string{"stop", "restart", "unpause"}},
		{"stopped", []string{"start", "delete"}},
		{"restarting", []string{}},
		{"removing", []string{}},
	}
	for _, tt := range tests {
		got := SupportedActions(tt.state)
		if len(got) != len(tt.want) {
			t.Fatalf("SupportedActions(%s) = %v, want %v", tt.state, got, tt.want)
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Fatalf("SupportedActions(%s) = %v, want %v", tt.state, got, tt.want)
			}
		}
	}
}

func TestActionAllowed(t *testing.T) {
	tests := []struct {
		state, action string
		want          bool
	}{
		{"stopped", "start", true},
		{"stopped", "delete", true},
		{"stopped", "stop", false},
		{"running", "stop", true},
		{"running", "pause", true},
		{"running", "start", false},
		{"paused", "unpause", true},
		{"paused", "pause", false},
		{"running", "restart", true},
		{"restarting", "start", false}, // 过渡态恒拒
		{"removing", "delete", false},
	}
	for _, tt := range tests {
		if got := ActionAllowed(tt.state, tt.action); got != tt.want {
			t.Errorf("ActionAllowed(%s,%s) = %v, want %v", tt.state, tt.action, got, tt.want)
		}
	}
}

// ---------- 校验（R-14） ----------

func TestParseTail(t *testing.T) {
	tests := []struct {
		in       string
		want     int
		wantFail bool
	}{
		{"", 500, false},
		{"1", 1, false},
		{"2000", 2000, false},
		{"500", 500, false},
		{"0", 0, true},
		{"2001", 0, true},
		{"-3", 0, true},
		{"abc", 0, true},
		{"12.5", 0, true},
	}
	for _, tt := range tests {
		got, err := ParseTail(tt.in)
		if tt.wantFail {
			if err == nil {
				t.Errorf("ParseTail(%q) = %d, want DK_001", tt.in, got)
			} else {
				wantSymbol(t, err, "DK_001")
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("ParseTail(%q) = %d,%v want %d", tt.in, got, err, tt.want)
		}
	}
}

func TestParseStateFilter(t *testing.T) {
	if f, err := ParseStateFilter(""); err != nil || f != nil {
		t.Fatalf("空筛选: %v %v", f, err)
	}
	f, err := ParseStateFilter("running,stopped")
	if err != nil || len(f) != 2 || !f["running"] || !f["stopped"] {
		t.Fatalf("多值: %v %v", f, err)
	}
	if _, err := ParseStateFilter("restarting"); err != nil {
		t.Fatalf("过渡态原始值应合法: %v", err)
	}
	for _, bad := range []string{"bogus", "running,", ",running", "RUNNING", "running,,stopped"} {
		if _, err := ParseStateFilter(bad); err == nil {
			t.Errorf("ParseStateFilter(%q) 应 DK_001", bad)
		} else {
			wantSymbol(t, err, "DK_001")
		}
	}
}

func TestValidateImageRef(t *testing.T) {
	valid := []string{
		"nginx", "nginx:1.25", "library/nginx", "localhost/foo:latest",
		"registry.example.com:5000/app/web:v1.0", "foo/bar/baz:tag",
		"my-app", "a.b.c/img_x", "docker.io/nginx",
	}
	for _, ref := range valid {
		if !ValidateImageRef(ref) {
			t.Errorf("ValidateImageRef(%q) = false, want true", ref)
		}
	}
	invalid := []string{
		"", "NGINX", "nginx:", "nginx:ta g", "a b", "-lead/x", "nginx:点的",
		"reg..bad/x", "foo@sha256:abc",
	}
	// docker tag 字符集允许大写（[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}）；x/y 为
	// 合法路径引用——两者如实接受。
	more := []string{"nginx:UPPER-TAG_ok", "x/y"}
	for _, ref := range more {
		if !ValidateImageRef(ref) {
			t.Errorf("ValidateImageRef(%q) = false, want true（docker 语法合法）", ref)
		}
	}
	for _, ref := range invalid {
		if ValidateImageRef(ref) {
			t.Errorf("ValidateImageRef(%q) = true, want false", ref)
		}
	}
}

// ---------- 探测与清单（D-01/D-02） ----------

func availableInfo() dockerx.DaemonInfo {
	v := "27.5.1"
	return dockerx.DaemonInfo{State: dockerx.StateAvailable, Version: &v}
}

func newTestService(gw *fakeGateway, runner *fakeRunner, rec *fakeAudit) *Service {
	return New(gw, runner, rec)
}

func TestDaemonStatus(t *testing.T) {
	gw := &fakeGateway{probe: availableInfo()}
	s := newTestService(gw, &fakeRunner{}, &fakeAudit{})
	data, err := s.DaemonStatus(context.Background())
	if err != nil {
		t.Fatalf("DaemonStatus: %v", err)
	}
	if data.State != "available" || data.Version == nil || *data.Version != "27.5.1" ||
		data.Detail != nil || data.GeneratedAt == "" {
		t.Fatalf("data = %+v", data)
	}
	// unavailable：detail 区分（D-01）。
	d := dockerx.DetailNotInstalled
	gw.probe = dockerx.DaemonInfo{State: dockerx.StateUnavailable, Detail: &d}
	data, _ = s.DaemonStatus(context.Background())
	if data.Version != nil || data.Detail == nil || *data.Detail != dockerx.DetailNotInstalled {
		t.Fatalf("unavailable data = %+v", data)
	}
}

func sampleFacts() []dockerx.ContainerFact {
	return []dockerx.ContainerFact{
		{ContainerID: "c1", Name: "alpha", Image: "nginx:1.25", State: "running",
			ComposeProject: "web"},
		{ContainerID: "c2", Name: "beta", Image: "redis:7", State: "stopped"},
		{ContainerID: "c3", Name: "gamma", Image: "app:dev", State: "paused",
			ComposeProject: "web"},
		{ContainerID: "c4", Name: "delta", Image: "job:1", State: "restarting"},
	}
}

func TestContainerListProjection(t *testing.T) {
	gw := &fakeGateway{probe: availableInfo(), list: sampleFacts()}
	s := newTestService(gw, &fakeRunner{}, &fakeAudit{})
	data, err := s.ContainerList(context.Background(), availableInfo(), "", "")
	if err != nil {
		t.Fatalf("ContainerList: %v", err)
	}
	if data.Total != 4 || len(data.Items) != 4 {
		t.Fatalf("total = %d", data.Total)
	}
	if data.Items[0].Name != "alpha" || data.Items[1].Name != "beta" ||
		data.Items[2].Name != "delta" || data.Items[3].Name != "gamma" {
		t.Fatalf("名称字典序: %+v", data.Items)
	}
	if got := data.Items[0].SupportedActions; len(got) != 3 || got[0] != "stop" {
		t.Fatalf("running 动作面 = %v", got)
	}
	if got := data.Items[3].SupportedActions; len(got) != 3 || got[2] != "unpause" {
		t.Fatalf("paused 动作面 = %v", got)
	}
	if got := data.Items[2].SupportedActions; len(got) != 0 {
		t.Fatalf("过渡态动作面 = %v, want 空", got)
	}
	if data.Items[0].ComposeProject == nil || *data.Items[0].ComposeProject != "web" {
		t.Fatalf("composeProject 标注 = %+v", data.Items[0].ComposeProject)
	}
	if data.Items[1].ComposeProject != nil {
		t.Fatalf("无项目应为 null")
	}
	if data.Daemon.State != "available" || data.Daemon.Version == nil {
		t.Fatalf("daemon 摘要 = %+v", data.Daemon)
	}
}

func TestContainerListFilter(t *testing.T) {
	s := newTestService(&fakeGateway{probe: availableInfo(), list: sampleFacts()},
		&fakeRunner{}, &fakeAudit{})
	data, err := s.ContainerList(context.Background(), availableInfo(), "running,paused", "")
	if err != nil || data.Total != 2 {
		t.Fatalf("state 筛选: %d %v", data.Total, err)
	}
	data, err = s.ContainerList(context.Background(), availableInfo(), "restarting", "")
	if err != nil || data.Total != 1 || data.Items[0].ContainerID != "c4" {
		t.Fatalf("过渡态筛选: %+v %v", data.Items, err)
	}
	data, err = s.ContainerList(context.Background(), availableInfo(), "", "web")
	if err != nil || data.Total != 2 {
		t.Fatalf("composeProject 等值: %d %v", data.Total, err)
	}
	if _, err := s.ContainerList(context.Background(), availableInfo(), "bogus", ""); err == nil {
		t.Fatal("非法筛选值应 DK_001")
	} else {
		wantSymbol(t, err, "DK_001")
	}
}

func TestContainerLogsChain(t *testing.T) {
	gw := &fakeGateway{probe: availableInfo(), list: sampleFacts(),
		logs: map[string][]string{"c1": {"l1", "l2", "l3"}}}
	s := newTestService(gw, &fakeRunner{}, &fakeAudit{})
	if _, err := s.ContainerLogs(context.Background(), "  ", "500"); err == nil {
		t.Fatal("空 containerId 应 DK_001")
	} else {
		wantSymbol(t, err, "DK_001")
	}
	if _, err := s.ContainerLogs(context.Background(), "gone", "500"); err == nil {
		t.Fatal("不存在应 DK_003")
	} else {
		wantSymbol(t, err, "DK_003")
	}
	if _, err := s.ContainerLogs(context.Background(), "c1", "9999"); err == nil {
		t.Fatal("tail 越界应 DK_001")
	} else {
		wantSymbol(t, err, "DK_001")
	}
	data, err := s.ContainerLogs(context.Background(), "c1", "")
	if err != nil || len(data.Lines) != 3 {
		t.Fatalf("logs: %+v %v", data, err)
	}
	// tail=2 → 尾两行。
	data, _ = s.ContainerLogs(context.Background(), "c1", "2")
	if len(data.Lines) != 2 || data.Lines[0] != "l2" {
		t.Fatalf("tail=2 = %v", data.Lines)
	}
}

func TestContainerStats(t *testing.T) {
	gw := &fakeGateway{probe: availableInfo(), list: sampleFacts(),
		stats: map[string]dockerx.StatsFact{"c1": {CPUPercent: 3.5, MemUsageByte: 100, MemLimitByte: 200, MemPercent: 50}}}
	s := newTestService(gw, &fakeRunner{}, &fakeAudit{})
	data, err := s.ContainerStats(context.Background(), "c1")
	if err != nil || data.CPUPercent != 3.5 || data.MemUsageByte != 100 ||
		data.MemLimitByte != 200 || data.MemPercent != 50 {
		t.Fatalf("stats: %+v %v", data, err)
	}
	if _, err := s.ContainerStats(context.Background(), "gone"); err == nil {
		t.Fatal("DK_003")
	} else {
		wantSymbol(t, err, "DK_003")
	}
}

// ---------- 容器生命周期（D-04 审计口径 A-D15） ----------

func TestContainerActionChain(t *testing.T) {
	gw := &fakeGateway{probe: availableInfo(), list: sampleFacts(),
		inspect: map[string]string{"c2": "running"}}
	rec := &fakeAudit{}
	s := newTestService(gw, &fakeRunner{}, rec)

	// 空引用：DK_001 零审计。
	if _, err := s.ContainerAction(context.Background(), "admin", "start", ""); err == nil {
		t.Fatal("DK_001")
	} else {
		wantSymbol(t, err, "DK_001")
	}
	if n := len(rec.snapshot()); n != 0 {
		t.Fatalf("DK_001 拒绝后审计 = %d, want 0", n)
	}

	// 不存在：DK_003 + failure 审计（A-D15）。
	if _, err := s.ContainerAction(context.Background(), "admin", "start", "gone"); err == nil {
		t.Fatal("DK_003")
	} else {
		wantSymbol(t, err, "DK_003")
	}
	entries := rec.snapshot()
	if len(entries) != 1 || entries[0].Result != audit.ResultFailure ||
		entries[0].Action != AuditContainerStarted || entries[0].ActorName != "admin" {
		t.Fatalf("DK_003 审计 = %+v", entries)
	}

	// 矩阵拒绝：stopped 态 "stop" 违例（stop←running|paused）→ DK_004 + data
	// 动作面，零审计。
	_, err := s.ContainerAction(context.Background(), "admin", "stop", "c2")
	wantSymbol(t, err, "DK_004")
	var ec *errcode.Error
	if !errors.As(err, &ec) {
		t.Fatalf("err = %v 非 errcode", err)
	}
	d, ok := ec.Data().(ActionStateData)
	if !ok {
		t.Fatalf("DK_004 data 类型 = %T", ec.Data())
	}
	if d.CurrentState != "stopped" || d.Action != "stop" {
		t.Fatalf("DK_004 data = %+v", d)
	}
	if len(d.AllowedActions) != 2 || d.AllowedActions[0] != "start" || d.AllowedActions[1] != "delete" {
		t.Fatalf("allowedActions = %v", d.AllowedActions)
	}
	if n := len(rec.snapshot()); n != 1 {
		t.Fatalf("DK_004 拒绝后审计 = %d, want 1（零新增）", n)
	}

	// 成功：start on c2（stopped）→ 即读实况 running（inspect 桩）→ success 审计。
	data, err := s.ContainerAction(context.Background(), "admin", "start", "c2")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if data.ContainerID != "c2" || data.CurrentState != "running" {
		t.Fatalf("data = %+v", data)
	}
	if calls := gw.actions(); len(calls) != 1 || calls[0] != "start:c2" {
		t.Fatalf("actionCalls = %v", calls)
	}
	entries = rec.snapshot()
	if len(entries) != 2 || entries[1].Result != audit.ResultSuccess ||
		entries[1].ObjectType != ObjectTypeContainer || entries[1].ObjectID != "c2" ||
		entries[1].Detail.(map[string]any)["name"] != "beta" {
		t.Fatalf("success 审计 = %+v", entries[1])
	}

	// SDK 失败：DK_017 reason 透传 + failure 审计。
	gw.actionErr = map[string]error{"c1": &dockerx.ExecError{Verb: "container stop", Reason: "boom"}}
	_, err = s.ContainerAction(context.Background(), "admin", "stop", "c1")
	wantSymbol(t, err, "DK_017")
	if got := errData(t, err)["reason"]; got != "boom" {
		t.Fatalf("reason = %v", got)
	}
	entries = rec.snapshot()
	if len(entries) != 3 || entries[2].Result != audit.ResultFailure {
		t.Fatalf("DK_017 审计 = %+v", entries)
	}
}

func TestContainerDeleteChain(t *testing.T) {
	gw := &fakeGateway{probe: availableInfo(), list: sampleFacts()}
	rec := &fakeAudit{}
	s := newTestService(gw, &fakeRunner{}, rec)

	if _, err := s.ContainerDelete(context.Background(), "admin", "gone"); err == nil {
		t.Fatal("DK_003")
	} else {
		wantSymbol(t, err, "DK_003")
	}
	if len(rec.snapshot()) != 1 {
		t.Fatalf("DK_003 审计 = %d", len(rec.snapshot()))
	}
	// running 容器删除 → DK_004（先停止提示）。
	var ecDelete *errcode.Error
	_, err := s.ContainerDelete(context.Background(), "admin", "c1")
	wantSymbol(t, err, "DK_004")
	if !errors.As(err, &ecDelete) {
		t.Fatalf("err = %v 非 errcode", err)
	}
	if dd, ok := ecDelete.Data().(ActionStateData); !ok || dd.CurrentState != "running" || dd.Action != "delete" {
		t.Fatalf("DK_004 data = %#v", ecDelete.Data())
	}
	if len(rec.snapshot()) != 1 { // DK_004 零新增
		t.Fatalf("DK_004 后审计 = %d, want 1", len(rec.snapshot()))
	}
	// stopped（c2）→ 成功。
	data, err := s.ContainerDelete(context.Background(), "admin", "c2")
	if err != nil || !data.Deleted || data.ContainerID != "c2" {
		t.Fatalf("delete: %+v %v", data, err)
	}
	if calls := gw.writes(); len(calls) != 1 || calls[0] != "container:c2" {
		t.Fatalf("deleteCalls = %v", calls)
	}
	entries := rec.snapshot()
	if len(entries) != 2 || entries[1].Action != AuditContainerDeleted ||
		entries[1].Detail.(map[string]any)["image"] != "redis:7" {
		t.Fatalf("delete 审计 = %+v", entries[1])
	}
}

// ---------- 镜像/卷/网络（D-05/D-08/I-05） ----------

func TestImagePullFlow(t *testing.T) {
	gw := &fakeGateway{probe: availableInfo(), pullFact: dockerx.ImageFact{ImageID: "sha256:abc", Ref: "nginx:1.25"}}
	rec := &fakeAudit{}
	s := newTestService(gw, &fakeRunner{}, rec)

	// 非法 ref → DK_012 + failure 审计（§4.2.4 明文）。
	_, err := s.ImagePull(context.Background(), "admin", "NGINX")
	wantSymbol(t, err, "DK_012")
	entries := rec.snapshot()
	if len(entries) != 1 || entries[0].Result != audit.ResultFailure || entries[0].Action != AuditImagePulled {
		t.Fatalf("DK_012 审计 = %+v", entries)
	}

	// 拉取失败 → DK_013 reason 透传 + failure。
	gw.pullErr = &dockerx.ExecError{Verb: "image pull", Reason: "manifest unknown"}
	_, err = s.ImagePull(context.Background(), "admin", "nginx:9.9.9")
	wantSymbol(t, err, "DK_013")
	if got := errData(t, err)["reason"]; got != "manifest unknown" {
		t.Fatalf("reason = %v", got)
	}
	if len(rec.snapshot()) != 2 {
		t.Fatalf("DK_013 审计 = %d", len(rec.snapshot()))
	}

	// 成功。
	gw.pullErr = nil
	data, err := s.ImagePull(context.Background(), "admin", "nginx:1.25")
	if err != nil || data.ImageID != "sha256:abc" || data.Ref != "nginx:1.25" {
		t.Fatalf("pull: %+v %v", data, err)
	}
	entries = rec.snapshot()
	if len(entries) != 3 || entries[2].Result != audit.ResultSuccess ||
		entries[2].Detail.(map[string]any)["ref"] != "nginx:1.25" {
		t.Fatalf("pull 审计 = %+v", entries[2])
	}
}

func TestResourceDeleteFlows(t *testing.T) {
	gw := &fakeGateway{
		probe:    availableInfo(),
		images:   []dockerx.ImageFact{{ImageID: "img-1", Ref: "nginx:1.25", SizeBytes: 10}},
		volumes:  []dockerx.VolumeFact{{VolumeName: "data", Mountpoint: "/var/lib/docker/volumes/data", SizeBytes: 5}},
		networks: []dockerx.NetworkFact{{NetworkID: "net-1", Name: "mynet", Driver: "bridge", Subnet: "172.18.0.0/16"}},
	}
	rec := &fakeAudit{}
	s := newTestService(gw, &fakeRunner{}, rec)

	// 镜像：不存在/被引用/成功。
	if _, err := s.ImageDelete(context.Background(), "admin", "gone"); err == nil {
		t.Fatal("DK_005")
	} else {
		wantSymbol(t, err, "DK_005")
	}
	gw.delErr = map[string]error{"image:img-1": fmt.Errorf("%w: conflict", dockerx.ErrInUse)}
	if _, err := s.ImageDelete(context.Background(), "admin", "img-1"); err == nil {
		t.Fatal("DK_006")
	} else {
		wantSymbol(t, err, "DK_006")
	}
	gw.delErr = nil
	data, err := s.ImageDelete(context.Background(), "admin", "img-1")
	if err != nil || !data.Deleted {
		t.Fatalf("image delete: %+v %v", data, err)
	}

	// 卷：不存在/被挂载/成功。
	if _, err := s.VolumeDelete(context.Background(), "admin", "gone"); err == nil {
		t.Fatal("DK_007")
	} else {
		wantSymbol(t, err, "DK_007")
	}
	gw.delErr = map[string]error{"volume:data": fmt.Errorf("%w: volume in use", dockerx.ErrInUse)}
	if _, err := s.VolumeDelete(context.Background(), "admin", "data"); err == nil {
		t.Fatal("DK_008")
	} else {
		wantSymbol(t, err, "DK_008")
	}
	gw.delErr = nil
	if _, err := s.VolumeDelete(context.Background(), "admin", "data"); err != nil {
		t.Fatalf("volume delete: %v", err)
	}

	// 网络：不存在/被接入/预定义/成功。
	if _, err := s.NetworkDelete(context.Background(), "admin", "gone"); err == nil {
		t.Fatal("DK_009")
	} else {
		wantSymbol(t, err, "DK_009")
	}
	gw.delErr = map[string]error{"network:net-1": fmt.Errorf("%w: has active endpoints", dockerx.ErrInUse)}
	if _, err := s.NetworkDelete(context.Background(), "admin", "net-1"); err == nil {
		t.Fatal("DK_010")
	} else {
		wantSymbol(t, err, "DK_010")
	}
	gw.delErr = map[string]error{"network:net-1": fmt.Errorf("%w: predefined", dockerx.ErrPredefinedNetwork)}
	if _, err := s.NetworkDelete(context.Background(), "admin", "net-1"); err == nil {
		t.Fatal("DK_011")
	} else {
		wantSymbol(t, err, "DK_011")
	}
	gw.delErr = nil
	if _, err := s.NetworkDelete(context.Background(), "admin", "net-1"); err != nil {
		t.Fatalf("network delete: %v", err)
	}

	// 审计计数：镜像 3（005 fail+006 fail+ok）卷 3 网络 4，全部恰一条/次。
	entries := rec.snapshot()
	if len(entries) != 10 {
		t.Fatalf("审计 = %d 条, want 10", len(entries))
	}
	for _, e := range entries {
		if e.Result != audit.ResultFailure && e.Action != AuditImageDeleted &&
			e.Action != AuditVolumeDeleted && e.Action != AuditNetworkDeleted {
			t.Fatalf("审计口径异常: %+v", e)
		}
	}
}

func TestListProjections(t *testing.T) {
	gw := &fakeGateway{
		probe:    availableInfo(),
		images:   []dockerx.ImageFact{{ImageID: "i1", Ref: "a:1", SizeBytes: 1}, {ImageID: "i2", Ref: "b:2", SizeBytes: 2}},
		volumes:  []dockerx.VolumeFact{{VolumeName: "v1", Mountpoint: "/m1", SizeBytes: 7}},
		networks: []dockerx.NetworkFact{{NetworkID: "n1", Name: "net", Driver: "bridge", Subnet: "10.0.0.0/24"}},
	}
	s := newTestService(gw, &fakeRunner{}, &fakeAudit{})
	imgs, err := s.ImageList(context.Background())
	if err != nil || imgs.Total != 2 || imgs.Items[0].Ref != "a:1" || imgs.Items[1].SizeBytes != 2 {
		t.Fatalf("ImageList: %+v %v", imgs, err)
	}
	vols, err := s.VolumeList(context.Background())
	if err != nil || vols.Total != 1 || vols.Items[0].Mountpoint != "/m1" {
		t.Fatalf("VolumeList: %+v %v", vols, err)
	}
	nets, err := s.NetworkList(context.Background())
	if err != nil || nets.Total != 1 || nets.Items[0].Subnet != "10.0.0.0/24" {
		t.Fatalf("NetworkList: %+v %v", nets, err)
	}
}

// ---------- Compose（D-06/I-06/I-08） ----------

func sampleFacts2() []dockerx.ComposeProjectFact {
	return []dockerx.ComposeProjectFact{
		{Project: "web", WorkingDir: "/srv/web", ConfigFile: "/srv/web/a.yml,/srv/web/b.yml",
			Containers: []dockerx.ComposeContainerRef{
				{ContainerID: "c1", Name: "web-app", State: "running"},
				{ContainerID: "c3", Name: "web-db", State: "stopped"},
			}, Locatable: true},
		{Project: "legacy", WorkingDir: "/gone", ConfigFile: "/gone/a.yml",
			Containers: []dockerx.ComposeContainerRef{{ContainerID: "c9", Name: "old", State: "running"}},
			Locatable:  false, Missing: dockerx.MissingWorkingDir},
	}
}

func TestComposeList(t *testing.T) {
	s := newTestService(&fakeGateway{probe: availableInfo()},
		&fakeRunner{facts: sampleFacts2()}, &fakeAudit{})
	data, err := s.ComposeList(context.Background())
	if err != nil {
		t.Fatalf("ComposeList: %v", err)
	}
	if data.Total != 2 {
		t.Fatalf("total = %d", data.Total)
	}
	web := data.Items[0]
	if web.Project != "web" || web.WorkingDir == nil || *web.WorkingDir != "/srv/web" ||
		web.ConfigFile == nil || *web.ConfigFile != "/srv/web/a.yml,/srv/web/b.yml" {
		t.Fatalf("web = %+v", web)
	}
	if len(web.Containers) != 2 || web.Containers[0].Name != "web-app" {
		t.Fatalf("containers = %+v", web.Containers)
	}
	if !web.Locatable || data.Items[1].Locatable {
		t.Fatalf("locatable = %v/%v", web.Locatable, data.Items[1].Locatable)
	}
}

func TestComposeActionChain(t *testing.T) {
	runner := &fakeRunner{facts: sampleFacts2()}
	rec := &fakeAudit{}
	s := newTestService(&fakeGateway{probe: availableInfo()}, runner, rec)

	// 空 project → DK_001 零审计。
	if _, err := s.ComposeAction(context.Background(), "admin", "", "up"); err == nil {
		t.Fatal("DK_001")
	} else {
		wantSymbol(t, err, "DK_001")
	}
	if len(rec.snapshot()) != 0 {
		t.Fatal("DK_001 零审计")
	}

	// 不存在 → DK_014 + failure 审计。
	_, err := s.ComposeAction(context.Background(), "admin", "ghost", "up")
	wantSymbol(t, err, "DK_014")
	if len(rec.snapshot()) != 1 {
		t.Fatalf("DK_014 审计 = %d", len(rec.snapshot()))
	}

	// 定位失败 → DK_015 data 携 label 路径 + failure。
	_, err = s.ComposeAction(context.Background(), "admin", "legacy", "down")
	wantSymbol(t, err, "DK_015")
	var ecLoc *errcode.Error
	if !errors.As(err, &ecLoc) {
		t.Fatalf("err = %v 非 errcode", err)
	}
	ld, ok := ecLoc.Data().(LocateFailedData)
	if !ok || ld.WorkingDir != "/gone" || ld.ConfigFile != "/gone/a.yml" ||
		ld.Missing != dockerx.MissingWorkingDir {
		t.Fatalf("DK_015 data = %#v", ecLoc.Data())
	}
	if len(rec.snapshot()) != 2 {
		t.Fatalf("DK_015 审计 = %d", len(rec.snapshot()))
	}

	// 互斥 → DK_018 busy=true + failure。
	runner.runErr = map[string]error{"web": fmt.Errorf("%w: web", dockerx.ErrBusy)}
	_, err = s.ComposeAction(context.Background(), "admin", "web", "restart")
	wantSymbol(t, err, "DK_018")
	if errData(t, err)["busy"] != true {
		t.Fatalf("DK_018 data = %v", errData(t, err))
	}
	if len(rec.snapshot()) != 3 {
		t.Fatalf("DK_018 审计 = %d", len(rec.snapshot()))
	}

	// CLI 失败 → DK_016 stderr 透传 + failure。
	runner.runErr = map[string]error{"web": &dockerx.ExecError{Verb: "compose up", Reason: "no such file"}}
	_, err = s.ComposeAction(context.Background(), "admin", "web", "up")
	wantSymbol(t, err, "DK_016")
	if errData(t, err)["reason"] != "no such file" {
		t.Fatalf("DK_016 data = %v", errData(t, err))
	}

	// 成功 → data done:true + success 审计（verb 与动作码映射）。
	runner.runErr = nil
	data, err := s.ComposeAction(context.Background(), "admin", "web", "up")
	if err != nil || data.Project != "web" || data.Verb != "up" || !data.Done {
		t.Fatalf("compose up: %+v %v", data, err)
	}
	entries := rec.snapshot()
	if len(entries) != 5 || entries[4].Action != AuditComposeUp ||
		entries[4].ObjectType != ObjectTypeComposeProject ||
		entries[4].Detail.(map[string]any)["workingDir"] != "/srv/web" {
		t.Fatalf("成功审计 = %+v", entries)
	}
	// CLI 失败尝试与成功各一次（失败不重试）；互斥拒绝零 CLI 触达。
	if runs := runner.runs(); len(runs) != 2 || runs[0] != "web:up" || runs[1] != "web:up" {
		t.Fatalf("runs = %v, want [web:up web:up]（失败一次+成功一次，busy 零触达）", runs)
	}
}

func TestComposeLogsChain(t *testing.T) {
	s := newTestService(&fakeGateway{probe: availableInfo()},
		&fakeRunner{facts: sampleFacts2()}, &fakeAudit{})
	if _, err := s.ComposeLogs(context.Background(), "ghost", ""); err == nil {
		t.Fatal("DK_014")
	} else {
		wantSymbol(t, err, "DK_014")
	}
	if _, err := s.ComposeLogs(context.Background(), "legacy", ""); err == nil {
		t.Fatal("DK_015")
	} else {
		wantSymbol(t, err, "DK_015")
	}
	if _, err := s.ComposeLogs(context.Background(), "web", "0"); err == nil {
		t.Fatal("tail DK_001")
	} else {
		wantSymbol(t, err, "DK_001")
	}
	data, err := s.ComposeLogs(context.Background(), "web", "100")
	if err != nil || len(data.Lines) != 2 {
		t.Fatalf("ComposeLogs: %+v %v", data, err)
	}
}

// ---------- WS 订阅握手链（D-03） ----------

func TestFollowSubscribeChains(t *testing.T) {
	gw := &fakeGateway{probe: availableInfo(), exists: map[string]bool{"c1": true}}
	runner := &fakeRunner{facts: sampleFacts2()}
	s := newTestService(gw, runner, &fakeAudit{})

	if _, err := s.ContainerLogFollow(context.Background(), "", ""); err == nil {
		t.Fatal("DK_001")
	} else {
		wantSymbol(t, err, "DK_001")
	}
	if _, err := s.ContainerLogFollow(context.Background(), "gone", ""); err == nil {
		t.Fatal("DK_003")
	} else {
		wantSymbol(t, err, "DK_003")
	}

	if _, err := s.ComposeLogFollow(context.Background(), "ghost", ""); err == nil {
		t.Fatal("DK_014")
	} else {
		wantSymbol(t, err, "DK_014")
	}
	if _, err := s.ComposeLogFollow(context.Background(), "legacy", ""); err == nil {
		t.Fatal("DK_015")
	} else {
		wantSymbol(t, err, "DK_015")
	}
}

// ---------- 审计降级（D-09） ----------

func TestAuditFallback(t *testing.T) {
	gw := &fakeGateway{probe: availableInfo(), list: sampleFacts(),
		inspect: map[string]string{"c2": "running"}}
	rec := &fakeAudit{failN: 1} // 首次失败、重试成功
	s := newTestService(gw, &fakeRunner{}, rec)
	data, err := s.ContainerAction(context.Background(), "admin", "start", "c2")
	if err != nil || data.CurrentState == "" {
		t.Fatalf("重试成功不影响操作结果: %+v %v", data, err)
	}
	if entries := rec.snapshot(); len(entries) != 1 {
		t.Fatalf("重试成功应恰一条: %d", len(entries))
	}

	// 持续失败：操作结果不受影响（日志兜底，不入库）。
	rec2 := &fakeAudit{failN: 1000}
	s2 := newTestService(gw, &fakeRunner{}, rec2)
	if _, err := s2.ContainerAction(context.Background(), "admin", "start", "c2"); err != nil {
		t.Fatalf("审计故障不应影响操作: %v", err)
	}
	if entries := rec2.snapshot(); len(entries) != 0 {
		t.Fatalf("持续失败零入库: %d", len(entries))
	}
}

// ---------- 常量核对（api §3.4） ----------

func TestAuditActionCodes(t *testing.T) {
	codes := map[string]string{
		AuditContainerStarted: "container_started", AuditContainerStopped: "container_stopped",
		AuditContainerRestarted: "container_restarted", AuditContainerPaused: "container_paused",
		AuditContainerUnpaused: "container_unpaused", AuditContainerDeleted: "container_deleted",
		AuditImagePulled: "image_pulled", AuditImageDeleted: "image_deleted",
		AuditVolumeDeleted: "volume_deleted", AuditNetworkDeleted: "network_deleted",
		AuditComposeUp: "compose_up", AuditComposeDown: "compose_down",
		AuditComposeRestarted: "compose_restarted",
	}
	if len(codes) != 13 {
		t.Fatalf("审计动作码 = %d, want 13", len(codes))
	}
	for got, want := range codes {
		if got != want {
			t.Errorf("动作码 %q != %q", got, want)
		}
	}
	if TailDefault != 500 || TailMin != 1 || TailMax != 2000 {
		t.Fatalf("tail 常量 = %d/%d/%d", TailDefault, TailMin, TailMax)
	}
}
