// Package dbconn 是数据库实例连接目标与凭据的共享传输类型（dbmysql/dbredis
// 适配器与 service 层共同消费；api-database-mgmt.md §4.1.5 DbGateway 契约的
// 入参载体）。
//
// 凭据明文仅存在于内存态建连链路（D-02/⚠️A18：vault 解密 → 网关组装子进程
// 环境变量 → 进程退出即弃；不落日志、不进响应、不进 argv）。
package dbconn

// Target 是一个数据库实例的连接目标（I-07：socket 模式 SocketPath 非空且
// Host/Port 为空；tcp 模式反之——形态互斥由 service 层 ConnTargetResolver
// 校验后传入，本类型不再防御）。
type Target struct {
	Engine     string // mysql | mariadb | redis | mongodb（db_engine 枚举）
	ConnMode   string // socket | tcp（db_conn_mode 枚举）
	SocketPath string // socket 模式连接路径
	Host       string // tcp 模式主机（默认 localhost 由 service 填充）
	Port       int    // tcp 模式端口（1~65535）
}

// Creds 是引擎连接凭据（凭据值对象内存态形态：mysql 为用户名+密码；redis
// 仅 Password 生效，Username 固定 default——A-DB6）。
type Creds struct {
	Username string
	Password string
}
