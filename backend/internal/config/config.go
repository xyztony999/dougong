// Package config 是面板配置（D-10：会话有效期两键载体=配置文件，重启生效 MVP）。
// 加载优先级：环境变量 > 配置文件 > 默认值；启动期一次性校验，非法即 fail-fast。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"
)

// Config 是面板运行配置。
type Config struct {
	// Addr HTTP 监听地址（默认 127.0.0.1:8687；由配置注入，业务代码不写死）。
	Addr string `json:"addr"`
	// DBPath SQLite 数据库文件路径（默认 data/dougong.db）。
	DBPath string `json:"db_path"`
	// CookieSecure 会话 Cookie Secure 位（HTTPS 部署时置 true）。
	CookieSecure bool `json:"cookie_secure"`
	// Session 会话有效期策略（ADR-019）。
	Session Session `json:"session"`
	// Sites 网站管理（site-mgmt）运行参数。
	Sites Sites `json:"sites"`
	// ACME 证书签发外呼参数（acme.directory_url 配置键，api-site-mgmt §3.4）。
	ACME ACME `json:"acme"`
	// DB 数据库管理（database-mgmt）CLI 客户端通道参数（重启生效）。
	DB DB `json:"db"`
	// Cron 计划任务与备份（cron-backup）CLI 通道参数（重启生效）。
	Cron Cron `json:"cron"`
	// Tools 数据库工具集成（db-tools）下载镜像参数（api-db-tools §3.4 配置项）。
	Tools Tools `json:"tools"`
	// Docker 容器管理（docker-mgmt）参数（api-docker-mgmt.md D-11）。
	Docker Docker `json:"docker"`
}

// Docker 是 docker-mgmt 模块参数。Compose 项目落盘根生产默认 /opt/dougong-compose
// （服务内 DefaultComposeRootDir 同值）；开放为配置键只为测试隔离与本机验证——
// 防逃逸依赖「根为部署方控制的固定目录」，生产部署保持默认即可。
type Docker struct {
	// ComposeRoot Compose 项目落盘根（默认 /opt/dougong-compose；须为绝对路径）。
	ComposeRoot string `json:"compose_root"`
}

// Tools 是 db-tools 模块的下载分发镜像参数（默认空=官方源；其余编译期
// 常量不入配置，⚠️A1 非用户可配）。
type Tools struct {
	// Mirror 下载镜像拼接式前缀（DOUGONG_MIRROR 同构，install.sh:38-45）。
	Mirror string `json:"mirror"`
	// NpmRegistry npm registry 镜像（--registry 选项；空=官方 registry）。
	NpmRegistry string `json:"npm_registry"`
}

// Sites 是 site-mgmt 模块的 nginx 通道参数（重启生效；惯例根下目录优先级
// sites-enabled > conf.d > vhost.d 为编译期常量 SCAN_DIR_PRIORITY，不配）。
type Sites struct {
	// NginxBin nginx 二进制（默认 "nginx"，经 exec.LookPath 解析；可绝对路径）。
	NginxBin string `json:"nginx_bin"`
	// ConfRoot nginx 配置根目录（默认 /etc/nginx）。
	ConfRoot string `json:"conf_root"`
}

// DB 是 database-mgmt 模块的引擎 CLI 客户端参数（api-database-mgmt §3.2：
// dbmysql/dbredis 适配器承载；默认名字经 exec.LookPath 解析，可绝对路径）。
type DB struct {
	// MySQLBin mysql 客户端二进制（默认 "mysql"）。
	MySQLBin string `json:"mysql_bin"`
	// MysqldumpBin mysqldump 二进制（默认 "mysqldump"）。
	MysqldumpBin string `json:"mysqldump_bin"`
	// RedisCliBin redis-cli 二进制（默认 "redis-cli"）。
	RedisCliBin string `json:"rediscli_bin"`
}

// Cron 是 cron-backup 模块的 CLI 通道参数（api-cron-backup.md §3.2：cronx/
// backupx 适配器承载；默认名字经 exec.LookPath 解析，可绝对路径）。
type Cron struct {
	// CrontabBin crontab 二进制（默认 "crontab"；读写 root crontab）。
	CrontabBin string `json:"crontab_bin"`
	// JournalctlBin journalctl 二进制（默认 "journalctl"；cron 日志尾部首选源）。
	JournalctlBin string `json:"journalctl_bin"`
	// RsyncBin rsync 二进制（默认 "rsync"；远程目标推送/拉回/清理）。
	RsyncBin string `json:"rsync_bin"`
	// ShellBin 代执行 shell（默认 "/bin/sh"；D-05 shell 语义）。
	ShellBin string `json:"shell_bin"`
}

