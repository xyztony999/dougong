// cli_test.go 面板管理 CLI 六命令族表驱动测试（api-panel-core.md v2.1 4.1.4
// 「CLI 入口」）：参数校验失败退出码、entrypath 三态、service 子命令 systemctl
// 参数数组断言、写命令审计 source=cli 与 restart 提示。真实临时 SQLite
// （不 mock 数据库，backend-03 §6）；systemctl 通道注入 fake 断言参数。
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	authsvc "github.com/dougong/dougong/internal/service/auth"
	panelsvc "github.com/dougong/dougong/internal/service/panel"
	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	storepanel "github.com/dougong/dougong/internal/store/panel"
)

// ---- 测试基座 ----

// fakeSystemctl 记录每次调用的参数数组并按序弹出预置结果（空=零值成功）。
type fakeSystemctl struct {
	calls   [][]string
	results []systemctlResult
}

func (f *fakeSystemctl) run(_ context.Context, args ...string) systemctlResult {
	f.calls = append(f.calls, append([]string(nil), args...))
	if len(f.results) > 0 {
		r := f.results[0]
		f.results = f.results[1:]
		return r
	}
	return systemctlResult{}
}

// cliTestEnv 每用例独立的 CLI 环境（独立临时库，避免跨用例污染）。
type cliTestEnv struct {
	env     cliEnv
	out     *bytes.Buffer
	fake    *fakeSystemctl
	dbPath  string
	cfgPath string
	setenv  func(k, v string)
}

