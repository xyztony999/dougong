/**
 * database-mgmt api 层 —— 30 Action 与 api-database-mgmt.md v2.0 一一对应（S7，无虚构接口；
 * v1.2 +mongo 7）。读 13 GET（10 两角色 + 3 admin 专属：databaseCharsets/redisKeyList/
 * redisKeyTtl）；写 17 POST（admin 专属 + CSRF——http.ts 统一注入 X-CSRF-Token）。
 * 特殊通道：backup/export 走 POST 流式（postDownload 双形态分流，D-06）；
 * backup/import 走 multipart（postForm，D-06）。函数名 = PageSpec 第 2 节 Action 名。
 */
import type { DbEngine } from '@/types/enums'
import type {
  BackupImportResult,
  CredentialsPayload,
  CredentialsResult,
  DatabaseCharsetsData,
  DatabaseCreatePayload,
  DatabaseCreateResult,
  DatabaseListData,
  DatabaseMutationResult,
  DbUserCreatePayload,
  DbUserGrantPayload,
  DbUserGrantResult,
  DbUserListData,
  DbUserMutationResult,
  DbUserRevokeResult,
  InstanceIdResult,
  InstanceListData,
  KeyListData,
  KeyListQuery,
  KeyTtlData,
  MongoDatabaseListData,
  MongoRoleResult,
  MongoStatusData,
  MongoUserCreatePayload,
  MongoUserListData,
  MongoUserPayload,
  MongoUserResult,
  MongoUserRolePayload,
  MysqlProcesslistData,
  MysqlStatusData,
  PasswordChangePayload,
  PasswordSyncResult,
  RedisInfoData,
  RedisSlowlogData,
  RescanResult
} from '@/types/database'
import { postDownload, postForm, request } from './http'

export interface InstanceListQuery {
  /** db_engine 筛选（空=全部，api 4.1.4） */
  engine?: DbEngine
}

/** instanceList：双引擎统一实例清单（进程内首次触发自动探测 D-07；按 discoveredAt 排序） */
export function instanceList(q: InstanceListQuery = {}): Promise<InstanceListData> {
  return request<InstanceListData>('/db/instance/list', { query: { engine: q.engine } })
}

/** instanceRescan：手动重新探测（admin，写审计 instance_rescanned；自动探测不写，D-07） */
export function instanceRescan(): Promise<RescanResult> {
  return request<RescanResult>('/db/instance/rescan', { method: 'POST', body: {} })
}

/** instanceCredentials：凭据配置与连接验证（D-03 先验证后落库；DBM_005/006 不落库） */
export function instanceCredentials(payload: CredentialsPayload): Promise<CredentialsResult> {
  return request<CredentialsResult>('/db/instance/credentials', {
    method: 'POST',
    body: payload
  })
}

/** instanceRemove：移除纳管（在途守卫 DBM_016；不卸载不删数据 ⚠️A20） */
export function instanceRemove(instanceId: number): Promise<InstanceIdResult> {
  return request<InstanceIdResult>('/db/instance/remove', {
    method: 'POST',
    body: { instanceId }
  })
}

/** mysqlStatus：状态概览（variables 白名单 ≤10 项 D-08；I-05 前置 engine∈mysql/mariadb） */
export function mysqlStatus(instanceId: number): Promise<MysqlStatusData> {
  return request<MysqlStatusData>('/db/mysql/status', { query: { instanceId } })
}

/** mysqlProcesslist：进程列表实时快照（全量返回，连接数量级小 D-09） */
export function mysqlProcesslist(instanceId: number): Promise<MysqlProcesslistData> {
  return request<MysqlProcesslistData>('/db/mysql/processlist', { query: { instanceId } })
}

/** mysqlRootPassword：root 密码修改（两步同步 D-05；DBM_019 data.recoverHint=credentials） */
export function mysqlRootPassword(payload: PasswordChangePayload): Promise<PasswordSyncResult> {
  return request<PasswordSyncResult>('/db/mysql/root-password', {
    method: 'POST',
    body: payload
  })
}

/** databaseList：库清单（实时投影 I-04，响应带 generatedAt） */
export function databaseList(instanceId: number): Promise<DatabaseListData> {
  return request<DatabaseListData>('/db/database/list', { query: { instanceId } })
}

/** databaseCharsets：建库字符集实况候选（admin 建库辅助，开放集合 D-11） */
export function databaseCharsets(instanceId: number): Promise<DatabaseCharsetsData> {
  return request<DatabaseCharsetsData>('/db/database/charsets', { query: { instanceId } })
}

/** databaseCreate：建库（charset 空取默认 utf8mb4 D-11；重名 DBM_008） */
export function databaseCreate(payload: DatabaseCreatePayload): Promise<DatabaseCreateResult> {
  return request<DatabaseCreateResult>('/db/database/create', {
    method: 'POST',
    body: payload
  })
}

/** databaseDelete：删库（不可逆，交互层二次确认前置；不存在 DBM_007 如实报错） */
export function databaseDelete(instanceId: number, name: string): Promise<DatabaseMutationResult> {
  return request<DatabaseMutationResult>('/db/database/delete', {
    method: 'POST',
    body: { instanceId, name }
  })
}

/** dbuserList：用户清单（凭据不回显 I-02；grants 为三档归并投影 D-10） */
export function dbuserList(instanceId: number): Promise<DbUserListData> {
  return request<DbUserListData>('/db/dbuser/list', { query: { instanceId } })
}

/** dbuserCreate：建用户（密码止步请求体；同名同 host DBM_010） */
export function dbuserCreate(payload: DbUserCreatePayload): Promise<DbUserMutationResult> {
  return request<DbUserMutationResult>('/db/dbuser/create', {
    method: 'POST',
    body: payload
  })
}

