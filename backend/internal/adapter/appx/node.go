// node.go —— appx 的 Node 通道（ADR-033 D-16/D-18：npm install 依赖安装与
// node/npm 绝对路径解析）。命令字面量参数数组（R-08），不经 shell。
package appx

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// npmCommand 返回执行 npm 的命令与前置参数。Windows 上 PATH 解析到的是
// npm.cmd（须经 shell 才能执行，R-08 禁止）——改为经 node 直接运行其包裹的
// npm-cli.js；其它平台直接执行 npm。
func npmCommand() (name string, pre []string, err error) {
	npm, err := exec.LookPath("npm")
	if err != nil {
		return "", nil, fmt.Errorf("appx: npm 不在 PATH 中: %w", err)
	}
	if strings.EqualFold(filepath.Ext(npm), ".cmd") {
		cli := filepath.Join(filepath.Dir(npm), "node_modules", "npm", "bin", "npm-cli.js")
		node, nerr := exec.LookPath("node")
		if nerr == nil {
			if _, serr := os.Stat(cli); serr == nil {
				return node, []string{cli}, nil
			}
		}
	}
	return npm, nil, nil
}

// NpmInstall 在项目目录安装依赖（npm install；ADR-033 D-18）。package.json
// 缺失由调用方跳过；失败原文透传供任务输出。
func (g *Gateway) NpmInstall(ctx context.Context, dir string, timeout time.Duration) (string, error) {
	name, pre, err := npmCommand()
	if err != nil {
		return "", err
	}
	return run(ctx, dir, timeout, name, append(pre, "install", "--no-audit", "--no-fund")...)
}

// ResolveBins 解析 node 与 npm 的绝对路径（D-16：启动器不依赖 PM2 守护进程
// 的 PATH——守护进程开机时由 systemd 拉起，其 PATH 未必含 node 安装位置）。
// 任一缺失返回错误（调用方按 APP_015 分层提示）。
func (g *Gateway) ResolveBins(_ context.Context) (nodeBin, npmBin string, err error) {
	nodeBin, err = exec.LookPath("node")
	if err != nil {
		return "", "", fmt.Errorf("appx: node 不在 PATH 中: %w", err)
	}
	npmBin, err = exec.LookPath("npm")
	if err != nil {
		return "", "", fmt.Errorf("appx: npm 不在 PATH 中: %w", err)
	}
	return nodeBin, npmBin, nil
}
