/**
 * database-mgmt API 类型 —— 与 api-database-mgmt.md v1.1 4.1.4/4.2.4/4.3.4/4.4.4 对齐（唯一事实源）。
 * 实例档案（db_instance 合一表 D-01）为唯一持久化投影；database/db_user/key/慢日志/进程列表
 * 为引擎实况投影（I-04 零台账：每请求实时读取，响应带 generatedAt）。
 * 凭据纪律（⚠️A18/I-02）：任何响应无密码/密文/密钥字段——credUsername 可见（连接账户名非秘密）；
 * 密码仅出现在 POST 请求体（credentials/dbuserCreate/root-password/redisPassword），零回显。
 * TTL 为整数秒（-1=无过期，Redis 语义如实透传，api 3.1 时间格式）。
 */
import type { DbConnMode, DbEngine, GrantLevel, InstanceConnState, MongoRole } from './enums'

/** ---- 实例档案（4.1.4 instance/list items；cred_cipher/密钥零下发 I-02） ---- */

export interface DbInstanceItem {
  /** 聚合根标识（instanceId，治理页路由参数） */
  id: number
  engine: DbEngine
  connMode: DbConnMode
  /** socket 模式必填；tcp 模式缺失（I-07 互斥） */
  socketPath?: string
  /** tcp 模式必填；socket 模式缺失 */
  host?: string
  port?: number
  /** 连接建立后实况刷新（D-04：探测初期可空，如实呈现 — ） */
  version?: string
  connState: InstanceConnState
  /** 派生属性（cred_username 非空即已配置，DBM_004 判据） */
  credentialsSet: boolean
  credUsername?: string
  credUpdatedAt?: string
  discoveredAt: string
}

/** GET /db/instance/list 响应 data */
export interface InstanceListData {
  items: DbInstanceItem[]
  generatedAt: string
}

/** POST /db/instance/rescan 响应 data（探测编排结果，PageSpec 4.1 toast 文案来源） */
export interface RescanResult {
  discovered: number
  updated: number
}

/** POST /db/instance/credentials req（D-03：连接目标纠正字段可选；密码仅入请求体零回显） */
export interface CredentialsPayload {
  instanceId: number
  connMode?: DbConnMode
  socketPath?: string
  host?: string
  port?: number
  username: string
  password: string
}

/** POST /db/instance/credentials 成功 data（先验证后落库——响应恒 reachable） */
export interface CredentialsResult {
  instanceId: number
  connState: InstanceConnState
  version?: string
}

/** POST /db/instance/remove 成功 data */
export interface InstanceIdResult {
  instanceId: number
}

/** ---- MySQL/MariaDB 观测（4.1.4；白名单 D-08，不透传全量 variables） ---- */

/** GET /db/mysql/status 响应 data（variables = MYSQL_STATUS_FIELDS 白名单 ≤10 项） */
export interface MysqlStatusData {
  version: string
  variables: Record<string, string>
}

/** GET /db/mysql/processlist items（info/state/db 可空——MySQL PROCESSLIST NULL 如实） */
export interface MysqlProcessItem {
  id: number
  user: string
  host: string
  db: string | null
  command: string
  timeSec: number
  state: string | null
  info: string | null
}

export interface MysqlProcesslistData {
  items: MysqlProcessItem[]
  generatedAt: string
}

/** POST /db/mysql/root-password / /db/redis/password req（密码零回显，仅请求体） */
export interface PasswordChangePayload {
  instanceId: number
  newPassword: string
}

/** 改密成功 data（两步同步 D-05；同步失败返回 DBM_019，data.recoverHint="credentials"） */
export interface PasswordSyncResult {
  instanceId: number
  synced: boolean
}

/** ---- Redis 观测（4.1.4；REDIS_INFO_FIELDS 白名单分组 D-08，不透传全量 info） ---- */

/** GET /db/redis/info 响应 data（sections 四分组白名单字段；值为 Redis 实况原样透传） */
export interface RedisInfoData {
  sections: {
    server: { version: string }
    clients: { connectedClients: string }
    memory: { usedMemoryHuman: string; maxMemoryHuman: string }
    stats: {
      totalConnectionsReceived: string
      keyspaceHits: string
      keyspaceMisses: string
    }
  }
}

/** GET /db/redis/slowlog items（durationUs 微秒——展示层转 ms，PageSpec 6.2） */
export interface RedisSlowlogItem {
  id: number
  /** ISO-8601 UTC（本地时区展示，公共约定） */
  timestamp: string
  durationUs: number
  command: string
}

export interface RedisSlowlogData {
  items: RedisSlowlogItem[]
  generatedAt: string
}

/** ---- database 投影（4.2.4；引擎实况零台账 I-04） ---- */

