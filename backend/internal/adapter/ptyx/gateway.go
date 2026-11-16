// Package ptyx 是本机 PTY 网关适配器（api-terminal.md §4.1.5 PtyGateway 契约，
// creack/pty 承载；main 装配注入 service/terminal）。
//
// spawn 一律参数数组（R-08 禁 shell 拼接）：
//   - panel_run_user：以面板运行用户直启默认登录 shell（/etc/passwd 当前 uid
//     条目 → $SHELL → /bin/sh 兜底；argv0 加 "-" 前缀取登录 shell 语义）；
//   - specified_system_user：su 语义代启指定用户登录 shell（exec 数组
//     ["su","-",<username>]；不代输密码——非 root 运行时 su 的口令提示由
//     操作者在终端内自助输入，D1 落定）。
//
// Windows 开发机为退化实现（pty_windows.go，ErrUnsupportedPlatform）——目标
// 环境 Linux 功能完整，实联冒烟归 Linux（fslocal A-F7 同口径）。
package ptyx

import (
	"context"
	"os/user"
)

// Gateway 实现 terminal.PtyGateway。
type Gateway struct{}

// New 构造网关（无状态；PTY 会话资源由各 handle 自持）。
func New() *Gateway { return &Gateway{} }

// CurrentUser 解析面板运行用户名（A-T3：ready 帧 runUser 权威值）。
func (g *Gateway) CurrentUser(ctx context.Context) (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return u.Username, nil
}
