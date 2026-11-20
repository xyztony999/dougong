// 本文件是面板管理 CLI 六命令族（api-panel-core.md v2.1 4.1.4「CLI 入口」，
// ADR-030/D-P17）：status / info / reset-password / set-port / entrypath /
// service。服务器终端本地通道——直接操作本机数据目录/SQLite、复用进程内
// service 层（无 HTTP 绕行、不经中间件链，权限=终端系统权限）；退出码
// 0=成功 / 2=参数校验失败（AUTH_012/PANEL_012 语义）/ 1=执行失败；
// systemctl 调用一律参数数组（R-08，无 shell）；写命令完成后统一输出
// restart 提示；变更审计 source=cli（服务层落点 service/{panel,auth}/cli.go）。
package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dougong/dougong/internal/config"
	"github.com/dougong/dougong/internal/errcode"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	panelsvc "github.com/dougong/dougong/internal/service/panel"
	"github.com/dougong/dougong/internal/store"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	storepanel "github.com/dougong/dougong/internal/store/panel"
)

const (
	// cliUnitName CLI status/service 子命令治理的 systemd 单元（FR-02）。
	cliUnitName = "dougong.service"
	// cliSystemctlTimeout systemctl 调用等待上限（service-mgmt ActionWaitLimit
	// 90s 同量级留裕量；is-active 秒回不受影响）。
	cliSystemctlTimeout = 120 * time.Second
	// restartHint 写命令完成后统一追加的生效提示（ADR-030 口径；实际生效
	// 分类仍按 config_effect_mode——entry_path 即时生效、listen_port 待重启）。
	restartHint = "请执行 systemctl restart dougong 使变更生效"
)

// CLI 退出码约定（D-P17）：0 成功 / 2 参数校验失败 / 1 执行失败。
const (
	cliExitOK    = 0
	cliExitFail  = 1
	cliExitUsage = 2
)

// systemctlResult 是一次 systemctl 调用的结果投影：ExitCode>0 = 进程非 0
// 退出（is-active 对非活跃态属正常输出、service 子命令按契约透传该码）；
// Err 非 nil = 通道级失败（二进制缺失/超时等，cmd 层退出码 1）。
type systemctlResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

// systemctlRunner 执行一次 systemctl 调用（可注入 fake 供表驱动测试断言参数
// 数组）。实参以字面量元素逐个传入，不拼接命令行。
type systemctlRunner func(ctx context.Context, args ...string) systemctlResult

// execSystemctl 真实通道：exec.CommandContext 参数数组（R-08：二进制名+
// 字面量参数，无 shell、无命令拼接）；ctx 取消即终止子进程。
func execSystemctl(ctx context.Context, args ...string) systemctlResult {
	cmd := exec.CommandContext(ctx, "systemctl", args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	res := systemctlResult{Stdout: outBuf.String(), Stderr: errBuf.String()}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code := exitErr.ExitCode()
			if code < 0 {
				code = cliExitFail // 被信号终止等：按执行失败呈现
			}
			res.ExitCode = code
			return res
		}
		res.Err = err
	}
	return res
}

// cliEnv 是 CLI 子命令运行环境（依赖注入点：表驱动测试注入 fake systemctl、
// 环境变量读取与输出缓冲）。
type cliEnv struct {
	configPath string              // 配置文件路径（-config 可覆盖）
	getenv     func(string) string // 默认 os.Getenv（DOUGONG_INIT_PASSWORD）
	systemctl  systemctlRunner     // 默认 execSystemctl
	stdout     io.Writer           // 默认 os.Stdout
	operator   string              // 变更审计 actor 标签（终端操作者）
}

// newCLIEnv 构造生产环境（裸 dougong 子命令入口用）。配置缺省优先已安装
// 服务数据目录（resolveConfigPath——CLI 在任意 CWD 都指向服务那套数据）。
func newCLIEnv() cliEnv {
	return cliEnv{
		configPath: resolveConfigPath(""),
		getenv:     os.Getenv,
		systemctl:  execSystemctl,
		stdout:     os.Stdout,
		operator:   cliOperator(),
	}
}