export interface DbDatabaseItem {
  name: string
  charset: string
}

export interface DatabaseListData {
  items: DbDatabaseItem[]
  generatedAt: string
}

/** GET /db/database/charsets 响应 data（实例实况开放集合，非固定枚举 D-11） */
export interface DatabaseCharsetsData {
  items: string[]
}

/** POST /db/database/create req（charset 空取默认 utf8mb4，D-11） */
export interface DatabaseCreatePayload {
  instanceId: number
  name: string
  charset?: string
}

/** POST /db/database/create 成功 data（charset 回填实际生效值） */
export interface DatabaseCreateResult {
  instanceId: number
  name: string
  charset: string
}

/** POST /db/database/delete 成功 data */
export interface DatabaseMutationResult {
  instanceId: number
  name: string
}

/** ---- db_user 投影（4.3.4；无任何密码字段 I-02 同源纪律） ---- */

/** 授权关系值对象投影（raw 权限经 D-10 映射表归并三档；库外/未识别权限不呈现） */
export interface DbGrantItem {
  database: string
  level: GrantLevel
}

export interface DbUserItem {
  username: string
  host: string
  grants: DbGrantItem[]
}

export interface DbUserListData {
  items: DbUserItem[]
  generatedAt: string
}

export interface DbUserCreatePayload {
  instanceId: number
  username: string
  host: string
  password: string
}

export interface DbUserMutationResult {
  instanceId: number
  username: string
  host: string
}

export interface DbUserGrantPayload {
  instanceId: number
  username: string
  host: string
  database: string
  level: GrantLevel
}

/** POST /db/dbuser/grant 成功 data（幂等：重复授权结果一致，api 4.1.4） */
export interface DbUserGrantResult {
  instanceId: number
  username: string
  host: string
  database: string
  level: GrantLevel
}

/** POST /db/dbuser/revoke 成功 data（第二次 revoke 报 DBM_011，如实） */
export interface DbUserRevokeResult {
  instanceId: number
  username: string
  host: string
  database: string
}

/** ---- key 浏览（4.4.4；SCAN 游标分页 D-09，COUNT=200 服务端常量，前端不传） ---- */

export interface KeyListQuery {
  instanceId: number
  /** 默认 "0"（首页/重查起点） */
  cursor?: string
  /** 默认 "*"；服务端形态校验 DBM_017（禁 KEYS 全量语义） */
  pattern?: string
}

export interface KeyListItem {
  key: string
  /** Redis 原生类型透传（string/list/hash/set/zset/stream…开放集合，非池枚举） */
  type: string
}

export interface KeyListData {
  nextCursor: string
  /** nextCursor="0" 即遍历完（D-09） */
  complete: boolean
  items: KeyListItem[]
  generatedAt: string
}

/** GET /db/key/ttl 响应 data（-1=无过期，Redis 语义如实透传；不存在报 DBM_015） */
export interface KeyTtlData {
  key: string
  ttlSec: number
}

/** ---- 备份（4.1.4；export 双形态响应 D-06：成功=octet-stream 流、失败=JSON 错误体） ---- */

/** POST /db/backup/import 成功 data（multipart 上传，临时文件用后即删） */
export interface BackupImportResult {
  instanceId: number
  database: string
  sizeBytes: number
}

/** ---- 错误码与展示常量（PageSpec 第 1 节错误处理 + api 3.4，逐条对应） ---- */

/**
 * DBM_001~020 错误文案（页面级分流——拦截器不含 DBM 码集，service-mgmt SVC 先例）：
 * DBM_005/006 凭据弹窗内联、DBM_002/018 守卫分流、DBM_019 强提示弹窗在各处理路径单独引用。
 */
export const DBM_ERROR_TEXT: Readonly<Record<string, string>> = {
  DBM_001: 'instanceId 非法',
  DBM_002: '实例档案不存在（可能已移除）',
  DBM_003: '实例不可达——治理操作无法执行（实例启停请前往服务管理）',
  DBM_004: '实例未配置凭据，暂不可治理（请先配置凭据）',
  DBM_005: '认证失败：用户名或密码错误',
  DBM_006: '连接失败：网络或 socket 不可达',
  DBM_007: '目标库不存在',
  DBM_008: '同名库已存在',
  DBM_009: '目标用户不存在',
  DBM_010: '同名同 host 用户已存在',
  DBM_011: '目标授权关系不存在（可能已被回收）',
  DBM_012: '备份导出执行失败',
  DBM_013: '导入执行失败（不自动回滚）',
  DBM_014: '导入文件超过 512MiB 上限',
  DBM_015: 'key 不存在（可能已过期）',
  DBM_016: '实例有导出/导入任务在途，请稍后重试',
  DBM_017: 'key 匹配模式非法（不支持全量语义）',
  DBM_018: '接口与实例引擎不符',
  DBM_019: '实例密码已修改成功，但面板凭据同步失败——请立即重新配置凭据',
  DBM_020: '该连接目标已存在其他实例档案',
  DBM_021: '引擎客户端通道不可用（宿主机未安装 mysql/redis-cli）',
  DBM_022: '引擎操作被拒（凭据账户权限不足）'
}

