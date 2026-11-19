/**
 * db-tools api 层 —— 8 fetch Action 与 api-db-tools.md v1.2 面板面一一对应（S7，无虚构接口）。
 * 读 3 GET 两角色（dbtoolList/dbtoolTaskList/dbtoolTaskDetail，ADR-018 全可看）；
 * 写 5 POST admin+CSRF（install/upgrade/uninstall/open/rotate——http.ts 统一注入 X-CSRF-Token）。
 * 第 9 个面板接口 GET /dbtool/access/reopen 为浏览器 302 顶层导航落地（phpMyAdmin SignonURL
 * 重认证，api D-06/D-09 v1.2）——非前端 fetch 调用，无对应函数（PageSpec 第 2 节声明）。
 * 内部端点 2（gate/verify、signon/verify）调用方为 nginx/PHP-FPM 服务端，非前端 Action。
 * 函数名 = PageSpec 第 2 节 Action 名。
 */
import { request } from './http'
import type {
  DbToolListData,
  DbToolOpenResult,
  DbToolRotateResult,
  DbToolTaskCreated,
  DbToolTaskDetail,
  DbToolTaskListData
} from '@/types/dbtool'
import type { DbToolTaskStatus, DbToolType } from '@/types/enums'

/** dbtoolList：工具全景（目录+实况+依赖就绪度；实时探测 D-02 无缓存，响应带 generatedAt） */
export function dbtoolList(): Promise<DbToolListData> {
  return request<DbToolListData>('/dbtool/tool/list')
}

/** dbtoolInstall：安装（依赖前置 DBT_001；mongo_express 必填 instanceId 部署时绑定 D-07；
 * phpmyadmin 忽略 instanceId——打开时绑定 D-09；异步任务 D-04） */
export function dbtoolInstall(toolType: DbToolType, instanceId?: number): Promise<DbToolTaskCreated> {
  return request<DbToolTaskCreated>('/dbtool/tool/install', {
    method: 'POST',
    body: instanceId === undefined ? { toolType } : { toolType, instanceId }
  })
}

/** dbtoolUpgrade：升级到目录钉死版本（D-14；已装前置 DBT_004；同版本 DBT_011；异步任务） */
export function dbtoolUpgrade(toolType: DbToolType): Promise<DbToolTaskCreated> {
  return request<DbToolTaskCreated>('/dbtool/tool/upgrade', { method: 'POST', body: { toolType } })
}

/** dbtoolUninstall：卸载（解除部署不删数据 D-15；强确认归前端；异步任务） */
export function dbtoolUninstall(toolType: DbToolType): Promise<DbToolTaskCreated> {
  return request<DbToolTaskCreated>('/dbtool/tool/uninstall', { method: 'POST', body: { toolType } })
}

/** dbtoolTaskList：任务台账（toolType/status 筛选 + 分页，默认 pageSize=50 D-13） */
export function dbtoolTaskList(q: {
  toolType?: DbToolType
  status?: DbToolTaskStatus
  page?: number
  pageSize?: number
}): Promise<DbToolTaskListData> {
  return request<DbToolTaskListData>('/dbtool/task/list', {
    query: { toolType: q.toolType, status: q.status, page: q.page, pageSize: q.pageSize }
  })
}

/** dbtoolTaskDetail：单任务详情（输出尾部排障；不存在 DBT_005） */
export function dbtoolTaskDetail(taskId: number): Promise<DbToolTaskDetail> {
  return request<DbToolTaskDetail>('/dbtool/task/detail', { query: { taskId } })
}

/** dbtoolOpen：打开工具（admin 专属 ⚠️A10；phpmyadmin 必填 instanceId 令牌绑定 D-09，
 * mongo_express 忽略——部署时已绑定；响应仅 url+tokenExpiresAt，凭据零回显 I-02；
 * 浏览器经面板会话 gating 进入工具——新标签页 window.open noopener，PageSpec 4.4 裁定） */
export function dbtoolOpen(toolType: DbToolType, instanceId?: number): Promise<DbToolOpenResult> {
  return request<DbToolOpenResult>('/dbtool/access/open', {
    method: 'POST',
    body: instanceId === undefined ? { toolType } : { toolType, instanceId }
  })
}

/** dbtoolRotate：basic auth 轮换（仅 mongo_express，phpmyadmin→DBT_009 D-08；
 * 响应无口令字段 I-02；旧口令即时失效，浏览器无感知服务端注入） */
export function dbtoolRotate(toolType: DbToolType): Promise<DbToolRotateResult> {
  return request<DbToolRotateResult>('/dbtool/access/rotate', { method: 'POST', body: { toolType } })
}