// cliOperator 终端操作者标签（审计 actor=操作终端的系统用户；解析失败兜底
// "cli"——CLI 无面板会话身份，D-P17）。
func cliOperator() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "cli"
}

// runCLI 六命令族分发（args[0]=子命令名）。返回进程退出码；main 层 os.Exit。
func runCLI(args []string, env cliEnv) int {
	if env.getenv == nil {
		env.getenv = func(string) string { return "" }
	}
	if env.stdout == nil {
		env.stdout = os.Stdout
	}
	if env.systemctl == nil {
		env.systemctl = execSystemctl
	}
	// 子命令级 -config 先于各命令自身参数剥离（沿 init 子命令 -config 先例）。
	rest, cfgPath, err := splitConfigFlag(args[1:], env.configPath)
	if err != nil {
		fmt.Fprintf(env.stdout, "AUTH_012: 参数错误 — %v\n", err)
		return cliExitUsage
	}
	env.configPath = cfgPath

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch args[0] {
	case "status":
		return cliStatus(ctx, env, rest)
	case "info":
		return cliInfo(ctx, env, rest)
	case "reset-password":
		return cliResetPassword(ctx, env, rest)
	case "set-port":
		return cliSetPort(ctx, env, rest)
	case "entrypath":
		return cliEntryPath(ctx, env, rest)
	case "service":
		return cliService(ctx, env, rest)
	case "rename-user":
		return cliRenameUser(ctx, env, rest)
	case "unbind-domain":
		return cliUnbindDomain(ctx, env, rest)
	default:
		fmt.Fprintf(env.stdout, "AUTH_012: 参数错误 — 未知子命令 %q（可用：status / info / reset-password / rename-user / unbind-domain / set-port / entrypath / service）\n", args[0])
		return cliExitUsage
	}
}

// splitConfigFlag 从参数中剥离 -config <path> / --config <path> /
// -config=<path>（缺值即参数错误）；其余参数原序返回交由各命令自身解析。
func splitConfigFlag(args []string, defaultPath string) ([]string, string, error) {
	cfgPath := defaultPath
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-config" || a == "--config":
			if i+1 >= len(args) {
				return nil, "", fmt.Errorf("%s 缺少配置文件路径", a)
			}
			cfgPath = args[i+1]
			i++
		case strings.HasPrefix(a, "-config=") || strings.HasPrefix(a, "--config="):
			cfgPath = a[strings.Index(a, "=")+1:]
		default:
			rest = append(rest, a)
		}
	}
	return rest, cfgPath, nil
}

// cliServices 是 CLI 命令的进程内服务装配（store → service，api 层不参与——
// D-P17 无 HTTP 绕行）。close 由调用方负责。
type cliServices struct {
	db       *sql.DB
	cfg      *config.Config
	authRepo *storeauth.Store
	authSvc  *authsvc.Service
	panelSvc *panelsvc.Service
}

// openCLIServices 打开配置指向的 SQLite 并完成迁移与服务装配（读命令服务
// 运行时可直读——WAL 并发读，SQL #2 通道，D-P17）。
func openCLIServices(ctx context.Context, env cliEnv) (*cliServices, error) {
	cfg, err := config.Load(env.configPath)
	if err != nil {
		return nil, err
	}
	// db_path 锚定到配置文件目录：CLI 与服务（run 同规则）永远同一套数据。
	cfg.DBPath = config.ResolveDBPath(env.configPath, cfg.DBPath)
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	if err := store.Migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	repo := storeauth.New(db)
	hasher, err := authsvc.NewBcryptHasher(12)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return &cliServices{
		db:       db,
		cfg:      cfg,
		authRepo: repo,
		authSvc:  authsvc.New(repo, hasher, authsvc.NewTotpService(nil), authsvc.DefaultSessionPolicy()),
		panelSvc: panelsvc.New(storepanel.New(db)),
	}, nil
}

