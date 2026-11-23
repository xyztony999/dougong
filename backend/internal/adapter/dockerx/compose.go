// compose.go 承载 ComposeRunner——`docker compose` CLI 通道（api §4.3.5 契约；
// R-08/ADR-013 全部参数数组执行，禁 shell 拼接）：label 聚合识别（R-13 三键）
// + locatable 定位判定（D-06）+ 动词执行（up/down/restart，per-project 串行
// 互斥 I-08、300s 上限）+ 项目聚合日志（尾部拼接 / 跟随多容器合并订阅 + 5s
// 对账，A-D12）。
package dockerx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

// Compose label 键（官方 label，R-13/D-06；api §3.4 业务常量表）。
const (
	LabelProject     = "com.docker.compose.project"
	LabelWorkingDir  = "com.docker.compose.project.working_dir"
	LabelConfigFiles = "com.docker.compose.project.config_files"
)

// Compose 操作集（compose_action 枚举承载值：up/down/restart 经路径动作段、
// logs 经 GET 端点；D-06）。
const (
	VerbUp      = "up"
	VerbDown    = "down"
	VerbRestart = "restart"
)

// 定位缺失项（DK_015 data.missing 透传属性）。
const (
	MissingWorkingDir = "working_dir"
	MissingConfigFile = "config_file"
)

// ComposeTimeout 单命令等待上限（300s，D-06；超时归一 DK_016，A-D17）。
const ComposeTimeout = 300 * time.Second

// projectReconcileInterval 项目跟随对账周期（var 供测试注入；生产 5s，A-D12）。
var projectReconcileInterval = 5 * time.Second

// dockerBin 是 Compose CLI 二进制名（LookPath 解析，不信任外部传入路径
// ——backend-03 §4）。
const dockerBin = "docker"

// lookPathFor 二进制解析（var 形态便于测试替身；生产=exec.LookPath）。
var lookPathFor = exec.LookPath

// ComposeContainerRef 是项目容器清单条目（api §4.3.1 containers 字段）。
type ComposeContainerRef struct {
	ContainerID string
	Name        string
	State       string // 映射态（与 container 投影同源，BA 附录B.1）
}

// ComposeProjectFact 是 label 聚合项目投影（api §4.3.1 项目投影条目）。
type ComposeProjectFact struct {
	Project    string                // label 聚合键（D-11 不透明引用）
	WorkingDir string                // label 透传
	ConfigFile string                // label 透传（原文，可含逗号多文件）
	Containers []ComposeContainerRef // 项目容器实况
	Locatable  bool                  // 服务端定位判定（D-06）
	Missing    string                // locatable=false 时首个缺失项（A-D9）
}

// LogSource 是 ComposeRunner 的 daemon 通道依赖（进程内组合——api §4.3.5
// 「经 DockerGateway.ListContainers 的 label 聚合」；日志通道复用容器级
// 订阅，实现为 *Gateway）。
type LogSource interface {
	ListContainers(ctx context.Context) ([]ContainerFact, error)
	GetContainerLogs(ctx context.Context, id string, tail int) ([]string, error)
	FollowContainerLogs(ctx context.Context, id string, tail int) (*LogStream, error)
}

// ComposeRunner 是 Compose CLI 编排器（api §4.3.5 契约实现）：定位→组装
// 参数→串行执行→结果归一。per-project 互斥（I-08）：进行中再提交 ErrBusy
// （→DK_018），不同项目并行不受影响。
type ComposeRunner struct {
	src  LogSource
	mu   sync.Mutex
	busy map[string]bool // 进行中项目标记
}

// NewComposeRunner 构造（src 通常为 *Gateway，main 装配注入）。
func NewComposeRunner(src LogSource) *ComposeRunner {
	return &ComposeRunner{src: src, busy: make(map[string]bool)}
}

// ListProjectFacts label 扫描聚合（无 label 容器不归属任何项目，R-13）+
// locatable 判定（工作目录存在且配置文件可读，D-06/A-D9）。项目名/容器名
// 字典序（A-D10）。
func (r *ComposeRunner) ListProjectFacts(ctx context.Context) ([]ComposeProjectFact, error) {
	containers, err := r.src.ListContainers(ctx)
	if err != nil {
		return nil, fmt.Errorf("compose list containers: %w", err)
	}
	index := make(map[string]int)
	facts := make([]ComposeProjectFact, 0, 8)
	for _, c := range containers {
		name := c.Labels[LabelProject]
		if name == "" {
			continue
		}
		i, ok := index[name]
		if !ok {
			facts = append(facts, ComposeProjectFact{
				Project:    name,
				WorkingDir: c.Labels[LabelWorkingDir],
				ConfigFile: c.Labels[LabelConfigFiles],
				Containers: nil,
			})
			i = len(facts) - 1
			index[name] = i
		}
		facts[i].Containers = append(facts[i].Containers, ComposeContainerRef{
			ContainerID: c.ContainerID, Name: c.Name, State: c.State,
		})
	}
	for i := range facts {
		sortContainers(facts[i].Containers)
		facts[i].Missing = locateMissing(facts[i].WorkingDir, facts[i].ConfigFile)
		facts[i].Locatable = facts[i].Missing == ""
	}
	sort.Slice(facts, func(a, b int) bool { return facts[a].Project < facts[b].Project })
	return facts, nil
}

