package dbmgmt

import (
	"encoding/json"
	"time"
)

// ---- instance（§4.1.4）----

// InstanceItem 是 GET /db/instance/list items 条目（cred_cipher/密钥零下发
// I-02；credentialsSet 为 DBM_004 判据派生属性）。
type InstanceItem struct {
	ID             int64      `json:"id"`
	Engine         string     `json:"engine"`
	ConnMode       string     `json:"connMode"`
	SocketPath     *string    `json:"socketPath,omitempty"`
	Host           *string    `json:"host,omitempty"`
	Port           *int       `json:"port,omitempty"`
	Version        *string    `json:"version,omitempty"`
	ConnState      string     `json:"connState"`
	CredentialsSet bool       `json:"credentialsSet"`
	CredUsername   *string    `json:"credUsername,omitempty"`
	CredUpdatedAt  *time.Time `json:"credUpdatedAt,omitempty"`
	DiscoveredAt   time.Time  `json:"discoveredAt"`
	// RedundantSocket v2.3 additive（D-21）：conn_mode=socket 且同 engine
	// 本机 TCP 档案存在且可达时派生 true（已配凭据冗余 socket 行保留+标注，
	// 引导改配 TCP 后手动 remove）；其余 false。
	RedundantSocket bool `json:"redundantSocket"`
}

// InstanceListData 是 instance/list data。
type InstanceListData struct {
	Items       []InstanceItem `json:"items"`
	GeneratedAt time.Time      `json:"generatedAt"`
}

// RescanData 是 instance/rescan data（{discovered, updated, merged}；
// merged 为 v2.3 additive——本轮保守清理的冗余 socket 行数，D-21）。
type RescanData struct {
	Discovered int `json:"discovered"`
	Updated    int `json:"updated"`
	Merged     int `json:"merged"`
}

// CredentialsRequest 是 instance/credentials 请求（连接目标纠正字段可选 D-03）。
type CredentialsRequest struct {
	InstanceID int64   `json:"instanceId"`
	ConnMode   *string `json:"connMode"`
	SocketPath *string `json:"socketPath"`
	Host       *string `json:"host"`
	Port       *int    `json:"port"`
	Username   string  `json:"username"`
	Password   string  `json:"password"`
}

// CredentialsData 是 credentials data。
type CredentialsData struct {
	InstanceID int64  `json:"instanceId"`
	ConnState  string `json:"connState"`
	Version    string `json:"version"`
}

// InstanceIDRequest 是 remove 等单字段请求（instanceId 数值形态）。
type InstanceIDRequest struct {
	InstanceID int64 `json:"instanceId"`
}

// RemoveData 是 remove data。
type RemoveData struct {
	InstanceID int64 `json:"instanceId"`
}

// ---- mysql 观测（§4.1.4）----

// MySQLStatusData 是 mysql/status data（D-08 白名单 ≤10 项）。
type MySQLStatusData struct {
	Version   string            `json:"version"`
	Variables map[string]string `json:"variables"`
}

// ProcessItem 是 processlist items 条目（db/state/info 可空）。
type ProcessItem struct {
	ID      int64   `json:"id"`
	User    string  `json:"user"`
	Host    string  `json:"host"`
	DB      *string `json:"db"`
	Command string  `json:"command"`
	TimeSec int64   `json:"timeSec"`
	State   *string `json:"state"`
	Info    *string `json:"info"`
}

// ProcessListData 是 processlist data。
type ProcessListData struct {
	Items       []ProcessItem `json:"items"`
	GeneratedAt time.Time     `json:"generatedAt"`
}

// ---- mongo 治理（§4.5，v0.1.4 反馈第 3 项增量）----

// MongoStatusData 是 mongo/status data（serverStatus 白名单，D-08 同构 ≤10 项）。
type MongoStatusData struct {
	Version              string `json:"version"`
	UptimeSec            int64  `json:"uptimeSec"`
	ConnectionsCurrent   int64  `json:"connectionsCurrent"`
	ConnectionsAvailable int64  `json:"connectionsAvailable"`
	MemResidentMb        int64  `json:"memResidentMb"`
	OpInsert             int64  `json:"opInsert"`
	OpQuery              int64  `json:"opQuery"`
	OpUpdate             int64  `json:"opUpdate"`
	OpDelete             int64  `json:"opDelete"`
}

