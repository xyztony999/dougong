/**
 * database-mgmt API 类型 —— 与 api-database-mgmt.md v2.3 对齐（唯一事实源；
 * 4.1.4/4.2.4/4.3.4/4.4.4/4.5.2 + v2.3 新增 4.7 表级/4.8 集合文档）。
 * 实例档案（db_instance 合一表 D-01）为唯一持久化投影；database/db_user/key/慢日志/进程列表/
 * 表/行/集合/文档为引擎实况投影（I-04 零台账：每请求实时读取，响应带 generatedAt）。
 * 凭据纪律（⚠️A18/I-02）：任何响应无密码/密文/密钥字段——credUsername 可见（连接账户名非秘密）；
 * 密码仅出现在 POST 请求体（credentials/dbuserCreate/root-password/redisPassword），零回显。
 * TTL 为整数秒（-1=无过期，Redis 语义如实透传，api 3.1 时间格式）。
 * v2.3：instance/list +redundantSocket（D-21 TCP 优先保守清理标注）、rescan +merged；
 * 行值/文档 JSON null≠TS undefined 显式 | null（api 4.7.4/4.8.4 DTO 逐形状）。
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
  /**
   * v2.3 D-21：conn_mode=socket 且同 engine 本机 TCP 档案存在且可达时派生 true——
   * 已配凭据的冗余 socket 行保留+标注（保守清理口径，不做凭据自动迁移）。
   */
  redundantSocket: boolean
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
  /** v2.3 D-21：本轮保守清理的冗余 socket 行数（仅无凭据行；additive 字段） */
  merged: number
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
 * DBM_001~029 错误文案（页面级分流——拦截器不含 DBM 码集，service-mgmt SVC 先例）：
 * DBM_005/006 凭据弹窗内联、DBM_002/018 守卫分流、DBM_019 强提示弹窗在各处理路径单独引用。
 * v2.3 续编 DBM_022~028（api 3.4/D-26）+ DBM_029 引擎操作被拒（A-DB13 编号裁定闭环：
 * 此前实现轮 ErrDBMEngineUnauthorized 曾占 DBM_022，与 v2.3 TABLE_NOT_FOUND 冲突，
 * 后端增量轮已改编号 DBM_029〔errcode/dbmgmt.go〕，前端映射/弹窗判定本轮同步）。
 */
export const DBM_ERROR_TEXT: Readonly<Record<string, string>> = {
  DBM_001: 'instanceId 非法',
  DBM_002: '实例档案不存在（可能已移除）',
  DBM_003: '实例不可达——管理操作无法执行（实例启停请前往服务管理）',
  DBM_004: '实例未配置账号密码，暂不可管理（请先配置账号密码）',
  DBM_005: '认证被拒：账户、host 或密码未通过服务器验证',
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
  DBM_019: '实例密码已修改成功，但面板保存的账号密码同步失败——请立即重新配置账号密码',
  DBM_020: '该连接目标已存在其他实例档案',
  DBM_021: '引擎客户端通道不可用（宿主机未安装 mysql/redis-cli）',
  DBM_022: '目标表不存在',
  DBM_023: '该表无单列主键，不支持行级编辑/删除',
  DBM_024: '行已不存在或已被修改',
  DBM_025: '目标集合不存在',
  DBM_026: '文档 JSON 非法',
  DBM_027: '文档已不存在',
  DBM_028: '系统库（admin/config/local）不可删除',
  DBM_029: '数据库拒绝操作：当前账号权限不足'
}

/** ApiError.data（unknown）→ data.reason 收窄（DBM_003/012/013 失败原因如实展示，api 3.4） */
export function reasonOf(data: unknown): string | null {
  if (typeof data === 'object' && data !== null) {
    const o = data as Record<string, unknown>
    if (typeof o.reason === 'string' && o.reason !== '') return o.reason
  }
  return null
}

/**
 * DBM 错误 → toast 文案（含 reason 的码：003/005/006/012/013/021/023/029 追加服务端原因；
 * 未知码返回 null 不提示）。DBM_005/006 追加 reason 为 v0.1.7 UX 增强——治理操作
 * 因凭据失效被拒时，服务器原文（如 1045 含 host 形态）随 toast 如实呈现辅助诊断。
 * v2.3：DBM_023 data.reason 如实提示无单列主键原因（无主键/复合主键，api 3.4）。
 * DBM_029（A-DB13 闭环）：引擎操作被拒 data.reason=引擎原文（如 Unauthorized code 13），
 * 区别于 DBM_005 认证被拒——身份对但无权执行该命令。
 */
