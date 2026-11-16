/**
 * site-mgmt 写动作错误兜底分流（PageSpec 第 1 节错误处理；file-manager errors.ts 先例）。
 * 专项交互由调用方先截获（SITE_003 回滚 diff 弹窗 / SITE_010 基线冲突弹窗 /
 * SITE_005 existingSiteId 查看引导 / CERT_003 existingCertId 引导 / CERT_005 引用清单弹窗），
 * 其余码经本函数取文案；SITE_004 附 output（调用方以弹窗呈现并提示检查环境后重试，D-05）。
 */
import { ApiError } from '@/api/http'
import { CERT_ERROR_TEXT, SITE_ERROR_TEXT, fieldOfString } from '@/types/site'

export interface SiteActionErrorInfo {
  level: 'warning' | 'error'
  text: string
  /** SITE_004 data.output（配置已保存、重载失败——弹窗呈现） */
  output?: string
}

export function siteActionError(e: unknown): SiteActionErrorInfo | null {
  if (!(e instanceof ApiError)) return null
  const text = SITE_ERROR_TEXT[e.code] ?? CERT_ERROR_TEXT[e.code]
  if (text === undefined) return null
  const output = fieldOfString(e.data, 'output')
  const reason = fieldOfString(e.data, 'reason')
  const detail = output !== null ? output : reason
  return {
    level: e.code === 'SITE_004' ? 'warning' : 'error',
    text: detail !== null ? `${text}：${detail}` : text,
    output: output !== null ? output : undefined
  }
}