// ACME 是证书签发配置（D-09：CA 目录 URL 可指向 staging 测试环境）。
type ACME struct {
	// DirectoryURL CA 目录 URL（默认 Let's Encrypt 生产）。
	DirectoryURL string `json:"directory_url"`
}

// Session 会话有效期配置键（session.absolute_timeout / session.idle_timeout）。
type Session struct {
	// AbsoluteTimeout 绝对有效期（默认 12h；RFC3339/Go duration 字符串）。
	AbsoluteTimeout string `json:"absolute_timeout"`
	// IdleTimeout 空闲有效期（默认 2h）。
	IdleTimeout string `json:"idle_timeout"`
}

// Defaults 返回默认配置。
func Defaults() Config {
	return Config{
		Addr:    "127.0.0.1:8687",
		DBPath:  "data/dougong.db",
		Session: Session{AbsoluteTimeout: "12h", IdleTimeout: "2h"},
		Sites:   Sites{NginxBin: "nginx", ConfRoot: "/etc/nginx"},
		ACME:    ACME{DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory"},
		DB:      DB{MySQLBin: "mysql", MysqldumpBin: "mysqldump", RedisCliBin: "redis-cli"},
		Cron:    Cron{CrontabBin: "crontab", JournalctlBin: "journalctl", RsyncBin: "rsync", ShellBin: "/bin/sh"},
		// 与 service/dockermgmt.DefaultComposeRootDir 同值（create_test.go 锁定两处一致）。
		Docker: Docker{ComposeRoot: "/opt/dougong-compose"},
	}
}

// Load 加载配置：默认值 → JSON 文件（不存在则跳过）→ 环境变量覆盖 → 校验。
func Load(path string) (*Config, error) {
	cfg := Defaults()
	if path != "" {
		b, err := os.ReadFile(path)
		if err == nil {
			if err := json.Unmarshal(b, &cfg); err != nil {
				return nil, fmt.Errorf("config: parse %s: %w", path, err)
			}
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("config: read %s: %w", path, err)
		}
	}
	overrideByEnv(&cfg)
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config: invalid: %w", err)
	}
	return &cfg, nil
}

// AbsoluteTimeoutDur 返回绝对有效期。
func (c *Config) AbsoluteTimeoutDur() (time.Duration, error) {
	return parseDuration(c.Session.AbsoluteTimeout)
}

// IdleTimeoutDur 返回空闲有效期。
func (c *Config) IdleTimeoutDur() (time.Duration, error) {
	return parseDuration(c.Session.IdleTimeout)
}

func overrideByEnv(cfg *Config) {
	if v := os.Getenv("DOUGONG_ADDR"); v != "" {
		cfg.Addr = v
	}
	if v := os.Getenv("DOUGONG_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("DOUGONG_COOKIE_SECURE"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.CookieSecure = b
		}
	}
	if v := os.Getenv("DOUGONG_SESSION_ABSOLUTE_TIMEOUT"); v != "" {
		cfg.Session.AbsoluteTimeout = v
	}
	if v := os.Getenv("DOUGONG_SESSION_IDLE_TIMEOUT"); v != "" {
		cfg.Session.IdleTimeout = v
	}
	if v := os.Getenv("DOUGONG_NGINX_BIN"); v != "" {
		cfg.Sites.NginxBin = v
	}
	if v := os.Getenv("DOUGONG_NGINX_CONF_ROOT"); v != "" {
		cfg.Sites.ConfRoot = v
	}
	if v := os.Getenv("DOUGONG_ACME_DIRECTORY_URL"); v != "" {
		cfg.ACME.DirectoryURL = v
	}
	if v := os.Getenv("DOUGONG_MYSQL_BIN"); v != "" {
		cfg.DB.MySQLBin = v
	}
	if v := os.Getenv("DOUGONG_MYSQLDUMP_BIN"); v != "" {
		cfg.DB.MysqldumpBin = v
	}
	if v := os.Getenv("DOUGONG_REDISCLI_BIN"); v != "" {
		cfg.DB.RedisCliBin = v
	}
	if v := os.Getenv("DOUGONG_CRONTAB_BIN"); v != "" {
		cfg.Cron.CrontabBin = v
	}
	if v := os.Getenv("DOUGONG_JOURNALCTL_BIN"); v != "" {
		cfg.Cron.JournalctlBin = v
	}
	if v := os.Getenv("DOUGONG_RSYNC_BIN"); v != "" {
		cfg.Cron.RsyncBin = v
	}
	if v := os.Getenv("DOUGONG_SHELL_BIN"); v != "" {
		cfg.Cron.ShellBin = v
	}
	if v := os.Getenv("DOUGONG_TOOLS_MIRROR"); v != "" {
		cfg.Tools.Mirror = v
	}
	if v := os.Getenv("DOUGONG_TOOLS_NPM_REGISTRY"); v != "" {
		cfg.Tools.NpmRegistry = v
	}
	if v := os.Getenv("DOUGONG_COMPOSE_ROOT"); v != "" {
		cfg.Docker.ComposeRoot = v
	}
}

