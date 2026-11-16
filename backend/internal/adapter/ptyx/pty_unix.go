//go:build !windows

package ptyx

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"syscall"

	"github.com/creack/pty"
	terminalsvc "github.com/dougong/dougong/internal/service/terminal"
)

// Spawn 创建 PTY 并启动登录 shell（参数数组，R-08）。StartWithSize 以
// Setsid+Setctty 使子进程自成会话/进程组（与面板信号隔离；进程组终止以
// kill(-pid) 触达整组，I-02）。
func (g *Gateway) Spawn(ctx context.Context, identity terminalsvc.RunIdentity) (terminalsvc.PtyHandle, error) {
	cmd, err := buildCommand(ctx, identity)
	if err != nil {
		return nil, err
	}
	if os.Getenv("TERM") == "" {
		cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	}
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: terminalsvc.DefaultCols, Rows: terminalsvc.DefaultRows})
	if err != nil {
		return nil, fmt.Errorf("ptyx spawn %s: %w", identity.Source, err)
	}
	return &handle{ptmx: ptmx, cmd: cmd}, nil
}

// buildCommand 按运行身份构造命令（字面量参数数组；用户名经 TERM_005 形态
// 校验与 TERM_002 存在性校验后作为独立 argv 元素传入）。
func buildCommand(ctx context.Context, identity terminalsvc.RunIdentity) (*exec.Cmd, error) {
	switch identity.Source {
	case terminalsvc.IdentitySourceSpecified:
		return exec.CommandContext(ctx, "su", "-", identity.SystemUsername), nil
	case terminalsvc.IdentitySourcePanel:
		shell, err := panelLoginShell()
		if err != nil {
			return nil, err
		}
		cmd := exec.CommandContext(ctx, shell)
		cmd.Args = []string{"-" + path.Base(shell)} // 登录 shell 语义（argv0 前缀 -）
		return cmd, nil
	default:
		return nil, fmt.Errorf("ptyx: unknown identity source %q", identity.Source)
	}
}

// panelLoginShell 解析面板运行用户默认登录 shell：/etc/passwd 当前 uid 条目
// → $SHELL → /bin/sh 兜底。
func panelLoginShell() (string, error) {
	uid := strconv.Itoa(syscall.Getuid())
	shell := ""
	f, err := os.Open("/etc/passwd") //nolint:gosec // 静态系统文件路径
	if err == nil {
		defer func() { _ = f.Close() }()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			fields := strings.Split(sc.Text(), ":")
			if len(fields) >= 7 && fields[2] == uid {
				shell = fields[6]
				break
			}
		}
	}
	if strings.TrimSpace(shell) == "" {
		shell = os.Getenv("SHELL")
	}
	if strings.TrimSpace(shell) == "" {
		shell = "/bin/sh"
	}
	return shell, nil
}

// handle 实现 terminal.PtyHandle（master fd + 子进程）。
type handle struct {
	ptmx *os.File
	cmd  *exec.Cmd
}

// Read 读取 PTY 输出（EOF=子进程退出）。
func (h *handle) Read(p []byte) (int, error) { return h.ptmx.Read(p) }

// Write 写入键入字节。
func (h *handle) Write(p []byte) error {
	_, err := h.ptmx.Write(p)
	return err
}

// Resize 同步终端尺寸（TIOCSWINSZ）。
func (h *handle) Resize(cols, rows uint16) error {
	return pty.Setsize(h.ptmx, &pty.Winsize{Cols: cols, Rows: rows})
}

// Kill 进程组终止（Setsid 使子进程自成进程组，kill(-pid) 触达整组——
// su 代启的 shell 及其子进程一并回收，I-02/D-03）+ master 关闭（SIGHUP 兜底）。
func (h *handle) Kill() error {
	if h.cmd.Process != nil {
		_ = syscall.Kill(-h.cmd.Process.Pid, syscall.SIGKILL)
	}
	return h.ptmx.Close()
}

// Wait 收割子进程（防孤儿）。
func (h *handle) Wait() error { return h.cmd.Wait() }
