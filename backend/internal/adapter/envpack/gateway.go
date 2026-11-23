// Package envpack 是环境管理（env-mgmt）包管理器通道适配器（api-env-mgmt.md
// §4.1.5 EnvPackGateway 契约）：apt（Debian 系）/ dnf·yum（RHEL 系）双系探测
// 择一（D-03，exec.LookPath 非外呼非 shell）与包操作执行（R-08 字面量参数
// 数组；D-10 超时 PKG_OP_TIMEOUT）。
//
// Windows/非 Linux 退化：通道不可用（ENV_001 语义）——变更能力如实拒绝，
// 不虚构（与 terminal ptyx/service-mgmt sysd 退化口径一致，实联归 Linux 冒烟）。
package envpack

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"time"
)

// OpTimeout 单包操作命令超时上限（D-10，api §3.4 PKG_OP_TIMEOUT）。
const OpTimeout = 10 * time.Minute

// Channel 是通道探测结论（package_manager_family；Available=false 时 Family
// 为空——「探测不可用」如实区分于「未装」，D-01）。
type Channel struct {
	Family    string // apt | dnf | yum | ""
	Available bool
}

// RunResult 是命令执行结果（输出尾部原文 + 错误；输出由调用方截尾采集）。
type RunResult struct {
	Output string
}

// Gateway 是 EnvPackGateway 适配器（并发安全：lookPath/runCmd 注入函数只读）。
type Gateway struct {
	// lookPath 二进制存在性探测注入点（生产 exec.LookPath；测试注入恒失败
	// 探测使「通道不可用」断言环境无关——CI Linux runner 预装 apt 会击穿
	// 「默认通道不存在」假设，沿 TestLogTail Options 注入先例）。
	lookPath func(string) (string, error)
	// runCmd 命令执行注入点（生产 runCaptureEnv 字面量参数数组 R-08；测试
	// 注入伪执行器承载 dpkg-query/rpm 输出分支断言，环境无关——CI runner
	// 包数据库状态不参与测试，第 5 起环境耦合预防）。
	runCmd func(ctx context.Context, env map[string]string, name string, args []string) (string, error)

	// 仓库代配依赖注入点（vendorrepo.go，D-06 v1.1）：os-release 读取/源文件
	// 写入/目录创建/公钥下载——测试全伪化（支持矩阵与文件内容断言环境无关）。
	readFile  func(string) ([]byte, error)
	writeFile func(string, []byte, fs.FileMode) error
	mkdirAll  func(string, fs.FileMode) error
	httpGet   func(ctx context.Context, url string) ([]byte, error)

	// npmRegistry npm 源镜像（ADR-033：Node 环境随装 PM2 用；main 装配注入
	// cfg.Tools.NpmRegistry，空=npm 默认源）。
	npmRegistry string
}

// SetNpmRegistry 设置 npm 源镜像（空=官方源）。
func (g *Gateway) SetNpmRegistry(r string) { g.npmRegistry = r }

// NewGateway 构造适配器（生产探测与执行）。
func NewGateway() *Gateway { return newGateway() }

// NewGatewayWithProbe 以自定义探测函数构造（probe 传 nil 回退生产探测）。
func NewGatewayWithProbe(probe func(string) (string, error)) *Gateway {
	g := newGateway()
	if probe != nil {
		g.lookPath = probe
	}
	return g
}

// NewGatewayWithRunners 以自定义探测与执行函数构造（service 层集成测试注入用；
// nil 参数回退生产实现）。
func NewGatewayWithRunners(probe func(string) (string, error),
	run func(ctx context.Context, env map[string]string, name string, args []string) (string, error)) *Gateway {
	g := newGateway()
	if probe != nil {
		g.lookPath = probe
	}
	if run != nil {
		g.runCmd = run
	}
	return g
}

// VendorIO 仓库代配文件/HTTP 依赖注入组（v1.3 Node 增量：跨包 service 层
// 集成测试注入用——envmgmt worker 链测试经此伪化 os-release/源文件写入/
// 公钥下载，零 runner 实况依赖）。nil 字段回退生产实现。
type VendorIO struct {
	ReadFile  func(string) ([]byte, error)
	WriteFile func(string, []byte, fs.FileMode) error
	MkdirAll  func(string, fs.FileMode) error
	HTTPGet   func(ctx context.Context, url string) ([]byte, error)
}