func (c *Config) validate() error {
	if c.Addr == "" {
		return fmt.Errorf("addr is empty")
	}
	if c.DBPath == "" {
		return fmt.Errorf("db_path is empty")
	}
	abs, err := parseDuration(c.Session.AbsoluteTimeout)
	if err != nil || abs <= 0 {
		return fmt.Errorf("session.absolute_timeout %q invalid", c.Session.AbsoluteTimeout)
	}
	idle, err := parseDuration(c.Session.IdleTimeout)
	if err != nil || idle <= 0 {
		return fmt.Errorf("session.idle_timeout %q invalid", c.Session.IdleTimeout)
	}
	if idle > abs {
		return fmt.Errorf("session.idle_timeout (%s) must not exceed absolute_timeout (%s)", c.Session.IdleTimeout, c.Session.AbsoluteTimeout)
	}
	if c.Sites.NginxBin == "" {
		return fmt.Errorf("sites.nginx_bin is empty")
	}
	if c.Sites.ConfRoot == "" {
		return fmt.Errorf("sites.conf_root is empty")
	}
	if c.ACME.DirectoryURL == "" {
		return fmt.Errorf("acme.directory_url is empty")
	}
	if c.DB.MySQLBin == "" || c.DB.MysqldumpBin == "" || c.DB.RedisCliBin == "" {
		return fmt.Errorf("db.* binary names must not be empty")
	}
	// 落盘根必须是「非文件系统根」的绝对路径：根下按项目名建目录，根为 / 时等于
	// 放弃根前缀约束。校验须同时容忍两种绝对路径形态——POSIX（/opt/dougong-compose
	// 是生产默认，Windows 上 filepath.IsAbs 不认「/…」，a56e054 同类跨平台坑：
	// 2026-11-23 Windows -race 全量 TestCLIRenameUser/TestCLIUnbindDomain/TestCLIStatus
	// 全挂即此因）与平台原生（Windows 开发机测试经 t.TempDir=C:\… 注入）。
	// 两种形态各自的文件系统根（POSIX / 与 Windows 卷根）均拒绝。
	root := c.Docker.ComposeRoot
	validComposeRoot := false
	switch {
	case path.IsAbs(root):
		validComposeRoot = path.Clean(root) != "/"
	case filepath.IsAbs(root):
		cleanRoot := filepath.Clean(root)
		validComposeRoot = cleanRoot != string(filepath.Separator) &&
			cleanRoot != filepath.VolumeName(cleanRoot)+string(filepath.Separator)
	}
	if !validComposeRoot {
		return fmt.Errorf("docker.compose_root %q must be an absolute path other than the filesystem root",
			root)
	}
	if c.Cron.CrontabBin == "" || c.Cron.JournalctlBin == "" ||
		c.Cron.RsyncBin == "" || c.Cron.ShellBin == "" {
		return fmt.Errorf("cron.* binary names must not be empty")
	}
	return nil
}

// parseDuration 支持 Go duration（12h）与整数秒两种书写。
func parseDuration(v string) (time.Duration, error) {
	if secs, err := strconv.ParseInt(v, 10, 64); err == nil {
		return time.Duration(secs) * time.Second, nil
	}
	return time.ParseDuration(v)
}
