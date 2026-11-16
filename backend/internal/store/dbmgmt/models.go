// Package dbmgmt 是数据库管理（database-mgmt，M4）模块的数据访问层（SQL 只
// 允许出现在本包；api-database-mgmt.md §5.2 SQL 清单 #1~#6 全参数化 D-13）。
//
// db_instance 为本模块唯一持久化对象（§5.3 合一档案表 D-01，engine 列区分
// mysql/mariadb/redis）；database/db_user/key/慢日志/进程列表为引擎实况投影
// 零台账（I-04），不在此层。
package dbmgmt

import (
	"fmt"
	"time"

	"github.com/dougong/dougong/internal/store"
)

// Executor 是 store.Executor 的别名（*sql.DB 与 *sql.Tx 共同满足）。
type Executor = store.Executor

// db_engine / db_conn_mode / instance_conn_state 取值域（DDL CHECK 兜底，§3.4）。
const (
	EngineMySQL   = "mysql"
	EngineMariaDB = "mariadb"
	EngineRedis   = "redis"

	ConnModeSocket = "socket"
	ConnModeTCP    = "tcp"

	ConnStateReachable   = "reachable"
	ConnStateUnreachable = "unreachable"
)

// IsMySQLEngine 报告引擎是否属 MySQL 族（I-05：mysql/database/dbuser/backup
// 接口要求 engine∈mysql/mariadb）。
func IsMySQLEngine(engine string) bool {
	return engine == EngineMySQL || engine == EngineMariaDB
}

// Instance 对应 db_instance 表一行（§4.1.1 字段清单；凭据为内嵌值对象列组，
// CredCipher 零出口——仅 vault 解密路径消费，I-02）。
type Instance struct {
	ID            int64
	IdentityKey   string
	Engine        string
	ConnMode      string
	SocketPath    *string
	Host          *string
	Port          *int
	Version       *string
	ConnState     string
	CredUsername  *string
	CredCipher    *string
	CredUpdatedAt *time.Time
	DiscoveredAt  time.Time
	UpdatedAt     time.Time
}

// CredentialsSet 报告凭据是否已配置（DBM_004 判据：cred_username 非空，§4.1.1）。
func (i *Instance) CredentialsSet() bool {
	return i.CredUsername != nil && *i.CredUsername != ""
}

// formatTime/scanTime 等时间辅助与 store/auth、store/sitemgmt 同法（RFC3339
// UTC 文本列；DATETIME 列经 any 扫描兼容 string/[]byte/time.Time 三形态）。
func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func formatTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatTime(*t)
}

func scanTime(src any) (time.Time, error) {
	switch v := src.(type) {
	case time.Time:
		return v.UTC(), nil
	case string:
		return time.Parse(time.RFC3339, v)
	case []byte:
		return time.Parse(time.RFC3339, string(v))
	case nil:
		return time.Time{}, fmt.Errorf("dbmgmt: NULL timestamp")
	default:
		return time.Time{}, fmt.Errorf("dbmgmt: unsupported timestamp type %T", src)
	}
}

func scanTimePtr(src any) (*time.Time, error) {
	if src == nil {
		return nil, nil
	}
	t, err := scanTime(src)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
