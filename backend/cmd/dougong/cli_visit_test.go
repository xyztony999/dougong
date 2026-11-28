// cli_visit_test.go CLI 访问地址与紧急关闭两步验证测试（2026-11-28 增量）：
// accessURLs 纯函数（域名优先/默认端口省略/入口路径前缀）、visit-urls 话术
// 按初始化态与绑定态自适应、disable-2fa 的 -yes 门与全因子清除。真实临时
// SQLite（不 mock 数据库，backend-03 §6）。
package main

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	storeauth "github.com/dougong/dougong/internal/store/auth"
	storepanel "github.com/dougong/dougong/internal/store/panel"
)

func TestAccessURLs(t *testing.T) {
	cases := []struct {
		name      string
		scheme    string
		addr      string
		domain    string
		entryPath string
		want      []string
	}{
		{
			name:   "绑定域名：仅域名地址，非默认端口显式携带",
			scheme: "https", addr: "0.0.0.0:2083", domain: "panel.example.com",
			want: []string{"https://panel.example.com:2083/"},
		},
		{
			name:   "绑定域名：https 默认端口省略",
			scheme: "https", addr: "0.0.0.0:443", domain: "panel.example.com",
			want: []string{"https://panel.example.com/"},
		},
		{
			name:   "绑定域名：http 默认端口省略",
			scheme: "http", addr: "0.0.0.0:80", domain: "panel.example.com",
			want: []string{"http://panel.example.com/"},
		},
		{
			name:   "未绑定：回退本机地址枚举（绑定具体地址时仅该地址）",
			scheme: "https", addr: "192.168.1.5:2083", domain: "",
			want: []string{"https://192.168.1.5:2083/"},
		},
		{
			name:   "绑定域名+安全入口：前缀追加",
			scheme: "https", addr: "0.0.0.0:2083", domain: "panel.example.com", entryPath: "Abcd1234",
			want: []string{"https://panel.example.com:2083/Abcd1234"},
		},
		{
			name:   "未绑定+安全入口：IP 地址前缀追加",
			scheme: "http", addr: "192.168.1.5:8080", domain: "", entryPath: "Abcd1234",
			want: []string{"http://192.168.1.5:8080/Abcd1234"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := accessURLs(tc.scheme, tc.addr, tc.domain, tc.entryPath)
			if len(got) != len(tc.want) {
				t.Fatalf("urls = %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("urls[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// seedFactors 给 admin 写入三类因子（passkey 2 个 + 恢复码 2 个 + TOTP 绑定）。
func (e *cliTestEnv) seedFactors(t *testing.T, userID int64) {
	t.Helper()
	ctx := context.Background()
	db := e.open(t)
	repo := storeauth.New(db)
	now := time.Now().UTC()
	for i, cid := range []string{"cred-a", "cred-b"} {
		if _, err := repo.Webauthn().Create(ctx, storeauth.WebauthnCredential{
			UserID: userID, CredentialID: []byte(cid), PublicKey: []byte("pk"),
			AttestationType: "none", SignCount: 0, Flags: 5, Name: "key" + string(rune('1'+i)),
			CreatedAt: now,
		}); err != nil {
			t.Fatalf("seed passkey: %v", err)
		}
	}
	if err := repo.Recovery().ReplaceUnused(ctx, userID, []string{"h1", "h2"}, now.Format(time.RFC3339)); err != nil {
		t.Fatalf("seed recovery: %v", err)
	}
	if err := repo.Totp().Upsert(ctx, storeauth.TotpCredential{UserID: userID, Secret: "SECRET", Enabled: true, ConfirmedAt: &now}); err != nil {
		t.Fatalf("seed totp: %v", err)
	}
}

func TestCLIVisitUrls(t *testing.T) {
	t.Run("未初始化：初始化向导话术 + IP 地址", func(t *testing.T) {
		e := newCLITestEnv(t)
		if got := runCLIArgs(t, e, "visit-urls"); got != 0 {
			t.Fatalf("exit = %d；输出:\n%s", got, e.out.String())
		}
		wantOut(t, e.out, "浏览器访问以下地址完成初始化向导：", "http://")
		wantNoOut(t, e.out, "已绑定域名")
	})
	t.Run("已初始化未绑域名：就绪话术 + IP 地址，无域名附注", func(t *testing.T) {
		e := newCLITestEnv(t)
		e.seedInitAdmin(t)
		if got := runCLIArgs(t, e, "visit-urls"); got != 0 {
			t.Fatalf("exit = %d；输出:\n%s", got, e.out.String())
		}
		wantOut(t, e.out, "面板已就绪，浏览器访问以下地址：", "http://")
		wantNoOut(t, e.out, "已绑定域名")
	})
	t.Run("已初始化且绑定域名：仅域名地址 + 附注（IP 不出现）", func(t *testing.T) {
		e := newCLITestEnv(t)
		e.seedInitAdmin(t)
		domain := "panel.example.com"
		e.mutateConfig(t, storepanel.ConfigChanges{
			BindDomain: &domain,
			HTTPSMode:  &[]string{"acme"}[0],
		})
		if got := runCLIArgs(t, e, "visit-urls"); got != 0 {
			t.Fatalf("exit = %d；输出:\n%s", got, e.out.String())
		}
		port := e.panelConfig(t).ListenPort
		wantOut(t, e.out, "面板已就绪，浏览器访问以下地址：", "https://panel.example.com", "已绑定域名，仅此地址可访问")
		wantNoOut(t, e.out, "http://")
		if !strings.Contains(e.out.String(), ":"+strconv.Itoa(int(port))) && port != 443 {
			t.Fatalf("域名地址应携带非默认端口 %d；输出:\n%s", port, e.out.String())
		}
	})
}

func TestCLIDisableSecondFactors(t *testing.T) {
	t.Run("缺 -username 退出码 2", func(t *testing.T) {
		e := newCLITestEnv(t)
		if got := runCLIArgs(t, e, "disable-2fa"); got != 2 {
			t.Fatalf("exit = %d；输出:\n%s", got, e.out.String())
		}
		wantOut(t, e.out, "缺少 -username")
	})
	t.Run("无 -yes 仅预告不执行（因子保留）", func(t *testing.T) {
		e := newCLITestEnv(t)
		id := e.seedInitAdmin(t)
		e.seedFactors(t, id)
		if got := runCLIArgs(t, e, "disable-2fa", "-username", "admin"); got != 2 {
			t.Fatalf("exit = %d；输出:\n%s", got, e.out.String())
		}
		wantOut(t, e.out, "追加 -yes")
		db := e.open(t)
		repo := storeauth.New(db)
		ctx := context.Background()
		if n, _ := repo.Webauthn().CountByUser(ctx, id); n != 2 {
			t.Fatalf("passkeys = %d, want 2（-yes 门不得执行清除）", n)
		}
		if n, _ := repo.Recovery().CountUnused(ctx, id); n != 2 {
			t.Fatalf("recovery unused = %d, want 2", n)
		}
		for _, a := range e.auditRows(t) {
			if a.Action == "second_factor_disabled" {
				t.Fatal("无 -yes 不得产生审计")
			}
		}
	})
	t.Run("带 -yes：三类因子全清 + 审计", func(t *testing.T) {
		e := newCLITestEnv(t)
		id := e.seedInitAdmin(t)
		e.seedFactors(t, id)
		if got := runCLIArgs(t, e, "disable-2fa", "-username", "admin", "-yes"); got != 0 {
			t.Fatalf("exit = %d；输出:\n%s", got, e.out.String())
		}
		wantOut(t, e.out, "已清除用户 admin 的两步验证因子", "passkey 2 个", "恢复码（未用）2 个", "登录已恢复为仅用户名+密码")
		db := e.open(t)
		repo := storeauth.New(db)
		ctx := context.Background()
		if n, _ := repo.Webauthn().CountByUser(ctx, id); n != 0 {
			t.Fatalf("passkeys = %d, want 0", n)
		}
		if n, _ := repo.Recovery().CountUnused(ctx, id); n != 0 {
			t.Fatalf("recovery unused = %d, want 0", n)
		}
		if _, err := repo.Totp().GetByUser(ctx, id); err == nil {
			t.Fatal("totp 应已解绑")
		}
		found := false
		for _, a := range e.auditRows(t) {
			if a.Action == "second_factor_disabled" {
				found = true
				if a.Source != "cli" || a.Category != "security" {
					t.Fatalf("审计 source/category = %s/%s", a.Source, a.Category)
				}
				if !strings.Contains(a.Detail, `"passkeys":2`) || !strings.Contains(a.Detail, `"totp":true`) {
					t.Fatalf("审计明细缺清除计数: %s", a.Detail)
				}
			}
		}
		if !found {
			t.Fatal("缺 second_factor_disabled 审计")
		}
	})
	t.Run("目标用户不存在退出码 1", func(t *testing.T) {
		e := newCLITestEnv(t)
		e.seedInitAdmin(t)
		if got := runCLIArgs(t, e, "disable-2fa", "-username", "ghost", "-yes"); got != 1 {
			t.Fatalf("exit = %d；输出:\n%s", got, e.out.String())
		}
		wantOut(t, e.out, "不存在")
	})
	t.Run("本无因子：零写入零审计", func(t *testing.T) {
		e := newCLITestEnv(t)
		e.seedInitAdmin(t)
		if got := runCLIArgs(t, e, "disable-2fa", "-username", "admin", "-yes"); got != 0 {
			t.Fatalf("exit = %d；输出:\n%s", got, e.out.String())
		}
		for _, a := range e.auditRows(t) {
			if a.Action == "second_factor_disabled" {
				t.Fatal("本无因子不得产生审计")
			}
		}
	})
}

func TestCLIMenuDisableSecondFactors(t *testing.T) {
	t.Run("菜单 11 交互确认后清除", func(t *testing.T) {
		e := newCLITestEnv(t)
		id := e.seedInitAdmin(t)
		e.seedFactors(t, id)
		code := cliMenu(e.env, strings.NewReader("11\nadmin\ny\n0\n"))
		if code != 0 {
			t.Fatalf("menu exit = %d；输出:\n%s", code, e.out.String())
		}
		db := e.open(t)
		repo := storeauth.New(db)
		ctx := context.Background()
		if n, _ := repo.Webauthn().CountByUser(ctx, id); n != 0 {
			t.Fatalf("passkeys = %d, want 0", n)
		}
		if n, _ := repo.Recovery().CountUnused(ctx, id); n != 0 {
			t.Fatalf("recovery unused = %d, want 0", n)
		}
	})
	t.Run("菜单 11 非 y 确认取消（因子保留）", func(t *testing.T) {
		e := newCLITestEnv(t)
		id := e.seedInitAdmin(t)
		e.seedFactors(t, id)
		code := cliMenu(e.env, strings.NewReader("11\nadmin\nn\n0\n"))
		if code != 0 {
			t.Fatalf("menu exit = %d", code)
		}
		db := e.open(t)
		repo := storeauth.New(db)
		if n, _ := repo.Webauthn().CountByUser(context.Background(), id); n != 2 {
			t.Fatalf("passkeys = %d, want 2（未确认不得清除）", n)
		}
	})
}

// itoa 免引 strconv 的极小转换（测试断言用）。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// itoa 已由 strconv 取代（保留占位注释避免空节）。