// close 关闭数据库句柄。
func (s *cliServices) close() {
	if err := s.db.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "close db: %v\n", err)
	}
}

// cliStatus `dougong status`（读）：systemd 单元状态（systemctl is-active
// 参数数组）+ 初始化态（init_state）+ 访问 URL 列举（scheme 按 https_mode
// 推导 + 监听地址:端口经 visitURLs 纯函数 + 开启时的安全入口路径前缀形态）。
func cliStatus(ctx context.Context, env cliEnv, _ []string) int {
	sctx, cancel := context.WithTimeout(ctx, cliSystemctlTimeout)
	res := env.systemctl(sctx, "is-active", cliUnitName)
	cancel()
	if res.Err != nil {
		fmt.Fprintf(env.stdout, "systemd 状态查询失败: %v\n", res.Err)
		return cliExitFail
	}
	unitState := strings.TrimSpace(res.Stdout)
	if unitState == "" {
		unitState = "unknown"
	}

	svc, err := openCLIServices(ctx, env)
	if err != nil {
		fmt.Fprintf(env.stdout, "读取面板状态失败: %v\n", err)
		return cliExitFail
	}
	defer svc.close()

	if err := svc.authRepo.Init().EnsureRow(ctx); err != nil {
		fmt.Fprintf(env.stdout, "读取初始化状态失败: %v\n", err)
		return cliExitFail
	}
	initialized, err := svc.authSvc.IsInitialized(ctx)
	if err != nil {
		fmt.Fprintf(env.stdout, "读取初始化状态失败: %v\n", err)
		return cliExitFail
	}
	pc, err := svc.panelSvc.Bootstrap(ctx) // 幂等单例自检 + 读配置（I-P01/SQL #2）
	if err != nil {
		fmt.Fprintf(env.stdout, "读取面板配置失败: %v\n", err)
		return cliExitFail
	}

	scheme := "http"
	if pc.HTTPSMode != storepanel.HTTPSModeOff {
		scheme = "https"
	}
	urls := visitURLs(scheme, panelListenAddr(pc))
	if pc.EntryPathEnabled && pc.EntryPath != nil && *pc.EntryPath != "" {
		for i, u := range urls { // 开启安全入口：URL 前缀形态 /{entryPath}（D-P03）
			urls[i] = strings.TrimSuffix(u, "/") + "/" + *pc.EntryPath
		}
	}

	fmt.Fprintf(env.stdout, "systemd 状态: %s=%s\n", cliUnitName, unitState)
	initState := "未初始化（先运行 dougong init）"
	if initialized {
		initState = "已初始化"
	}
	fmt.Fprintf(env.stdout, "面板初始化: %s\n", initState)
	fmt.Fprintf(env.stdout, "访问入口:\n")
	for _, u := range urls {
		fmt.Fprintf(env.stdout, "  %s\n", u)
	}
	return cliExitOK
}

// cliInfo `dougong info`（读）：版本（ldflags，同 /panel/system/version 数据源）
// + 监听地址:端口 + 安全入口路径（开关+值）+ HTTPS 模式 + 数据目录路径。
func cliInfo(ctx context.Context, env cliEnv, _ []string) int {
	svc, err := openCLIServices(ctx, env)
	if err != nil {
		fmt.Fprintf(env.stdout, "读取面板信息失败: %v\n", err)
		return cliExitFail
	}
	defer svc.close()

	pc, err := svc.panelSvc.Bootstrap(ctx)
	if err != nil {
		fmt.Fprintf(env.stdout, "读取面板配置失败: %v\n", err)
		return cliExitFail
	}
	v := panelsvc.GetVersionInfo()
	fmt.Fprintf(env.stdout, "版本: %s (commit %s, built %s)\n", v.Version, v.GitCommit, v.BuildTime)
	fmt.Fprintf(env.stdout, "监听: %s（范围 %s，端口 %d）\n", panelListenAddr(pc), pc.ListenScope, pc.ListenPort)
	entryState := "关闭"
	if pc.EntryPathEnabled {
		if pc.EntryPath != nil && *pc.EntryPath != "" {
			entryState = "开启，路径 /" + *pc.EntryPath
		} else {
			entryState = "开启（路径未生成）"
		}
	}
	fmt.Fprintf(env.stdout, "安全入口: %s\n", entryState)
	fmt.Fprintf(env.stdout, "HTTPS 模式: %s\n", pc.HTTPSMode)
	fmt.Fprintf(env.stdout, "数据目录: %s\n", filepath.Dir(svc.cfg.DBPath))
	return cliExitOK
}

