// Package dbtools 是数据库工具集成（db-tools，D0 Q1~Q3 拍板 2026-11-19）
// 应用服务（api-db-tools.md v1.2）：ToolCatalog 编译期目录（D-13）、实时探测
// 投影（D-02）、异步部署任务（D-04：同工具互斥 I-06、终态档案同事务 I-C01、
// 重启恢复 I-09）、下载钉版本+sha256 强制（D-05）、phpMyAdmin signon 桥接
// （D-06/D-09）、mongo-express systemd+env 注入（D-07）、basic auth 服务端
// 注入与轮换（D-08）、面板域 nginx location 注入（D-03）、会话 gating 与
// 内部端点三重防护（D-11）。安全底线：工具仅绑 localhost、凭据 vault 永不
// 回显（I-02）、卸载不删数据库数据（I-C03）、零遥测（R-09）。
package dbtools

import "time"

// 编译期常量表（api §3.4 D-13；⚠️A1/A12 非用户可配）。
const (
	// PHPMyAdminVersion 钉死版本（官方 tarball）。
	PHPMyAdminVersion = "5.2.2"
	// PHPMyAdminSha256 官方发布页取证（tasks.md A-DBT2①：curl
	// files.phpmyadmin.net/phpMyAdmin/5.2.2/…tar.gz.sha256 原文，2026-11-19）。
	PHPMyAdminSha256 = "8551c8bf3b166f232d5cf64bac877472e9d0cb8f2fe1858fab24f975e7d765b6"
	// PHPMyAdminTarballURL 官方 tarball URL（镜像前缀拼接式覆盖，D-05）。
	PHPMyAdminTarballURL = "https://files.phpmyadmin.net/phpMyAdmin/" + PHPMyAdminVersion +
		"/phpMyAdmin-" + PHPMyAdminVersion + "-all-languages.tar.gz"
	// MongoExpressVersion npm 精确版本（registry 取证存在，tasks.md A-DBT2②）。
	MongoExpressVersion = "1.0.0"
	// MongoExpressPackage npm 包名。
	MongoExpressPackage = "mongo-express"
	// MongoExpressPort 监听端口（编译期定死，⚠️A6）。
	MongoExpressPort = 8081
	// MongoExpressBind 绑定地址（仅 localhost，⚠️A6 安全底线）。
	MongoExpressBind = "127.0.0.1"
	// MongoExpressUnitName systemd 单元全名（D-07）。
	MongoExpressUnitName = "dougong-mongo-express.service"
	// MongoExpressUnitPath 单元文件落点（秘密文件通道 0600）。
	MongoExpressUnitPath = "/etc/systemd/system/" + MongoExpressUnitName
	// NodeBinaryPath NodeSource nodejs 包二进制路径（ExecStart 用）。
	NodeBinaryPath = "/usr/bin/node"

	// ToolPathPrefixPHPMyAdmin 面板域工具路径前缀（D-03）。
	ToolPathPrefixPHPMyAdmin = "/dbtool/phpmyadmin/"
	// ToolPathPrefixMongoExpress 同上（ME_CONFIG_SITE_BASEURL）。
	ToolPathPrefixMongoExpress = "/dbtool/mongo-express/"
	// GateLocationPath 内部 gate location（internal 指令，D-11）。
	GateLocationPath = "/dbtool-gate-internal"
	// SignonReopenPath SignonURL 重认证落地路径（v1.2 W3）。
	SignonReopenPath = "/dbtool/access/reopen"
	// ToolInstanceCookie 非敏感实例标识 cookie（HttpOnly，Path=工具路径，
	// SameSite=Lax；仅 db_instance.id 非凭据，D-06）。
	ToolInstanceCookie = "dg_tool_instance"

	// SignonTokenTTL signon 令牌时效（D-09）。
	SignonTokenTTL = 60 * time.Second
	// TokenSweepInterval 令牌消费台账 TTL 清扫间隔（goroutine 入 wg）。
	TokenSweepInterval = 30 * time.Second
	// SignonKeyBytes signon 密钥随机字节数（crypto/rand）。
	SignonKeyBytes = 32
	// BasicAuthPasswordBytes basic auth 口令随机字节数（→base64url 32 字符）。
	BasicAuthPasswordBytes = 24
	// BasicAuthUsername basic auth 固定用户名（D-08）。
	BasicAuthUsername = "dougong"

	// TaskOutputTailLines 任务输出尾部采集行数（终态定格）。
	TaskOutputTailLines = 200
	// DeployOpTimeout 单部署命令超时（npm/解压/服务操作）。
	DeployOpTimeout = 600 * time.Second
	// DownloadTimeout tarball 下载超时。
	DownloadTimeout = 300 * time.Second
	// TaskListPageSize 任务列表默认分页。
	TaskListPageSize = 50

	// CredKeyFile 工具凭据密钥文件（dataDir 下，0600，沿 db-cred.key 先例）。
	CredKeyFile = "dbtool-cred.key"
	// InternalSecretName 内部秘密文件名（tools 目录下，0600，D-11）。
	InternalSecretName = "internal-secret"
	// MongoExpressAuthConfName basic auth 注入秘密文件（0600，D-08）。
	MongoExpressAuthConfName = "mongo-express-auth.conf"
	// PHPMyAdminFastCGIConfName FastCGI 参数模板文件（0644）。
	PHPMyAdminFastCGIConfName = "phpmyadmin-fastcgi.conf"
	// InternalSecretHeader 内部秘密头（nginx/SignonScript 注入，D-11）。
	InternalSecretHeader = "X-Dbtool-Internal"
)

// PHPMyAdminRequiredExts PHP 扩展必需清单（D-06/D-13，php -m 比对）。
var PHPMyAdminRequiredExts = []string{"mysqli", "mbstring", "zip", "gd", "json", "curl", "openssl"}

// 审计动作码（D-16，R-03 同事务脱敏；category 均 change；detail 不含
// 凭据/口令/令牌明文）。gating 逐请求校验不落审计（⚠️A11 防写放大）。
const (
	AuditActionInstallStarted    = "dbt_install_started"
	AuditActionUpgradeStarted    = "dbt_upgrade_started"
	AuditActionUninstallStarted  = "dbt_uninstall_started"
	AuditActionTaskFinished      = "dbt_task_finished"
	AuditActionToolAccessed      = "dbt_tool_accessed"
	AuditActionCredentialRotated = "dbt_credential_rotated"
)

// 审计对象类型（audit_log.object_type 业务扩展取值）。
const (
	ObjectTypeTool     = "db_tool"
	ObjectTypeToolTask = "tool_deploy_task"
)

// 注入标记（A-DBT8：span 注入/撤除 byte 级还原锚点）。
const (
	markerBeginPrefix = "# BEGIN dougong-dbtool "
	markerEndPrefix   = "# END dougong-dbtool "
	markerKeyGate     = "gate"
)
