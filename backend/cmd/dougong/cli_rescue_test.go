// cli_rescue_test.go 紧急救援 CLI 扩展测试（2026-11-20 用户反馈）：rename-user /
// unbind-domain 子命令（参数校验/成功审计 source=cli/幂等 noop）+ 交互菜单 cliMenu
// （编号分发/无效编号/退出）。真实临时 SQLite + fake systemctl（沿 cli_test.go 基座）。
package main

import (
	"context"
	"strings"
	"testing"

	storepanel "github.com/dougong/dougong/internal/store/panel"
)

func TestCLIRenameUser(t *testing.T) {
	t.Run("成功改名+审计+会话吊销提示", func(t *testing.T) {
		e := newCLITestEnv(t)
		e.seedInitAdmin(t)
		code := runCLIArgs(t, e, "rename-user", "-username", "admin", "-new", "rescued")
		if code != cliExitOK {
			t.Fatalf("exit=%d out=%s", code, e.out.String())
		}
		wantOut(t, e.out, "用户名已修改: admin → rescued", "会话已吊销")
		db := e.open(t)
		var name string
		if err := db.QueryRowContext(context.Background(), `SELECT username FROM panel_user`).Scan(&name); err != nil {
			t.Fatalf("query username: %v", err)
		}
		if name != "rescued" {
			t.Fatalf("username=%q want rescued", name)
		}
		var found bool
		for _, r := range e.auditRows(t) {
			if r.Action == "username_changed" && r.Source == "cli" && r.Category == "security" {
				found = true
			}
		}
		if !found {
			t.Fatalf("缺少 username_changed security source=cli 审计: %+v", e.auditRows(t))
		}
	})
	t.Run("原用户不存在退出码1", func(t *testing.T) {
		e := newCLITestEnv(t)
		e.seedInitAdmin(t)
		code := runCLIArgs(t, e, "rename-user", "-username", "ghost", "-new", "x")
		if code != cliExitFail {
			t.Fatalf("exit=%d want 1, out=%s", code, e.out.String())
		}
		wantOut(t, e.out, "不存在")
	})
	t.Run("缺参数退出码2", func(t *testing.T) {
		e := newCLITestEnv(t)
		code := runCLIArgs(t, e, "rename-user", "-username", "admin")
		if code != cliExitUsage {
			t.Fatalf("exit=%d want 2", code)
		}
	})
	t.Run("同名零写入", func(t *testing.T) {
		e := newCLITestEnv(t)
		e.seedInitAdmin(t)
		code := runCLIArgs(t, e, "rename-user", "-username", "admin", "-new", "admin")
		if code != cliExitOK {
			t.Fatalf("exit=%d out=%s", code, e.out.String())
		}
		wantOut(t, e.out, "无需变更")
	})
}

func TestCLIUnbindDomain(t *testing.T) {
	t.Run("已绑定→解除+审计+IP恢复提示", func(t *testing.T) {
		e := newCLITestEnv(t)
		domain := "panel.example.com"
		e.mutateConfig(t, storepanel.ConfigChanges{BindDomain: &domain})
		code := runCLIArgs(t, e, "unbind-domain")
		if code != cliExitOK {
			t.Fatalf("exit=%d out=%s", code, e.out.String())
		}
		wantOut(t, e.out, "已解除域名绑定", "panel.example.com", "IP 直接访问")
		if pc := e.panelConfig(t); pc.BindDomain != nil {
			t.Fatalf("bind_domain=%v want nil", *pc.BindDomain)
		}
		var found bool
		for _, r := range e.auditRows(t) {
			if r.Action == "panel_config_changed" && r.Source == "cli" {
				found = true
			}
		}
		if !found {
			t.Fatalf("缺少 panel_config_changed source=cli 审计")
		}
	})
	t.Run("未绑定零写入如实提示", func(t *testing.T) {
		e := newCLITestEnv(t)
		e.ensureConfigRow(t)
		code := runCLIArgs(t, e, "unbind-domain")
		if code != cliExitOK {
			t.Fatalf("exit=%d out=%s", code, e.out.String())
		}
		wantOut(t, e.out, "未绑定域名，无需解除")
	})
	t.Run("多余参数退出码2", func(t *testing.T) {
		e := newCLITestEnv(t)
		code := runCLIArgs(t, e, "unbind-domain", "extra")
		if code != cliExitUsage {
			t.Fatalf("exit=%d want 2", code)
		}
	})
}

func TestCLIMenu(t *testing.T) {
	t.Run("编号2查看信息后0退出", func(t *testing.T) {
		e := newCLITestEnv(t)
		e.ensureConfigRow(t)
		code := cliMenu(e.env, strings.NewReader("2\n0\n"))
		if code != cliExitOK {
			t.Fatalf("exit=%d out=%s", code, e.out.String())
		}
		wantOut(t, e.out, "斗拱面板管理", "版本", "请输入编号")
	})
	t.Run("无效编号提示后退出", func(t *testing.T) {
		e := newCLITestEnv(t)
		e.ensureConfigRow(t)
		code := cliMenu(e.env, strings.NewReader("99\n0\n"))
		if code != cliExitOK {
			t.Fatalf("exit=%d", code)
		}
		wantOut(t, e.out, "无效编号")
	})
	t.Run("EOF退出", func(t *testing.T) {
		e := newCLITestEnv(t)
		code := cliMenu(e.env, strings.NewReader(""))
		if code != cliExitOK {
			t.Fatalf("exit=%d", code)
		}
	})
	t.Run("q退出", func(t *testing.T) {
		e := newCLITestEnv(t)
		code := cliMenu(e.env, strings.NewReader("q\n"))
		if code != cliExitOK {
			t.Fatalf("exit=%d", code)
		}
	})
}