// MongoDatabaseItem 是 mdb/list items 条目（只读投影，D-18）。
type MongoDatabaseItem struct {
	Name        string `json:"name"`
	SizeBytes   int64  `json:"sizeBytes"`
	Collections int64  `json:"collections"`
}

// MongoDatabaseListData 是 mdb/list data。
type MongoDatabaseListData struct {
	Items       []MongoDatabaseItem `json:"items"`
	GeneratedAt time.Time           `json:"generatedAt"`
}

// MongoUserRole 是用户持有的库级角色（mongo_role，D-19）。
type MongoUserRole struct {
	DB   string `json:"db"`
	Role string `json:"role"`
}

// MongoUserItem 是 muser/list items 条目（密码零出口 I-02）。
type MongoUserItem struct {
	Username   string          `json:"username"`
	Mechanisms []string        `json:"mechanisms"`
	Roles      []MongoUserRole `json:"roles"`
}

// MongoUserListData 是 muser/list data。
type MongoUserListData struct {
	Items       []MongoUserItem `json:"items"`
	GeneratedAt time.Time       `json:"generatedAt"`
}

// MongoUserCreateRequest 是 muser/create 请求（首库首角色，D-19）。
type MongoUserCreateRequest struct {
	InstanceID int64  `json:"instanceId"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	DB         string `json:"db"`
	Role       string `json:"role"`
}

// MongoUserRequest 是 muser/delete 请求。
type MongoUserRequest struct {
	InstanceID int64  `json:"instanceId"`
	Username   string `json:"username"`
}

// MongoUserRoleRequest 是 muser/grant、muser/revoke 请求。
type MongoUserRoleRequest struct {
	InstanceID int64  `json:"instanceId"`
	Username   string `json:"username"`
	DB         string `json:"db"`
	Role       string `json:"role"`
}

// MongoUserActionResult 是 muser/create、muser/delete data。
type MongoUserActionResult struct {
	InstanceID int64  `json:"instanceId"`
	Username   string `json:"username"`
}

// MongoRoleActionResult 是 muser/grant、muser/revoke data。
type MongoRoleActionResult struct {
	InstanceID int64  `json:"instanceId"`
	Username   string `json:"username"`
	DB         string `json:"db"`
	Role       string `json:"role"`
}

// PasswordRequest 是 root-password / redis/password 请求。
type PasswordRequest struct {
	InstanceID  int64  `json:"instanceId"`
	NewPassword string `json:"newPassword"`
}

// PasswordChangeData 是改密成功 data。
type PasswordChangeData struct {
	InstanceID int64 `json:"instanceId"`
	Synced     bool  `json:"synced"`
}

// ---- database（§4.2.4）----

// DatabaseItem 是 database/list items 条目。
type DatabaseItem struct {
	Name    string `json:"name"`
	Charset string `json:"charset"`
}

// DatabaseListData 是 database/list data。
type DatabaseListData struct {
	Items       []DatabaseItem `json:"items"`
	GeneratedAt time.Time      `json:"generatedAt"`
}

// CharsetsData 是 database/charsets data（实况开放集合，D-11）。
type CharsetsData struct {
	Items []string `json:"items"`
}

// DatabaseCreateRequest 是 database/create 请求（charset 空取默认 utf8mb4）。
type DatabaseCreateRequest struct {
	InstanceID int64  `json:"instanceId"`
	Name       string `json:"name"`
	Charset    string `json:"charset"`
}

// DatabaseCreateData 是 database/create data。
type DatabaseCreateData struct {
	InstanceID int64  `json:"instanceId"`
	Name       string `json:"name"`
	Charset    string `json:"charset"`
}

// DatabaseDeleteRequest 是 database/delete 请求。
type DatabaseDeleteRequest struct {
	InstanceID int64  `json:"instanceId"`
	Name       string `json:"name"`
}

// DatabaseDeleteData 是 database/delete data。
type DatabaseDeleteData struct {
	InstanceID int64  `json:"instanceId"`
	Name       string `json:"name"`
}

// ---- dbuser（§4.3.4）----

// GrantItem 是授权关系值对象投影（D-10 三档）。
type GrantItem struct {
	Database string `json:"database"`
	Level    string `json:"level"`
}

// DBUserItem 是 dbuser/list items 条目（无任何密码字段，I-02 同源纪律）。
type DBUserItem struct {
	Username string      `json:"username"`
	Host     string      `json:"host"`
	Grants   []GrantItem `json:"grants"`
}

// DBUserListData 是 dbuser/list data。
type DBUserListData struct {
	Items       []DBUserItem `json:"items"`
	GeneratedAt time.Time    `json:"generatedAt"`
}

// DBUserCreateRequest 是 dbuser/create 请求（密码零回显）。
type DBUserCreateRequest struct {
	InstanceID int64  `json:"instanceId"`
	Username   string `json:"username"`
	Host       string `json:"host"`
	Password   string `json:"password"`
}

// DBUserData 是 dbuser create/delete data。
type DBUserData struct {
	InstanceID int64  `json:"instanceId"`
	Username   string `json:"username"`
	Host       string `json:"host"`
}

// DBUserGrantRequest 是 dbuser/grant 请求。
type DBUserGrantRequest struct {
	InstanceID int64  `json:"instanceId"`
	Username   string `json:"username"`
	Host       string `json:"host"`
	Database   string `json:"database"`
	Level      string `json:"level"`
}

// DBUserGrantData 是 dbuser/grant data。
type DBUserGrantData struct {
	InstanceID int64  `json:"instanceId"`
	Username   string `json:"username"`
	Host       string `json:"host"`
	Database   string `json:"database"`
	Level      string `json:"level"`
}

// DBUserRevokeRequest 是 dbuser/revoke 请求。
type DBUserRevokeRequest struct {
	InstanceID int64  `json:"instanceId"`
	Username   string `json:"username"`
	Host       string `json:"host"`
	Database   string `json:"database"`
}

// DBUserRevokeData 是 dbuser/revoke data。
type DBUserRevokeData struct {
	InstanceID int64  `json:"instanceId"`
	Username   string `json:"username"`
	Host       string `json:"host"`
	Database   string `json:"database"`
}

// ---- redis（§4.1.4）----

// RedisInfoData 是 redis/info data（REDIS_INFO_FIELDS 白名单 sections 骨架）。
type RedisInfoData struct {
	Sections map[string]map[string]string `json:"sections"`
}

// SlowlogItem 是 redis/slowlog items 条目。
type SlowlogItem struct {
	ID         int64     `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	DurationUs int64     `json:"durationUs"`
	Command    string    `json:"command"`
}

