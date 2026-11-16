//go:build windows

package cronx

import "os/exec"

// setProcGroup Windows 无进程组语义（Job Object 不在本模块范围）；目标环境
// 为 Linux，此处仅保证开发机可编译与基本测试（超时回退 CommandContext 默认
// 单进程终止）。
func setProcGroup(cmd *exec.Cmd) {}

// killProcGroup Windows 回退：终止子进程本身。
func killProcGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