/** dbuserDelete：删用户（引擎语义连带回收既有授权；不存在 DBM_009） */
export function dbuserDelete(
  instanceId: number,
  username: string,
  host: string
): Promise<DbUserMutationResult> {
  return request<DbUserMutationResult>('/db/dbuser/delete', {
    method: 'POST',
    body: { instanceId, username, host }
  })
}

/** dbuserGrant：库级授权（三档 grant_level D-10；幂等） */
export function dbuserGrant(payload: DbUserGrantPayload): Promise<DbUserGrantResult> {
  return request<DbUserGrantResult>('/db/dbuser/grant', {
    method: 'POST',
    body: payload
  })
}

/** dbuserRevoke：按库回收授权（第二次报 DBM_011 如实） */
export function dbuserRevoke(
  instanceId: number,
  username: string,
  host: string,
  database: string
): Promise<DbUserRevokeResult> {
  return request<DbUserRevokeResult>('/db/dbuser/revoke', {
    method: 'POST',
    body: { instanceId, username, host, database }
  })
}

/**
 * backupExport：mysqldump 导出（POST 文件流，D-06 双形态——失败 JSON 体 DBM_012[data.reason]、
 * 成功 octet-stream 流）；触发浏览器保存（fallback 文件名 {db}.sql）。
 */
export async function backupExport(instanceId: number, database: string): Promise<void> {
  const { blob, filename } = await postDownload('/db/backup/export', { instanceId, database })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename ?? `${database}.sql`
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

/** backupImport：SQL 文件导入（multipart；非幂等——交互层二次确认前置，PRD FR-16） */
export function backupImport(
  instanceId: number,
  database: string,
  file: File
): Promise<BackupImportResult> {
  const form = new FormData()
  form.append('instanceId', String(instanceId))
  form.append('database', database)
  form.append('file', file)
  return postForm<BackupImportResult>('/db/backup/import', form)
}

/** redisInfo：info 概览（REDIS_INFO_FIELDS 白名单分组 D-08；I-05 前置 engine=redis） */
export function redisInfo(instanceId: number): Promise<RedisInfoData> {
  return request<RedisInfoData>('/db/redis/info', { query: { instanceId } })
}

/** redisSlowlog：慢日志清单（条数默认 50 上限 200，D-09 SLOWLOG_LIMIT） */
export function redisSlowlog(instanceId: number, limit?: number): Promise<RedisSlowlogData> {
  return request<RedisSlowlogData>('/db/redis/slowlog', {
    query: { instanceId, limit }
  })
}

/** redisPassword：Redis 密码设置（两步同步 D-05——面板建连即旧密码验证） */
export function redisPassword(payload: PasswordChangePayload): Promise<PasswordSyncResult> {
  return request<PasswordSyncResult>('/db/redis/password', {
    method: 'POST',
    body: payload
  })
}

/** redisKeyList：key 清单（SCAN 游标分页 D-09；COUNT=200 服务端常量，前端不传） */
export function redisKeyList(q: KeyListQuery): Promise<KeyListData> {
  return request<KeyListData>('/db/key/list', {
    query: { instanceId: q.instanceId, cursor: q.cursor, pattern: q.pattern }
  })
}

/** redisKeyTtl：单 key TTL（-1=无过期；不存在 DBM_015 如实） */
export function redisKeyTtl(instanceId: number, key: string): Promise<KeyTtlData> {
  return request<KeyTtlData>('/db/key/ttl', { query: { instanceId, key } })
}

/** ---- mongo 族（api v2.0 §4.5.2；v0.1.4 反馈第 3 项增量；I-05 前置 engine=mongodb） ---- */

/** mongoStatus：serverStatus 白名单摘要（D-08 同构 ≤10 项） */
export function mongoStatus(instanceId: number): Promise<MongoStatusData> {
  return request<MongoStatusData>('/db/mongo/status', { query: { instanceId } })
}

/** mongoDatabaseList：库清单只读投影（D-18：隐式建库语义，无建/删库动作） */
export function mongoDatabaseList(instanceId: number): Promise<MongoDatabaseListData> {
  return request<MongoDatabaseListData>('/db/mdb/list', { query: { instanceId } })
}

/** mongoUserList：用户清单实况投影（admin 库 users；密码零出口 I-02） */
export function mongoUserList(instanceId: number): Promise<MongoUserListData> {
  return request<MongoUserListData>('/db/muser/list', { query: { instanceId } })
}

/** mongoUserCreate：建用户（admin 库 + 首库首角色 D-19；重名 DBM_010） */
export function mongoUserCreate(payload: MongoUserCreatePayload): Promise<MongoUserResult> {
  return request<MongoUserResult>('/db/muser/create', { method: 'POST', body: payload })
}

/** mongoUserDelete：删用户（连带回收全部角色，引擎语义；不存在 DBM_009） */
export function mongoUserDelete(payload: MongoUserPayload): Promise<MongoUserResult> {
  return request<MongoUserResult>('/db/muser/delete', { method: 'POST', body: payload })
}

/** mongoUserGrant：授予库级角色（mongo_role 三档；幂等） */
export function mongoUserGrant(payload: MongoUserRolePayload): Promise<MongoRoleResult> {
  return request<MongoRoleResult>('/db/muser/grant', { method: 'POST', body: payload })
}

/** mongoUserRevoke：回收库级角色（授权关系不存在 DBM_011——service 实况前置校验） */
export function mongoUserRevoke(payload: MongoUserRolePayload): Promise<MongoRoleResult> {
  return request<MongoRoleResult>('/db/muser/revoke', { method: 'POST', body: payload })
}
