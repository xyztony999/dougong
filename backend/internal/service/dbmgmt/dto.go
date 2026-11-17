package dbmgmt

import "time"

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
}

// InstanceListData 是 instance/list data。
type InstanceListData struct {
	Items       []InstanceItem `json:"items"`
	GeneratedAt time.Time      `json:"generatedAt"`
}

// RescanData 是 instance/rescan data（{discovered, updated}）。
type RescanData struct {
	Discovered int `json:"discovered"`
	Updated    int `json:"updated"`
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