// Run 执行 Compose 动词（docker compose --project-name/--project-directory/
// -f/<verb> 参数数组；up 补 -d 适配同步终态 A-D8；≤300s；失败 stderr 原文
// 透传 D-08/A-D17；同项目进行中 ErrBusy）。
func (r *ComposeRunner) Run(ctx context.Context, fact ComposeProjectFact, verb string) error {
	if verb != VerbUp && verb != VerbDown && verb != VerbRestart {
		return fmt.Errorf("dockerx: unknown compose verb %q", verb)
	}
	if !r.tryAcquire(fact.Project) {
		return fmt.Errorf("%w: project %q", ErrBusy, fact.Project)
	}
	defer r.release(fact.Project)

	bin, err := lookPathFor(dockerBin)
	if err != nil {
		return &ExecError{Verb: "compose " + verb, Reason: err.Error()}
	}
	cctx, cancel := context.WithTimeout(ctx, ComposeTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, bin, composeArgs(fact, verb)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if runErr := cmd.Run(); runErr != nil {
		reason := strings.TrimSpace(stderr.String())
		if reason == "" {
			reason = strings.TrimSpace(stdout.String())
		}
		if reason == "" {
			reason = runErr.Error()
		}
		if cctx.Err() != nil {
			reason = fmt.Sprintf("timeout after %s: %s", ComposeTimeout, reason)
		}
		return &ExecError{Verb: "compose " + verb, Reason: reason}
	}
	return nil
}

// composeArgs 组装 CLI 参数数组（纯函数；config_files 逗号多文件逐个 -f，
// A-D9；up 补 -d，A-D8）。fact 值均来自 daemon label（非用户输入），且各
// 自作为独立 argv 元素——无 shell 语义（R-08）。
//
// -d（--detach）是 up 子命令的选项，不是 docker compose 的全局选项：必须排在
// up 之后。此前写成 `… -d up`，CLI 直接拒绝（unknown shorthand flag: 'd' in -d），
// Compose「启动」自 v0.1.0 起必失败（2026-11-23 以 Compose v5.5.1 实测复现）。
func composeArgs(fact ComposeProjectFact, verb string) []string {
	args := []string{"compose", "--project-name", fact.Project,
		"--project-directory", fact.WorkingDir}
	for _, f := range splitConfigFiles(fact.ConfigFile) {
		args = append(args, "-f", f)
	}
	args = append(args, verb)
	if verb == VerbUp {
		args = append(args, "-d")
	}
	return args
}

// splitConfigFiles 拆分 config_files label（逗号分隔多文件；空白容忍）。
func splitConfigFiles(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// locateMissing 定位判定（D-06/A-D9）：目录缺失→working_dir；任一配置文件
// 不可读→config_file；全部就绪返回空（=locatable）。
func locateMissing(workingDir, configFile string) string {
	if workingDir == "" {
		return MissingWorkingDir
	}
	if info, err := os.Stat(workingDir); err != nil || !info.IsDir() {
		return MissingWorkingDir
	}
	if configFile == "" {
		return MissingConfigFile
	}
	for _, f := range splitConfigFiles(configFile) {
		if info, err := os.Stat(f); err != nil || info.IsDir() {
			return MissingConfigFile
		}
	}
	return ""
}

// tryAcquire per-project 互斥获取（I-08：非阻塞——进行中即拒绝 DK_018）。
func (r *ComposeRunner) tryAcquire(project string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.busy[project] {
		return false
	}
	r.busy[project] = true
	return true
}

// release 释放互斥标记。
func (r *ComposeRunner) release(project string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.busy, project)
}

// Logs 项目聚合尾部日志（D-03：全部容器 tail 行拼接，容器名字典序——
// 确定性聚合；容器在读取间隙被移除（面板外变更）跳过如实聚合不虚构失败）。
func (r *ComposeRunner) Logs(ctx context.Context, fact ComposeProjectFact, tail int) ([]string, error) {
	refs := append([]ComposeContainerRef(nil), fact.Containers...)
	sortContainers(refs)
	var lines []string
	for _, c := range refs {
		ls, err := r.src.GetContainerLogs(ctx, c.ContainerID, tail)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue // 聚合间隙容器已删（R-12 接管哲学：面板外变更如实）
			}
			return nil, err
		}
		lines = append(lines, ls...)
	}
	return lines, nil
}

// sortContainers 容器名字典序。
func sortContainers(refs []ComposeContainerRef) {
	sort.Slice(refs, func(a, b int) bool { return refs[a].Name < refs[b].Name })
}