// SlowlogData 是 redis/slowlog data。
type SlowlogData struct {
	Items       []SlowlogItem `json:"items"`
	GeneratedAt time.Time     `json:"generatedAt"`
}

// ---- key（§4.4.4）----

// KeyItem 是 key/list items 条目。
type KeyItem struct {
	Key  string `json:"key"`
	Type string `json:"type"`
}

// KeyListData 是 key/list data（complete = nextCursor=="0"）。
type KeyListData struct {
	NextCursor  string    `json:"nextCursor"`
	Complete    bool      `json:"complete"`
	Items       []KeyItem `json:"items"`
	GeneratedAt time.Time `json:"generatedAt"`
}

// KeyTTLData 是 key/ttl data（-1=无过期，Redis 语义如实透传）。
type KeyTTLData struct {
	Key    string `json:"key"`
	TTLSec int64  `json:"ttlSec"`
}

// ---- backup（§4.1.4）----

// ImportData 是 backup/import data。
type ImportData struct {
	InstanceID int64  `json:"instanceId"`
	Database   string `json:"database"`
	SizeBytes  int64  `json:"sizeBytes"`
}

// ---- table/row（v2.3 §4.7.4；可空字段显式 null——JSON null≠TS undefined，
// children:null 先例同口径，不用 omitempty 省略）----

