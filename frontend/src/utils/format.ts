/**
 * 展示格式化（PageSpec 第 8 节 #10：时间展示本地时区、提交 ISO UTC——公共约定）
 */

/** ISO-8601 UTC → 本地时区 YYYY-MM-DD HH:mm:ss；null/undefined → '—' */
export function formatDateTime(iso: string | null | undefined): string {
  if (iso === null || iso === undefined || iso === '') return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const pad = (n: number): string => String(n).padStart(2, '0')
  return (
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ` +
    `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  )
}

/** ISO-8601 UTC → 本地时区 HH:mm:ss（site-mgmt「扫描于」投影时刻，PageSpec 4.1/第 8 节 #10） */
export function formatTime(iso: string | null | undefined): string {
  if (iso === null || iso === undefined || iso === '') return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const pad = (n: number): string => String(n).padStart(2, '0')
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

/** 秒数 → mm:ss（AUTH_004 锁定倒计时） */
export function formatCountdown(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(totalSeconds))
  const pad = (n: number): string => String(n).padStart(2, '0')
  return `${pad(Math.floor(s / 60))}:${pad(s % 60)}`
}

/** 时间戳（本地时区选择值）→ ISO-8601 UTC 提交值；endOfDay=true 补 23:59:59.999 使区间闭合 */
export function toIsoUtc(ms: number, endOfDay = false): string {
  const d = new Date(ms)
  if (endOfDay) d.setHours(23, 59, 59, 999)
  return d.toISOString()
}

/**
 * ---- monitoring 度量单位格式化（api-monitoring.md D-02 前端落法，PageSpec 第 3 节） ----
 * bytes/bps 一律 1024 进制自适应 + 1 位小数；接口侧保持 SI 基本单位整数。
 */

const SIZE_UNITS = ['B', 'KiB', 'MiB', 'GiB', 'TiB'] as const
const RATE_UNITS = ['B/s', 'KiB/s', 'MiB/s', 'GiB/s'] as const

/** 自适应换算：返回同量级值与单位（1024 进制；0/非法输入停在首级） */
function scaleBytes(
  value: number,
  units: readonly string[]
): { value: number; unit: string } {
  if (!Number.isFinite(value) || value < 0) return { value: 0, unit: units[0] }
  let v = value
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i += 1
  }
  return { value: v, unit: units[i] }
}

/** bytes → 自适应 B/KiB/MiB/GiB/TiB（1 位小数；null → '—'，D-08 缺失不虚构） */
export function formatBytes(bytes: number | null | undefined): string {
  if (bytes === null || bytes === undefined) return '—'
  const { value, unit } = scaleBytes(bytes, SIZE_UNITS)
  return `${value.toFixed(1)} ${unit}`
}

/** 速率（bytes/s）→ 自适应 B/s、KiB/s、MiB/s（1 位小数；null → '—'） */
export function formatBps(bps: number | null | undefined): string {
  if (bps === null || bps === undefined) return '—'
  const { value, unit } = scaleBytes(bps, RATE_UNITS)
  return `${value.toFixed(1)} ${unit}`
}

/** 百分比 → 原值 + %（1 位小数口径与接口一致；null → '—'） */
export function formatPercent(percent: number | null | undefined): string {
  if (percent === null || percent === undefined) return '—'
  return `${percent.toFixed(1)}%`
}

/** 运行时长秒 → 「N 天 N 小时 N 分钟」（≥2 级粒度；不足 1 分钟退化为秒级；null → '—'） */
export function formatUptime(seconds: number | null | undefined): string {
  if (seconds === null || seconds === undefined) return '—'
  const total = Math.max(0, Math.floor(seconds))
  const days = Math.floor(total / 86400)
  const hours = Math.floor((total % 86400) / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  if (days > 0) return `${days} 天 ${hours} 小时`
  if (hours > 0) return `${hours} 小时 ${minutes} 分钟`
  if (minutes > 0) return `${minutes} 分钟`
  return `${total} 秒`
}

/** 负载原值（2 位小数口径与接口一致；null → '—'） */
export function formatLoad(value: number | null | undefined): string {
  if (value === null || value === undefined) return '—'
  return value.toFixed(2)
}

/**
 * 用量百分比（used/total×100，1 位小数）——n-progress percentage 专用（期望 0-100，
 * Gate 2 E2：不得直传字节数）；任一缺失或 total≤0 → null（进度条不渲染，文本占位）。
 */
export function usagePercent(
  used: number | null | undefined,
  total: number | null | undefined
): number | null {
  if (
    used === null ||
    used === undefined ||
    total === null ||
    total === undefined ||
    total <= 0
  ) {
    return null
  }
  return Math.round((used / total) * 1000) / 10
}

/**
 * ---- cron-backup 展示格式化（PageSpec 4.2.2/4.2.4：durationMs 派生人性化「1m23s」、
 * lastRun finishedAt 相对时间） ----
 */

/** 毫秒时长 → 「1m23s」形态（<60s → 「45s」；≥1h → 「1h02m03s」；null → '—'） */
export function formatDurationMs(ms: number | null | undefined): string {
  if (ms === null || ms === undefined) return '—'
  const total = Math.max(0, Math.floor(ms / 1000))
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const seconds = total % 60
  const pad = (n: number): string => String(n).padStart(2, '0')
  if (hours > 0) return `${hours}h${pad(minutes)}m${pad(seconds)}s`
  if (minutes > 0) return `${minutes}m${pad(seconds)}s`
  return `${seconds}s`
}

/**
 * ISO-8601 → 相对时间（「N 秒/分钟/小时/天 前」，PageSpec 4.2.2 最近执行列）。
 * 超 7 天或未来时刻（时钟偏差）回退绝对日期；null → '—'。
 */
export function formatRelative(iso: string | null | undefined): string {
  if (iso === null || iso === undefined || iso === '') return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const diffMs = Date.now() - d.getTime()
  if (diffMs < 60_000) {
    if (diffMs < 0) return formatDateTime(iso) // 未来时刻：时钟偏差，回退绝对
    return `${Math.floor(diffMs / 1000)} 秒前`
  }
  const minutes = Math.floor(diffMs / 60_000)
  if (minutes < 60) return `${minutes} 分钟前`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} 小时前`
  const days = Math.floor(hours / 24)
  if (days <= 7) return `${days} 天前`
  return formatDateTime(iso)
}
