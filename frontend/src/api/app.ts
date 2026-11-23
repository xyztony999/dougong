/**
 * app-mgmt api 层 —— 16 Action 与 api-app-mgmt.md §4.3 一一对应（S7，无虚构
 * 接口）。读 5 GET（app/list·task/list·task/detail 两角色；app/env·log admin
 * 专属 ⚠️A8）；写 11 POST admin+CSRF。
 */
import { request } from './http'
import type {
  AppCreateRequest,
  AppDeleteRequest,
  AppEnvData,
  AppListData,
  AppLogData,
  AppParamsSaveData,
  AppParamsSaveRequest,
  AppServiceOpData,
  AppSiteLinkData,
  AppTaskCreated,
  AppTaskDetailData,
  AppTaskListData
} from '@/types/app'
import type { AppRuntime, AppTaskAction, AppTaskStatus } from '@/types/enums'

/** appList：应用全景（档案+实况投影；实时探测 D-01 无缓存）。runtime 可选过滤
 *  （ADR-033：网站管理页的 Python / Node 分栏各取其一；缺省返回全部） */
export function appList(runtime?: AppRuntime): Promise<AppListData> {
  return request<AppListData>('/app/app/list', { query: { runtime } })
}

/** appCreate：创建应用（前置校验链 D-03 → 异步任务 D-04/D-05；APP_001~006/012；
 *  Node 运行时另有 APP_015 环境不完整 / APP_016 启动参数非法） */
export function appCreate(req: AppCreateRequest): Promise<AppTaskCreated> {
  return request<AppTaskCreated>('/app/app/create', { method: 'POST', body: req })
}

/** appDelete：删除应用（purgeDir 默认 false——代码目录默认保留 ⚠️A5） */
export function appDelete(req: AppDeleteRequest): Promise<AppTaskCreated> {
  return request<AppTaskCreated>('/app/app/delete', { method: 'POST', body: req })
}

/** appEnvGet：.env 现文件内容（admin 专属参数面 ⚠️A8；D-09） */
export function appEnvGet(appId: number): Promise<AppEnvData> {
  return request<AppEnvData>('/app/app/env', { query: { appId } })
}

/** appParamsSave：参数保存（基线比对乐观锁 D-09；保存后需重启生效提示） */
export function appParamsSave(req: AppParamsSaveRequest): Promise<AppParamsSaveData> {
  return request<AppParamsSaveData>('/app/app/params', { method: 'POST', body: req })
}

/** appDepsReinstall：依赖重装（venv 前置 APP_009；失败可重试 D-13） */
export function appDepsReinstall(appId: number): Promise<AppTaskCreated> {
  return request<AppTaskCreated>('/app/app/deps', { method: 'POST', body: { appId } })
}

/** appTaskList：任务台账（appId/action/status 筛选 + 分页） */
export function appTaskList(q: {
  appId?: number
  action?: AppTaskAction
  status?: AppTaskStatus
  page?: number
  pageSize?: number
}): Promise<AppTaskListData> {
  return request<AppTaskListData>('/app/task/list', {
    query: { appId: q.appId, action: q.action, status: q.status, page: q.page, pageSize: q.pageSize }
  })
}

/** appTaskDetail：单任务详情（输出尾部排障） */
export function appTaskDetail(taskId: number): Promise<AppTaskDetailData> {
  return request<AppTaskDetailData>('/app/task/detail', { query: { taskId } })
}

/** appTaskRetry：失败任务重试（D-13：仅 create/deps_install，APP_008 语义拒绝） */
export function appTaskRetry(taskId: number): Promise<AppTaskCreated> {
  return request<AppTaskCreated>('/app/task/retry', { method: 'POST', body: { taskId } })
}

/** appServiceOp：服务操作（start/stop/restart/enable/disable；双入口 ⚠️A12） */
export function appServiceOp(appId: number, op: 'start' | 'stop' | 'restart' | 'enable' | 'disable'): Promise<AppServiceOpData> {
  return request<AppServiceOpData>(`/app/service/${op}`, { method: 'POST', body: { appId } })
}

/** appLogGet：应用日志尾部（admin 专属 ⚠️A8；档位 200/500/1000 D-11） */
export function appLogGet(appId: number, lines?: number): Promise<AppLogData> {
  return request<AppLogData>('/app/log', { query: { appId, lines } })
}

/** appSiteLink：建站联动（消费站点管理建站能力；站点引用值快照 D-12） */
export function appSiteLink(appId: number, domain?: string): Promise<AppSiteLinkData> {
  return request<AppSiteLinkData>('/app/site/link', { method: 'POST', body: { appId, domain } })
}