// FollowLogs 项目聚合跟随（D-03）：初始对全部项目容器 FollowContainerLogs
// （回放 tail+跟随单流，A-D16），泵逐行合并投递；5s 对账（A-D12）：label
// 聚合空→project_gone、locatable=false→locate_failed、ListContainers 失败→
// daemon_lost；容器扰动重订阅仅限 running/paused（tail=0 抑制重复回放；
// 停止容器流自然收束不重订——compose logs -f 等待重启同构语义）。
func (r *ComposeRunner) FollowLogs(ctx context.Context, fact ComposeProjectFact, tail int) (*LogStream, error) {
	sctx, cancel := context.WithCancel(ctx)
	lines := make(chan string, 256)
	end := make(chan string, 1)
	f := &projectFollow{
		src: r.src, project: fact.Project, tail: tail,
		sctx: sctx, lines: lines, end: end,
		subs:   make(map[string]*LogStream),
		subEnd: make(chan string, 64),
	}
	go f.run()
	return &LogStream{Lines: lines, End: end, Cancel: cancel}, nil
}

// projectFollow 是项目跟随会话（生命周期归属=LogStream 句柄，Cancel 收敛）。
type projectFollow struct {
	src     LogSource
	project string
	tail    int

	sctx  context.Context
	lines chan string
	end   chan string

	mu      sync.Mutex
	subs    map[string]*LogStream // 容器 ID → 活跃子流
	subEnd  chan string           // 子流收束通知（容器 ID）
	closing bool
}

// run 对账主循环：初始全量订阅→事件驱动（子流收束/ticker）对账→终因收束。
func (p *projectFollow) run() {
	defer p.shutdown()
	if reason := p.reconcile(true); reason != "" {
		p.finish(reason)
		return
	}
	ticker := time.NewTicker(projectReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.sctx.Done():
			return // 客户端断开：静默收敛，不发 End
		case <-p.subEnd:
			if reason := p.reconcile(false); reason != "" {
				p.finish(reason)
				return
			}
		case <-ticker.C:
			if reason := p.reconcile(false); reason != "" {
				p.finish(reason)
				return
			}
		}
	}
}

// reconcile 对账一轮：label 重扫→终因判定（project_gone/locate_failed/
// daemon_lost）→容器集订阅收敛（初始全量含回放；扰动重订仅 running/paused
// 且 tail=0）。返回非空即终因。
func (p *projectFollow) reconcile(initial bool) string {
	containers, err := p.src.ListContainers(p.sctx)
	if err != nil {
		return ReasonDaemonLost
	}
	var refs []ContainerFact
	for _, c := range containers {
		if c.Labels[LabelProject] == p.project {
			refs = append(refs, c)
		}
	}
	if len(refs) == 0 {
		return ReasonProjectGone
	}
	// locatable 复核（A-D12：跟随中工作目录/配置文件被移除→locate_failed）。
	var workingDir, configFile string
	for _, c := range refs {
		workingDir = c.Labels[LabelWorkingDir]
		configFile = c.Labels[LabelConfigFiles]
		break
	}
	if missing := locateMissing(workingDir, configFile); missing != "" {
		return ReasonLocateFailed
	}
	for _, c := range refs {
		if p.hasSub(c.ContainerID) {
			continue
		}
		subTail := 0 // 重订阅只取新增行（A-D16 注）
		if initial {
			subTail = p.tail // 初始订阅承载回放（Follow+Tail 单流）
		} else if c.State != StatusRunning && c.State != StatusPaused {
			continue // 停止/过渡态容器不重订（等待重启由 ticker 再对账）
		}
		st, subErr := p.src.FollowContainerLogs(p.sctx, c.ContainerID, subTail)
		if subErr != nil {
			if errors.Is(subErr, ErrNotFound) {
				continue // 对账间隙已删：下轮对账收敛
			}
			return ReasonDaemonLost
		}
		p.addSub(c.ContainerID, st)
		go p.forward(c.ContainerID, st)
	}
	return ""
}

// forward 子流转发泵：逐行合并投递→子流收束→摘除登记+通知对账。
func (p *projectFollow) forward(id string, st *LogStream) {
	for line := range st.Lines {
		select {
		case p.lines <- line:
		case <-p.sctx.Done():
			st.Cancel()
			return
		}
	}
	p.removeSub(id)
	select {
	case p.subEnd <- id:
	case <-p.sctx.Done():
	}
}

// addSub 登记子流（容器 ID 键——子流与容器一对一）。
func (p *projectFollow) addSub(id string, st *LogStream) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closing {
		st.Cancel()
		return
	}
	p.subs[id] = st
}

// hasSub 查询容器是否活跃订阅。
func (p *projectFollow) hasSub(containerID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.subs[containerID]
	return ok
}

// removeSub 摘除容器订阅登记。
func (p *projectFollow) removeSub(containerID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.subs, containerID)
}

// finish 终因收束：End 投递→关闭行通道（服务端随后关闭连接，D-03）。
func (p *projectFollow) finish(reason string) {
	p.end <- reason
	close(p.lines)
}

// shutdown 取消全部子流并清理（客户端断开/终因路径共用；run defer 收敛）。
func (p *projectFollow) shutdown() {
	p.mu.Lock()
	p.closing = true
	subs := make([]*LogStream, 0, len(p.subs))
	for _, st := range p.subs {
		subs = append(subs, st)
	}
	p.subs = nil
	p.mu.Unlock()
	for _, st := range subs {
		st.Cancel()
	}
}