export function dbmErrorText(code: string, data: unknown): string | null {
  const base = DBM_ERROR_TEXT[code]
  if (base === undefined) return null
  if (
    code === 'DBM_003' || code === 'DBM_005' || code === 'DBM_006' ||
    code === 'DBM_012' || code === 'DBM_013' || code === 'DBM_021' || code === 'DBM_023' ||
    code === 'DBM_029'
  ) {
    const reason = reasonOf(data)
    if (reason !== null) return `${base}：${reason}`
  }
  return base
}

/**
 * DBM_005 认证被拒的可操作排查指引（v0.1.7 UX 增强，凭据弹窗内联呈现）。
 * 按引擎族给出——MySQL 族 1045 不区分账户不存在/host 落空/密码不符，指引覆盖
 * 全部可能而非断言密码错；纯函数（弹窗与单测共用，CI 无引擎依赖）。
 */
export function dbmAuthRejectedTips(engine: string): string[] {
  if (engine === 'mysql' || engine === 'mariadb') {
    return [
      '确认账户已创建：MySQL 对不存在的账户同样返回 Access denied——可在服务器执行 sudo mysql -e "SELECT user,host,plugin FROM mysql.user;" 核查',
      '确认账户 host 覆盖连接来源：TCP 连 localhost 时账户 host 须为 localhost / 127.0.0.1 / %（面板「用户管理」建用户默认 host=localhost）',
      '确认密码与创建时一致：@ 等特殊字符面板原样传递；遗忘可在服务器经 ALTER USER 重置，或经面板「用户管理」删除重建（需实例已有有效凭据）'
    ]
  }
  if (engine === 'redis') {
    return [
      '确认密码与服务器 requirepass（或 ACL 用户密码）一致；Redis 6+ ACL 账户需用户名与密码同时匹配'
    ]
  }
  if (engine === 'mongodb') {
    return [
      '确认账户存在于 admin 库（面板固定 authSource=admin 验证）且密码正确；可经「用户治理」核查或重建'
    ]
  }
  return ['确认账户已创建且密码正确；认证被拒的具体原因见上方服务器原文']
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

/** ---- MySQL 表级治理（api v2.3 §4.7；data_table/table_row 实况投影零台账 I-04） ---- */

/** GET /db/table/list items（information_schema.TABLES；可空字段显式 | null——JSON null≠TS undefined） */
export interface DataTableItem {
  name: string
  /** 存储引擎实况开放集合（非字典，PageSpec 第 3 节偏差声明） */
  engine: string
  /** TABLE_ROWS 统计估算（InnoDB 非精确值；视图/特殊引擎可空） */
  rowEstimate: number | null
  dataSizeBytes: number | null
  collation: string | null
  comment: string | null
}

export interface TableListData {
  items: DataTableItem[]
  generatedAt: string
}

/** GET /db/table/structure columns（information_schema.COLUMNS；key=引擎原值 ""/PRI/UNI/MUL 开放字符串） */
export interface TableColumnDef {
  column: string
  type: string
  nullable: boolean
  key: string
  default: string | null
  extra: string
  comment: string | null
}

/** GET /db/table/structure 响应 data（singlePk=恰好 1 列 PRI 派生；行写入口禁用判定同源 I-10） */
export interface TableStructureData {
  table: string
  columns: TableColumnDef[]
  singlePk: boolean
  pkColumn: string | null
  generatedAt: string
}

/** 行单元格值（JSON_OBJECT 原生映射：string/number/boolean/null——null=NULL 语义≠TS undefined） */
export type RowCellValue = string | number | boolean | null

/** GET /db/row/list 行（列名→值；columns 来自结构列序） */
export type TableRow = Record<string, RowCellValue>

/** GET /db/row/list query（page 默认 1；pageSize 默认 20 上限 100=TABLE_PAGE_LIMIT，D-22） */
export interface RowListQuery {
  instanceId: number
  database: string
  table: string
  page?: number
  pageSize?: number
}

export interface RowListData {
  columns: string[]
  rows: TableRow[]
  total: number
  page: number
  pageSize: number
  generatedAt: string
}

/** POST /db/table/drop req（DROP TABLE 不可逆，交互层强确认前置） */
export interface TableDropPayload {
  instanceId: number
  database: string
  table: string
}

export interface TableMutationResult {
  instanceId: number
  database: string
  table: string
}

/** POST /db/row/update req（values 仅含变更列且不含主键列；null→SET col=NULL，D-22） */
export interface RowUpdatePayload {
  instanceId: number
  database: string
  table: string
  pkValue: string
  values: Record<string, string | null>
}

/** POST /db/row/delete req（WHERE 主键=旧值参数化定位，I-10） */
export interface RowDeletePayload {
  instanceId: number
  database: string
  table: string
  pkValue: string
}

export interface RowMutationResult {
  instanceId: number
  database: string
  table: string
  pkValue: string
  affected: number
}

/** ---- MongoDB 集合与文档治理（api v2.3 §4.8；mongo_collection/mongo_document 实况投影） ---- */

/** GET /db/mcoll/list items（docCount=estimatedDocumentCount 估算语义如实） */
export interface MongoCollectionItem {
  name: string
  docCount: number
  sizeBytes: number
}

export interface MongoCollectionListData {
  items: MongoCollectionItem[]
  generatedAt: string
}

/** POST /db/mcoll/create req（隐式建库语义：库随首集合写入产生，D-25） */
export interface MongoCollectionCreatePayload {
  instanceId: number
  database: string
  name: string
}

/** POST /db/mcoll/drop req（dropCollection 不可逆） */
export interface MongoCollectionDropPayload {
  instanceId: number
  database: string
  name: string
}

export interface MongoCollectionMutationResult {
  instanceId: number
  database: string
  name: string
}

/** POST /db/mdb/dropdb req（系统库 admin/config/local 服务端硬校验 DBM_028，I-12） */
export interface MongoDatabaseDropPayload {
  instanceId: number
  database: string
}

export interface MongoDatabaseDropResult {
  instanceId: number
  database: string
}

/**
 * 文档 JSON 值（BSON→JSON 映射如实——ObjectId `{"$oid":"hex"}`/$date 扩展 JSON 透传；
 * 递归 unknown 收窄呈现，禁 any）。
 */
export type MongoDocValue =
  | string
  | number
  | boolean
  | null
  | MongoDocValue[]
  | { [field: string]: MongoDocValue }

/** GET /db/mdoc/list 文档（_id 置顶呈现归前端排序） */
export type MongoDocument = Record<string, MongoDocValue>

/** GET /db/mdoc/list query（page 默认 1；pageSize 默认 20 上限 100=DOC_PAGE_LIMIT，D-23） */
export interface MongoDocumentListQuery {
  instanceId: number
  database: string
  collection: string
  page?: number
  pageSize?: number
}

export interface MongoDocumentListData {
  documents: MongoDocument[]
  total: number
  page: number
  pageSize: number
  generatedAt: string
}

/** POST /db/mdoc/insert req（document._id 可选，缺省引擎自动生成 ObjectId） */
export interface MongoDocumentInsertPayload {
  instanceId: number
  database: string
  collection: string
  document: MongoDocument
}

/** POST /db/mdoc/update req（id=行文档 _id 原值透传；ReplaceOne 整体替换，_id 不可变更） */
export interface MongoDocumentUpdatePayload {
  instanceId: number
  database: string
  collection: string
  id: MongoDocValue
  document: MongoDocument
}

/** POST /db/mdoc/delete req（DeleteOne by _id） */
export interface MongoDocumentDeletePayload {
  instanceId: number
  database: string
  collection: string
  id: MongoDocValue
}

export interface MongoDocumentInsertResult {
  instanceId: number
  database: string
  collection: string
  insertedId: MongoDocValue
}

export interface MongoDocumentUpdateResult {
  instanceId: number
  database: string
  collection: string
  id: MongoDocValue
  matched: number
}

export interface MongoDocumentDeleteResult {
  instanceId: number
  database: string
  collection: string
  id: MongoDocValue
  deleted: number
}

/** ---- v2.3 展示常量与纯函数（PageSpec 4.1/5.12~5.15/6.4~6.7；弹窗/页面与单测共用） ---- */

/** 浏览页分页默认/上限（TABLE_PAGE_LIMIT=DOC_PAGE_LIMIT=100，api D-22/D-23） */
export const BROWSE_PAGE_SIZE_DEFAULT = 20
export const BROWSE_PAGE_SIZE_LIMIT = 100
export const BROWSE_PAGE_SIZES: number[] = [20, 50, 100]

/** 分页 pageSize 收窄（非法/缺省→默认 20；超限截断至上限——服务端 AUTH_012 同口径前端镜像） */
export function clampBrowsePageSize(v: number | null | undefined): number {
  if (v === null || v === undefined || !Number.isFinite(v) || v < 1) {
    return BROWSE_PAGE_SIZE_DEFAULT
  }
  return Math.min(Math.floor(v), BROWSE_PAGE_SIZE_LIMIT)
}

/** Mongo 系统库（I-12/D-25：删除保护前端镜像，服务端 DBM_028 硬校验为最终守卫） */
export const MONGO_SYSTEM_DATABASES: readonly string[] = ['admin', 'config', 'local']

export function isMongoSystemDatabase(name: string): boolean {
  return MONGO_SYSTEM_DATABASES.includes(name)
}

/** 集合名形态白名单（R-14/api 4.8.4：非空/禁 $ 前缀/禁空字符；服务端终审） */
export function validateCollectionName(name: string): string | null {
  if (name.trim() === '') return '请输入集合名'
  if (name.startsWith('$')) return '集合名不可以 $ 开头（引擎保留前缀）'
  if (/[\s\0]/.test(name)) return '集合名不可含空白或空字符'
  return null
}

/** F2 语义 tooltip 文案（PageSpec 4.1：connState/credentialsSet NTooltip 包裹，治理页摘要条复用） */
export const CONN_STATE_TOOLTIP =
  '绿色=可达（面板能连上该实例）；橙色=不可达。仅反映连通性，不代表账号密码正确。'
export const CREDENTIALS_TOOLTIP =
  '是否已在面板配置该实例登录凭据（加密存储、不回显）。未配置时治理与观测不可用。'

/** F1 TCP 优先图例文案（PageSpec 4.1 清单说明行；D-21 探测策略呈现面） */
export const TCP_FIRST_LEGEND =
  '面板优先经 TCP 纳管；仅当实例关闭 TCP（skip-networking）时经本机 Socket 兜底发现。'
export const REDUNDANT_SOCKET_TOOLTIP =
  '该实例已可经 TCP 纳管；此 Socket 档案因已配置账号密码而保留。建议改配 TCP 行凭据后手动移除本行。'

/** I-10 行写禁用态 tooltip（singlePk=false 同源判定，DBM_023 服务端兜底） */
export const NO_SINGLE_PK_TOOLTIP =
  '该表无单列主键（无主键/复合主键），不支持行级编辑/删除'

/** 行单元格文本呈现（null→「NULL」由视图层 NTag 承载；此处返回文本形态，布尔/数值原样 String） */
export function rowCellText(value: RowCellValue): string {
  if (value === null) return 'NULL'
  return String(value)
}

/**
 * 文档 JSON 双层校验前端拦截层（api D-23：JSON.parse 先行，服务端 DBM_026 兜底）。
 * 成功返回解析对象（须为 JSON 对象非数组/标量——文档语义）；失败返回错误文案（内联呈现）。
 */
export function parseDocumentJson(text: string): { ok: true; doc: MongoDocument } | { ok: false; error: string } {
  let parsed: unknown
  try {
    parsed = JSON.parse(text)
  } catch (e) {
    return { ok: false, error: `JSON 解析失败：${e instanceof Error ? e.message : String(e)}` }
  }
  if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
    return { ok: false, error: '文档须为 JSON 对象（{…} 形态）' }
  }
  return { ok: true, doc: parsed as MongoDocument }
}