/** ApiError.data（unknown）→ data.reason 收窄（DBM_003/012/013 失败原因如实展示，api 3.4） */
export function reasonOf(data: unknown): string | null {
  if (typeof data === 'object' && data !== null) {
    const o = data as Record<string, unknown>
    if (typeof o.reason === 'string' && o.reason !== '') return o.reason
  }
  return null
}

/** DBM 错误 → toast 文案（含 reason 的码：003/012/013 追加服务端原因；未知码返回 null 不提示） */
export function dbmErrorText(code: string, data: unknown): string | null {
  const base = DBM_ERROR_TEXT[code]
  if (base === undefined) return null
  if (code === 'DBM_003' || code === 'DBM_012' || code === 'DBM_013' || code === 'DBM_021' || code === 'DBM_022') {
    const reason = reasonOf(data)
    if (reason !== null) return `${base}：${reason}`
  }
  return base
}

/** ---- 观测白名单展示常量（D-08 REDIS_INFO_FIELDS/MYSQL_STATUS_FIELDS 前端标签映射） ---- */

/** MySQL status variables 白名单 ≤10 项的中文标签（api D-08 定稿清单；未识别键原样展示） */
export const MYSQL_STATUS_FIELD_TEXT: Readonly<Record<string, string>> = {
  version_comment: '服务器版本说明',
  max_connections: '最大连接数',
  character_set_server: '服务器字符集',
  collation_server: '服务器排序规则',
  innodb_buffer_pool_size: 'InnoDB 缓冲池大小',
  max_allowed_packet: '最大报文大小',
  wait_timeout: '等待超时（秒）',
  connect_timeout: '连接超时（秒）',
  slow_query_log: '慢查询日志',
  general_log: '通用查询日志'
}

/** Redis info 白名单字段中文标签（api D-08；sections 分组 server/clients/memory/stats） */
export const REDIS_INFO_FIELD_TEXT: Readonly<Record<string, string>> = {
  version: 'Redis 版本',
  connectedClients: '已连接客户端数',
  usedMemoryHuman: '已用内存',
  maxMemoryHuman: '内存上限',
  totalConnectionsReceived: '累计连接数',
  keyspaceHits: '键空间命中',
  keyspaceMisses: '键空间未命中'
}

/** 导入文件大小上限（DUMP_SIZE_LIMIT=512MiB，api D-06；前端预检，服务端 DBM_014 为准） */
export const DUMP_SIZE_LIMIT_BYTES = 512 * 1024 * 1024

/** ---- mongo 治理（api-database-mgmt.md v2.0 §4.5.2；v0.1.4 反馈第 3 项增量） ---- */

/** mongoStatus 响应 data（serverStatus 白名单 ≤10 项，D-08 同构） */
export interface MongoStatusData {
  version: string
  uptimeSec: number
  connectionsCurrent: number
  connectionsAvailable: number
  memResidentMb: number
  opInsert: number
  opQuery: number
  opUpdate: number
  opDelete: number
}

/** mdb/list items 条目（只读投影 D-18：名称/大小/集合数；系统库 admin/config/local 如实返回） */
export interface MongoDatabaseItem {
  name: string
  sizeBytes: number
  collections: number
}

export interface MongoDatabaseListData {
  items: MongoDatabaseItem[]
  generatedAt: string
}

/** 用户持有的库级角色（mongo_role 三档 D-19） */
export interface MongoUserRole {
  db: string
  role: string
}

/** muser/list items 条目（密码/凭证零出口 I-02） */
export interface MongoUserItem {
  username: string
  mechanisms: string[]
  roles: MongoUserRole[]
}

export interface MongoUserListData {
  items: MongoUserItem[]
  generatedAt: string
}

/** mongoUserCreate payload（首库首角色 D-19） */
export interface MongoUserCreatePayload {
  instanceId: number
  username: string
  password: string
  db: string
  role: MongoRole
}

/** mongoUserDelete payload */
export interface MongoUserPayload {
  instanceId: number
  username: string
}

/** mongoUserGrant / mongoUserRevoke payload（幂等授予/实况校验回收） */
export interface MongoUserRolePayload {
  instanceId: number
  username: string
  db: string
  role: MongoRole
}

export interface MongoUserResult {
  instanceId: number
  username: string
}

export interface MongoRoleResult {
  instanceId: number
  username: string
  db: string
  role: string
}