// cliResetPassword `dougong reset-password -username <name>`（写）：新密码仅经
// DOUGONG_INIT_PASSWORD 环境变量注入（缺失=PANEL_012 退出码 2，不经 argv——
// 防 ps 泄密，同 init 口径）；复用 account-auth 密码重置业务（A10 校验/会话
// 级联吊销/审计 password_reset security source=cli）；目标用户不存在=退出码 1。
func cliResetPassword(ctx context.Context, env cliEnv, args []string) int {
	fs := flag.NewFlagSet("reset-password", flag.ContinueOnError)
	fs.SetOutput(env.stdout)
	username := fs.String("username", "", "目标用户名（必填）")
	if err := fs.Parse(args); err != nil {
		return cliExitUsage
	}
	if strings.TrimSpace(*username) == "" {
		fmt.Fprintln(env.stdout, "AUTH_012: 参数错误 — 缺少 -username（用法：dougong reset-password -username <用户名>）")
		return cliExitUsage
	}
	password := env.getenv("DOUGONG_INIT_PASSWORD")
	if password == "" {
		fmt.Fprintf(env.stdout, "PANEL_012: %s\n", errcode.ErrInitPasswordMissing.Message())
		return cliExitUsage
	}

	svc, err := openCLIServices(ctx, env)
	if err != nil {
		fmt.Fprintf(env.stdout, "打开面板数据失败: %v\n", err)
		return cliExitFail
	}
	defer svc.close()

	res, err := svc.authSvc.ResetPasswordViaCLI(ctx, env.operator, *username, password)
	if err != nil {
		if errors.Is(err, authsvc.ErrUserNotFound) {
			fmt.Fprintf(env.stdout, "目标用户 %q 不存在\n", *username)
			return cliExitFail
		}
		if _, usage := printErrcode(env.stdout, err); usage {
			return cliExitUsage // AUTH_012（A10 长度校验）语义
		}
		var ec *errcode.Error
		if errors.As(err, &ec) {
			return cliExitFail // 业务码已打印（非校验语义）
		}
		fmt.Fprintf(env.stdout, "重置密码失败: %v\n", err)
		return cliExitFail
	}
	fmt.Fprintf(env.stdout, "已重置用户 %s 的密码（其全部面板会话已吊销）\n", res.Username)
	if res.Locked {
		// 如实呈现：重置不解锁（与 HTTP 同源语义）；锁定期满由登录路径惰性解锁（I-08）。
		until := "锁定期满后自动恢复"
		if res.LockedUntil != nil {
			until = "至 " + res.LockedUntil.Local().Format("2006-01-02 15:04:05 MST") + " 到期后自动恢复"
		}
		fmt.Fprintf(env.stdout, "注意: 该用户当前处于锁定状态（%s），重置密码不会解锁\n", until)
	}
	fmt.Fprintln(env.stdout, restartHint)
	return cliExitOK
}

