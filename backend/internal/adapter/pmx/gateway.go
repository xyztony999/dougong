// Package pmx 是应用管理 Node 运行时的 PM2 进程管理适配层（ADR-033，
// api-app-mgmt.md v1.2 D-16~D-18）。全部外部命令字面量参数数组（R-08），
// 不经 shell；PM2 可执行文件与数据目录可注入（开发机隔离验证与测试用），
// 生产默认经 PATH 解析 pm2、数据目录为当前用户主目录下的 .pm2。
//
// 实测口径（PM2 7.1.2，2026-11-23 隔离探测）：jlist 返回纯 JSON；save 记录
// 全部进程及其当时状态；resurrect 按保存时状态恢复（保存前已停止的仍停止）。
// 因此开机自启状态直接读保存文件判定，不经 pm2 命令——任何 pm2 命令在守护
// 进程缺席时都会自动拉起守护进程（探测实证），查询不应有此副作用。
package pmx

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

//go:embed launcher.js
var launcherFS embed.FS

const (
	// OpTimeout 进程操作与保存的超时（npm install 不在本层，归 appx）。
	OpTimeout = 120 * time.Second
	// ListTimeout 状态清单查询超时。
	ListTimeout = 30 * time.Second
	// LogTimeout 日志尾部读取超时。
	LogTimeout = 10 * time.Second
	// LauncherName 启动器脚本文件名（落面板数据目录，D-16）。
	LauncherName = "pm2-launcher.js"
)

// Proc 是 PM2 进程实况投影（jlist 收窄：只取面板呈现与寻址所需字段；
// status 原值透传不翻译——online/stopped/errored 等由前端字典映射）。
type Proc struct {
	Name        string
	Status      string
	Pid         int
	Restarts    int
	MemoryBytes int64
	UptimeMs    int64
	OutLogPath  string
	ErrLogPath  string
}

// StartSpec 注册 PM2 进程的参数（执行目标=面板启动器脚本，D-16）。
type StartSpec struct {
	Name     string
	Cwd      string
	Launcher string
	Args     []string // 启动器参数：项目目录/模式/node 路径/npm 路径/file|script/目标
}

// Gateway 是 PM2 通道（零状态；并发安全）。
type Gateway struct {
	bin     string // pm2 可执行文件；为 JS 文件时配 nodeBin 解释执行
	nodeBin string // bin 为 JS 文件时的 node 解释器（开发机验证用）
	home    string // PM2_HOME（子进程环境与保存文件定位）
}

// Option 构造选项。
type Option func(*Gateway)

// WithPM2Bin 覆盖 pm2 可执行文件（可为 JS 文件，配 WithNodeBin）。
func WithPM2Bin(p string) Option { return func(g *Gateway) { g.bin = p } }

// WithNodeBin 指定解释执行 bin 的 node（bin 为 JS 文件时使用）。
func WithNodeBin(p string) Option { return func(g *Gateway) { g.nodeBin = p } }

// WithHome 覆盖 PM2 数据目录（隔离验证与测试）。
func WithHome(dir string) Option { return func(g *Gateway) { g.home = dir } }

// New 构造适配器。缺省：PM2_HOME 取 DOUGONG_PM2_HOME 或 ~/.pm2；pm2 取
// DOUGONG_PM2_BIN（配 DOUGONG_NODE_BIN 解释执行）或 PATH 中的 pm2。
// 解析不到不报错——就绪度探测如实呈现不可用，命令调用时给出明确错误。
func New(opts ...Option) *Gateway {
	g := &Gateway{}
	g.home = os.Getenv("DOUGONG_PM2_HOME")
	if g.home == "" {
		if hd, err := os.UserHomeDir(); err == nil {
			g.home = filepath.Join(hd, ".pm2")
		}
	}
	if b := os.Getenv("DOUGONG_PM2_BIN"); b != "" {
		g.bin = b
		g.nodeBin = os.Getenv("DOUGONG_NODE_BIN")
	} else if p, err := exec.LookPath("pm2"); err == nil {
		g.bin = p
	}
	for _, o := range opts {
		o(g)
	}
	return g
}

// Home 返回 PM2 数据目录（保存文件与日志定位用）。
func (g *Gateway) Home() string { return g.home }

// Available 判定 pm2 可执行文件是否已解析到（就绪度探测用）。
func (g *Gateway) Available() bool { return g.bin != "" }

// run 执行 pm2 命令（R-08 参数数组；PM2_HOME 经子进程环境注入）。
func (g *Gateway) run(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
	if g.bin == "" {
		return "", fmt.Errorf("pmx: pm2 不可用（未找到可执行文件；请经环境管理安装 Node 环境）")
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	name, argv := g.bin, args
	if g.nodeBin != "" {
		name, argv = g.nodeBin, append([]string{g.bin}, args...)
	}
	cmd := exec.CommandContext(cctx, name, argv...)
	cmd.Env = append(os.Environ(), "PM2_HOME="+g.home)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return buf.String(), fmt.Errorf("pmx: %s %v: %w", name, argv, err)
	}
	return buf.String(), nil
}

// buildStartArgs 组装注册参数（纯函数；-- 之后为启动器参数）。
func buildStartArgs(spec StartSpec) []string {
	args := []string{"start", spec.Launcher, "--name", spec.Name, "--cwd", spec.Cwd, "--"}
	return append(args, spec.Args...)
}

// Start 注册并启动 PM2 进程（执行目标=启动器脚本）。同名进程已存在时 pm2
// 如实报错，由 service 层按幂等语义处理（已停止进程用 StartByName）。
func (g *Gateway) Start(ctx context.Context, spec StartSpec) (string, error) {
	return g.run(ctx, OpTimeout, buildStartArgs(spec)...)
}

