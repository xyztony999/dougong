// dockerx_test.go 是适配器纯函数/白盒表驱动测试（实机 daemon/CLI 交互归
// Linux 目标环境冒烟——Windows 开发机无 daemon，firewallx/sysd 同口径；
// CLI 参数组装/解析/聚合/跟随编排逻辑此处全覆盖）。
package dockerx

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
)

// ---------- MapState（BA 附录B.1） ----------

func TestMapState(t *testing.T) {
	tests := []struct{ raw, want string }{
		{"running", "running"},
		{"paused", "paused"},
		{"created", "stopped"},
		{"exited", "stopped"},
		{"dead", "stopped"},
		{"", "stopped"},
		{"restarting", "restarting"}, // 过渡态透传
		{"removing", "removing"},     // 过渡态透传
	}
	for _, tt := range tests {
		if got := MapState(tt.raw); got != tt.want {
			t.Errorf("MapState(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

// ---------- tailStr ----------

func TestTailStr(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{{-1, "all"}, {0, "0"}, {1, "1"}, {500, "500"}, {2000, "2000"}}
	for _, tt := range tests {
		if got := tailStr(tt.in); got != tt.want {
			t.Errorf("tailStr(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// ---------- 日志流解析（stdcopy 解复用 / TTY 直读） ----------

// muxFrame 构造 docker 多路复用帧（STREAM_TYPE,size,payload）。
func muxFrame(stream byte, payload string) []byte {
	head := make([]byte, 8)
	head[0] = stream
	binary.BigEndian.PutUint32(head[4:8], uint32(len(payload)))
	return append(head, []byte(payload)...)
}

func TestReadLogLinesMultiplexed(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(muxFrame(1, "stdout-line-1\n"))
	buf.Write(muxFrame(2, "stderr-line\r\n"))
	buf.Write(muxFrame(1, "partial-tail"))
	lines, err := readLogLines(&buf)
	if err != nil {
		t.Fatalf("readLogLines: %v", err)
	}
	want := []string{"stdout-line-1", "stderr-line", "partial-tail"}
	if len(lines) != len(want) {
		t.Fatalf("lines = %v, want %v", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("lines[%d] = %q, want %q", i, lines[i], want[i])
		}
	}
}

func TestReadLogLinesPlainTTY(t *testing.T) {
	// 首字节 0x68('h') 非 1/2 → peek 判定直读（TTY 流）。
	r := strings.NewReader("hello\nworld\n")
	lines, err := readLogLines(r)
	if err != nil {
		t.Fatalf("readLogLines: %v", err)
	}
	if len(lines) != 2 || lines[0] != "hello" || lines[1] != "world" {
		t.Fatalf("lines = %v", lines)
	}
}

func TestIsMultiplexedShort(t *testing.T) {
	if isMultiplexed(newBufReader([]byte("abc"))) {
		t.Fatal("short stream should be plain")
	}
}

// newBufReader 小助手（peek 判定与生产同型 *bufio.Reader）。
func newBufReader(b []byte) *bufio.Reader { return bufio.NewReader(bytes.NewReader(b)) }

// ---------- scanPullError ----------

func TestScanPullError(t *testing.T) {
	ok := `{"status":"Pulling from library/nginx"}
{"status":"Downloading","progressDetail":{"current":10,"total":100}}
{"status":"Download complete"}
{"status":"Status: Downloaded newer image"}`
	if got := scanPullError(strings.NewReader(ok)); got != "" {
		t.Fatalf("scanPullError(正常流) = %q, want empty", got)
	}
	fail := `{"status":"Pulling"}
{"errorDetail":{"message":"manifest for nginx:9.9.9 not found"},"error":"manifest for nginx:9.9.9 not found"}`
	want := "manifest for nginx:9.9.9 not found"
	if got := scanPullError(strings.NewReader(fail)); got != want {
		t.Fatalf("scanPullError(失败流) = %q, want %q", got, want)
	}
}

// ---------- computeStats（D-07 快照公式） ----------

func TestComputeStats(t *testing.T) {
	v := container.StatsResponse{}
	v.CPUStats.CPUUsage.TotalUsage = 300
	v.PreCPUStats.CPUUsage.TotalUsage = 100
	v.CPUStats.SystemUsage = 2000
	v.PreCPUStats.SystemUsage = 1000
	v.CPUStats.OnlineCPUs = 2
	v.MemoryStats.Usage = 900
	v.MemoryStats.Stats = map[string]uint64{"inactive_file": 100}
	v.MemoryStats.Limit = 1000
	got := computeStats(v)
	if got.MemUsageByte != 800 {
		t.Errorf("memUsage = %d, want 800（扣除 inactive_file）", got.MemUsageByte)
	}
	if got.MemLimitByte != 1000 {
		t.Errorf("memLimit = %d, want 1000", got.MemLimitByte)
	}
	if want := 80.0; got.MemPercent != want {
		t.Errorf("memPercent = %v, want %v", got.MemPercent, want)
	}
	if want := 40.0; got.CPUPercent != want { // (200/1000)*2*100
		t.Errorf("cpuPercent = %v, want %v", got.CPUPercent, want)
	}
}

func TestComputeStatsZeroGuard(t *testing.T) {
	got := computeStats(container.StatsResponse{}) // 全零：不得除零/NaN
	if got.CPUPercent != 0 || got.MemPercent != 0 {
		t.Fatalf("zero stats should be all-zero, got %+v", got)
	}
}

// ---------- composeArgs / splitConfigFiles（R-08 参数数组组装） ----------

func TestComposeArgs(t *testing.T) {
	fact := ComposeProjectFact{Project: "web", WorkingDir: "/srv/web",
		ConfigFile: "/srv/web/docker-compose.yml"}
	got := composeArgs(fact, VerbUp)
	want := []string{"compose", "--project-name", "web", "--project-directory", "/srv/web",
		"-f", "/srv/web/docker-compose.yml", "-d", "up"}
	if !equalStrings(got, want) {
		t.Fatalf("composeArgs(up) = %v, want %v（A-D8：up 补 -d）", got, want)
	}
	got = composeArgs(fact, VerbDown)
	want = []string{"compose", "--project-name", "web", "--project-directory", "/srv/web",
		"-f", "/srv/web/docker-compose.yml", "down"}
	if !equalStrings(got, want) {
		t.Fatalf("composeArgs(down) = %v, want %v（down 不补）", got, want)
	}
	multi := ComposeProjectFact{Project: "p", WorkingDir: "/d",
		ConfigFile: "/d/a.yml, /d/b.yml"}
	got = composeArgs(multi, VerbRestart)
	want = []string{"compose", "--project-name", "p", "--project-directory", "/d",
		"-f", "/d/a.yml", "-f", "/d/b.yml", "restart"}
	if !equalStrings(got, want) {
		t.Fatalf("composeArgs(multi-file) = %v, want %v（A-D9 逐个 -f）", got, want)
	}
}

func TestSplitConfigFiles(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a.yml", []string{"a.yml"}},
		{"a.yml,b.yml", []string{"a.yml", "b.yml"}},
		{" a.yml , b.yml ", []string{"a.yml", "b.yml"}},
		{"a.yml,,b.yml", []string{"a.yml", "b.yml"}},
	}
	for _, tt := range tests {
		got := splitConfigFiles(tt.in)
		if !equalStrings(got, tt.want) {
			t.Errorf("splitConfigFiles(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	if a == nil && b == nil {
		return true
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ---------- locateMissing（D-06/A-D9） ----------

func TestLocateMissing(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(file, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatalf("write compose file: %v", err)
	}
	if got := locateMissing(dir, file); got != "" {
		t.Errorf("齐全时 = %q, want 空", got)
	}
	if got := locateMissing(filepath.Join(dir, "gone"), file); got != MissingWorkingDir {
		t.Errorf("目录缺失 = %q, want %q", got, MissingWorkingDir)
	}
	if got := locateMissing(dir, filepath.Join(dir, "gone.yml")); got != MissingConfigFile {
		t.Errorf("配置缺失 = %q, want %q", got, MissingConfigFile)
	}
	if got := locateMissing("", file); got != MissingWorkingDir {
		t.Errorf("label 空 working_dir = %q, want %q", got, MissingWorkingDir)
	}
	// 多文件：第二个缺失 → config_file。
	if got := locateMissing(dir, file+","+filepath.Join(dir, "b.yml")); got != MissingConfigFile {
		t.Errorf("多文件缺失 = %q, want %q", got, MissingConfigFile)
	}
}

// ---------- ListProjectFacts（R-13 label 聚合） ----------

// fakeLogSource 是 LogSource 进程内替身（followSend 持有子流双向通道，
// 供用例向订阅者注入行）。
type fakeLogSource struct {
	mu         sync.Mutex
	containers []ContainerFact
	follow     map[string]*LogStream
	followSend map[string]chan string
	tailLogs   map[string][]string
}

func (f *fakeLogSource) ListContainers(context.Context) ([]ContainerFact, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ContainerFact(nil), f.containers...), nil
}

func (f *fakeLogSource) GetContainerLogs(_ context.Context, id string, tail int) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	lines := f.tailLogs[id]
	if len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}
	return lines, nil
}

func (f *fakeLogSource) FollowContainerLogs(_ context.Context, id string, tail int) (*LogStream, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if st, ok := f.follow[id]; ok {
		return st, nil
	}
	send := make(chan string, 8)
	end := make(chan string, 1)
	sctx, cancel := context.WithCancel(context.Background())
	st := &LogStream{Lines: send, End: end, Cancel: cancel}
	f.follow[id] = st
	f.followSend[id] = send
	go func() { <-sctx.Done() }() // 受控泵：静止直至取消（测试用）
	return st, nil
}

// sendLine 向指定容器的订阅流注入一行（等待订阅建立；不持锁发送）。
func (f *fakeLogSource) sendLine(t *testing.T, id, line string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		f.mu.Lock()
		ch, ok := f.followSend[id]
		f.mu.Unlock()
		if ok {
			select {
			case ch <- line:
				return
			case <-time.After(3 * time.Second):
				t.Fatalf("sendLine(%s) 发送超时", id)
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("等待容器 %s 订阅建立超时", id)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (f *fakeLogSource) setContainers(cs ...ContainerFact) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.containers = cs
}

func labeledContainer(id, name, state, project, workdir, config string) ContainerFact {
	return ContainerFact{ContainerID: id, Name: name, State: state,
		Labels: map[string]string{
			LabelProject:     project,
			LabelWorkingDir:  workdir,
			LabelConfigFiles: config,
		},
		ComposeProject: project,
	}
}

func TestListProjectFacts(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(file, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	src := &fakeLogSource{follow: map[string]*LogStream{}, followSend: map[string]chan string{}}
	src.setContainers(
		labeledContainer("c2", "web-db", StatusRunning, "web", dir, file),
		labeledContainer("c1", "web-app", StatusStopped, "web", dir, file),
		labeledContainer("c3", "standalone", StatusRunning, "", "", ""), // 无 label 不归属
		labeledContainer("c4", "edge", StatusRunning, "aa", filepath.Join(dir, "gone"), file),
	)
	r := NewComposeRunner(src)
	facts, err := r.ListProjectFacts(context.Background())
	if err != nil {
		t.Fatalf("ListProjectFacts: %v", err)
	}
	if len(facts) != 2 {
		t.Fatalf("facts = %d 项, want 2（无 label 容器不归属）", len(facts))
	}
	if facts[0].Project != "aa" || facts[1].Project != "web" {
		t.Fatalf("项目字典序: %q, %q", facts[0].Project, facts[1].Project)
	}
	web := facts[1]
	if !web.Locatable || web.Missing != "" {
		t.Errorf("web locatable = %v missing = %q, want true/空", web.Locatable, web.Missing)
	}
	if len(web.Containers) != 2 || web.Containers[0].Name != "web-app" || web.Containers[1].Name != "web-db" {
		t.Fatalf("web 容器名字典序: %+v", web.Containers)
	}
	if facts[0].Locatable || facts[0].Missing != MissingWorkingDir {
		t.Errorf("aa locatable = %v missing = %q, want false/working_dir", facts[0].Locatable, facts[0].Missing)
	}
}

// ---------- per-project 互斥（I-08 白盒） ----------

func TestTryAcquireBusy(t *testing.T) {
	r := NewComposeRunner(&fakeLogSource{follow: map[string]*LogStream{}, followSend: map[string]chan string{}})
	if !r.tryAcquire("web") {
		t.Fatal("首次获取应成功")
	}
	if r.tryAcquire("web") {
		t.Fatal("进行中再提交应拒绝（DK_018）")
	}
	if !r.tryAcquire("other") {
		t.Fatal("不同项目应并行")
	}
	r.release("web")
	if !r.tryAcquire("web") {
		t.Fatal("释放后应可再获取")
	}
}

// ---------- FollowLogs 项目跟随（A-D12 对账/终因） ----------

func TestFollowLogsProjectGone(t *testing.T) {
	old := projectReconcileInterval
	projectReconcileInterval = 50 * time.Millisecond
	t.Cleanup(func() { projectReconcileInterval = old })

	dir := t.TempDir()
	file := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(file, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	src := &fakeLogSource{follow: map[string]*LogStream{}, followSend: map[string]chan string{}}
	src.setContainers(labeledContainer("c1", "web-app", StatusRunning, "web", dir, file))
	r := NewComposeRunner(src)

	facts, err := r.ListProjectFacts(context.Background())
	if err != nil || len(facts) != 1 {
		t.Fatalf("facts: %v %v", facts, err)
	}
	stream, err := r.FollowLogs(context.Background(), facts[0], 100)
	if err != nil {
		t.Fatalf("FollowLogs: %v", err)
	}
	defer stream.Cancel()

	// 初始子流：投递两行（经合并通道到达；项目级终因由对账决定）。
	src.sendLine(t, "c1", "line-1")
	src.sendLine(t, "c1", "line-2")

	got := readStreamLine(t, stream.Lines)
	if got != "line-1" {
		t.Fatalf("merged line[0] = %q", got)
	}
	got = readStreamLine(t, stream.Lines)
	if got != "line-2" {
		t.Fatalf("merged line[1] = %q", got)
	}

	// 项目消失（label 聚合空）→ 对账判定 project_gone → End。
	src.setContainers()
	select {
	case reason := <-stream.End:
		if reason != ReasonProjectGone {
			t.Fatalf("end reason = %q, want project_gone", reason)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("等待 project_gone 超时")
	}
}

func TestFollowLogsLocateFailed(t *testing.T) {
	old := projectReconcileInterval
	projectReconcileInterval = 50 * time.Millisecond
	t.Cleanup(func() { projectReconcileInterval = old })

	dir := t.TempDir()
	file := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(file, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	src := &fakeLogSource{follow: map[string]*LogStream{}, followSend: map[string]chan string{}}
	src.setContainers(labeledContainer("c1", "web-app", StatusRunning, "web", dir, file))
	r := NewComposeRunner(src)
	facts, _ := r.ListProjectFacts(context.Background())
	stream, err := r.FollowLogs(context.Background(), facts[0], 10)
	if err != nil {
		t.Fatalf("FollowLogs: %v", err)
	}
	defer stream.Cancel()

	// 跟随中配置文件被移除 → 对账 locate_failed。
	if err := os.Remove(file); err != nil {
		t.Fatalf("remove file: %v", err)
	}
	select {
	case reason := <-stream.End:
		if reason != ReasonLocateFailed {
			t.Fatalf("end reason = %q, want locate_failed", reason)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("等待 locate_failed 超时")
	}
}

// readStreamLine 带超时读一行。
func readStreamLine(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case line := <-ch:
		return line
	case <-time.After(3 * time.Second):
		t.Fatal("读行超时")
		return ""
	}
}

// ---------- Logs 项目聚合尾部（容器名序） ----------

func TestComposeLogsAggregation(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(file, []byte("x\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	src := &fakeLogSource{
		follow: map[string]*LogStream{},
		tailLogs: map[string][]string{
			"a": {"from-a"},
			"b": {"from-b-1", "from-b-2"},
		},
	}
	src.setContainers(
		labeledContainer("a", "web-app", StatusRunning, "web", dir, file),
		labeledContainer("b", "web-db", StatusRunning, "web", dir, file),
	)
	r := NewComposeRunner(src)
	facts, _ := r.ListProjectFacts(context.Background())
	lines, err := r.Logs(context.Background(), facts[0], 500)
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	want := []string{"from-a", "from-b-1", "from-b-2"} // 容器名字典序拼接
	if !equalStrings(lines, want) {
		t.Fatalf("聚合行 = %v, want %v", lines, want)
	}
}

// ---------- Run：LookPath 失败归一（无 docker 二进制时；有则跳过——真实
// CLI 交互归 Linux 目标环境冒烟，不在单测执行外部命令） ----------

func TestRunNoBinary(t *testing.T) {
	if _, err := execLookPath(dockerBin); err == nil {
		t.Skip("宿主机有 docker 二进制：真实 CLI 交互归 Linux 冒烟（不在单测执行）")
	}
	r := NewComposeRunner(&fakeLogSource{follow: map[string]*LogStream{}, followSend: map[string]chan string{}})
	fact := ComposeProjectFact{Project: "web", WorkingDir: ".", ConfigFile: "x.yml"}
	err := r.Run(context.Background(), fact, VerbUp)
	var exec *ExecError
	if !errors.As(err, &exec) {
		t.Fatalf("err = %v, want ExecError", err)
	}
	if !strings.Contains(exec.Error(), "compose up") {
		t.Fatalf("err = %v, want verb 语境", err)
	}
}

// execLookPath 测试别名（避免测试直接引 os/exec 与生产 import 重复）。
func execLookPath(name string) (string, error) { return lookPathFor(name) }

// ---------- ExecError（reason 透传形状） ----------

func TestExecErrorMessage(t *testing.T) {
	e := &ExecError{Verb: "image pull", Reason: "network unreachable"}
	if want := "dockerx: image pull failed: network unreachable"; e.Error() != want {
		t.Fatalf("Error() = %q, want %q", e.Error(), want)
	}
	var _ error = e // 接口断言
}
