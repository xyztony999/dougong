// 本文件是裸 `dougong` 的交互式管理菜单（宝塔 bt 风格，2026-11-20 用户反馈）：
// 在交互终端（TTY）输入 dougong 不再直接启动服务（systemd 已在跑，裸启动撞端口），
// 而是弹出编号菜单供选择管理/紧急救援操作（查状态/版本、重置密码、改用户名、
// 解除域名绑定、关安全入口、改端口、启停服务）。非交互场景（systemd ExecStart、
// 管道、CI——stdin 非 TTY）裸 dougong 仍=启动面板服务（保既有部署兼容）。
// 菜单各操作复用既有 CLI 命令函数（进程内 service 通道，source=cli 审计）。
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mattn/go-isatty"
)

// isInteractive 判定是否交互终端（stdin 与 stdout 均为 TTY）。systemd 服务 stdin
// 为 /dev/null（非 TTY）、curl|bash 管道 stdin 非 TTY → 均返回 false → 裸 dougong
// 走启动服务路径（保 install.sh ExecStart=/usr/local/bin/dougong 既有部署兼容）。
func isInteractive() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())
}

const menuBanner = `================ 斗拱面板管理 ================
  1) 查看面板状态
  2) 查看版本与访问信息
  3) 重置面板密码（紧急救援）
  4) 修改面板用户名（紧急救援）
  5) 解除域名绑定（紧急，恢复 IP 访问）
  6) 关闭安全入口（紧急救援）
  7) 修改监听端口
  8) 重启面板服务
  9) 启动面板服务
 10) 停止面板服务
  0) 退出
==============================================`

// cliMenu 交互菜单循环（in 默认 os.Stdin，可注入供测试）。每次操作后回到菜单，
// 选 0/q/quit/exit 或 EOF 退出。返回进程退出码（菜单本身恒 0，操作结果已各自呈现）。
func cliMenu(env cliEnv, in io.Reader) int {
	if env.stdout == nil {
		env.stdout = os.Stdout
	}
	if env.getenv == nil {
		env.getenv = os.Getenv
	}
	if env.systemctl == nil {
		env.systemctl = execSystemctl
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	reader := bufio.NewReader(in)
	for {
		fmt.Fprintln(env.stdout, menuBanner)
		fmt.Fprint(env.stdout, "请输入编号: ")
		line, err := reader.ReadString('\n')
		choice := strings.TrimSpace(line)
		if err != nil && choice == "" {
			fmt.Fprintln(env.stdout)
			return cliExitOK // EOF（Ctrl-D）退出
		}
		switch choice {
		case "0", "q", "quit", "exit":
			return cliExitOK
		case "1":
			cliStatus(ctx, env, nil)
		case "2":
			cliInfo(ctx, env, nil)
		case "3":
			menuResetPassword(ctx, env, reader)
		case "4":
			menuRenameUser(ctx, env, reader)
		case "5":
			cliUnbindDomain(ctx, env, nil)
		case "6":
			cliEntryPath(ctx, env, []string{"off"})
		case "7":
			menuSetPort(ctx, env, reader)
		case "8":
			cliService(ctx, env, []string{"restart"})
		case "9":
			cliService(ctx, env, []string{"start"})
		case "10":
			cliService(ctx, env, []string{"stop"})
		case "":
			// 空回车：重印菜单
		default:
			fmt.Fprintf(env.stdout, "无效编号 %q，请重新选择\n", choice)
		}
		fmt.Fprintln(env.stdout)
	}
}

// prompt 输出一行提示并读取一行输入（trim 空白；EOF 返回空串）。
func prompt(env cliEnv, reader *bufio.Reader, label string) string {
	fmt.Fprint(env.stdout, label)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

// menuResetPassword 菜单 3：交互问用户名+新密码，复用 cliResetPassword（密码经
// 派生 env.getenv 注入 DOUGONG_INIT_PASSWORD，不经 argv——同 reset-password 防 ps 口径）。
func menuResetPassword(ctx context.Context, env cliEnv, reader *bufio.Reader) {
	username := prompt(env, reader, "目标用户名: ")
	if username == "" {
		fmt.Fprintln(env.stdout, "已取消（用户名不能为空）")
		return
	}
	password := prompt(env, reader, "新密码（输入可见，本地终端）: ")
	if password == "" {
		fmt.Fprintln(env.stdout, "已取消（密码不能为空）")
		return
	}
	sub := env
	sub.getenv = func(key string) string {
		if key == "DOUGONG_INIT_PASSWORD" {
			return password
		}
		return env.getenv(key)
	}
	cliResetPassword(ctx, sub, []string{"-username", username})
}

// menuRenameUser 菜单 4：交互问原用户名+新用户名，复用 cliRenameUser。
func menuRenameUser(ctx context.Context, env cliEnv, reader *bufio.Reader) {
	oldName := prompt(env, reader, "原用户名: ")
	newName := prompt(env, reader, "新用户名: ")
	if oldName == "" || newName == "" {
		fmt.Fprintln(env.stdout, "已取消（用户名不能为空）")
		return
	}
	cliRenameUser(ctx, env, []string{"-username", oldName, "-new", newName})
}

// menuSetPort 菜单 7：交互问新端口，复用 cliSetPort。
func menuSetPort(ctx context.Context, env cliEnv, reader *bufio.Reader) {
	port := prompt(env, reader, "新端口 (1024-65535): ")
	if port == "" {
		fmt.Fprintln(env.stdout, "已取消（端口不能为空）")
		return
	}
	cliSetPort(ctx, env, []string{port})
}