// NewGatewayWithVendorIO 以自定义探测/执行/代配 IO 构造（nil 参数回退生产）。
func NewGatewayWithVendorIO(probe func(string) (string, error),
	run func(ctx context.Context, env map[string]string, name string, args []string) (string, error),
	vio *VendorIO) *Gateway {
	g := NewGatewayWithRunners(probe, run)
	if vio != nil {
		if vio.ReadFile != nil {
			g.readFile = vio.ReadFile
		}
		if vio.WriteFile != nil {
			g.writeFile = vio.WriteFile
		}
		if vio.MkdirAll != nil {
			g.mkdirAll = vio.MkdirAll
		}
		if vio.HTTPGet != nil {
			g.httpGet = vio.HTTPGet
		}
	}
	return g
}

// newGateway 生产默认装配（全部依赖显式落位；测试经字段覆写/WithRunners 注入）。
func newGateway() *Gateway {
	return &Gateway{
		lookPath: exec.LookPath,
		runCmd:   runCaptureEnv,
		readFile: os.ReadFile,
		writeFile: func(path string, data []byte, mode fs.FileMode) error {
			return os.WriteFile(path, data, mode)
		},
		mkdirAll: os.MkdirAll,
		httpGet:  fetchURL,
	}
}

// DetectChannel 探测包管理器通道（D-03：apt → dnf → yum 择一；LookPath
// 二进制存在性检查零副作用）。非 Linux 退化（ENV_001）由探测本身承载——
// Windows 等环境 LookPath 三连落空即 Channel{}，不做 GOOS 前置短路（短路会
// 使注入探测失效、测试环境相关——v1.1 修正；生产语义不变）。
func (g *Gateway) DetectChannel() Channel {
	for _, f := range []struct {
		name string
		bin  string
	}{
		{"apt", "apt"}, {"dnf", "dnf"}, {"yum", "yum"},
	} {
		if _, err := g.lookPath(f.bin); err == nil {
			return Channel{Family: f.name, Available: true}
		}
	}
	return Channel{}
}

