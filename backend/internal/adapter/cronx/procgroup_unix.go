//go:build unix

package cronx

import (
	"os/exec"
	"syscall"
)

// setProcGroup 将子进程置为独立进程组组长（D-05 进程组终止的前提：shell 的
// 管道/后台子孙进程可整组回收）。
func setProcGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killProcGroup 终止整个进程组（组长 pid 即组 id；信号发给 -pgid）。
func killProcGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		// 组已回收（竞态）退回单进程终止，保证兜底。
		return cmd.Process.Kill()
	}
	return nil
}
