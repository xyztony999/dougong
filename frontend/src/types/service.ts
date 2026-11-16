/**
 * service-mgmt API 类型 —— 与 api-service-mgmt.md v1.0 4.1.4 对齐（唯一事实源）。
 * 投影条目字段（列表/属性/动作响应共同数据源，4.1.1 字段清单）：
 * - enabledState/subState/loadState 为 systemd 透传 string（D-04；enabledState 可含 static
 *   等非两态值 ⚠️A14/D-05——展示走 dictItem 兜底原值灰字）；
 * - activeState 为三态枚举（service_active_state 定稿）；
 * - canOperate/canToggleAutostart 为服务端权威计算布尔（I-07：前端不自行推断）。
 * 无分页（D-02 全量返回，total=过滤后条数）；generatedAt=投影生成时刻（数据新旧可辨识）。
 */
import type { ServiceActiveState, UnitType } from './enums'

/** 列表条目（list items；不含 description——单点 detail 承载，载荷考虑） */
export interface ServiceUnitItem {
  /** unit 全名（如 nginx.service），投影标识与动作寻址（D-09） */
  name: string
  unitType: UnitType
  activeState: ServiceActiveState
  /** 运行细分透传（running/exited/waiting…，跨类型不统一，D-04 原值展示） */
  subState: string
  /** 自启状态透传（两态主轴 enabled/disabled，可含 static 等，⚠️A14/D-05） */
  enabledState: string
  /** 服务端计算：unitType==service（操作入口判定，D-05） */
  canOperate: boolean
  /** 服务端计算：canOperate ∧ enabledState∈两态（⚠️A14；readonly 恒不渲染开关） */
  canToggleAutostart: boolean
}

/** GET /services/unit/list 响应 data */
export interface ServiceUnitListData {
  items: ServiceUnitItem[]
  total: number
  generatedAt: string
}

/** GET /services/unit/detail 响应 data（含 description/loadState，可 null） */
export interface ServiceUnitDetail extends ServiceUnitItem {
  description: string | null
  loadState: string | null
}

/** POST start/stop/restart 成功 data：操作后投影快照（免二次查询，D-03） */
export interface ServiceUnitActionResult {
  name: string
  activeState: ServiceActiveState
  subState: string
  enabledState: string
}

/** POST enable/disable 成功 data：{name, enabledState}（刷新值） */
export interface ServiceUnitEnableResult {
  name: string
  enabledState: string
}

/** SVC_007 data：{name, reason, activeState}——结果 Modal 渲染失败原因 + 实际终态（api 3.4） */
export interface ServiceUnitFailResult {
  name: string
  reason: string
  /** 后端可为 null（调用期拒绝无终态，dto.go ActionFailureData.ActiveState *string）——null 时 Modal 展示「—」（Gate 2 I1 修复）；非 null 透传 string，非三态经 dictItem 兜底（防御收窄） */
  activeState: string | null
}

/**
 * SVC_001~008 错误文案（PageSpec 第 1 节错误处理，逐条对应 api 3.4）：
 * 001/002/004/005=「参数非法」兜底（正常 UI 由控件与权威布尔约束不可达）；
 * 003/006/007/008 为行为分支文案（SVC_007 Modal / SVC_008 页面横幅在其处理路径引用）。
 */
export const SVC_ERROR_TEXT: Readonly<Record<string, string>> = {
  SVC_001: '参数非法',
  SVC_002: '参数非法',
  SVC_003: '该服务已不存在（可能已被卸载）',
  SVC_004: '参数非法',
  SVC_005: '参数非法',
  SVC_006: '操作仍在执行，请稍后刷新列表确认终态',
  SVC_007: '操作失败',
  SVC_008: 'systemd 服务通道不可用，无法获取服务列表'
}
