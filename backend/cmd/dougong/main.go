// dougong 是斗拱面板单二进制入口（Go 1.22，gin + modernc.org/sqlite + slog）。
//
// 用法：
//
//	dougong                                   交互终端→管理菜单（宝塔 bt 风格，含紧急救援）；
//	                                         非交互（systemd/管道）→启动面板服务
//	dougong start|serve [-config dougong.json] 显式启动面板服务（systemd ExecStart 推荐；
//	                                         生产默认静音 gin，DOUGONG_DEBUG=1/GIN_MODE 恢复）
//	dougong init -username <u> [-password <p>] CLI 初始化首个 admin（§4.4.4；
//	                                         密码推荐经 DOUGONG_INIT_PASSWORD 注入）
//	dougong version                           打印版本/commit/构建时间
//	dougong status                            systemd 状态+初始化态+访问 URL（CLI 通道，
//	                                         v2.1 4.1.4/ADR-030，详见 cmd/dougong/cli.go）
//	dougong info                              版本/监听/安全入口/HTTPS/数据目录
//	dougong reset-password -username <u>      重置密码（新密码仅经 DOUGONG_INIT_PASSWORD）
//	dougong rename-user -username <old> -new <new> 强制改面板用户名（紧急救援）
//	dougong unbind-domain                     解除域名绑定（紧急，恢复 IP 直连访问）
//	dougong set-port <port>                   改监听端口（1024..65535，重启生效）
//	dougong entrypath show|regen|off          安全入口查看/重生成/关闭
//	dougong service start|stop|restart|status systemctl 治理 dougong.service
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	acmegw "github.com/dougong/dougong/internal/adapter/acme"
	"github.com/dougong/dougong/internal/adapter/appx"
	"github.com/dougong/dougong/internal/adapter/backupx"
	"github.com/dougong/dougong/internal/adapter/cronx"
	"github.com/dougong/dougong/internal/adapter/dbmongo"
	"github.com/dougong/dougong/internal/adapter/dbmysql"
	"github.com/dougong/dougong/internal/adapter/dbredis"
	"github.com/dougong/dougong/internal/adapter/dockerx"
	"github.com/dougong/dougong/internal/adapter/envpack"
	"github.com/dougong/dougong/internal/adapter/firewallx"
	"github.com/dougong/dougong/internal/adapter/fslocal"
	"github.com/dougong/dougong/internal/adapter/netlisten"
	"github.com/dougong/dougong/internal/adapter/nginxx"
	"github.com/dougong/dougong/internal/adapter/phpfpm"
	"github.com/dougong/dougong/internal/adapter/pmx"
	"github.com/dougong/dougong/internal/adapter/ptyx"
	"github.com/dougong/dougong/internal/adapter/sysd"
	"github.com/dougong/dougong/internal/adapter/sysuser"
	"github.com/dougong/dougong/internal/adapter/toolx"
	"github.com/dougong/dougong/internal/adapter/webauthnx"
	"github.com/dougong/dougong/internal/api"
	appapi "github.com/dougong/dougong/internal/api/appmgmt"
	authapi "github.com/dougong/dougong/internal/api/auth"
	cronapi "github.com/dougong/dougong/internal/api/cronback"
	dbapi "github.com/dougong/dougong/internal/api/dbmgmt"
	dbtoolapi "github.com/dougong/dougong/internal/api/dbtools"
	dockerapi "github.com/dougong/dougong/internal/api/dockermgmt"
	envapi "github.com/dougong/dougong/internal/api/envmgmt"
	fileapi "github.com/dougong/dougong/internal/api/fileman"
	fwapi "github.com/dougong/dougong/internal/api/firewall"
	"github.com/dougong/dougong/internal/api/middleware"
	monitoringapi "github.com/dougong/dougong/internal/api/monitoring"
	panelapi "github.com/dougong/dougong/internal/api/panel"
	svcapi "github.com/dougong/dougong/internal/api/servicemgmt"
	siteapi "github.com/dougong/dougong/internal/api/sitemgmt"
	terminalapi "github.com/dougong/dougong/internal/api/terminal"
	"github.com/dougong/dougong/internal/config"
	"github.com/dougong/dougong/internal/errcode"
	appsvc "github.com/dougong/dougong/internal/service/appmgmt"
	auditsvc "github.com/dougong/dougong/internal/service/audit"
	authsvc "github.com/dougong/dougong/internal/service/auth"
	cronbacksvc "github.com/dougong/dougong/internal/service/cronback"
	dbmgmtsvc "github.com/dougong/dougong/internal/service/dbmgmt"
	dbtoolssvc "github.com/dougong/dougong/internal/service/dbtools"
	dockersvc "github.com/dougong/dougong/internal/service/dockermgmt"
	envsvc "github.com/dougong/dougong/internal/service/envmgmt"
	filemansvc "github.com/dougong/dougong/internal/service/fileman"
	firewallsvc "github.com/dougong/dougong/internal/service/firewall"
	monitoringsvc "github.com/dougong/dougong/internal/service/monitoring"
	panelsvc "github.com/dougong/dougong/internal/service/panel"
	svcsvc "github.com/dougong/dougong/internal/service/servicemgmt"
	sitesvc "github.com/dougong/dougong/internal/service/sitemgmt"
	terminalsvc "github.com/dougong/dougong/internal/service/terminal"
	"github.com/dougong/dougong/internal/store"
	storeapp "github.com/dougong/dougong/internal/store/appmgmt"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	storecronback "github.com/dougong/dougong/internal/store/cronback"
	storedbmgmt "github.com/dougong/dougong/internal/store/dbmgmt"
	storedbtools "github.com/dougong/dougong/internal/store/dbtools"
	storeenv "github.com/dougong/dougong/internal/store/envmgmt"
	storefileman "github.com/dougong/dougong/internal/store/fileman"
	storepanel "github.com/dougong/dougong/internal/store/panel"
	storesitemgmt "github.com/dougong/dougong/internal/store/sitemgmt"

	"github.com/gin-gonic/gin"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	args := os.Args[1:]
	if len(args) > 0 && args[0] == "init" {
		if err := runInit(args[1:]); err != nil {
			slog.Error("dougong init failed", "err", err)
			os.Exit(1)
		}
		return
	}
	if len(args) > 0 && args[0] == "version" {
		v := panelsvc.GetVersionInfo()
		fmt.Printf("dougong %s (commit %s, built %s)\n", v.Version, v.GitCommit, v.BuildTime)
		return
	}
	// 面板管理 CLI 命令族（v2.1 4.1.4「CLI 入口」，ADR-030/D-P17 + 2026-11-20 救援扩展）：
	// 终端本地通道，进程内复用 service 层（无 HTTP 绕行）。退出码 0/2/1（systemctl 失败透传）。
	explicitStart := false
	if len(args) > 0 {
		switch args[0] {
		case "status", "info", "reset-password", "rename-user", "unbind-domain", "disable-2fa", "visit-urls", "set-port", "entrypath", "service":
			os.Exit(runCLI(args, newCLIEnv()))
		case "start", "serve":
			// 显式启动别名（systemd ExecStart 推荐 dougong start）；剥离后走启动路径。
			explicitStart = true
			args = args[1:]
		}
	}
	// 裸 dougong（无子命令）：交互终端→管理菜单（宝塔 bt 风格，含紧急救援）；
	// 非交互（systemd ExecStart=/usr/local/bin/dougong、管道、CI——stdin 非 TTY）→
	// 启动面板服务（保既有部署兼容，无需迁移 systemd unit）。
	if len(args) == 0 && !explicitStart && isInteractive() {
		os.Exit(cliMenu(newCLIEnv(), os.Stdin))
	}
	// 生产二进制默认静音 gin 调试横幅；DOUGONG_DEBUG=1 保留调试输出。
	if os.Getenv("GIN_MODE") == "" && os.Getenv("DOUGONG_DEBUG") == "" {
		gin.SetMode(gin.ReleaseMode)
	}
	if err := run(args); err != nil {
		slog.Error("dougong exit", "err", err)
		os.Exit(1)
	}
}

