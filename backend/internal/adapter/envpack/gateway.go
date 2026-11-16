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
	"os/exec"
	"runtime"
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

// Gateway 是 EnvPackGateway 适配器（并发安全：无状态）。
type Gateway struct{}

// NewGateway 构造适配器。
func NewGateway() *Gateway { return &Gateway{} }

// DetectChannel 探测包管理器通道（D-03：apt → dnf → yum 择一；exec.LookPath
// 二进制存在性检查零副作用）。
func (g *Gateway) DetectChannel() Channel {
	if runtime.GOOS != "linux" {
		return Channel{}
	}
	for _, f := range []struct {
		name string
		bin  string
	}{
		{"apt", "apt"}, {"dnf", "dnf"}, {"yum", "yum"},
	} {
		if _, err := exec.LookPath(f.bin); err == nil {
			return Channel{Family: f.name, Available: true}
		}
	}
	return Channel{}
}

// QueryInstalled 查询包安装态与版本（⚠️A7 包名由 service 层目录映射表给出；
// apt：dpkg-query -W -f=${Version}；dnf/yum：rpm -q --qf）。
func (g *Gateway) QueryInstalled(ctx context.Context, family, pkgName string) (string, bool, error) {
	if family == "" {
		return "", false, errors.New("envpack: channel unavailable")
	}
	var name string
	var args []string
	switch family {
	case "apt":
		name, args = "dpkg-query", []string{"-W", "-f=${Version}", pkgName}
	default: // dnf / yum 查询经 rpm 数据库（只读不依赖网络仓库元数据）
		name, args = "rpm", []string{"-q", "--qf", "%{VERSION}-%{RELEASE}", pkgName}
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := runCapture(cctx, name, args)
	if err != nil {
		// 包未安装（dpkg/rpm 查无此包退出码非 0）——如实区分：输出含 "not installed"
		// 或 rpm "not installed" 语义视为未装而非通道错误。
		if strings.Contains(out, "not installed") || strings.Contains(out, "未安装软件包") {
			return "", false, nil
		}
		return "", false, fmt.Errorf("envpack: query %s: %w", pkgName, err)
	}
	v := strings.TrimSpace(out)
	if v == "" {
		return "", false, nil
	}
	return v, true, nil
}

// RefreshIndex 刷新包索引（D-05：apt update / dnf makecache / yum makecache）。
func (g *Gateway) RefreshIndex(ctx context.Context, family string) (string, error) {
	switch family {
	case "apt":
		return g.run(ctx, "apt", "update")
	case "dnf":
		return g.run(ctx, "dnf", "makecache")
	case "yum":
		return g.run(ctx, "yum", "makecache")
	}
	return "", errors.New("envpack: channel unavailable")
}

// Install 安装包（-y 非交互；D-10 超时）。
func (g *Gateway) Install(ctx context.Context, family, pkgName string) (string, error) {
	switch family {
	case "apt":
		// env DEBIAN_FRONTEND=noninteractive 经命令 Env 注入（不进 argv）。
		return g.runEnv(ctx, map[string]string{"DEBIAN_FRONTEND": "noninteractive"},
			"apt", "install", "-y", pkgName)
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
			"apt", "remove", "-y", pkgName)
	case "dnf":
		return g.run(ctx, "dnf", "remove", "-y", pkgName)
	case "yum":
		return g.run(ctx, "yum", "remove", "-y", pkgName)
	}
	return "", errors.New("envpack: channel unavailable")
}

// run/runEnv 执行（字面量参数数组 R-08；stdout+stderr 合并输出）。
func (g *Gateway) run(ctx context.Context, name string, args ...string) (string, error) {
	return g.runEnv(ctx, nil, name, args...)
}

func (g *Gateway) runEnv(ctx context.Context, env map[string]string, name string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, OpTimeout)
	defer cancel()
	return runCaptureEnv(cctx, env, name, args)
}

func runCapture(ctx context.Context, name string, args []string) (string, error) {
	return runCaptureEnv(ctx, nil, name, args)
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
