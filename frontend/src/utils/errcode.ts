/**
 * 错误码通道归一（Gate 2 E1 修复 2026-11-14；W-3 注释订正 2026-11-15）。
 *
 * 契约基准：api-account-auth.md §3.4——错误码 code 列取值为符号 `AUTH_001`~`AUTH_014`，
 * 全站比对一律使用符号串（http.ts/stores/views 共 16 处，不再改动）。
 * 后端现状（Gate 2 E1 双侧修复后）：backend/internal/api/response/response.go Fail
 * 直接上线 errcode symbol 符号串（成功恒为数值 0），不再发送数值错误码。
 *
 * normalizeCode 保留双通道归一：符号串原样透传；数值 401NN → AUTH_0NN 为 E1 期间
 * 后端数值通道的历史映射，留作兼容兜底（后端若误发数值不致全站符号比对失效）。
 * 跨模块公共码（backend errcode）：40400→NOT_FOUND、50000→INTERNAL。
 */

const AUTH_CODE_BASE = 40100

export function normalizeCode(code: number | string): string {
  if (typeof code === 'string') return code
  if (Number.isInteger(code)) {
    if (code >= 40101 && code <= 40114) {
      return `AUTH_${String(code - AUTH_CODE_BASE).padStart(3, '0')}`
    }
    if (code === 40400) return 'NOT_FOUND'
    if (code === 50000) return 'INTERNAL'
  }
  return String(code)
}