// cliSetPort `dougong set-port <port>`（写）：port ∈ 1024..65535（I-P02/R-14，
// 非法=退出码 2）；更新 panel_config.listen_port（SQL #3 单事务 +
// panel_config_changed 审计 source=cli）；需重启生效（restart_required，D-P02）。
func cliSetPort(ctx context.Context, env cliEnv, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(env.stdout, "AUTH_012: 参数错误 — 用法：dougong set-port <port>（1024..65535）")
		return cliExitUsage
	}
	port, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Fprintf(env.stdout, "AUTH_012: 参数错误 — 端口须为整数（得到 %q）\n", args[0])
		return cliExitUsage
	}

	svc, err := openCLIServices(ctx, env)
	if err != nil {
		fmt.Fprintf(env.stdout, "打开面板数据失败: %v\n", err)
		return cliExitFail
	}
	defer svc.close()

	if _, err := svc.panelSvc.Bootstrap(ctx); err != nil { // 幂等单例自检（I-P01）
		fmt.Fprintf(env.stdout, "读取面板配置失败: %v\n", err)
		return cliExitFail
	}
	before, after, changed, err := svc.panelSvc.SetListenPortViaCLI(ctx, env.operator, port)
	if err != nil {
		if _, usage := printErrcode(env.stdout, err); usage {
			return cliExitUsage
		}
		var ec *errcode.Error
		if errors.As(err, &ec) {
			return cliExitFail // 业务码已打印（非校验语义）
		}
		fmt.Fprintf(env.stdout, "设置端口失败: %v\n", err)
		return cliExitFail
	}
	if !changed {
		fmt.Fprintf(env.stdout, "监听端口已是 %d，无需变更\n", after)
		return cliExitOK
	}
	fmt.Fprintf(env.stdout, "监听端口已更新: %d → %d（重启后生效）\n", before, after)
	fmt.Fprintln(env.stdout, restartHint)
	return cliExitOK
}

// cliEntryPath `dougong entrypath show|regen|off`：show（读）输出开关与当前
// 路径值；regen（写）EntryPathGenerator 生成新值替换（保持 enabled 不变，
// I-P03，审计脱敏口径同接口）；off（写）entry_path_enabled=0（SQL #3）。
// 即时生效（EntryPathGuard 下一请求即用）；写分支完成后输出 restart 提示
// （ADR-030 统一口径不阻塞）。
func cliEntryPath(ctx context.Context, env cliEnv, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(env.stdout, "AUTH_012: 参数错误 — 用法：dougong entrypath show|regen|off")
		return cliExitUsage
	}
	sub := args[0]
	if sub != "show" && sub != "regen" && sub != "off" {
		fmt.Fprintf(env.stdout, "AUTH_012: 参数错误 — 未知子命令 %q（可用：show / regen / off）\n", sub)
		return cliExitUsage
	}

	svc, err := openCLIServices(ctx, env)
	if err != nil {
		fmt.Fprintf(env.stdout, "打开面板数据失败: %v\n", err)
		return cliExitFail
	}
	defer svc.close()

	switch sub {
	case "show":
		pc, err := svc.panelSvc.Bootstrap(ctx)
		if err != nil {
			fmt.Fprintf(env.stdout, "读取安全入口状态失败: %v\n", err)
			return cliExitFail
		}
		state := "关闭"
		if pc.EntryPathEnabled {
			state = "开启"
		}
		pathVal := "（无）"
		if pc.EntryPath != nil && *pc.EntryPath != "" {
			pathVal = "/" + *pc.EntryPath
		}
		fmt.Fprintf(env.stdout, "安全入口开关: %s\n", state)
		fmt.Fprintf(env.stdout, "安全入口路径: %s\n", pathVal)
		return cliExitOK
	case "regen":
		if _, err := svc.panelSvc.Bootstrap(ctx); err != nil { // 幂等单例自检（I-P01）
			fmt.Fprintf(env.stdout, "读取面板配置失败: %v\n", err)
			return cliExitFail
		}
		newPath, err := svc.panelSvc.EntryPathRegenViaCLI(ctx, env.operator)
		if err != nil {
			fmt.Fprintf(env.stdout, "重新生成安全入口失败: %v\n", err)
			return cliExitFail
		}
		fmt.Fprintf(env.stdout, "安全入口路径已重新生成: /%s\n", newPath)
		fmt.Fprintln(env.stdout, restartHint)
		return cliExitOK
	default: // "off"（上方白名单已收敛）
		if _, err := svc.panelSvc.Bootstrap(ctx); err != nil { // 幂等单例自检（I-P01）
			fmt.Fprintf(env.stdout, "读取面板配置失败: %v\n", err)
			return cliExitFail
		}
		changed, err := svc.panelSvc.EntryPathOffViaCLI(ctx, env.operator)
		if err != nil {
			fmt.Fprintf(env.stdout, "关闭安全入口失败: %v\n", err)
			return cliExitFail
		}
		if changed {
			fmt.Fprintln(env.stdout, "安全入口已关闭")
		} else {
			fmt.Fprintln(env.stdout, "安全入口已是关闭状态")
		}
		fmt.Fprintln(env.stdout, restartHint)
		return cliExitOK
	}
}