/** ObjectId 扩展 JSON 缩写呈现（{"$oid":"hex"} → 「ObjectId(hex)」；其余 JSON 序列化） */
export function formatDocId(id: MongoDocValue): string {
  if (typeof id === 'object' && id !== null && !Array.isArray(id)) {
    const oid = id.$oid
    if (typeof oid === 'string') return `ObjectId(${oid})`
  }
  return JSON.stringify(id)
}

/** _id 置顶排序（api 4.8.4：呈现归前端；其余键保持引擎自然序） */
export function docWithIdFirst(doc: MongoDocument): MongoDocument {
  if (!('_id' in doc)) return doc
  const rest: MongoDocument = {}
  for (const [k, v] of Object.entries(doc)) {
    if (k !== '_id') rest[k] = v
  }
  return { _id: doc._id, ...rest }
}

/**
 * 行编辑变更列收集（PageSpec 5.12：values 仅含变更列，未改列不提交减少隐式转换面；
 * NULL 勾选→null 提交；主键列排除——定位键不可编辑）。纯函数（弹窗与单测共用）。
 */
export function buildChangedRowValues(
  original: TableRow,
  edited: Record<string, { text: string; isNull: boolean }>,
  pkColumn: string | null
): Record<string, string | null> {
  const values: Record<string, string | null> = {}
  for (const [col, state] of Object.entries(edited)) {
    if (col === pkColumn) continue
    const next: string | null = state.isNull ? null : state.text
    const prev = original[col]
    // JSON null≠TS undefined：原值 null 与勾选 NULL 等价；原值非 null 按文本比对
    const unchanged =
      prev === null ? state.isNull : !state.isNull && prev !== undefined && String(prev) === state.text
    if (!unchanged) values[col] = next
  }
  return values
}

/** rescan 成功 toast（PageSpec 4.1：新增/更新计数；v1.4 merged>0 追加保守清理统计，D-21） */
export function rescanToastText(result: RescanResult): string {
  const base = `重新探测完成：新增 ${result.discovered} 个实例，更新 ${result.updated} 个状态`
  const merged = result.merged ?? 0
  return merged > 0 ? `${base}，清理 ${merged} 个冗余 Socket 档案` : base
}
