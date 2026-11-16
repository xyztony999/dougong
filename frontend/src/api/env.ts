/**
 * env-mgmt api 层 —— 10 Action 与 api-env-mgmt.md 4.1.4 一一对应（S7，无虚构接口）。
 * 读 3 GET 两角色（envList/taskList/taskDetail，ADR-018 全可看）；写 7 POST
 * admin+CSRF（envInstall/envUninstall/service×5）。
 */
import { request } from './http'
import type {
  EnvListData,
  EnvServiceOp,
  EnvServiceOpResult,
  EnvTaskCreated,
  EnvTaskDetail,
  EnvTaskListData
} from '@/types/env'
import type { EnvTaskStatus, EnvType } from '@/types/enums'

/** envList：环境全景（目录+实况+通道状态；实时探测 D-01 无缓存） */
export function envList(): Promise<EnvListData> {
  return request<EnvListData>('/env/env/list')
}

/** envInstall：安装/升级（已装=升级幂等 D-05；异步任务 D-04；ENV_001/002/003） */
export function envInstall(envType: EnvType): Promise<EnvTaskCreated> {
  return request<EnvTaskCreated>('/env/env/install', { method: 'POST', body: { envType } })
}

/** envUninstall：卸载（保守 remove D-07；已装前置 ENV_004；强确认归前端） */
export function envUninstall(envType: EnvType): Promise<EnvTaskCreated> {
  return request<EnvTaskCreated>('/env/env/uninstall', { method: 'POST', body: { envType } })
}

/** envTaskList：任务台账（envType/status 筛选 + 分页） */
export function envTaskList(q: {
  envType?: EnvType
  status?: EnvTaskStatus
  page?: number
  pageSize?: number
}): Promise<EnvTaskListData> {
  return request<EnvTaskListData>('/env/task/list', { query: { envType: q.envType, status: q.status, page: q.page, pageSize: q.pageSize } })
}

/** envTaskDetail：单任务详情（输出尾部排障，D-08） */
export function envTaskDetail(taskId: number): Promise<EnvTaskDetail> {
  return request<EnvTaskDetail>('/env/task/detail', { query: { taskId } })
}

/** envServiceOp：服务启停/自启（SysdGateway 复用 D-09；ENV_004/006） */
export function envServiceOp(op: EnvServiceOp, envType: EnvType): Promise<EnvServiceOpResult> {
  return request<EnvServiceOpResult>(`/env/service/${op}`, { method: 'POST', body: { envType } })
}
