/**
 * 动作错误分流决策（PageSpec 第 1 节错误处理 + 4.3 终态反馈）——从 index.vue
 * handleActionError 抽取的纯决策函数（Gate 2 W1 修复轮 2026-11-16：八分支可测化，
 * 行为与抽取前逐分支一致）。副作用（SVC_007 结果 Modal / message 提示 / store
 * 直更与重拉）由 index.vue 按 route.kind 落地，本模块不发请求不触 UI。
 */
import { ApiError } from '@/api/http'
import type { ServiceUnitFailResult } from '@/types/service'
import { SVC_ERROR_TEXT } from '@/types/service'

/** 分流去向：fail-result=SVC_007 结果框 / warn=SVC_006 超时提示 / warn-reload=SVC_003 提示+列表重拉 / unavailable=SVC_008 页面横幅 / error=参数非法兜底 / silent=静默（拦截器已提示或非 ApiError） */
export type ActionErrorRoute =
  | { kind: 'fail-result'; data: ServiceUnitFailResult }
  | { kind: 'warn'; text: string }
  | { kind: 'warn-reload'; text: string }
  | { kind: 'unavailable' }
  | { kind: 'error'; text: string }
  | { kind: 'silent' }

/** SVC_007 data 收窄（ApiError.data 为 unknown；activeState 可为 null——I1 后端调用期拒绝无终态） */
export function isFailResult(d: unknown): d is ServiceUnitFailResult {
  if (typeof d !== 'object' || d === null) return false
  const o = d as Record<string, unknown>
  return (
    typeof o.name === 'string' &&
    typeof o.reason === 'string' &&
    (typeof o.activeState === 'string' || o.activeState === null)
  )
}

/** 八分支分流（分支序=SVC_007 两形态 → SVC_006 → SVC_003 → SVC_008 → 参数非法兜底 → 未知码静默；非 ApiError 静默） */
export function routeActionError(e: unknown): ActionErrorRoute {
  if (!(e instanceof ApiError)) return { kind: 'silent' }
  if (e.is('SVC_007')) {
    return isFailResult(e.data) ? { kind: 'fail-result', data: e.data } : { kind: 'silent' }
  }
  if (e.is('SVC_006')) {
    // 等待已达服务端 90s 上限：行 inFlight 由调用方 finally 解除
    return { kind: 'warn', text: SVC_ERROR_TEXT['SVC_006'] }
  }
  if (e.is('SVC_003')) {
    return { kind: 'warn-reload', text: SVC_ERROR_TEXT['SVC_003'] }
  }
  if (e.is('SVC_008')) {
    return { kind: 'unavailable' }
  }
  // SVC_001/002/004/005「参数非法」兜底（正常 UI 由控件与 canOperate/canToggleAutostart 权威值约束不可达）
  const text = SVC_ERROR_TEXT[e.code]
  return text !== undefined ? { kind: 'error', text } : { kind: 'silent' }
}