// cliService `dougong service start|stop|restart|status`（写=OS 层治理）：
// systemctl 参数数组调用 dougong.service（R-08/FR-02）；status 输出 systemd
// 状态摘要；非 0 退出码透传 systemctl 失败（stdout/stderr 呈现原因）；不产生
// 面板数据变更、不输出 restart 提示（本命令族即启停治理本身）。
func cliService(ctx context.Context, env cliEnv, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(env.stdout, "AUTH_012: 参数错误 — 用法：dougong service start|stop|restart|status")
		return cliExitUsage
	}
	verb := args[0]
	var ctlArgs []string
	switch verb {
	case "start", "stop", "restart":
		ctlArgs = []string{verb, cliUnitName}
	case "status":
		ctlArgs = []string{"status", cliUnitName, "--no-pager"}
	default:
		fmt.Fprintf(env.stdout, "AUTH_012: 参数错误 — 未知子命令 %q（可用：start / stop / restart / status）\n", verb)
		return cliExitUsage
	}

	sctx, cancel := context.WithTimeout(ctx, cliSystemctlTimeout)
	defer cancel()
	res := env.systemctl(sctx, ctlArgs...)
	out := strings.TrimSpace(res.Stdout)
	if out == "" {
		out = strings.TrimSpace(res.Stderr)
	}
	if out != "" {
		fmt.Fprintln(env.stdout, out)
	}
	switch {
	case res.Err != nil:
		fmt.Fprintf(env.stdout, "systemctl 执行失败: %v\n", res.Err)
		return cliExitFail
	case res.ExitCode != 0:
		return res.ExitCode // 非 0 透传 systemctl 失败
	}
	return cliExitOK
}

// printErrcode 若 err 为业务错误码则向 w 打印「code: message」与逐字段原因
// （AUTH_012 fields），返回 (symbol, 是否参数校验语义)。非 errcode 错误返回
// ("", false) 由调用方兜底打印。
func printErrcode(w io.Writer, err error) (string, bool) {
	var ec *errcode.Error
	if !errors.As(err, &ec) {
		return "", false
	}
	fmt.Fprintf(w, "%s: %s\n", ec.Symbol(), ec.Message())
	if vd, ok := ec.Data().(errcode.ValidationData); ok {
		for _, f := range vd.Fields {
			fmt.Fprintf(w, "  %s: %s\n", f.Field, f.Message)
		}
	}
	return ec.Symbol(), ec.Symbol() == errcode.ErrValidation.Symbol()
}