// TableItem 是 table/list items 条目（rowEstimate=TABLE_ROWS 统计估算，
// InnoDB 非精确值；视图/特殊引擎可空如实；comment 无注释 null）。
type TableItem struct {
	Name          string  `json:"name"`
	Engine        string  `json:"engine"`
	RowEstimate   *int64  `json:"rowEstimate"`
	DataSizeBytes *int64  `json:"dataSizeBytes"`
	Collation     *string `json:"collation"`
	Comment       *string `json:"comment"`
}

// TableListData 是 table/list data。
type TableListData struct {
	Items       []TableItem `json:"items"`
	GeneratedAt time.Time   `json:"generatedAt"`
}

// ColumnItem 是 table/structure columns 条目（key=引擎 COLUMN_KEY 原值
// ""/PRI/UNI/MUL 开放字符串非枚举；default 无默认 null）。
type ColumnItem struct {
	Column   string  `json:"column"`
	Type     string  `json:"type"`
	Nullable bool    `json:"nullable"`
	Key      string  `json:"key"`
	Default  *string `json:"default"`
	Extra    string  `json:"extra"`
	Comment  *string `json:"comment"`
}

// TableStructureData 是 table/structure data（singlePk=恰好 1 列 key=PRI
// 派生；pkColumn 无单列主键时 null——前端行写入口禁用判定同源，I-10）。
type TableStructureData struct {
	Table       string       `json:"table"`
	Columns     []ColumnItem `json:"columns"`
	SinglePk    bool         `json:"singlePk"`
	PkColumn    *string      `json:"pkColumn"`
	GeneratedAt time.Time    `json:"generatedAt"`
}

// RowListData 是 row/list data（columns 来自结构列序；行值 JSON null=NULL
// 语义；total=COUNT(*) 实况）。
type RowListData struct {
	Columns     []string         `json:"columns"`
	Rows        []map[string]any `json:"rows"`
	Total       int64            `json:"total"`
	Page        int              `json:"page"`
	PageSize    int              `json:"pageSize"`
	GeneratedAt time.Time        `json:"generatedAt"`
}

// TableDropRequest 是 table/drop 请求。
type TableDropRequest struct {
	InstanceID int64  `json:"instanceId"`
	Database   string `json:"database"`
	Table      string `json:"table"`
}

// TableDropData 是 table/drop data。
type TableDropData struct {
	InstanceID int64  `json:"instanceId"`
	Database   string `json:"database"`
	Table      string `json:"table"`
}

// RowUpdateRequest 是 row/update 请求（values 文本值 string|null——null→
// SET col=NULL；主键列不可编辑 I-10；pkValue 为定位旧值）。
type RowUpdateRequest struct {
	InstanceID int64          `json:"instanceId"`
	Database   string         `json:"database"`
	Table      string         `json:"table"`
	PKValue    string         `json:"pkValue"`
	Values     map[string]any `json:"values"`
}

// RowUpdateData 是 row/update data。
type RowUpdateData struct {
	InstanceID int64  `json:"instanceId"`
	Database   string `json:"database"`
	Table      string `json:"table"`
	PKValue    string `json:"pkValue"`
	Affected   int64  `json:"affected"`
}

// RowDeleteRequest 是 row/delete 请求。
type RowDeleteRequest struct {
	InstanceID int64  `json:"instanceId"`
	Database   string `json:"database"`
	Table      string `json:"table"`
	PKValue    string `json:"pkValue"`
}

// RowDeleteData 是 row/delete data。
type RowDeleteData struct {
	InstanceID int64  `json:"instanceId"`
	Database   string `json:"database"`
	Table      string `json:"table"`
	PKValue    string `json:"pkValue"`
	Affected   int64  `json:"affected"`
}

// ---- mcoll/mdoc/mdb-dropdb（v2.3 §4.8.4）----

// MongoCollectionItem 是 mcoll/list items 条目（docCount=estimatedDocumentCount
// 估算语义如实）。
type MongoCollectionItem struct {
	Name      string `json:"name"`
	DocCount  int64  `json:"docCount"`
	SizeBytes int64  `json:"sizeBytes"`
}

// MongoCollectionListData 是 mcoll/list data（空库返回空列表非错误）。
type MongoCollectionListData struct {
	Items       []MongoCollectionItem `json:"items"`
	GeneratedAt time.Time             `json:"generatedAt"`
}

