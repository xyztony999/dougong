// Package dbmgmt 是数据库管理（database-mgmt，M4）业务层：实例探测纳管
// （D-07）/凭据密文接管（D-02/D-03）/MySQL 库用户治理（D-10/D-11）/双引擎
// 观测白名单（D-08/D-09）/备份导出导入（D-06）+在途互斥（D-12）/Redis key
// 只读浏览。分层依赖 api → service → store/adapter；引擎交互收敛于
// MySQLGateway/RedisGateway（§4.1.5 DbGateway 契约，A-DB1 接口隔离拆分），
// SQLite 侧档案与审计同事务（R-03）。
package dbmgmt

import (
	"time"

	"github.com/dougong/dougong/internal/adapter/dbmysql"
)

// ---- 审计动作码（D-13：13 个，category 全部=change；detail 不含密码/密文/密钥）----

const (
	ActionInstanceRescanned   = "instance_rescanned"
	ActionInstanceCredentials = "instance_credentials_set"
	ActionInstanceRemoved     = "instance_removed"
	ActionDatabaseCreated     = "database_created"
	ActionDatabaseDropped     = "database_dropped"
	ActionDBUserCreated       = "dbuser_created"
	ActionDBUserDeleted       = "dbuser_deleted"
	ActionDBUserGranted       = "dbuser_granted"
	ActionDBUserRevoked       = "dbuser_revoked"
	ActionRootPasswordChanged = "instance_root_password_changed"
	ActionDatabaseExported    = "database_exported"
	ActionDatabaseImported    = "database_imported"
	ActionRedisPasswordSet    = "redis_password_set"
)

// 审计对象类型（audit_log.object_type 业务扩展取值）。
const (
	ObjectTypeInstance = "db_instance"
	ObjectTypeDatabase = "database"
	ObjectTypeDBUser   = "db_user"
)

// ---- 探测（D-07）----

// ProbeTarget 是 DISCOVERY_PROBE_SET 单项（编译期常量探测集：engine 自带
// 引擎语义，A-DB5）。
type ProbeTarget struct {
	Engine     string
	ConnMode   string
	SocketPath string
	Host       string
	Port       int
}

// DiscoveryProbeSet 默认探测集（D-07：MySQL socket 惯例路径 ×4 + TCP
// localhost:3306 + Redis TCP localhost:6379；编译期常量）。
var DiscoveryProbeSet = []ProbeTarget{
	{Engine: "mysql", ConnMode: "socket", SocketPath: "/tmp/mysql.sock"},
	{Engine: "mysql", ConnMode: "socket", SocketPath: "/var/run/mysqld/mysqld.sock"},
	{Engine: "mysql", ConnMode: "socket", SocketPath: "/run/mysqld/mysqld.sock"},
	{Engine: "mysql", ConnMode: "socket", SocketPath: "/var/lib/mysql/mysql.sock"},
	{Engine: "mysql", ConnMode: "tcp", Host: "localhost", Port: 3306},
	{Engine: "redis", ConnMode: "tcp", Host: "localhost", Port: 6379},
}

// probeTimeout 传输层探测拨号超时（socket 存在性/TCP 拨通）。
const probeTimeout = 2 * time.Second

// ---- 观测白名单（D-08）----

// MySQLStatusFields MySQL status 概览 variables 白名单（≤10 项，呈现口径）。
var MySQLStatusFields = []string{
	"version_comment",
	"max_connections",
	"character_set_server",
	"collation_server",
	"innodb_buffer_pool_size",
	"max_allowed_packet",
	"wait_timeout",
	"connect_timeout",
	"slow_query_log",
	"general_log",
}

// redisInfoField 是 REDIS_INFO_FIELDS 单项（service 节小写 → 呈现字段 →
// INFO 原始字段名）。
type redisInfoField struct {
	Section string // 响应 sections 键（小写）
	Field   string // 响应字段名（契约 camelCase）
	Source  string // INFO 原始字段名（snake_case）
}

// RedisInfoFields Redis info 概览白名单（D-08；sections 骨架 server/clients/
// memory/stats）。
var RedisInfoFields = []redisInfoField{
	{Section: "server", Field: "version", Source: "redis_version"},
	{Section: "clients", Field: "connectedClients", Source: "connected_clients"},
	{Section: "memory", Field: "usedMemoryHuman", Source: "used_memory_human"},
	{Section: "memory", Field: "maxMemoryHuman", Source: "maxmemory_human"},
	{Section: "stats", Field: "totalConnectionsReceived", Source: "total_connections_received"},
	{Section: "stats", Field: "keyspaceHits", Source: "keyspace_hits"},
	{Section: "stats", Field: "keyspaceMisses", Source: "keyspace_misses"},
}

// ---- key 浏览与慢日志（D-09）----

const (
	// KeyScanCount SCAN COUNT 常量（D-09）。
	KeyScanCount = 200
	// SlowlogDefault 慢日志条数默认值。
	SlowlogDefault = 50
	// SlowlogLimit 慢日志条数上限。
	SlowlogLimit = 200
	// MaxPatternLen key 匹配模式长度上限（DBM_017 判据之一）。
	MaxPatternLen = 128
)

// ---- 备份（D-06/D-12）----

const (
	// DumpSizeLimit 导入文件大小上限（512MiB，DBM_014 判据）。
	DumpSizeLimit int64 = 512 << 20
	// DefaultCharset 建库默认字符集（D-11）。
	DefaultCharset = "utf8mb4"
)

// ---- grant_level（D-10）----

// 授权档位枚举（grant_level；正向映射表在 adapter/dbmysql levelPrivileges）。
const (
	LevelReadonly = dbmysql.LevelReadonly
	LevelWrite    = dbmysql.LevelWrite
	LevelAll      = dbmysql.LevelAll
)

// ---- 凭据（D-02）----

// CredKeyFile 凭据密钥文件名（面板数据目录独立文件，0600，不入 SQLite）。
const CredKeyFile = "db-cred.key"

// RedisDefaultUsername Redis 凭据用户名固定值（A-DB6：Redis 6 ACL 默认用户，
// 满足 I-09 同进同空与 DBM_004 判据统一）。
const RedisDefaultUsername = "default"

// ---- 标识符白名单（R-14 / A-DB3）----

const (
	// maxDatabaseNameLen MySQL 库名长度上限。
	maxDatabaseNameLen = 64
	// maxUsernameLen MySQL 用户名长度上限。
	maxUsernameLen = 32
	// maxHostLen MySQL host 长度上限。
	maxHostLen = 255
	// maxPasswordLen 引擎密码长度上限（改密/建用户入参）。
	maxPasswordLen = 128
	// maxKeyLen Redis key 长度上限（TTL 查询入参）。
	maxKeyLen = 512
)