// cliRenameUser `dougong rename-user -username <old> -new <new>`（写，紧急救援）：
// 终端强制改面板用户名（遗忘用户名/被锁时救回访问）；复用 auth RenameUserViaCLI
// （A10 校验/会话级联吊销/审计 username_changed security source=cli）。原用户不存在
// =退出码 1；新名占用/校验失败按 errcode 语义（AUTH_012 退出码 2）。
func cliRenameUser(ctx context.Context, env cliEnv, args []string) int {
	fs := flag.NewFlagSet("rename-user", flag.ContinueOnError)
	fs.SetOutput(env.stdout)
	username := fs.String("username", "", "原用户名（必填）")
	newName := fs.String("new", "", "新用户名（必填）")
	if err := fs.Parse(args); err != nil {
		return cliExitUsage
	}
	if strings.TrimSpace(*username) == "" || strings.TrimSpace(*newName) == "" {
		fmt.Fprintln(env.stdout, "AUTH_012: 参数错误 — 用法：dougong rename-user -username <原用户名> -new <新用户名>")
		return cliExitUsage
	}
	svc, err := openCLIServices(ctx, env)
	if err != nil {
		fmt.Fprintf(env.stdout, "打开面板数据失败: %v\n", err)
		return cliExitFail
	}
	defer svc.close()

	res, err := svc.authSvc.RenameUserViaCLI(ctx, env.operator, *username, *newName)
	if err != nil {
		if errors.Is(err, authsvc.ErrUserNotFound) {
			fmt.Fprintf(env.stdout, "原用户 %q 不存在\n", *username)
			return cliExitFail
		}
		if _, usage := printErrcode(env.stdout, err); usage {
			return cliExitUsage
		}
		var ec *errcode.Error
		if errors.As(err, &ec) {
			return cliExitFail
		}
		fmt.Fprintf(env.stdout, "修改用户名失败: %v\n", err)
		return cliExitFail
	}
	if !res.Changed {
		fmt.Fprintf(env.stdout, "用户名已是 %s，无需变更\n", res.NewUsername)
		return cliExitOK
	}
	fmt.Fprintf(env.stdout, "用户名已修改: %s → %s（其全部面板会话已吊销）\n", res.OldUsername, res.NewUsername)
	fmt.Fprintln(env.stdout, restartHint)
	return cliExitOK
}

// cliUnbindDomain `dougong unbind-domain`（写，紧急救援）：解除面板域名绑定
// （bind_domain 置 NULL）——绑域名后 DNS 未生效/域名失联被锁在外时，恢复 IP 直连
// 访问。复用 panel UnbindDomainViaCLI（即时生效 HostGuard 下一请求放行全部 Host，
// 审计 panel_config_changed source=cli）。未绑定=零写入如实提示。
func cliUnbindDomain(ctx context.Context, env cliEnv, args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(env.stdout, "AUTH_012: 参数错误 — 用法：dougong unbind-domain")
		return cliExitUsage
	}
	svc, err := openCLIServices(ctx, env)
	if err != nil {
		fmt.Fprintf(env.stdout, "打开面板数据失败: %v\n", err)
		return cliExitFail
	}
	defer svc.close()

	if _, err := svc.panelSvc.Bootstrap(ctx); err != nil { // 幂等单例自检（I-P01）
		fmt.Fprintf(env.stdout, "读取面板配置失败: %v\n", err)
		return cliExitFail
	}
	before, changed, err := svc.panelSvc.UnbindDomainViaCLI(ctx, env.operator)
	if err != nil {
		if _, usage := printErrcode(env.stdout, err); usage {
			return cliExitUsage
		}
		var ec *errcode.Error
		if errors.As(err, &ec) {
			return cliExitFail
		}
		fmt.Fprintf(env.stdout, "解除域名绑定失败: %v\n", err)
		return cliExitFail
	}
	if !changed {
		fmt.Fprintln(env.stdout, "面板未绑定域名，无需解除")
		return cliExitOK
	}
	fmt.Fprintf(env.stdout, "已解除域名绑定（原域名 %s）——现在可用 IP 直接访问面板\n", before)
	fmt.Fprintln(env.stdout, restartHint)
	return cliExitOK
}
