package nginxx

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// isPidFileErr 判定 reload 输出是否为 pid 文件类失败（存量环境坑：
// /run/nginx.pid 空/缺失/指向他处——nginx -s reload 与 systemd ExecReload
// （kill -HUP $(cat pid)）同时失败，而 master 实际存活；实机 v0.1.9 反馈）。
func isPidFileErr(out string) bool {
	return strings.Contains(out, "invalid PID number") ||
		strings.Contains(out, ".pid failed") ||
		(strings.Contains(out, ".pid") && strings.Contains(out, "No such file or directory"))
}

// masterPid 解析 nginx master 进程 PID：systemd MainPID 优先；不可用则扫
// /proc cmdline 的 "nginx: master process"（纯标准库；非 Linux 自然报错，
// 调用方回退原错误路径）。
func masterPid(ctx context.Context) (int, error) {
	if pid, err := systemdMainPID(ctx); err == nil && pid > 0 {
		return pid, nil
	}
	return masterPidFromProc("/proc")
}

// systemdMainPID 经 systemctl 取 MainPID（字面量参数数组，R-08）。
func systemdMainPID(ctx context.Context) (int, error) {
	cmd := exec.CommandContext(ctx, "systemctl", "show", "nginx.service", "--property=MainPID", "--value")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(buf.String()))
	if err != nil || pid <= 0 {
		return 0, strconv.ErrSyntax
	}
	return pid, nil
}

// masterPidFromProc 扫 procfs 目录（目录可注入——单测用临时目录）。
func masterPidFromProc(dir string) (int, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(dir, e.Name(), "cmdline"))
		if rerr != nil {
			continue
		}
		if strings.Contains(string(raw), "nginx: master process") {
			return pid, nil
		}
	}
	return 0, os.ErrNotExist
}

// signalHup 向 master 发 SIGHUP（平滑重载，与 nginx -s reload 同语义）。
// 非 Unix 平台 Process.Signal 返回错误——调用方回退原错误路径，行为不变。
func signalHup(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGHUP)
}