// StartByName 启动已注册的（停止态）进程。
func (g *Gateway) StartByName(ctx context.Context, name string) (string, error) {
	return g.run(ctx, OpTimeout, "start", name)
}

// Stop 停止进程（保留注册，PM2 不自行拉起）。
func (g *Gateway) Stop(ctx context.Context, name string) (string, error) {
	return g.run(ctx, OpTimeout, "stop", name)
}

// Restart 重启进程。
func (g *Gateway) Restart(ctx context.Context, name string) (string, error) {
	return g.run(ctx, OpTimeout, "restart", name)
}

// Delete 删除进程注册。
func (g *Gateway) Delete(ctx context.Context, name string) (string, error) {
	return g.run(ctx, OpTimeout, "delete", name)
}

// Save 保存进程列表（开机恢复依据；仅创建/删除/切换开机自启时调用，D-17）。
func (g *Gateway) Save(ctx context.Context) (string, error) {
	return g.run(ctx, OpTimeout, "save")
}

// jlistEntry 是 pm2 jlist 单条目的收窄形状。
type jlistEntry struct {
	Name string `json:"name"`
	Pid  int    `json:"pid"`
	Env  struct {
		Status      string `json:"status"`
		RestartTime int    `json:"restart_time"`
		PmUptime    int64  `json:"pm_uptime"`
		OutLogPath  string `json:"pm_out_log_path"`
		ErrLogPath  string `json:"pm_err_log_path"`
	} `json:"pm2_env"`
	Monit struct {
		Memory int64 `json:"memory"`
	} `json:"monit"`
}

// parseJList 解析 jlist 输出（纯函数；uptime 由 pm_uptime 起始时刻推算，
// 非 online 态为 0）。
func parseJList(data []byte, now time.Time) ([]Proc, error) {
	var entries []jlistEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("pmx: jlist 解析失败: %w", err)
	}
	out := make([]Proc, 0, len(entries))
	for _, e := range entries {
		p := Proc{Name: e.Name, Status: e.Env.Status, Pid: e.Pid,
			Restarts: e.Env.RestartTime, MemoryBytes: e.Monit.Memory,
			OutLogPath: e.Env.OutLogPath, ErrLogPath: e.Env.ErrLogPath}
		if e.Env.Status == "online" && e.Env.PmUptime > 0 {
			p.UptimeMs = now.UnixMilli() - e.Env.PmUptime
			if p.UptimeMs < 0 {
				p.UptimeMs = 0
			}
		}
		out = append(out, p)
	}
	return out, nil
}

// List 读取进程实况清单。
func (g *Gateway) List(ctx context.Context) ([]Proc, error) {
	out, err := g.run(ctx, ListTimeout, "jlist")
	if err != nil {
		return nil, err
	}
	return parseJList([]byte(out), time.Now())
}

// SavedStatus 读保存文件中某进程的保存时状态（开机自启判定，D-17）。
// 返回（状态, 是否存在）；文件不存在=尚未保存过，视为未开启。不经 pm2 命令。
func (g *Gateway) SavedStatus(name string) (string, bool, error) {
	data, err := os.ReadFile(filepath.Join(g.home, "dump.pm2"))
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("pmx: 读取保存文件失败: %w", err)
	}
	var entries []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return "", false, fmt.Errorf("pmx: 保存文件解析失败: %w", err)
	}
	for _, e := range entries {
		if e.Name == name {
			return e.Status, true, nil
		}
	}
	return "", false, nil
}

// tailLines 取文本尾部 n 行（纯函数；空行保留，末尾换行不产生空行）。
func tailLines(text string, n int) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

// Tail 读取 PM2 日志文件尾部（out 与 error 两路合并；error 行加「[错误]」
// 标记以便区分，D-16 日志口径）。文件不存在=该路无输出，跳过不报错。
func (g *Gateway) Tail(ctx context.Context, outPath, errPath string, lines int) ([]string, error) {
	cctx, cancel := context.WithTimeout(ctx, LogTimeout)
	defer cancel()
	read := func(p string) ([]string, error) {
		if p == "" {
			return nil, nil
		}
		data, err := os.ReadFile(p)
		if os.IsNotExist(err) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("pmx: 读取日志失败: %w", err)
		}
		return tailLines(string(data), lines), nil
	}
	var out []string
	if cctx.Err() != nil {
		return nil, cctx.Err()
	}
	o, err := read(outPath)
	if err != nil {
		return nil, err
	}
	out = append(out, o...)
	e, err := read(errPath)
	if err != nil {
		return nil, err
	}
	for _, l := range e {
		out = append(out, "[错误] "+l)
	}
	return out, nil
}

// EnsureLauncher 把内嵌启动器脚本落到指定目录（内容不一致时覆盖；返回路径）。
// 启动器是面板资产而非用户文件，覆盖安全。
func EnsureLauncher(dir string) (string, error) {
	data, err := launcherFS.ReadFile("launcher.js")
	if err != nil {
		return "", fmt.Errorf("pmx: 读取内嵌启动器失败: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("pmx: 创建启动器目录失败: %w", err)
	}
	p := filepath.Join(dir, LauncherName)
	if old, err := os.ReadFile(p); err == nil && bytes.Equal(old, data) {
		return p, nil
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return "", fmt.Errorf("pmx: 写入启动器失败: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("pmx: 落位启动器失败: %w", err)
	}
	return p, nil
}
