/**
 * file-manager Action 错误分流（PageSpec 第 1 节错误处理，逐条对应 api 3.4 FILE_001~015）。
 * FILE_005 不在本表——冲突决策 Modal（4.4.8）专属，调用方以 isConflictList 收窄后走重发流；
 * AUTH_001/002/003/008 已由 http 拦截器全局分流（返回 null 不重复提示）。
 */
import type { ApiError } from '@/api/http'
import { FILE_ERROR_TEXT, fsReason, scopeReason } from '@/types/file'

export interface FileActionErrorInfo {
  level: 'warning' | 'error'
  text: string
  /** true = 随后重拉当前列表（FILE_003 实况同步 / FILE_012 目标消失） */
  refreshAfter: boolean
}

/** FILE_* Action 错误 → 提示级别/文案/刷新语义；null = 非本模块码（拦截器已处理或调用方专有分支） */
export function fileActionError(e: ApiError): FileActionErrorInfo | null {
  if (e.code === 'FILE_005') return null // 冲突决策流（4.4.8）
  if (e.code === 'FILE_002') {
    const reason = scopeReason(e.data)
    return {
      level: 'error',
      text: reason !== null ? `该路径不受管理（${reason}）` : FILE_ERROR_TEXT['FILE_002'],
      refreshAfter: false
    }
  }
  if (e.code === 'FILE_013') {
    const reason = fsReason(e.data)
    return {
      level: 'error',
      text: reason !== null ? `文件系统错误：${reason}` : FILE_ERROR_TEXT['FILE_013'],
      refreshAfter: false
    }
  }
  const warningCodes: ReadonlySet<string> = new Set(['FILE_003', 'FILE_004', 'FILE_006', 'FILE_011', 'FILE_012', 'FILE_014'])
  const refreshCodes: ReadonlySet<string> = new Set(['FILE_003', 'FILE_012', 'FILE_014'])
  const text = FILE_ERROR_TEXT[e.code]
  if (text === undefined) return null
  return {
    level: warningCodes.has(e.code) ? 'warning' : 'error',
    text,
    refreshAfter: refreshCodes.has(e.code)
  }
}