func newCLITestEnv(t *testing.T) *cliTestEnv {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	cfgPath := filepath.Join(dir, "dougong.json")
	if err := os.WriteFile(cfgPath, []byte(fmt.Sprintf(`{"db_path": %q}`, dbPath)), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	envMap := map[string]string{}
	fake := &fakeSystemctl{}
	out := &bytes.Buffer{}
	return &cliTestEnv{
		env: cliEnv{
			configPath: cfgPath,
			getenv:     func(k string) string { return envMap[k] },
			systemctl:  fake.run,
			stdout:     out,
			operator:   "root",
		},
		out:     out,
		fake:    fake,
		dbPath:  dbPath,
		cfgPath: cfgPath,
		setenv:  func(k, v string) { envMap[k] = v },
	}
}

// open 打开同一临时库做断言（先幂等应用迁移——参数失败用例下命令可能从未
// 打开过库，查询表存在性属测试基建而非命令行为；调用方经 Cleanup 关闭）。
func (e *cliTestEnv) open(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(e.dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// seedInitAdmin 经 auth 服务真实初始化通道建首个 admin（InitViaCLI），
// 返回其用户 id。
func (e *cliTestEnv) seedInitAdmin(t *testing.T) int64 {
	t.Helper()
	ctx := context.Background()
	db := e.open(t)
	repo := storeauth.New(db)
	if err := repo.Init().EnsureRow(ctx); err != nil {
		t.Fatalf("ensure init row: %v", err)
	}
	hasher, err := authsvc.NewBcryptHasher(10)
	if err != nil {
		t.Fatalf("hasher: %v", err)
	}
	svc := authsvc.New(repo, hasher, authsvc.NewTotpService(nil), authsvc.DefaultSessionPolicy())
	if err := svc.InitViaCLI(ctx, "admin", "init-password-123", "", ""); err != nil {
		t.Fatalf("init admin: %v", err)
	}
	var id int64
	if err := db.QueryRowContext(ctx, `SELECT id FROM panel_user WHERE username = 'admin'`).Scan(&id); err != nil {
		t.Fatalf("query admin id: %v", err)
	}
	return id
}

// auditRow 审计断言投影。
type auditRow struct {
	Action    string
	Source    string
	ActorName string
	Category  string
	Detail    string
}

func (e *cliTestEnv) auditRows(t *testing.T) []auditRow {
	t.Helper()
	db := e.open(t)
	rows, err := db.QueryContext(context.Background(),
		`SELECT action, source, actor_name, category, IFNULL(detail,'') FROM audit_log ORDER BY id`)
	if err != nil {
		t.Fatalf("query audit: %v", err)
	}
	defer rows.Close()
	var out []auditRow
	for rows.Next() {
		var r auditRow
		if err := rows.Scan(&r.Action, &r.Source, &r.ActorName, &r.Category, &r.Detail); err != nil {
			t.Fatalf("scan audit: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("audit rows: %v", err)
	}
	return out
}

// panelConfig 直读 panel_config 单行。
func (e *cliTestEnv) panelConfig(t *testing.T) storepanel.Config {
	t.Helper()
	db := e.open(t)
	cfg, err := storepanel.New(db).Config().Get(context.Background())
	if err != nil {
		t.Fatalf("get panel config: %v", err)
	}
	return cfg
}

// ensureConfigRow 幂等落 panel_config 单行（预置态前置——UPDATE 对无行零生效）。
func (e *cliTestEnv) ensureConfigRow(t *testing.T) {
	t.Helper()
	db := e.open(t)
	if err := storepanel.New(db).Config().EnsureSingleton(context.Background(), 18443, time.Now()); err != nil {
		t.Fatalf("ensure singleton: %v", err)
	}
}

// mutateConfig 经 store 通道直改配置（测试预置态，非业务通道断言对象）。
func (e *cliTestEnv) mutateConfig(t *testing.T, ch storepanel.ConfigChanges) {
	t.Helper()
	e.ensureConfigRow(t)
	db := e.open(t)
	if err := storepanel.New(db).Config().Update(context.Background(), ch, time.Now()); err != nil {
		t.Fatalf("mutate config: %v", err)
	}
}

func runCLIArgs(t *testing.T, e *cliTestEnv, args ...string) int {
	t.Helper()
	return runCLI(args, e.env)
}

func wantOut(t *testing.T, out *bytes.Buffer, substrs ...string) {
	t.Helper()
	s := out.String()
	for _, sub := range substrs {
		if !strings.Contains(s, sub) {
			t.Fatalf("输出缺少 %q；完整输出:\n%s", sub, s)
		}
	}
}

func wantNoOut(t *testing.T, out *bytes.Buffer, substr string) {
	t.Helper()
	if strings.Contains(out.String(), substr) {
		t.Fatalf("输出不应包含 %q；完整输出:\n%s", substr, out.String())
	}
}

// ---- 纯函数 ----

func TestSplitConfigFlag(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		want    []string
		wantCfg string
		wantErr bool
	}{
		{name: "无 flag 原样返回", args: []string{"show"}, want: []string{"show"}, wantCfg: "dougong.json"},
		{name: "-config 空格形态", args: []string{"-config", "/tmp/a.json", "show"}, want: []string{"show"}, wantCfg: "/tmp/a.json"},
		{name: "--config= 等号形态", args: []string{"--config=/tmp/b.json"}, want: []string{}, wantCfg: "/tmp/b.json"},
		{name: "缺值报错", args: []string{"show", "-config"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rest, cfg, err := splitConfigFlag(tc.args, "dougong.json")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got rest=%v cfg=%s", rest, cfg)
				}
				return
			}
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if cfg != tc.wantCfg {
				t.Fatalf("cfg = %q, want %q", cfg, tc.wantCfg)
			}
			if len(rest) != len(tc.want) {
				t.Fatalf("rest = %v, want %v", rest, tc.want)
			}
			for i := range rest {
				if rest[i] != tc.want[i] {
					t.Fatalf("rest = %v, want %v", rest, tc.want)
				}
			}
		})
	}
}

func TestVisitURLs(t *testing.T) {
	t.Run("绑定具体地址仅列该地址", func(t *testing.T) {
		urls := visitURLs("https", "192.0.2.10:9443")
		if len(urls) != 1 || urls[0] != "https://192.0.2.10:9443/" {
			t.Fatalf("urls = %v", urls)
		}
	})
	t.Run("通配绑定至少一个候选且回退环回可达", func(t *testing.T) {
		urls := visitURLs("http", "0.0.0.0:8080")
		if len(urls) < 1 || len(urls) > 4 {
			t.Fatalf("urls = %v", urls)
		}
		if !strings.HasPrefix(urls[0], "http://") || !strings.HasSuffix(urls[0], ":8080/") {
			t.Fatalf("urls[0] = %q", urls[0])
		}
	})
	t.Run("不可解析地址返回空", func(t *testing.T) {
		if got := visitURLs("http", "no-port"); got != nil {
			t.Fatalf("got %v", got)
		}
	})
}

// ---- status / info（读） ----

func TestCLIStatus(t *testing.T) {
	cases := []struct {
		name       string
		systemctl  systemctlResult
		wantExit   int
		wantSubstr []string
		noSubstr   []string
	}{
		{
			name:       "active 输出三段信息",
			systemctl:  systemctlResult{Stdout: "active\n"},
			wantExit:   0,
			wantSubstr: []string{"dougong.service=active", "面板初始化: 未初始化", "访问入口:", "http://"},
		},
		{
			name: "inactive 非零退出码属正常输出",
			systemctl: systemctlResult{
				Stdout: "inactive\n", Stderr: "", ExitCode: 3,
			},
			wantExit:   0,
			wantSubstr: []string{"dougong.service=inactive"},
		},
		{
			name:       "通道级失败退出码 1",
			systemctl:  systemctlResult{Err: fmt.Errorf("exec: systemctl: executable file not found")},
			wantExit:   1,
			wantSubstr: []string{"systemd 状态查询失败"},
			noSubstr:   []string{"访问入口"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newCLITestEnv(t)
			e.fake.results = []systemctlResult{tc.systemctl}
			got := runCLIArgs(t, e, "status")
			if got != tc.wantExit {
				t.Fatalf("exit = %d, want %d；输出:\n%s", got, tc.wantExit, e.out.String())
			}
			wantOut(t, e.out, tc.wantSubstr...)
			for _, s := range tc.noSubstr {
				wantNoOut(t, e.out, s)
			}
			if len(e.fake.calls) != 1 {
				t.Fatalf("systemctl 调用 %d 次, want 1", len(e.fake.calls))
			}
			if want := []string{"is-active", "dougong.service"}; fmt.Sprint(e.fake.calls[0]) != fmt.Sprint(want) {
				t.Fatalf("systemctl args = %v, want %v", e.fake.calls[0], want)
			}
		})
	}

	t.Run("已初始化与安全入口前缀与 https scheme", func(t *testing.T) {
		e := newCLITestEnv(t)
		e.seedInitAdmin(t)
		e.mutateConfig(t, storepanel.ConfigChanges{
			HTTPSMode:        &[]string{"self_signed"}[0],
			EntryPathEnabled: &[]bool{true}[0],
			EntryPath:        &[]string{"Abcd1234Wxyz5678"}[0],
		})
		got := runCLIArgs(t, e, "status")
		if got != 0 {
			t.Fatalf("exit = %d；输出:\n%s", got, e.out.String())
		}
		port := fmt.Sprint(e.panelConfig(t).ListenPort)
		wantOut(t, e.out, "面板初始化: 已初始化", "https://", ":"+port+"/Abcd1234Wxyz5678")
		wantNoOut(t, e.out, "http://")
	})
}

func TestCLIInfo(t *testing.T) {
	t.Run("默认态五要素", func(t *testing.T) {
		e := newCLITestEnv(t)
		got := runCLIArgs(t, e, "info")
		if got != 0 {
			t.Fatalf("exit = %d；输出:\n%s", got, e.out.String())
		}
		dir := filepath.Dir(e.dbPath)
		wantOut(t, e.out,
			"版本: dev (commit none, built unknown)",
			"监听: 0.0.0.0:", "范围 all",
			"安全入口: 关闭",
			"HTTPS 模式: off",
			"数据目录: "+dir)
		wantNoOut(t, e.out, restartHint)
	})
	t.Run("-config 覆盖生效", func(t *testing.T) {
		e := newCLITestEnv(t)
		got := runCLIArgs(t, e, "info", "-config", e.cfgPath)
		if got != 0 {
			t.Fatalf("exit = %d；输出:\n%s", got, e.out.String())
		}
		wantOut(t, e.out, "HTTPS 模式: off")
	})
}

// ---- reset-password（写） ----

func TestCLIResetPassword(t *testing.T) {
	usageCases := []struct {
		name       string
		args       []string
		env        bool // 是否注入 DOUGONG_INIT_PASSWORD
		password   string
		wantExit   int
		wantSubstr []string
	}{
		{
			name:       "缺少 -username 退出码 2",
			args:       []string{"reset-password"},
			env:        true,
			wantExit:   2,
			wantSubstr: []string{"AUTH_012", "-username"},
		},
		{
			name:     "flag 解析错误退出码 2",
			args:     []string{"reset-password", "-username", "admin", "-bogus"},
			env:      true,
			wantExit: 2,
		},
		{
			name:       "未设 DOUGONG_INIT_PASSWORD=PANEL_012 退出码 2",
			args:       []string{"reset-password", "-username", "admin"},
			env:        false,
			wantExit:   2,
			wantSubstr: []string{"PANEL_012", "DOUGONG_INIT_PASSWORD"},
		},
		{
			name:       "密码短于 A10 下限=AUTH_012 退出码 2",
			args:       []string{"reset-password", "-username", "admin"},
			env:        true,
			password:   "short",
			wantExit:   2,
			wantSubstr: []string{"AUTH_012"},
		},
	}
	for _, tc := range usageCases {
		t.Run(tc.name, func(t *testing.T) {
			e := newCLITestEnv(t)
			e.seedInitAdmin(t)
			if tc.env {
				e.setenv("DOUGONG_INIT_PASSWORD", tc.password)
			}
			got := runCLIArgs(t, e, tc.args...)
			if got != tc.wantExit {
				t.Fatalf("exit = %d, want %d；输出:\n%s", got, tc.wantExit, e.out.String())
			}
			wantOut(t, e.out, tc.wantSubstr...)
			// 参数失败零审计零写库。
			if rows := e.auditRows(t); len(rows) != 1 { // 仅 init_completed
				t.Fatalf("audit rows = %d, want 1（init）；%+v", len(rows), rows)
			}
		})
	}

	t.Run("目标用户不存在退出码 1", func(t *testing.T) {
		e := newCLITestEnv(t)
		e.seedInitAdmin(t)
		e.setenv("DOUGONG_INIT_PASSWORD", "new-password-456")
		got := runCLIArgs(t, e, "reset-password", "-username", "nobody")
		if got != 1 {
			t.Fatalf("exit = %d, want 1；输出:\n%s", got, e.out.String())
		}
		wantOut(t, e.out, "不存在")
	})

	t.Run("成功路径：哈希更新+会话吊销+审计 source=cli+restart 提示", func(t *testing.T) {
		e := newCLITestEnv(t)
		uid := e.seedInitAdmin(t)
		ctx := context.Background()
		db := e.open(t)
		repo := storeauth.New(db)
		now := time.Now().UTC()
		if err := repo.Sessions().Create(ctx, storeauth.Session{
			TokenHash: "deadbeef", UserID: uid, CsrfToken: "csrf",
			CreatedAt: now, LastActiveAt: now, ExpiresAt: now.Add(time.Hour), IdleExpiresAt: now.Add(time.Hour),
		}); err != nil {
			t.Fatalf("seed session: %v", err)
		}
		e.setenv("DOUGONG_INIT_PASSWORD", "new-password-456")
		got := runCLIArgs(t, e, "reset-password", "-username", "ADMIN") // NOCASE 解析
		if got != 0 {
			t.Fatalf("exit = %d；输出:\n%s", got, e.out.String())
		}
		wantOut(t, e.out, "已重置用户 admin", restartHint)
		// 哈希已更新为新密码。
		var hash string
		if err := db.QueryRowContext(ctx, `SELECT password_hash FROM panel_user WHERE id = ?`, uid).Scan(&hash); err != nil {
			t.Fatalf("query hash: %v", err)
		}
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte("new-password-456")) != nil {
			t.Fatal("password hash 未更新为新密码")
		}
		// 会话级联吊销（end_reason=cascade）。
		var revoked int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM panel_session WHERE user_id = ? AND revoked_at IS NOT NULL AND end_reason = 'cascade'`, uid).Scan(&revoked); err != nil {
			t.Fatalf("query revoked: %v", err)
		}
		if revoked != 1 {
			t.Fatalf("revoked sessions = %d, want 1", revoked)
		}
		// 审计 password_reset security source=cli。
		rows := e.auditRows(t)
		var found bool
		for _, r := range rows {
			if r.Action == "password_reset" {
				found = true
				if r.Source != "cli" || r.Category != "security" || r.ActorName != "root" {
					t.Fatalf("password_reset 审计 = %+v", r)
				}
			}
		}
		if !found {
			t.Fatalf("未落 password_reset 审计: %+v", rows)
		}
	})

	t.Run("锁定用户如实提示且不解锁", func(t *testing.T) {
		e := newCLITestEnv(t)
		uid := e.seedInitAdmin(t)
		ctx := context.Background()
		db := e.open(t)
		until := time.Now().UTC().Add(10 * time.Minute)
		if err := storeauth.New(db).Users().UpdateLockState(ctx, uid, 5, &until, storeauth.UserStatusLocked, time.Now().UTC()); err != nil {
			t.Fatalf("lock user: %v", err)
		}
		e.setenv("DOUGONG_INIT_PASSWORD", "new-password-456")
		got := runCLIArgs(t, e, "reset-password", "-username", "admin")
		if got != 0 {
			t.Fatalf("exit = %d；输出:\n%s", got, e.out.String())
		}
		wantOut(t, e.out, "锁定状态", "不会解锁")
		var status string
		var lockedUntil sql.NullString
		if err := db.QueryRowContext(ctx, `SELECT status, locked_until FROM panel_user WHERE id = ?`, uid).Scan(&status, &lockedUntil); err != nil {
			t.Fatalf("query lock: %v", err)
		}
		if status != storeauth.UserStatusLocked || !lockedUntil.Valid {
			t.Fatalf("锁定态被意外变更: status=%s locked_until=%v", status, lockedUntil)
		}
	})
}

// ---- set-port（写） ----

func TestCLISetPort(t *testing.T) {
	usageCases := []struct {
		name       string
		args       []string
		wantExit   int
		wantSubstr []string
	}{
		{name: "缺参退出码 2", args: []string{"set-port"}, wantExit: 2, wantSubstr: []string{"AUTH_012"}},
		{name: "多参退出码 2", args: []string{"set-port", "9000", "9001"}, wantExit: 2},
		{name: "非数字退出码 2", args: []string{"set-port", "abc"}, wantExit: 2, wantSubstr: []string{"AUTH_012", "整数"}},
		{name: "低于下限退出码 2", args: []string{"set-port", "80"}, wantExit: 2, wantSubstr: []string{"AUTH_012", "1024"}},
		{name: "超上限退出码 2", args: []string{"set-port", "70000"}, wantExit: 2, wantSubstr: []string{"AUTH_012", "65535"}},
	}
	for _, tc := range usageCases {
		t.Run(tc.name, func(t *testing.T) {
			e := newCLITestEnv(t)
			got := runCLIArgs(t, e, tc.args...)
			if got != tc.wantExit {
				t.Fatalf("exit = %d, want %d；输出:\n%s", got, tc.wantExit, e.out.String())
			}
			wantOut(t, e.out, tc.wantSubstr...)
			if rows := e.auditRows(t); len(rows) != 0 {
				t.Fatalf("参数失败不应落审计: %+v", rows)
			}
		})
	}

	t.Run("成功：落库+审计 listenPort delta source=cli+restart 提示", func(t *testing.T) {
		e := newCLITestEnv(t)
		e.ensureConfigRow(t)
		cur := e.panelConfig(t).ListenPort
		next := cur + 1
		if next > panelsvc.ListenPortMax {
			next = cur - 1
		}
		got := runCLIArgs(t, e, "set-port", fmt.Sprint(next))
		if got != 0 {
			t.Fatalf("exit = %d；输出:\n%s", got, e.out.String())
		}
		wantOut(t, e.out, fmt.Sprintf("%d → %d", cur, next), restartHint)
		if p := e.panelConfig(t).ListenPort; p != next {
			t.Fatalf("listen_port = %d, want %d", p, next)
		}
		rows := e.auditRows(t)
		if len(rows) != 1 {
			t.Fatalf("audit rows = %d, want 1: %+v", len(rows), rows)
		}
		r := rows[0]
		if r.Action != "panel_config_changed" || r.Source != "cli" || r.ActorName != "root" || r.Category != "change" {
			t.Fatalf("audit = %+v", r)
		}
		var delta []map[string]any
		if err := json.Unmarshal([]byte(r.Detail), &delta); err != nil {
			t.Fatalf("detail 非法: %v (%s)", err, r.Detail)
		}
		if len(delta) != 1 || delta[0]["field"] != "listenPort" ||
			int(delta[0]["before"].(float64)) != cur || int(delta[0]["after"].(float64)) != next {
			t.Fatalf("delta = %+v, want field=listenPort %d→%d", delta, cur, next)
		}
	})

	t.Run("同值零写入零审计", func(t *testing.T) {
		e := newCLITestEnv(t)
		e.ensureConfigRow(t)
		cur := e.panelConfig(t).ListenPort
		got := runCLIArgs(t, e, "set-port", fmt.Sprint(cur))
		if got != 0 {
			t.Fatalf("exit = %d；输出:\n%s", got, e.out.String())
		}
		wantOut(t, e.out, "无需变更")
		wantNoOut(t, e.out, restartHint)
		if rows := e.auditRows(t); len(rows) != 0 {
			t.Fatalf("同值不应落审计: %+v", rows)
		}
	})
}

// ---- entrypath（读 show + 写 regen/off） ----

func TestCLIEntryPath(t *testing.T) {
	usageCases := []struct {
		name     string
		args     []string
		wantExit int
	}{
		{name: "缺子命令退出码 2", args: []string{"entrypath"}, wantExit: 2},
		{name: "未知子命令退出码 2", args: []string{"entrypath", "toggle"}, wantExit: 2},
	}
	for _, tc := range usageCases {
		t.Run(tc.name, func(t *testing.T) {
			e := newCLITestEnv(t)
			got := runCLIArgs(t, e, tc.args...)
			if got != tc.wantExit {
				t.Fatalf("exit = %d, want %d；输出:\n%s", got, tc.wantExit, e.out.String())
			}
			wantOut(t, e.out, "AUTH_012")
		})
	}

	t.Run("show 默认态", func(t *testing.T) {
		e := newCLITestEnv(t)
		if got := runCLIArgs(t, e, "entrypath", "show"); got != 0 {
			t.Fatalf("exit = %d；输出:\n%s", got, e.out.String())
		}
		wantOut(t, e.out, "安全入口开关: 关闭", "安全入口路径: （无）")
		wantNoOut(t, e.out, restartHint)
	})

	t.Run("regen 生成合法值+审计脱敏+restart 提示", func(t *testing.T) {
		e := newCLITestEnv(t)
		if got := runCLIArgs(t, e, "entrypath", "regen"); got != 0 {
			t.Fatalf("exit = %d；输出:\n%s", got, e.out.String())
		}
		cfg := e.panelConfig(t)
		if cfg.EntryPath == nil || !regexp.MustCompile(`^[A-Za-z0-9_-]{16}$`).MatchString(*cfg.EntryPath) {
			t.Fatalf("entry_path = %v", cfg.EntryPath)
		}
		if cfg.EntryPathEnabled {
			t.Fatal("regen 不应改变 enabled（保持关闭）")
		}
		wantOut(t, e.out, "已重新生成: /"+*cfg.EntryPath, restartHint)
		rows := e.auditRows(t)
		if len(rows) != 1 || rows[0].Action != "panel_entry_path_regenerated" || rows[0].Source != "cli" {
			t.Fatalf("audit = %+v", rows)
		}
		if strings.Contains(rows[0].Detail, *cfg.EntryPath) {
			t.Fatalf("审计 detail 泄露明文路径: %s", rows[0].Detail)
		}
	})

	t.Run("off 开→关 落审计 entryPathEnabled", func(t *testing.T) {
		e := newCLITestEnv(t)
		e.mutateConfig(t, storepanel.ConfigChanges{
			EntryPathEnabled: &[]bool{true}[0],
			EntryPath:        &[]string{"Abcd1234Wxyz5678"}[0],
		})
		if got := runCLIArgs(t, e, "entrypath", "off"); got != 0 {
			t.Fatalf("exit = %d；输出:\n%s", got, e.out.String())
		}
		wantOut(t, e.out, "安全入口已关闭", restartHint)
		if e.panelConfig(t).EntryPathEnabled {
			t.Fatal("entry_path_enabled 应为 0")
		}
		rows := e.auditRows(t)
		if len(rows) != 1 || rows[0].Action != "panel_config_changed" || rows[0].Source != "cli" {
			t.Fatalf("audit = %+v", rows)
		}
		if !strings.Contains(rows[0].Detail, `"entryPathEnabled"`) {
			t.Fatalf("detail 缺 entryPathEnabled: %s", rows[0].Detail)
		}
	})

	t.Run("off 已关闭 幂等零审计", func(t *testing.T) {
		e := newCLITestEnv(t)
		if got := runCLIArgs(t, e, "entrypath", "off"); got != 0 {
			t.Fatalf("exit = %d；输出:\n%s", got, e.out.String())
		}
		wantOut(t, e.out, "已是关闭状态")
		if rows := e.auditRows(t); len(rows) != 0 {
			t.Fatalf("幂等不应落审计: %+v", rows)
		}
	})
}

// ---- service（OS 层治理，systemctl 参数数组断言） ----

func TestCLIService(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		result    systemctlResult
		wantExit  int
		wantArgs  []string
		wantSubst []string
	}{
		{
			name: "start 参数数组与成功", args: []string{"service", "start"},
			wantExit: 0, wantArgs: []string{"start", "dougong.service"},
		},
		{
			name: "stop 参数数组", args: []string{"service", "stop"},
			wantExit: 0, wantArgs: []string{"stop", "dougong.service"},
		},
		{
			name: "restart 参数数组", args: []string{"service", "restart"},
			wantExit: 0, wantArgs: []string{"restart", "dougong.service"},
		},
		{
			name: "status 参数数组含 --no-pager 并透传摘要", args: []string{"service", "status"},
			result:   systemctlResult{Stdout: "● dougong.service - Dougong Panel\n   Active: active (running)\n"},
			wantExit: 0, wantArgs: []string{"status", "dougong.service", "--no-pager"},
			wantSubst: []string{"Active: active (running)"},
		},
		{
			name: "未知子命令退出码 2", args: []string{"service", "reload"},
			wantExit: 2,
		},
		{
			name: "缺子命令退出码 2", args: []string{"service"},
			wantExit: 2,
		},
		{
			name: "systemctl 非 0 退出码透传", args: []string{"service", "stop"},
			result:   systemctlResult{Stderr: "Failed to stop dougong.service\n", ExitCode: 5},
			wantExit: 5, wantArgs: []string{"stop", "dougong.service"},
			wantSubst: []string{"Failed to stop"},
		},
		{
			name: "通道级失败退出码 1", args: []string{"service", "start"},
			result:   systemctlResult{Err: fmt.Errorf("exec: not found")},
			wantExit: 1, wantArgs: []string{"start", "dougong.service"},
			wantSubst: []string{"systemctl 执行失败"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newCLITestEnv(t)
			e.fake.results = []systemctlResult{tc.result}
			got := runCLIArgs(t, e, tc.args...)
			if got != tc.wantExit {
				t.Fatalf("exit = %d, want %d；输出:\n%s", got, tc.wantExit, e.out.String())
			}
			if tc.wantArgs != nil {
				if len(e.fake.calls) != 1 {
					t.Fatalf("systemctl 调用 %d 次, want 1", len(e.fake.calls))
				}
				if fmt.Sprint(e.fake.calls[0]) != fmt.Sprint(tc.wantArgs) {
					t.Fatalf("systemctl args = %v, want %v", e.fake.calls[0], tc.wantArgs)
				}
			} else if len(e.fake.calls) != 0 {
				t.Fatalf("不应调用 systemctl: %v", e.fake.calls)
			}
			wantOut(t, e.out, tc.wantSubst...)
			// service 命令族不输出 restart 提示、不产生面板数据变更。
			wantNoOut(t, e.out, restartHint)
			if rows := e.auditRows(t); len(rows) != 0 {
				t.Fatalf("service 命令不应落审计: %+v", rows)
			}
		})
	}
}

// ---- 分发兜底 ----

func TestRunCLIUnknownSubcommand(t *testing.T) {
	e := newCLITestEnv(t)
	if got := runCLIArgs(t, e, "bogus"); got != cliExitUsage {
		t.Fatalf("exit = %d, want %d", got, cliExitUsage)
	}
	wantOut(t, e.out, "AUTH_012", "未知子命令")
}
