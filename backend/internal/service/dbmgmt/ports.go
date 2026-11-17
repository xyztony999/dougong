package dbmgmt

import (
	"context"
	"io"
	"time"

	"github.com/dougong/dougong/internal/adapter/dbconn"
	"github.com/dougong/dougong/internal/adapter/dbmongo"
	"github.com/dougong/dougong/internal/adapter/dbmysql"
	"github.com/dougong/dougong/internal/adapter/dbredis"
	"github.com/dougong/dougong/internal/service/audit"
	storeauth "github.com/dougong/dougong/internal/store/auth"
	storedb "github.com/dougong/dougong/internal/store/dbmgmt"
)

// MySQLGateway 是 §4.1.5 DbGateway 契约 MySQL 族方法（A-DB1：按引擎拆分的
// 消费方接口；adapter/dbmysql.Gateway 实现，main 装配注入。凭据明文仅在
// 网关内组装子进程环境使用，⚠️A18/D-02）。
type MySQLGateway interface {
	// Available 通道可用性（二进制未检出时携带原因）。
	Available() error
	// Ping 连接验证（认证/网络错误归一哨兵：dbmysql.ErrAuthRejected/ErrConnFailed）。
	Ping(ctx context.Context, t dbconn.Target, c dbconn.Creds) error
	// ServerVersion SELECT VERSION() 实况（档案 version 刷新，D-04）。
	ServerVersion(ctx context.Context, t dbconn.Target, c dbconn.Creds) (string, error)
	// ListDatabases 实况库清单（I-04 每请求实时）。
	ListDatabases(ctx context.Context, t dbconn.Target, c dbconn.Creds) ([]dbmysql.DatabaseInfo, error)
	// DatabaseExists 库实况存在性（DBM_007/008 前置）。
	DatabaseExists(ctx context.Context, t dbconn.Target, c dbconn.Creds, name string) (bool, error)
	// CreateDatabase 建库（重名 → dbmysql.ErrDatabaseExists）。
	CreateDatabase(ctx context.Context, t dbconn.Target, c dbconn.Creds, name, charset string) error
	// DropDatabase 删库（不存在 → dbmysql.ErrDatabaseNotFound）。
	DropDatabase(ctx context.Context, t dbconn.Target, c dbconn.Creds, name string) error
	// ShowCharsets 字符集实况候选（D-11）。
	ShowCharsets(ctx context.Context, t dbconn.Target, c dbconn.Creds) ([]string, error)
	// ListUsers 实况用户清单（grants raw 投影；D-10 反向映射数据源）。
	ListUsers(ctx context.Context, t dbconn.Target, c dbconn.Creds) ([]dbmysql.UserGrants, error)
	// ShowGrants 指定用户授权行（用户不存在 → dbmysql.ErrUserNotFound）。
	ShowGrants(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, host string) ([]dbmysql.GrantLine, error)
	// CreateUser 建用户（密码止步网关；重名 → dbmysql.ErrUserExists）。
	CreateUser(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, host, password string) error
	// DropUser 删用户（连带回收授权；不存在 → dbmysql.ErrUserNotFound）。
	DropUser(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, host string) error
	// Grant 库级三档授权（level=readonly|write|all，D-10）。
	Grant(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, host, db, level string) error
	// Revoke 按库回收（无授权 → dbmysql.ErrGrantMissing）。
	Revoke(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, host, db string) error
	// AlterRootPassword 当前连接账户改密（A-DB8）。
	AlterRootPassword(ctx context.Context, t dbconn.Target, c dbconn.Creds, newPassword string) error
	// StatusVariables 全量变量+版本（D-08 白名单过滤在本层）。
	StatusVariables(ctx context.Context, t dbconn.Target, c dbconn.Creds) (version string, vars map[string]string, err error)
	// ProcessList 进程列表快照。
	ProcessList(ctx context.Context, t dbconn.Target, c dbconn.Creds) ([]dbmysql.ProcessRow, error)
	// DumpDatabase mysqldump 导出（D-06 参数模板；R-08）。
	DumpDatabase(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, charset string) (io.ReadCloser, error)
	// ImportDatabase mysql 客户端导入（临时文件由调用方治理）。
	ImportDatabase(ctx context.Context, t dbconn.Target, c dbconn.Creds, db string, r io.Reader) error
}

// RedisGateway 是 §4.1.5 DbGateway 契约 Redis 族方法（adapter/dbredis.Gateway
// 实现）。
type RedisGateway interface {
	// Available 通道可用性。
	Available() error
	// Ping 连接验证（哨兵 dbredis.ErrAuthRejected/ErrConnFailed）。
	Ping(ctx context.Context, t dbconn.Target, c dbconn.Creds) error
	// ServerVersion INFO server 的 redis_version（档案 version 刷新，D-04）。
	ServerVersion(ctx context.Context, t dbconn.Target, c dbconn.Creds) (string, error)
	// InfoFields 全量 INFO（D-08 白名单过滤在本层）。
	InfoFields(ctx context.Context, t dbconn.Target, c dbconn.Creds) (dbredis.InfoSections, error)
	// Slowlog 慢日志清单（条数上限 D-09）。
	Slowlog(ctx context.Context, t dbconn.Target, c dbconn.Creds, n int) ([]dbredis.SlowlogEntry, error)
	// SetRequirepass 设置 requirepass（旧密码验证语义 D-05）。
	SetRequirepass(ctx context.Context, t dbconn.Target, c dbconn.Creds, currentPassword, newPassword string) error
	// ScanKeys SCAN 游标分页（禁 KEYS，D-09）。
	ScanKeys(ctx context.Context, t dbconn.Target, c dbconn.Creds, cursor, pattern string, count int) (string, []dbredis.KeyItem, error)
	// KeyTTL 单 key TTL（-2 → dbredis.ErrKeyNotFound）。
	KeyTTL(ctx context.Context, t dbconn.Target, c dbconn.Creds, key string) (int64, error)
}