// QueryInstalled 查询包安装态与版本（⚠️A7 包名由 service 层目录映射候选数组
// 逐个给出；判定口径 D-14——数据字段优先，不依赖本地化消息文本）。
//
// apt：dpkg-query -W -f='${Status} ${Version}'——Status 为包数据库数据字段
// （install/hold ok installed=已装；deinstall ok config-files、unknown ok
// not-installed=未装），命令 Env 注入 LC_ALL=C 消除其余输出本地化影响（沿
// DEBIAN_FRONTEND 注入先例，不进 argv）；退出码非 0 且输出含
// "no packages found matching"=包名完全不在数据库（未装，非通道错误——
// W1 实机缺陷 2 根因：该形态曾被误归错误致 UI「探测不可用」）。
// dnf/yum：rpm -q --qf（只读不依赖网络仓库元数据）；退出码 1 且输出含
// "not installed"=未装；其余非 0=通道错误。
func (g *Gateway) QueryInstalled(ctx context.Context, family, pkgName string) (string, bool, error) {
	if family == "" {
		return "", false, errors.New("envpack: channel unavailable")
	}
	var name string
	var args []string
	switch family {
	case "apt":
		name, args = "dpkg-query", []string{"-W", "-f=${Status} ${Version}", pkgName}
	default: // dnf / yum 查询经 rpm 数据库
		name, args = "rpm", []string{"-q", "--qf", "%{VERSION}-%{RELEASE}", pkgName}
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := g.runCmd(cctx, map[string]string{"LC_ALL": "C"}, name, args)
	if err != nil {
		switch {
		case family == "apt" && strings.Contains(out, "no packages found matching"):
			return "", false, nil // 包名不在 dpkg 数据库=未装（D-14）
		case family != "apt" && strings.Contains(out, "not installed"):
			return "", false, nil // rpm 未装语义
		}
		return "", false, fmt.Errorf("envpack: query %s: %w", pkgName, err)
	}
	if family == "apt" {
		// 状态字段判定：install/hold ok installed 前缀（数据字段，locale 无关）。
		for _, p := range []string{"install ok installed", "hold ok installed"} {
			if strings.HasPrefix(out, p) {
				return strings.TrimSpace(strings.TrimPrefix(out, p)), true, nil
			}
		}
		return "", false, nil // deinstall ok config-files / unknown ok not-installed / 空=未装
	}
	v := strings.TrimSpace(out)
	if v == "" {
		return "", false, nil
	}
	return v, true, nil
}

// BinaryVersion 运行时二进制版本查询（v1.3 Node 增量，prd FR-08②：版本经
// `node --version` 参数数组 R-08；二进制不存在=错误如实，调用方 version=null
// 不虚构）。bin 经 lookPath 解析（不信任外部传入可执行路径，backend-03 §4）。
func (g *Gateway) BinaryVersion(ctx context.Context, bin string) (string, error) {
	if _, err := g.lookPath(bin); err != nil {
		return "", fmt.Errorf("envpack: binary %s not found: %w", bin, err)
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := g.runCmd(cctx, nil, bin, []string{"--version"})
	if err != nil {
		return "", fmt.Errorf("envpack: %s --version: %w", bin, err)
	}
	return strings.TrimSpace(out), nil
}

// ProbeVenvSupport 探测 venv 支撑能力位（v1.4 ⚠️A18，FR-09②：python3 -m
// venv --help 退出 0=有 venv 模块；Debian 系需 python3-venv 单独包——缺失时
// 命令失败如实返回 false）。python3 缺席=错误（调用方分层呈现 missing）。
func (g *Gateway) ProbeVenvSupport(ctx context.Context) (bool, error) {
	if _, err := g.lookPath("python3"); err != nil {
		return false, fmt.Errorf("envpack: python3 not found: %w", err)
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := g.runCmd(cctx, nil, "python3", []string{"-m", "venv", "--help"}); err != nil {
		return false, nil // python3 在而 venv 模块不可用=能力位 false（非通道错误）
	}
	return true, nil
}

// ProbePipAvailable 探测 pip 能力位（v1.4 ⚠️A18：python3 -m pip --version
// 退出 0=可用；不依赖 pip3 独立可执行名）。
func (g *Gateway) ProbePipAvailable(ctx context.Context) (bool, error) {
	if _, err := g.lookPath("python3"); err != nil {
		return false, fmt.Errorf("envpack: python3 not found: %w", err)
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := g.runCmd(cctx, nil, "python3", []string{"-m", "pip", "--version"}); err != nil {
		return false, nil // pip 不可用=能力位 false（非通道错误）
	}
	return true, nil
}

// ProbeNpmAvailable 探测 npm 可用性（ADR-033：Node 项目依赖安装通道）。
// npm 缺席或不可执行=能力位 false（非通道错误）。
func (g *Gateway) ProbeNpmAvailable(ctx context.Context) (bool, error) {
	if _, err := g.lookPath("npm"); err != nil {
		return false, nil
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := g.runCmd(cctx, nil, "npm", []string{"--version"}); err != nil {
		return false, nil
	}
	return true, nil
}

// ProbePm2Available 探测 PM2 可用性（ADR-033 D-16）。仅做存在性探测，不执行
// pm2 命令——实测任何 pm2 命令在守护进程缺席时都会自动拉起守护进程，就绪度
// 探测不应有此副作用。DOUGONG_PM2_BIN 覆盖时以该路径判定（开发机隔离验证）。
func (g *Gateway) ProbePm2Available(ctx context.Context) (bool, error) {
	if b := os.Getenv("DOUGONG_PM2_BIN"); b != "" {
		if _, err := os.Stat(b); err == nil {
			return true, nil
		}
		return false, nil
	}
	if _, err := g.lookPath("pm2"); err != nil {
		return false, nil
	}
	return true, nil
}

// NpmInstallGlobal 全局安装 npm 包（ADR-033：Node 环境随装 PM2；R-08 参数
// 数组；源镜像经 SetNpmRegistry 注入）。
func (g *Gateway) NpmInstallGlobal(ctx context.Context, pkg string) (string, error) {
	args := []string{"install", "-g", pkg, "--no-audit", "--no-fund"}
	if g.npmRegistry != "" {
		args = append(args, "--registry", g.npmRegistry)
	}
	return g.run(ctx, "npm", args...)
}

// Pm2Startup 注册 PM2 开机自启（ADR-033：Linux systemd 生成 pm2 启动单元，
// 开机时恢复已保存的进程列表）。非 Linux / 无 systemd 如实失败，调用方
// best-effort 记录不阻断环境安装终态。
func (g *Gateway) Pm2Startup(ctx context.Context) (string, error) {
	return g.run(ctx, "pm2", "startup", "systemd")
}

// RefreshIndex 刷新包索引（D-05：apt-get update / dnf makecache / yum makecache；
// 脚本场景用 apt-get——apt 自警无稳定 CLI 接口）。
func (g *Gateway) RefreshIndex(ctx context.Context, family string) (string, error) {
	switch family {
	case "apt":
		return g.run(ctx, "apt-get", "update")
	case "dnf":
		return g.run(ctx, "dnf", "makecache")
	case "yum":
		return g.run(ctx, "yum", "makecache")
	}
	return "", errors.New("envpack: channel unavailable")
}

// Install 安装/升级包（脚本场景用 apt-get；D-10 超时）。
//
// apt 系无人值守三保险（v0.1.3 实机教训：升级 nginx.org 1.30 时 dpkg 对
// /etc/nginx/nginx.conf 弹交互式 conffile Y/N 提示，-y 与 DEBIAN_FRONTEND 均
// 不代答 → EOF on stdin → 包卡半配置）：
//  1. DEBIAN_FRONTEND=noninteractive（Env 注入不进 argv）压掉大部分交互；
//  2. --force-confdef：未被本地修改的 conffile 才取发行版新版；
//  3. --force-confold：本地已改的 conffile 保持现版，新版落 .dpkg-dist 旁置
//     供管理员事后 diff——无人值守升级绝不静默覆盖生产配置。
func (g *Gateway) Install(ctx context.Context, family, pkgName string) (string, error) {
	switch family {
	case "apt":
		return g.runEnv(ctx, map[string]string{"DEBIAN_FRONTEND": "noninteractive"},
			"apt-get", "-y",
			"-o", "Dpkg::Options::=--force-confdef",
			"-o", "Dpkg::Options::=--force-confold",
			"install", pkgName)
	case "dnf":
		return g.run(ctx, "dnf", "install", "-y", pkgName)
	case "yum":
		return g.run(ctx, "yum", "install", "-y", pkgName)
	}
	return "", errors.New("envpack: channel unavailable")
}

// Remove 卸载包（保留配置与数据，⚠️A6：无 purge 选项）。
func (g *Gateway) Remove(ctx context.Context, family, pkgName string) (string, error) {
	switch family {
	case "apt":
		return g.runEnv(ctx, map[string]string{"DEBIAN_FRONTEND": "noninteractive"},
			"apt-get", "remove", "-y", pkgName)
	case "dnf":
		return g.run(ctx, "dnf", "remove", "-y", pkgName)
	case "yum":
		return g.run(ctx, "yum", "remove", "-y", pkgName)
	}
	return "", errors.New("envpack: channel unavailable")
}

// run/runEnv 执行（字面量参数数组 R-08；stdout+stderr 合并输出；经 runCmd
// 注入点路由——生产实现 runCaptureEnv）。
func (g *Gateway) run(ctx context.Context, name string, args ...string) (string, error) {
	return g.runEnv(ctx, nil, name, args...)
}

func (g *Gateway) runEnv(ctx context.Context, env map[string]string, name string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, OpTimeout)
	defer cancel()
	return g.runCmd(cctx, env, name, args)
}

func runCaptureEnv(ctx context.Context, env map[string]string, name string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...) // R-08：参数数组，不经 shell
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if len(env) > 0 {
		e := cmd.Environ()
		for k, v := range env {
			e = append(e, k+"="+v)
		}
		cmd.Env = e
	}
	err := cmd.Run()
	out := buf.String()
	if err != nil {
		if ctx.Err() != nil {
			return out, fmt.Errorf("envpack: %s: timeout: %w (cause: %v)", name, ctx.Err(), err)
		}
		return out, fmt.Errorf("envpack: %s: %w", name, err)
	}
	return out, nil
}
