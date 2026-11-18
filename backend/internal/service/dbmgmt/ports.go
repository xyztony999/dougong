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
	// ---- 表级治理（v2.3，D-22；§4.1.5 扩展）----
	// ListTables 表清单实况投影（information_schema.TABLES；估算/可空如实）。
	ListTables(ctx context.Context, t dbconn.Target, c dbconn.Creds, db string) ([]dbmysql.TableInfo, error)
	// TableStructure 表结构实况投影（COLUMNS 列序；表不存在 → dbmysql.ErrTableNotFound）。
	TableStructure(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, table string) ([]dbmysql.ColumnInfo, error)
	// TableRows 行数据分页（JSON_OBJECT 逐行聚合；total=COUNT(*) 实况）。
	TableRows(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, table string, offset, limit int) ([]map[string]any, int64, error)
	// DropTable 删表（不存在 → dbmysql.ErrTableNotFound；不可逆）。
	DropTable(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, table string) error
	// UpdateRow 单列主键行编辑（WHERE pk=旧值 quoteString；null→NULL；affected=0 → DBM_024）。
	UpdateRow(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, table, pkColumn, pkValue string, values map[string]any) (int64, error)
	// DeleteRow 单列主键行删除（affected=0 → DBM_024）。
	DeleteRow(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, table, pkColumn, pkValue string) (int64, error)
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
	// ---- 集合与文档治理（v2.3，D-23/D-25；§4.5.1 扩展）----
	// DropDatabase 删库（dropDatabase 不可逆；系统库保护 I-12 由 service 前置）。
	DropDatabase(ctx context.Context, t dbconn.Target, c dbconn.Creds, db string) error
	// ListCollections 集合清单实况投影（名称/文档数估算/大小）。
	ListCollections(ctx context.Context, t dbconn.Target, c dbconn.Creds, db string) ([]dbmongo.CollectionInfo, error)
	// CreateCollection 建集合（隐式建库语义；重名 → dbmongo.ErrCollectionExists）。
	CreateCollection(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, name string) error
	// DropCollection 删集合（不存在 → dbmongo.ErrCollectionNotFound）。
	DropCollection(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, name string) error
	// ListDocuments 文档分页（find skip/limit + CountDocuments；扩展 JSON 形态）。
	ListDocuments(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, coll string, skip, limit int) ([]map[string]any, int64, error)
	// InsertDocument 文档插入（InsertOne；_id 冲突 → dbmongo.ErrDocumentExists）。
	InsertDocument(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, coll string, doc map[string]any) (any, error)
	// ReplaceDocument 文档编辑（ReplaceOne by _id 整体替换；matched=0 → DBM_027）。
	ReplaceDocument(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, coll string, id any, doc map[string]any) (int64, error)
	// DeleteDocument 文档删除（DeleteOne by _id；deleted=0 → DBM_027）。
	DeleteDocument(ctx context.Context, t dbconn.Target, c dbconn.Creds, db, coll string, id any) (int64, error)
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