// MongoGateway 是 §4.5.1 MongoGateway 契约（adapter/dbmongo.Gateway 实现——
// 进程内 mongo-driver v2 通道，D-15；凭据明文仅在建连选项组装使用，⚠️A18）。
type MongoGateway interface {
	// Available 通道可用性（进程内库恒 nil——无二进制依赖）。
	Available() error
	// Ping 连接验证（SCRAM + authSource=admin，D-16；哨兵 dbmongo.ErrAuthRejected/ErrConnFailed）。
	Ping(ctx context.Context, t dbconn.Target, c dbconn.Creds) error
	// ServerVersion buildInfo.version（档案 version 刷新，D-04）。
	ServerVersion(ctx context.Context, t dbconn.Target, c dbconn.Creds) (string, error)
	// ListDatabases 库清单只读投影（名称/大小/集合数，D-18）。
	ListDatabases(ctx context.Context, t dbconn.Target, c dbconn.Creds) ([]dbmongo.DatabaseInfo, error)
	// StatusSummary serverStatus 白名单摘要（D-08 同构 ≤10 项）。
	StatusSummary(ctx context.Context, t dbconn.Target, c dbconn.Creds) (*dbmongo.StatusSummary, error)
	// ListUsers 用户清单实况投影（密码零出口 I-02）。
	ListUsers(ctx context.Context, t dbconn.Target, c dbconn.Creds) ([]dbmongo.UserItem, error)
	// CreateUser 建用户（admin 库 + 首库首角色，D-19；重名 → dbmongo.ErrUserExists）。
	CreateUser(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, password, db, role string) error
	// DropUser 删用户（连带回收全部角色；不存在 → dbmongo.ErrUserNotFound）。
	DropUser(ctx context.Context, t dbconn.Target, c dbconn.Creds, username string) error
	// GrantRole 授予库级角色（幂等）。
	GrantRole(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, db, role string) error
	// RevokeRole 回收库级角色（授权关系不存在语义由 service 前置校验承载——引擎不必然报错）。
	RevokeRole(ctx context.Context, t dbconn.Target, c dbconn.Creds, username, db, role string) error
}

// InstanceRepoAPI 是 db_instance 仓储契约（§4.1.5 InstanceRepository；实现
// 在 store/dbmgmt，接口定义在消费方）。
type InstanceRepoAPI interface {
	List(ctx context.Context, engine string) ([]storedb.Instance, error)
	GetByID(ctx context.Context, id int64) (*storedb.Instance, error)
	GetByIdentity(ctx context.Context, key string) (*storedb.Instance, error)
	UpsertDiscovered(ctx context.Context, inst storedb.Instance, connState string, version *string) (discovered bool, err error)
	UpdateCredentials(ctx context.Context, id int64, username, cipher string, version *string) error
	UpdateTargetAndCredentials(ctx context.Context, id int64, identityKey, connMode string, socketPath, host *string, port *int, username, cipher string, version *string) error
	UpdateConnState(ctx context.Context, id int64, connState string, version *string) error
	Delete(ctx context.Context, id int64) error
}

// TxAPI 是事务绑定仓储视图（档案变更与审计同事务，R-03/§3.2）。
type TxAPI interface {
	Instance() InstanceRepoAPI
	Audit() *storeauth.AuditRepo
	Commit() error
	Rollback() error
}

// StoreAPI 是仓储聚合契约（BeginTx 供同事务编排）。
type StoreAPI interface {
	Instance() InstanceRepoAPI
	BeginTx(ctx context.Context) (TxAPI, error)
}

// AuditRecorder 是审计 OHS 契约（R-03；service/audit.Service 实现）。
type AuditRecorder interface {
	Record(ctx context.Context, e audit.Entry) error
}

// clockNow 是可注入时钟（测试用）。
type clockNow func() time.Time

// storeAdapter / txAdapter 将 *storedb.Store 与 *storedb.TxStore 适配为
// StoreAPI/TxAPI（BeginTx 返回具体类型与接口签名差异的桥接；行为纯转发）。
type storeAdapter struct{ s *storedb.Store }

// AdaptStore 包装 *storedb.Store 为 StoreAPI（main 装配与测试共用）。
func AdaptStore(s *storedb.Store) StoreAPI { return storeAdapter{s: s} }

// Instance 返回档案仓储（*InstanceRepo 方法集满足 InstanceRepoAPI）。
func (a storeAdapter) Instance() InstanceRepoAPI { return a.s.Instance() }

// BeginTx 开启事务并返回接口视图。
func (a storeAdapter) BeginTx(ctx context.Context) (TxAPI, error) {
	tx, err := a.s.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	return txAdapter{t: tx}, nil
}

type txAdapter struct{ t *storedb.TxStore }

// Instance 返回事务内档案仓储。
func (a txAdapter) Instance() InstanceRepoAPI { return a.t.Instance() }

// Audit 返回事务内审计仓储（同事务写入，R-03）。
func (a txAdapter) Audit() *storeauth.AuditRepo { return a.t.Audit() }

// Commit 提交事务。
func (a txAdapter) Commit() error { return a.t.Commit() }

// Rollback 回滚事务。
func (a txAdapter) Rollback() error { return a.t.Rollback() }
