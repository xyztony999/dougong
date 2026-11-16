/**
 * service-mgmt api 层 —— 7 Action 与 api-service-mgmt.md 4.1.4 一一对应（S7，无虚构接口）。
 * 读 2 GET（admin+readonly，ADR-018 全可看）；写 5 POST（admin 专属 + CSRF——
 * http.ts 统一注入 X-CSRF-Token）。函数名 = PageSpec 第 2 节 Action 名。
 */
import type {
  ServiceUnitActionResult,
  ServiceUnitDetail,
  ServiceUnitEnableResult,
  ServiceUnitListData
} from '@/types/service'
import { request } from './http'

export interface UnitListQuery {
  /** 逗号分隔的 service_active_state 多值（如 active,failed）；空=全部 */
  state?: string
  /** 名称子串（不区分大小写，服务端执行 D-02） */
  search?: string
}

/** unitList：服务全集列表（全量不分页 D-02；服务端过滤/搜索 R-14；名称字典序升序） */
export function unitList(q: UnitListQuery = {}): Promise<ServiceUnitListData> {
  return request<ServiceUnitListData>('/services/unit/list', {
    query: { state: q.state, search: q.search }
  })
}

/** unitDetail：单 unit 属性快照（含 description/loadState） */
export function unitDetail(name: string): Promise<ServiceUnitDetail> {
  return request<ServiceUnitDetail>('/services/unit/detail', { query: { name } })
}

/**
 * unitStart/unitStop/unitRestart：req {name}；同步等待终态 ≤90s（D-01/I-05，
 * 无超时兜底 UI——服务端上限保证必返回）；成功 data=操作后投影快照。
 */
export function unitStart(name: string): Promise<ServiceUnitActionResult> {
  return request<ServiceUnitActionResult>('/services/unit/start', {
    method: 'POST',
    body: { name }
  })
}

export function unitStop(name: string): Promise<ServiceUnitActionResult> {
  return request<ServiceUnitActionResult>('/services/unit/stop', {
    method: 'POST',
    body: { name }
  })
}

export function unitRestart(name: string): Promise<ServiceUnitActionResult> {
  return request<ServiceUnitActionResult>('/services/unit/restart', {
    method: 'POST',
    body: { name }
  })
}

/** unitEnable/unitDisable：轻量同步（无 Job 等待）；成功 data={name, enabledState 刷新值} */
export function unitEnable(name: string): Promise<ServiceUnitEnableResult> {
  return request<ServiceUnitEnableResult>('/services/unit/enable', {
    method: 'POST',
    body: { name }
  })
}

export function unitDisable(name: string): Promise<ServiceUnitEnableResult> {
  return request<ServiceUnitEnableResult>('/services/unit/disable', {
    method: 'POST',
    body: { name }
  })
}