// MongoCollectionCreateRequest 是 mcoll/create 请求（隐式建库语义——库随
// 首集合写入产生，无独立建库接口，D-25）。
type MongoCollectionCreateRequest struct {
	InstanceID int64  `json:"instanceId"`
	Database   string `json:"database"`
	Name       string `json:"name"`
}

// MongoCollectionCreateData 是 mcoll/create data。
type MongoCollectionCreateData struct {
	InstanceID int64  `json:"instanceId"`
	Database   string `json:"database"`
	Name       string `json:"name"`
}

// MongoCollectionDropRequest 是 mcoll/drop 请求。
type MongoCollectionDropRequest struct {
	InstanceID int64  `json:"instanceId"`
	Database   string `json:"database"`
	Name       string `json:"name"`
}

// MongoCollectionDropData 是 mcoll/drop data。
type MongoCollectionDropData struct {
	InstanceID int64  `json:"instanceId"`
	Database   string `json:"database"`
	Name       string `json:"name"`
}

// MongoDatabaseDropRequest 是 mdb/dropdb 请求（系统库保护 I-12/DBM_028）。
type MongoDatabaseDropRequest struct {
	InstanceID int64  `json:"instanceId"`
	Database   string `json:"database"`
}

// MongoDatabaseDropData 是 mdb/dropdb data。
type MongoDatabaseDropData struct {
	InstanceID int64  `json:"instanceId"`
	Database   string `json:"database"`
}

// MongoDocumentListData 是 mdoc/list data（documents 保引擎自然序，_id 置顶
// 呈现归前端排序；BSON→扩展 JSON 形态 $oid/$date 透传）。
type MongoDocumentListData struct {
	Documents   []map[string]any `json:"documents"`
	Total       int64            `json:"total"`
	Page        int              `json:"page"`
	PageSize    int              `json:"pageSize"`
	GeneratedAt time.Time        `json:"generatedAt"`
}

// MongoDocumentInsertRequest 是 mdoc/insert 请求（document 为 JSON 对象文本
// ——服务端解析校验 DBM_026；_id 可选缺省引擎自动生成）。
type MongoDocumentInsertRequest struct {
	InstanceID int64           `json:"instanceId"`
	Database   string          `json:"database"`
	Collection string          `json:"collection"`
	Document   json.RawMessage `json:"document"`
}

// MongoDocumentInsertData 是 mdoc/insert data（insertedId=插入文档 _id 的
// JSON 形态，ObjectId 为 {"$oid":"hex"}）。
type MongoDocumentInsertData struct {
	InstanceID int64  `json:"instanceId"`
	Database   string `json:"database"`
	Collection string `json:"collection"`
	InsertedID any    `json:"insertedId"`
}

// MongoDocumentUpdateRequest 是 mdoc/update 请求（id=mdoc/list 响应 _id
// 原值透传；document._id 若存在必须与 id 一致——_id 不可变更 D-23）。
type MongoDocumentUpdateRequest struct {
	InstanceID int64           `json:"instanceId"`
	Database   string          `json:"database"`
	Collection string          `json:"collection"`
	ID         any             `json:"id"`
	Document   json.RawMessage `json:"document"`
}

// MongoDocumentUpdateData 是 mdoc/update data。
type MongoDocumentUpdateData struct {
	InstanceID int64  `json:"instanceId"`
	Database   string `json:"database"`
	Collection string `json:"collection"`
	ID         any    `json:"id"`
	Matched    int64  `json:"matched"`
}

// MongoDocumentDeleteRequest 是 mdoc/delete 请求。
type MongoDocumentDeleteRequest struct {
	InstanceID int64  `json:"instanceId"`
	Database   string `json:"database"`
	Collection string `json:"collection"`
	ID         any    `json:"id"`
}

// MongoDocumentDeleteData 是 mdoc/delete data。
type MongoDocumentDeleteData struct {
	InstanceID int64  `json:"instanceId"`
	Database   string `json:"database"`
	Collection string `json:"collection"`
	ID         any    `json:"id"`
	Deleted    int64  `json:"deleted"`
}