// run 启动服务：配置 → 依赖装配（store → service → api）→ HTTP + 审计清理
// 后台任务 → 优雅退出（停入站 → 排空任务 → 关库）。
func run(args []string) error {
	fs := flag.NewFlagSet("dougong", flag.ContinueOnError)
	cfgPath := fs.String("config", resolveConfigPath(""), "配置文件路径（JSON，不存在则用默认值；缺省优先已安装服务数据目录）")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	// db_path 锚定到配置文件目录（config.ResolveDBPath 语义；与 CLI 同锚点）。
	cfg.DBPath = config.ResolveDBPath(*cfgPath, cfg.DBPath)
	absTimeout, err := cfg.AbsoluteTimeoutDur()
	if err != nil {
		return err
	}
	idleTimeout, err := cfg.IdleTimeoutDur()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 依赖装配（自底向上）。
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	if err := store.Migrate(ctx, db); err != nil {
		closeErr := db.Close()
		return errors.Join(err, closeErr)
	}
	repo := storeauth.New(db)
	// 启动自检：保证 init_state 单行存在（SQL 清单 #1，幂等）。
	if err := repo.Init().EnsureRow(ctx); err != nil {
		closeErr := db.Close()
		return errors.Join(fmt.Errorf("ensure init state: %w", err), closeErr)
	}

	hasher, err := authsvc.NewBcryptHasher(12)
	if err != nil {
		closeErr := db.Close()
		return errors.Join(err, closeErr)
	}
	authSvc := authsvc.New(repo, hasher, authsvc.NewTotpService(nil),
		authsvc.SessionPolicy{AbsoluteTimeout: absTimeout, IdleTimeout: idleTimeout},
		authsvc.WithWebAuthnFactory(func(rpID string, origins []string) authsvc.WebAuthnProvider {
			p, err := webauthnx.New(rpID, origins)
			if err != nil {
				slog.Warn("webauthn config invalid", "rpID", rpID, "err", err)
				return nil
			}
			return p
		}))
	auditSvc := auditsvc.New(repo.Audit(), time.Now)

	// panel-core 启动自检（I-P01）：panel_config 单例落库（缺失时随机高位
	// 端口——安装期语义兜底）+ 进程启动监听基线（D-P02 pendingRestart 判定）。
	// v2.0（ADR-029）：面板证书 ACME 通道注入——与 site-mgmt 共用同一
	// acme.Gateway（账户密钥/CA 目录/webroot 同源，D-P13）与 nginxx.Gateway
	//（挑战 location 安全写回通道，D-P15），故网关在此提前构造。
	panelDataDir := filepath.Dir(cfg.DBPath)
	acmeGw := acmegw.NewGateway(cfg.ACME.DirectoryURL, panelDataDir)
	nginxxGw := nginxx.NewGateway(cfg.Sites.NginxBin, cfg.Sites.ConfRoot, sitesvc.ScanDirNames)
	panelStore := storepanel.New(db)
	panelSvc := panelsvc.New(panelStore,
		panelsvc.WithSessionPolicy(panelsvc.SessionPolicy{
			AbsoluteTimeout: absTimeout, IdleTimeout: idleTimeout,
		}),
		panelsvc.WithAcmeChannel(acmeGw, nginxxGw, panelDataDir),
		// 版本源接线（D-P09，A-P8 延期落地）：GitHub Releases 最新 tag，检查更新可用。
		panelsvc.WithVersionSource(&panelsvc.GitHubReleaseSource{}))
	bootCfg, err := panelSvc.Bootstrap(ctx)
	if err != nil {
		closeErr := db.Close()
		return errors.Join(fmt.Errorf("panel bootstrap: %w", err), closeErr)
	}
	// 构建期菜单登记（D-P07：ModuleMenuRegistry 单一数据源；业务模块随
	// 其 Module 挂载在此追加；面板设置/关于为本模块自身页面，PageSpec 5.1/5.2）。
	panelSvc.Menus().Register(
		panelsvc.MenuItem{Key: "panel-settings", Title: "面板设置", Path: "/settings",
			Icon: "settings", Order: 900, Visibility: []string{storeauth.RoleAdmin, storeauth.RoleReadonly}},
		panelsvc.MenuItem{Key: "panel-about", Title: "关于", Path: "/about",
			Icon: "info", Order: 910, Visibility: []string{storeauth.RoleAdmin, storeauth.RoleReadonly}},
	)

	// account-auth 菜单分组（v2.2 D-P18，v0.1.2 反馈第 6 项）：「用户与安全」
	// 分组四子项两角色可达——设计侧 PageSpec account-auth 第 7 节本有菜单登记，
	// 实现侧此前未登记致 /users 不可达，本增量实现回归设计并升格为分组
	//（MenuItem.children 两级：分组 path 空、visibility 并集）。
	panelSvc.Menus().Register(
		panelsvc.MenuItem{Key: "user-security", Title: "用户与安全", Path: "",
			Icon: "people", Order: 150, Visibility: []string{storeauth.RoleAdmin, storeauth.RoleReadonly},
			Children: []panelsvc.MenuItem{
				{Key: "users", Title: "用户管理", Path: "/users", Icon: "person", Order: 10,
					Visibility: []string{storeauth.RoleAdmin, storeauth.RoleReadonly}},
				{Key: "sessions", Title: "在线会话", Path: "/sessions", Icon: "time", Order: 20,
					Visibility: []string{storeauth.RoleAdmin, storeauth.RoleReadonly}},
				{Key: "audit-logs", Title: "审计日志", Path: "/audit-logs", Icon: "document", Order: 30,
					Visibility: []string{storeauth.RoleAdmin, storeauth.RoleReadonly}},
				{Key: "profile", Title: "个人安全设置", Path: "/profile", Icon: "person-circle", Order: 40,
					Visibility: []string{storeauth.RoleAdmin, storeauth.RoleReadonly}},
			}},
	)

	// monitoring：零持久化监控服务（ADR-014 无表无 DDL；缓冲随服务创建，采集
	// 链路下方以后台任务启动）。菜单登记对齐 PageSpec 第 7 节路由（A-M12）。
	monSvc := monitoringsvc.NewService()
	panelSvc.Menus().Register(
		panelsvc.MenuItem{Key: "monitoring", Title: "监控概览", Path: "/monitoring",
			Icon: "pulse", Order: 100, Visibility: []string{storeauth.RoleAdmin, storeauth.RoleReadonly}},
		panelsvc.MenuItem{Key: "monitoring-history", Title: "历史趋势", Path: "/monitoring/history",
			Icon: "trend", Order: 110, Visibility: []string{storeauth.RoleAdmin, storeauth.RoleReadonly}},
	)

	// service-mgmt：systemd 服务管理（D-07 零持久化 dbus 投影；无后台任务——
	// D-03 按需拉取，连接惰性建立）。菜单登记对齐 PageSpec 第 7 节路由
	// /services（admin/readonly 均可见——readonly 可进入查看，FL 附录A/ADR-018）。
	sysdGateway := sysd.NewGateway(svcsvc.ActionWaitLimit)
	svcMgmtSvc := svcsvc.New(sysdGateway, auditSvc)
	panelSvc.Menus().Register(
		panelsvc.MenuItem{Key: "services", Title: "服务管理", Path: "/services",
			Icon: "services", Order: 200, Visibility: []string{storeauth.RoleAdmin, storeauth.RoleReadonly}},
	)

	// file-manager：文件管理器（trash_entry + fs_path_scope 两表；PathScopeGuard
	// 受管根白名单内存副本 R-04/D-02；FSGateway 本机文件系统通道，fslocal 纯
	// Go 实现无外部命令 R-08/R-09）。启动自检：fs_path_scope 单行落库（SQL 清单
	// #1，幂等）+ 副本加载（#2）。菜单对齐 PageSpec 第 7 节路由（/files、
	// /files/trash 两角色可见——entry/list 与 trash/list 全可看 ADR-018；
	// edit 页为 /files 内导航非菜单项）。
	fileStore := storefileman.New(db)
	if err := fileStore.Scope().EnsureRow(ctx, time.Now()); err != nil {
		closeErr := db.Close()
		return errors.Join(fmt.Errorf("ensure fs path scope: %w", err), closeErr)
	}
	scopeRow, err := fileStore.Scope().Get(ctx)
	if err != nil {
		closeErr := db.Close()
		return errors.Join(fmt.Errorf("load fs path scope: %w", err), closeErr)
	}
	fsGuard := filemansvc.NewGuard(filemansvc.ScopeConfig{
		RootPath: scopeRow.RootPath, ExcludedDirs: scopeRow.ExcludedDirs})
	fileSvc := filemansvc.New(fslocal.New(), fileStore, auditSvc, fsGuard)
	panelSvc.Menus().Register(
		panelsvc.MenuItem{Key: "files", Title: "文件管理", Path: "/files",
			Icon: "files", Order: 300, Visibility: []string{storeauth.RoleAdmin, storeauth.RoleReadonly}},
		panelsvc.MenuItem{Key: "files-trash", Title: "回收站", Path: "/files/trash",
			Icon: "trash", Order: 310, Visibility: []string{storeauth.RoleAdmin, storeauth.RoleReadonly}},
	)

	// site-mgmt：网站管理（核心卖点，M3）——nginxx 自研解析器（ADR-010 承载
	// round-trip 保真 span 模型）+ WritePipeline 安全写入管线（R-06 四步）+
	// AcmeGateway ACL（D-09，R-09 零遥测唯一外呼：用户主动 ACME 申请）+
	// PhpFpmGateway pool 实况（D-14）+ 自动续期每日调度（4.1.3，A-SM15）。
	// 菜单对齐 PageSpec 第 7 节（/sites、/certificates 两角色可见——查看类
	// 全可看 D0 口径 + ADR-018）。
	siteStore := storesitemgmt.New(db)
	phpfpmGw := phpfpm.NewGateway()
	// v2.0（⚠️A20/Gate 1 改判）：DNS 凭据保险库——独立密钥文件 dns-cred.key
	// 0600（沿 db-cred.key CredentialVault 先例，风险域隔离）；打开失败属
	// 启动硬错误（凭据功能不可用须显式暴露，不静默降级）。
	dnsVault, err := sitesvc.OpenDnsCredentialVault(panelDataDir)
	if err != nil {
		closeErr := db.Close()
		return errors.Join(fmt.Errorf("open dns credential vault: %w", err), closeErr)
	}
	siteSvc := sitesvc.New(nginxxGw, phpfpmGw, acmeGw, siteStore, auditSvc,
		sitesvc.Options{DataDir: panelDataDir, DnsVault: dnsVault})
	panelSvc.Menus().Register(
		panelsvc.MenuItem{Key: "sites", Title: "网站管理", Path: "/sites",
			Icon: "globe", Order: 400, Visibility: []string{storeauth.RoleAdmin, storeauth.RoleReadonly}},
		panelsvc.MenuItem{Key: "certificates", Title: "SSL 证书", Path: "/certificates",
			Icon: "lock", Order: 410, Visibility: []string{storeauth.RoleAdmin, storeauth.RoleReadonly}},
	)

	// database-mgmt：数据库管理（M4）——db_instance 合一档案表（D-01）+
	// CredentialVault 凭据密文（D-02：密钥 db-cred.key 0600 独立于 SQLite，
	// 首次启动随机生成）+ dbmysql/dbredis CLI 网关（mysql/mysqldump/redis-cli
	// 字面量参数数组 R-08；密码经 MYSQL_PWD/REDISCLI_AUTH 环境变量注入子进程，
	// 不进 argv）+ 导出/导入在途互斥（D-12 进程内任务表）。无新增后台
	// goroutine（首次探测由 list 触发 sync.Once，D-07）。菜单对齐 PageSpec
	// 路由 /databases（两角色可见——查看类接口全可看 ADR-018）。
	dbStore := storedbmgmt.New(db)
	dbVault, err := dbmgmtsvc.OpenVault(panelDataDir)
	if err != nil {
		closeErr := db.Close()
		return errors.Join(fmt.Errorf("open credential vault: %w", err), closeErr)
	}
	mysqlGw := dbmysql.NewGateway(cfg.DB.MySQLBin, cfg.DB.MysqldumpBin)
	redisGw := dbredis.NewGateway(cfg.DB.RedisCliBin)
	mongoGw := dbmongo.NewGateway() // 进程内 mongo-driver v2 通道（D-15：无二进制依赖，v0.1.4 反馈第 3 项）
	dbSvc := dbmgmtsvc.New(dbmgmtsvc.AdaptStore(dbStore), mysqlGw, redisGw, mongoGw, dbVault, auditSvc)
	panelSvc.Menus().Register(
		panelsvc.MenuItem{Key: "database", Title: "数据库", Path: "/databases",
			Icon: "database", Order: 500, Visibility: []string{storeauth.RoleAdmin, storeauth.RoleReadonly}},
	)

	// db-tools：数据库工具集成（phpMyAdmin/mongo-express，ADR-032）——菜单构建期
	// 登记（D-P18，Order 位于数据库管理之后）；两角色同项（ADR-018 目录/任务可看，
	// 工具访问整体 admin 专属 ⚠️A10：readonly 操作面不渲染 + 工具路径 gating 网络层
	// 闸门不可达）。HTTP 模块（/dbtool 路由组）随后端 B 阶段落仓挂载。
	panelSvc.Menus().Register(
		panelsvc.MenuItem{Key: "db-tools", Title: "数据库工具", Path: "/db-tools",
			Icon: "dbtool", Order: 550, Visibility: []string{storeauth.RoleAdmin, storeauth.RoleReadonly}},
	)

	// terminal：Web 终端（M6）——零持久化内存态会话（I-04 无表无 DDL 无 SQL）；
	// ptyx PTY 网关（creack/pty，参数数组 R-08）+ sysuser 系统用户实况直读
	//（D-02）+ VerifyWs 复核联动（D-07）+ 开合两段审计（D-06）；限流 ADR-023
	// 编译期常量（每用户 5 / 空闲 30min / 硬上限 8h）。生命周期扫描器（10s
	// tick：空闲/硬上限/30s 复核/30s 心跳）下方以后台任务启动。菜单 admin
	// 专属——readonly 整域禁用（ADR-018），visibility 仅 admin。
	termSvc := terminalsvc.New(ptyx.New(), sysuser.New(), auditSvc, authSvc,
		terminalapi.NewPanelDirectory(authSvc), time.Now)
	panelSvc.Menus().Register(
		panelsvc.MenuItem{Key: "terminal", Title: "终端", Path: "/terminal",
			Icon: "terminal", Order: 600, Visibility: []string{storeauth.RoleAdmin}},
	)

	// 监听绑定（D-P02）：panel_config listen.* 为唯一事实源（all=0.0.0.0 为
	// api §3.4 枚举表明文映射，非业务代码写死）；DOUGONG_ADDR 显式注入时
	// 覆盖（开发/测试逃生通道，tasks.md A-P5 裁定）。提前解析——防火墙面板
	// 端口保护基准（A-W9）与下方 http.Server 共用同一绑定结论。
	addr := panelListenAddr(bootCfg)
	if v := os.Getenv("DOUGONG_ADDR"); v != "" {
		addr = v
	}

	// firewall：主机网络入口安全（M7）——零持久化投影（D-10 无表无 SQL）；
	// ufw/firewalld 经 firewallx 命令通道（R-08 字面量参数数组，单命令 30s
	// 上限）；监听端口对照经 netlisten（gopsutil 统一取数，D1 口径-3）；
	// 面板端口保护基准=进程实际绑定端口静态注入（A-W9）。菜单对齐 PageSpec
	// 路由 /firewall 两角色可见（查看类全可看 ADR-018）。无后台 goroutine
	//（D-01 按需即时探测）。
	fwPort, err := panelListenPort(addr)
	if err != nil {
		closeErr := db.Close()
		return errors.Join(fmt.Errorf("resolve panel listen port: %w", err), closeErr)
	}
	fwSvc := firewallsvc.New(firewallx.New(), netlisten.New(),
		firewallsvc.StaticPanelListen(fwPort), auditSvc)
	panelSvc.Menus().Register(
		panelsvc.MenuItem{Key: "firewall", Title: "防火墙", Path: "/firewall",
			Icon: "shield", Order: 700, Visibility: []string{storeauth.RoleAdmin, storeauth.RoleReadonly}},
	)

	// docker-mgmt：容器管理（M9）——零持久化投影（D-10 无表无 SQL）；官方
	// Docker SDK 网关（dockerx，unix:///var/run/docker.sock，DOCKER_HOST 可
	// 覆盖——运维逃生通道）+ Compose CLI 参数数组通道（R-08/ADR-013）；
	// DaemonGuard 业务面前置（D-01 即时探测）。菜单对齐 PageSpec 路由
	// /docker 两角色可见（查看类全可看 ADR-018）。无后台 goroutine（探测/
	// 读取/CLI 均按需即时执行，I-01）。
	dockerGw, err := dockerx.New()
	if err != nil {
		closeErr := db.Close()
		return errors.Join(fmt.Errorf("docker gateway: %w", err), closeErr)
	}
	composeRunner := dockerx.NewComposeRunner(dockerGw)
	// Compose 项目落盘根来自配置 docker.compose_root（默认 /opt/dougong-compose；
	// 测试隔离/本机验证经 DOUGONG_COMPOSE_ROOT 覆盖，生产保持默认）。
	dockerSvc := dockersvc.New(dockerGw, composeRunner, auditSvc,
		dockersvc.WithComposeRootDir(cfg.Docker.ComposeRoot),
		// 应用建站消费 site-mgmt 反向代理建站（ADR-033 S-62-05；与 app-mgmt
		// 建站联动同一通道）。
		dockersvc.WithSiteLinker(dockersvc.AdaptSiteLinker(siteSvc)))
	panelSvc.Menus().Register(
		panelsvc.MenuItem{Key: "docker", Title: "容器管理", Path: "/docker",
			Icon: "container", Order: 800, Visibility: []string{storeauth.RoleAdmin, storeauth.RoleReadonly}},
	)

	// cron-backup：计划任务与备份（M8）——cron_job 零台账（crontab 实况投影，
	// D2 口径-1；cronx 承载 crontab 读写/日志尾部/代执行，R-08 参数数组）+
	// backup_job/backup_run 两表（idx_run_running 部分唯一索引执行互斥 I-01）+
	// DbGateway.DumpDatabase 消费登记兑现（dbSvc OHS，凭据解密在其域内）+
	// BackupScheduler 分钟 tick（D-13）与 BackupExecutor 异步执行（D-07，调度/
	// 手动共用通道 A-C9）。启动恢复（D-08）：全部孤儿 running 置 failure。菜单
	// /cron 与 /cron/backup 两角色可见（PageSpec 第 7 节，readonly 可看 ADR-018）。
	cronStore := storecronback.New(db)
	backupExec := backupx.NewExecutor(cfg.Cron.RsyncBin, dbSvc) // DumpSource=dbmgmt OHS
	cronSvc := cronbacksvc.New(cronx.New(cfg.Cron.CrontabBin),
		cronx.NewLogSource(cfg.Cron.JournalctlBin),
		cronx.NewRunner(cfg.Cron.ShellBin),
		backupExec, cronbacksvc.AdaptStore(cronStore), auditSvc, dbSvc, panelDataDir)
	if orphans, oerr := cronSvc.RecoverOrphans(ctx); oerr != nil {
		closeErr := db.Close()
		return errors.Join(fmt.Errorf("recover orphan backup runs: %w", oerr), closeErr)
	} else if orphans > 0 {
		slog.Warn("recovered orphan backup runs", "count", orphans)
	}
	panelSvc.Menus().Register(
		panelsvc.MenuItem{Key: "cron", Title: "计划任务", Path: "/cron",
			Icon: "cron", Order: 750, Visibility: []string{storeauth.RoleAdmin, storeauth.RoleReadonly}},
		panelsvc.MenuItem{Key: "cron-backup", Title: "备份", Path: "/cron/backup",
			Icon: "backup", Order: 760, Visibility: []string{storeauth.RoleAdmin, storeauth.RoleReadonly}},
	)

	// env-mgmt：环境管理（v0.1.2 用户反馈第 4 项新模块）——env_install_task
	// 四态异步任务台账（⚠️A3，迁移 008）+ envpack 包管理器双系通道（D-03
	// exec.LookPath；R-08 参数数组，单命令 10min 上限 D-10）+ 服务操作复用
	// sysdGateway（D-09，与 service-mgmt 共享适配层资产）；启动恢复 I-04
	//（running→failed「面板重启中断」）；任务 worker 为创建即触发的有限时
	// goroutine（无长驻循环，G-06）。菜单对齐 PageSpec 路由 /env 两角色可见
	//（查看类全可看 ADR-018）。
	envStore := storeenv.New(db)
	// 安装后配置就绪（v1.2）：db-mgmt 引导窄接口注入（与治理网关同一适配器实例）
	// + env-mgmt 消费 db-mgmt BootstrapAfterInstall OHS（单向依赖，D-15）。
	dbSvc.SetBootstrapAdapters(mysqlGw, redisGw, mongoGw)
	envSvc := envsvc.New(envpack.NewGateway(), sysdGateway, envStore, envsvc.Options{Bootstrap: dbSvc})
	envSvc.RecoverInFlight(ctx)
	panelSvc.Menus().Register(
		panelsvc.MenuItem{Key: "env", Title: "环境管理", Path: "/env",
			Icon: "env", Order: 850, Visibility: []string{storeauth.RoleAdmin, storeauth.RoleReadonly}},
	)

	// app-mgmt：运行时项目托管（ADR-033，api-app-mgmt v1.2）——app/app_task
	// 双聚合（迁移 015/018）+ appx 适配器（venv/pip/npm 参数数组 R-08/单元安全
	// 写入 备份→原子写→verify→失败回滚 D-06/journalctl 日志）+ pmx 适配器
	// （Node 项目由 PM2 托管进程：注册/启停/保存/jlist 投影/日志文件尾部）+
	// 删除保守保全（purgeDir 显式，绝不静默删代码 ⚠️A5）+ 跨模块 OHS 消费
	// （env-mgmt PythonReadiness / NodeReadiness 就绪度读契约 / site-mgmt 建站
	// D-12；service-mgmt SysdGateway 共享适配层双入口 ⚠️A12）。启动恢复 I-08；
	// 任务 worker 为创建即触发的有限时 goroutine（G-06）。
	// 不再登记独立菜单（ADR-033 Q2）：面板由网站管理页「Python 项目 / Node
	// 项目」分栏承载；路由 /app 重定向到 /sites。容器运行形态已下线
	// （D-app-04），容器化部署归 docker-mgmt「应用」页签。
	appStore := storeapp.New(db)
	appSvc := appsvc.New(appx.New(), sysdGateway, appStore, appsvc.Options{
		EnvReady: envSvc,                          // PythonReadiness / NodeReadiness 读契约 OHS（D-08/D-16）
		SiteLink: appsvc.AdaptSiteLinker(siteSvc), // 建站 OHS 消费（D-12）
		Node:     appx.New(),                      // npm install / node·npm 路径解析（D-16）
		PM2:      pmx.New(),                       // PM2 进程托管（D-16~D-18）
		DataDir:  panelDataDir,                    // 启动器脚本落位（<数据目录>/app/pm2-launcher.js）
	})
	appSvc.RecoverInFlight(ctx)

	// db-tools：数据库工具集成（D0 Q1~Q3 拍板 2026-11-19，api-db-tools v1.2）——
	// db_tool/tool_deploy_task 双聚合（迁移 013）+ toolx 部署网关（下载 sha256
	// 强制/npm 参数数组/stdlib 解压 zip-slip 防线）+ 凭据保险库 dbtool-cred.key
	// 0600（I-02 永不回显）+ 跨模块 OHS 消费（db-mgmt 凭据网关/site-mgmt R-06
	// 管线/env-mgmt 环境实况/panel-core 访问形态/account-auth 只读会话校验）。
	// 安全底线：工具仅绑 localhost + nginx auth_request 面板会话 admin gating +
	// signon HMAC 令牌四重校验 + 卸载不删数据库数据。启动恢复 I-09；signon
	// 令牌台账 TTL 清扫下方以后台任务启动（goroutine 入 wg，G-06）。菜单
	// /db-tools 两角色可见（查看类全可看 ADR-018；工具本体 admin 专属 ⚠️A10）。
	dbtoolsStore := storedbtools.New(db)
	dbtoolsVault, err := dbtoolssvc.OpenToolVault(panelDataDir)
	if err != nil {
		closeErr := db.Close()
		return errors.Join(fmt.Errorf("open dbtools credential vault: %w", err), closeErr)
	}
	dbtoolsSvc := dbtoolssvc.New(dbtoolsStore, dbtoolsVault, dbtoolssvc.Deps{
		Deploy:   toolx.NewGateway(),
		Creds:    dbSvc,   // ToolCredentialGateway OHS（vault 解密仅服务端）
		Pipeline: siteSvc, // ConfigWritePipeline OHS（R-06 四步）
		Env:      envSvc,  // 环境实况读契约（PHP-FPM/Node/nginx 依赖就绪度）
		Sysd:     sysdGateway,
		PhpFpm:   phpfpmGw,
		Nginx:    nginxxGw,
		Panel:    panelSvc,
		Session:  authSvc, // VerifyWs 只读会话校验（gating/signon 绑定）
		Audit:    auditSvc,
	}, dbtoolssvc.Options{DataDir: panelDataDir, Mirror: cfg.Tools.Mirror, NpmRegistry: cfg.Tools.NpmRegistry})
	dbtoolsSvc.RecoverInFlight(ctx)
	// v1.3（api-db-tools D-20③⑤，S-11-01）：bindDomain 变更钩子注册——
	// 自动落地块 server_name 域名联动主通道（钩子失败不阻断配置变更，
	// install/open 惰性校正兜底）。
	panelSvc.RegisterConfigChangeHook(dbtoolsSvc)
	// 菜单已在构建期登记（icon=dbtool，Order 550）——此处不再重复 Register（W1 修复：
	// MenuRegistry 纯 append 无去重，重复登记致侧栏渲染两个「数据库工具」项）。

	// HTTP 骨架 + 模块挂载（后续模块在此追加 RegisterModules 参数）。
	server := api.NewServer()
	// 下发入口 HTML 时按安全入口状态注入 <base>（D-P03：页面、脚本、接口都在入口路径之下）。
	server.SetEntryPathSource(panelSvc)
	server.RegisterModules(
		authapi.NewModule(authSvc, auditSvc, cfg.CookieSecure),
		panelapi.NewModule(authSvc, panelSvc),
		monitoringapi.NewModule(authSvc, monSvc),
		svcapi.NewModule(authSvc, svcMgmtSvc),
		fileapi.NewModule(authSvc, fileSvc),
		siteapi.NewModule(authSvc, siteSvc),
		dbapi.NewModule(authSvc, dbSvc),
		terminalapi.NewModule(authSvc, termSvc),
		fwapi.NewModule(authSvc, fwSvc),
		dockerapi.NewModule(authSvc, dockerSvc),
		cronapi.NewModule(authSvc, cronSvc),
		envapi.NewModule(authSvc, envSvc),
		appapi.NewModule(authSvc, appSvc),
		dbtoolapi.NewModule(authSvc, dbtoolsSvc, cfg.CookieSecure),
	)

	// D-P03：EntryPathGuard 为全局第一环，以 net/http 包装器包裹整个 gin
	// engine（gin 路由解析先于其任何中间件，前缀改写必须在 gin 之前发生），
	// 位于 account-auth InitGuard 等之前、作用于全部路由（含 /auth/* 与未认证访问）。
	// D-P16（v2.0，ADR-028）：HostGuard 域名绑定强校验紧随其后（Entry → Host →
	// 引擎中间件链）——bind_domain 非空时 Host 不匹配（含 IP 直连）一律标准 404。
	// v1.3（api-db-tools D-21/4.4.5）：两包装器对内部端点路径前缀
	// /dbtool/gate/、/dbtool/signon/ 最小化前置直通豁免（绑域名后 nginx
	// auth_request 子请求/SignonScript loopback 回调 Host≠域名不被误拦）——
	// 豁免 Host/入口路径校验而非鉴权，loopback+秘密头+会话/令牌防护链不变。
	handler := middleware.EntryPathHandler(panelSvc,
		middleware.HostGuard(panelSvc, server.Handler()))

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// 审计清理后台任务（每日；goroutine 由 main 编排，ctx 取消退出——G-06）。
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil { // 任务 panic 隔离（backend-03 §5）
				slog.Error("audit worker panic", "panic", r)
			}
		}()
		if err := auditsvc.NewWorker(auditSvc, 24*time.Hour).Run(ctx); err != nil {
			slog.ErrorContext(ctx, "audit worker exited", "err", err)
		}
	}()

	// monitoring 采集链路后台任务（3s 采样 → 1min 聚合 → 环形缓冲；goroutine 由
	// main 编排，ctx 取消退出——G-06；单轮 panic 已在 Collector.tick 隔离）。
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil { // 任务 panic 隔离（backend-03 §5，双保险）
				slog.Error("monitoring collector panic", "panic", r)
			}
		}()
		if err := monitoringsvc.NewCollector(monSvc).Run(ctx); err != nil {
			slog.ErrorContext(ctx, "monitoring collector exited", "err", err)
		}
	}()

	// file-manager 分块任务清理后台任务（D-10：1h 无活动作废 + 临时分块目录
	// 清理；goroutine 由 main 编排，ctx 取消退出——G-06；单轮 panic 隔离）。
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil { // 任务 panic 隔离（backend-03 §5）
				slog.Error("file chunk sweeper panic", "panic", r)
			}
		}()
		if err := fileSvc.ChunkSweeper(ctx, 10*time.Minute); err != nil {
			slog.ErrorContext(ctx, "file chunk sweeper exited", "err", err)
		}
	}()

	// site-mgmt 自动续期后台任务（4.1.3：启动 1 分钟后首轮 + 24h ticker；
	// goroutine 由 main 编排，ctx 取消退出——G-06；单轮 panic 隔离在
	// runRenewalPass 之外再由本层兜底）。
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil { // 任务 panic 隔离（backend-03 §5）
				slog.Error("cert renewal scheduler panic", "panic", r)
			}
		}()
		if err := siteSvc.RunRenewalScheduler(ctx); err != nil {
			slog.ErrorContext(ctx, "cert renewal scheduler exited", "err", err)
		}
	}()

	// panel-core 面板证书自动续期后台任务（v2.0 4.1.3，ADR-029：启动 1 分钟后
	// 首轮 + 24h ticker；panel_cert.source=acme 且推算 expiring_soon 时单次尝试；
	// goroutine 由 main 编排，ctx 取消退出——G-06；单轮 panic 隔离在
	// runPanelRenewalPass 之内，本层 recover 再兜底）。
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil { // 任务 panic 隔离（backend-03 §5）
				slog.Error("panel cert renewal scheduler panic", "panic", r)
			}
		}()
		if err := panelSvc.RunPanelRenewalScheduler(ctx); err != nil {
			slog.ErrorContext(ctx, "panel cert renewal scheduler exited", "err", err)
		}
	}()

	// terminal 生命周期扫描后台任务（I-03/D-03/D-07：10s tick 空闲/硬上限
	// 判定 + 30s 面板会话复核 + 30s 心跳；goroutine 由 main 编排，ctx 取消
	// 退出——G-06；单轮 panic 隔离在 scanOnce 之外再由本层兜底）。
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil { // 任务 panic 隔离（backend-03 §5）
				slog.Error("terminal lifecycle scanner panic", "panic", r)
			}
		}()
		if err := termSvc.RunLifecycle(ctx, terminalsvc.ScanTick); err != nil {
			slog.ErrorContext(ctx, "terminal lifecycle scanner exited", "err", err)
		}
	}()

	// cron-backup 调度器后台任务（D-13：分钟级 tick 检查全部 job 的
	// schedule_cron 到期；重启重算触发点、错过的调度点跳过不留痕；goroutine
	// 由 main 编排，ctx 取消退出——G-06；tick 级 panic 隔离双保险）。
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil { // 任务 panic 隔离（backend-03 §5）
				slog.Error("backup scheduler panic", "panic", r)
			}
		}()
		if err := cronSvc.RunScheduler(ctx); err != nil {
			slog.ErrorContext(ctx, "backup scheduler exited", "err", err)
		}
	}()

	// cron-backup 执行 worker 池（A-C9：与立即执行共用同一 BackupExecutor
	// 通道；goroutine 由 main 编排，ctx 取消退出——G-06；单条执行 panic 在
	// runOneSafely 隔离之外再由本层兜底）。
	for i := 0; i < cronbacksvc.ExecutorWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil { // 任务 panic 隔离（backend-03 §5）
					slog.Error("backup executor worker panic", "panic", r)
				}
			}()
			if err := cronSvc.RunExecutor(ctx); err != nil {
				slog.ErrorContext(ctx, "backup executor worker exited", "err", err)
			}
		}()
	}

	// db-tools signon 令牌消费台账 TTL 清扫后台任务（D-09：30s tick 移除
	// 过期 jti；goroutine 由 main 编排，ctx 取消退出——G-06；台账内存态
	// 面板重启全部失效=安全保守）。
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil { // 任务 panic 隔离（backend-03 §5）
				slog.Error("dbtools token sweeper panic", "panic", r)
			}
		}()
		if err := dbtoolsSvc.RunTokenSweeper(ctx); err != nil {
			slog.ErrorContext(ctx, "dbtools token sweeper exited", "err", err)
		}
	}()

	serveErr := make(chan error, 1)
	// D-P06：boot 时 https_mode != off → TLS 监听，证书经 GetCertificate 按
	// 连接热读取（证书替换即时生效；协议模式切换随 systemctl 重启生效——
	// 与监听同属部署层动作，tasks.md A-P3 裁定）。
	useTLS := bootCfg.HTTPSMode != storepanel.HTTPSModeOff
	if useTLS {
		ln, lerr := net.Listen("tcp", addr)
		if lerr != nil {
			closeErr := db.Close()
			return errors.Join(fmt.Errorf("listen %s: %w", addr, lerr), closeErr)
		}
		httpSrv.TLSConfig = &tls.Config{
			GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
				cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				certPEM, keyPEM, cerr := panelSvc.CertMaterial(cctx)
				if cerr != nil {
					return nil, fmt.Errorf("panel tls material: %w", cerr)
				}
				pair, kerr := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
				if kerr != nil {
					return nil, fmt.Errorf("panel tls keypair: %w", kerr)
				}
				return &pair, nil
			},
		}
		go func() {
			if err := httpSrv.ServeTLS(ln, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
				serveErr <- fmt.Errorf("https serve: %w", err)
			}
		}()
	} else {
		go func() {
			if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				serveErr <- fmt.Errorf("http serve: %w", err)
			}
		}()
	}
	slog.Info("panel server listening", "addr", addr, "tls", useTLS,
		"listenScope", bootCfg.ListenScope, "entryPathEnabled", bootCfg.EntryPathEnabled)
	visitScheme := "http"
	if useTLS {
		visitScheme = "https"
	}
	visitDomain := ""
	if bootCfg.BindDomain != nil {
		visitDomain = *bootCfg.BindDomain
	}
	visitEntry := ""
	if bootCfg.EntryPathEnabled && bootCfg.EntryPath != nil {
		visitEntry = *bootCfg.EntryPath
	}
	logVisitURLs(visitScheme, addr, visitDomain, visitEntry)

	var runErr error
	select {
	case err := <-serveErr:
		runErr = err
		stop() // HTTP 失败 → 触发其余依赖收尾
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		runErr = errors.Join(runErr, fmt.Errorf("http shutdown: %w", err))
	}
	wg.Wait() // 等审计清理任务退出
	if err := dockerGw.Close(); err != nil {
		runErr = errors.Join(runErr, fmt.Errorf("close docker gateway: %w", err))
	}
	if err := db.Close(); err != nil {
		runErr = errors.Join(runErr, fmt.Errorf("close db: %w", err))
	}
	return runErr
}

// panelListenAddr 由 panel_config 监听值推导绑定地址（D-P02；api-panel-core
// §3.4 枚举表明文映射：all=0.0.0.0、localhost=127.0.0.1、custom=listen_address）。
func panelListenAddr(cfg storepanel.Config) string {
	host := "0.0.0.0"
	switch cfg.ListenScope {
	case storepanel.ListenScopeLocalhost:
		host = "127.0.0.1"
	case storepanel.ListenScopeCustom:
		if cfg.ListenAddress != nil && *cfg.ListenAddress != "" {
			host = *cfg.ListenAddress
		}
	}
	return net.JoinHostPort(host, strconv.Itoa(cfg.ListenPort))
}

// panelListenPort 解析实际监听地址的端口（A-W9：防火墙面板端口保护基准=
// 进程实际绑定端口；0 端口=临时分配不可预知，返回 0 使涉险判定 inert）。
func panelListenPort(addr string) (uint16, error) {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, fmt.Errorf("split listen addr %q: %w", addr, err)
	}
	p, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("parse listen port %q: %w", portStr, err)
	}
	return uint16(p), nil
}

// runInit 实现 `dougong init` CLI 子命令（AuthService.InitViaCLI 服务消费方）。
func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	cfgPath := fs.String("config", resolveConfigPath(""), "配置文件路径（缺省优先已安装服务数据目录）")
	username := fs.String("username", "", "初始管理员用户名")
	password := fs.String("password", "", "初始管理员密码（推荐改用 DOUGONG_INIT_PASSWORD 环境变量注入）")
	totpSecret := fs.String("totp-secret", "", "可选：TOTP Base32 密钥（与 totp-code 成对）")
	totpCode := fs.String("totp-code", "", "可选：TOTP 当前动态码")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse init flags: %w", err)
	}
	// 口令优先取环境变量（凭据来自环境变量注入，避免进入 shell 历史——G-03 纪律）。
	if v := os.Getenv("DOUGONG_INIT_PASSWORD"); v != "" {
		*password = v
	}
	if *username == "" || *password == "" {
		return errors.New("init 需要 -username 与 -password（或 DOUGONG_INIT_PASSWORD）")
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	cfg.DBPath = config.ResolveDBPath(*cfgPath, cfg.DBPath) // 锚定配置目录（同 run/CLI）
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			slog.Error("close db after init", "err", cerr)
		}
	}()
	if err := store.Migrate(ctx, db); err != nil {
		return err
	}
	repo := storeauth.New(db)
	if err := repo.Init().EnsureRow(ctx); err != nil {
		return err
	}
	hasher, err := authsvc.NewBcryptHasher(12)
	if err != nil {
		return err
	}
	svc := authsvc.New(repo, hasher, authsvc.NewTotpService(nil), authsvc.DefaultSessionPolicy())
	if err := svc.InitViaCLI(ctx, *username, *password, *totpSecret, *totpCode); err != nil {
		var ec *errcode.Error
		if errors.As(err, &ec) && ec.Symbol() == "AUTH_009" {
			slog.Error("初始化被拒绝：系统已初始化（AUTH_009）")
			os.Exit(2) // 非 0 退出码（§4.4.4 CLI 契约）
		}
		return err
	}
	slog.Info("初始化完成", "username", *username, "method", "cli")
	return nil
}
