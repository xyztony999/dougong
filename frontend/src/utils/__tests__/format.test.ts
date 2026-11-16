import { describe, expect, it } from 'vitest'
import {
  formatBps,
  formatBytes,
  formatCountdown,
  formatDateTime,
  formatDurationMs,
  formatLoad,
  formatPercent,
  formatRelative,
  formatTime,
  formatUptime,
  toIsoUtc,
  usagePercent
} from '../format'

const pad = (n: number): string => String(n).padStart(2, '0')

function localFull(d: Date): string {
  return (
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ` +
    `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  )
}

describe('formatDateTime（本地时区展示，PageSpec §8 #10）', () => {
  it('null/undefined/空串显示 —', () => {
    expect(formatDateTime(null)).toBe('—')
    expect(formatDateTime(undefined)).toBe('—')
    expect(formatDateTime('')).toBe('—')
  })

  it('ISO UTC 转本地时区 YYYY-MM-DD HH:mm:ss（按运行环境时区计算期望值）', () => {
    const iso = '2026-11-14T12:34:56Z'
    expect(formatDateTime(iso)).toBe(localFull(new Date(iso)))
  })

  it('非法字符串原样返回（不抛错）', () => {
    expect(formatDateTime('not-a-date')).toBe('not-a-date')
  })
})

describe('formatCountdown（AUTH_004 锁定倒计时 mm:ss）', () => {
  it('常规秒数', () => {
    expect(formatCountdown(0)).toBe('00:00')
    expect(formatCountdown(5)).toBe('00:05')
    expect(formatCountdown(65)).toBe('01:05')
    expect(formatCountdown(900)).toBe('15:00')
  })

  it('负数钳制为 00:00', () => {
    expect(formatCountdown(-3)).toBe('00:00')
  })
})

describe('toIsoUtc（提交 ISO UTC，公共约定）', () => {
  it('时间戳 → ISO UTC', () => {
    const ms = Date.UTC(2026, 10, 14, 8, 30, 0)
    expect(toIsoUtc(ms)).toBe('2026-11-14T08:30:00.000Z')
  })

  it('endOfDay 补 23:59:59.999（本地时区，区间闭合）', () => {
    const ms = Date.UTC(2026, 10, 14, 0, 0, 0)
    const expected = new Date(ms)
    expected.setHours(23, 59, 59, 999)
    expect(toIsoUtc(ms, true)).toBe(expected.toISOString())
  })
})

describe('formatBytes（monitoring D-02：1024 进制自适应，1 位小数）', () => {
  it('null/undefined 显示 —（D-08 缺失不虚构）', () => {
    expect(formatBytes(null)).toBe('—')
    expect(formatBytes(undefined)).toBe('—')
  })

  it('量级自适应 B/KiB/MiB/GiB/TiB', () => {
    expect(formatBytes(0)).toBe('0.0 B')
    expect(formatBytes(512)).toBe('512.0 B')
    expect(formatBytes(1024)).toBe('1.0 KiB')
    expect(formatBytes(1536)).toBe('1.5 KiB')
    expect(formatBytes(1024 * 1024 * 1.5)).toBe('1.5 MiB')
    expect(formatBytes(1024 ** 3)).toBe('1.0 GiB')
    expect(formatBytes(1024 ** 4)).toBe('1.0 TiB')
  })

  it('超大值封顶 TiB 不越级', () => {
    expect(formatBytes(1024 ** 5)).toBe('1024.0 TiB')
  })

  it('负数/NaN 兜底 0.0 B', () => {
    expect(formatBytes(-5)).toBe('0.0 B')
    expect(formatBytes(Number.NaN)).toBe('0.0 B')
  })
})

describe('formatBps（monitoring D-02：速率自适应 B/s、KiB/s、MiB/s）', () => {
  it('null/undefined 显示 —', () => {
    expect(formatBps(null)).toBe('—')
    expect(formatBps(undefined)).toBe('—')
  })

  it('量级自适应', () => {
    expect(formatBps(128)).toBe('128.0 B/s')
    expect(formatBps(2048)).toBe('2.0 KiB/s')
    expect(formatBps(1.5 * 1024 * 1024)).toBe('1.5 MiB/s')
  })

  it('超大值封顶 GiB/s', () => {
    expect(formatBps(1024 ** 4)).toBe('1024.0 GiB/s')
  })
})

describe('formatPercent / formatLoad（monitoring D-02：原值加单位）', () => {
  it('null 显示 —', () => {
    expect(formatPercent(null)).toBe('—')
    expect(formatLoad(null)).toBe('—')
  })

  it('百分比 1 位小数 + %；负载 2 位小数原值', () => {
    expect(formatPercent(12.34)).toBe('12.3%')
    expect(formatPercent(0)).toBe('0.0%')
    expect(formatLoad(0.42)).toBe('0.42')
    expect(formatLoad(1)).toBe('1.00')
  })
})

describe('formatUptime（monitoring D-02：「N 天 N 小时 N 分钟」≥2 级粒度）', () => {
  it('null 显示 —', () => {
    expect(formatUptime(null)).toBe('—')
    expect(formatUptime(undefined)).toBe('—')
  })

  it('秒级（<1 分钟）与分钟级', () => {
    expect(formatUptime(0)).toBe('0 秒')
    expect(formatUptime(59)).toBe('59 秒')
    expect(formatUptime(60)).toBe('1 分钟')
    expect(formatUptime(3599)).toBe('59 分钟')
  })

  it('小时级（<1 天：N 小时 N 分钟）', () => {
    expect(formatUptime(3600)).toBe('1 小时 0 分钟')
    expect(formatUptime(3661)).toBe('1 小时 1 分钟')
  })

  it('天级（≥1 天：N 天 N 小时）', () => {
    expect(formatUptime(86400)).toBe('1 天 0 小时')
    expect(formatUptime(86400 + 2 * 3600 + 3 * 60)).toBe('1 天 2 小时')
  })

  it('负数钳制 0 秒', () => {
    expect(formatUptime(-1)).toBe('0 秒')
  })
})

describe('usagePercent（Gate 2 E2：进度条 percentage=used/total×100，非字节数）', () => {
  it('null/undefined 任一缺失 → null（进度条不渲染）', () => {
    expect(usagePercent(null, 100)).toBeNull()
    expect(usagePercent(50, null)).toBeNull()
    expect(usagePercent(undefined, 100)).toBeNull()
    expect(usagePercent(50, undefined)).toBeNull()
  })

  it('used/total×100，1 位小数四舍五入', () => {
    expect(usagePercent(0, 500)).toBe(0)
    expect(usagePercent(250, 500)).toBe(50)
    expect(usagePercent(1, 3)).toBe(33.3)
    expect(usagePercent(2, 3)).toBe(66.7)
    expect(usagePercent(500, 500)).toBe(100)
  })

  it('total≤0 → null（除零/伪文件系统防御，不虚构百分比）', () => {
    expect(usagePercent(10, 0)).toBeNull()
    expect(usagePercent(10, -5)).toBeNull()
  })

  it('字节数直传不再恒满格（E2 回归：GiB 级 used 对 KiB 级 total 才可能 >100）', () => {
    // 修复前 diskPercent 直接返回 usedBytes（如 21474836480）→ n-progress 恒满格
    expect(usagePercent(10 * 1024 ** 3, 20 * 1024 ** 3)).toBe(50)
  })
})

describe('formatTime（site-mgmt「扫描于」投影时刻，本地时区 HH:mm:ss）', () => {
  it('null/undefined/空串显示 —', () => {
    expect(formatTime(null)).toBe('—')
    expect(formatTime(undefined)).toBe('—')
    expect(formatTime('')).toBe('—')
  })

  it('非法字符串原样透传（不虚构解析）', () => {
    expect(formatTime('not-a-date')).toBe('not-a-date')
  })

  it('ISO UTC → 本地 HH:mm:ss（两位补零，按运行环境时区计算期望值）', () => {
    const iso = '2026-11-14T12:34:56Z'
    const d = new Date(iso)
    const expected = `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
    expect(formatTime(iso)).toBe(expected)
    expect(formatTime(iso)).toMatch(/^\d{2}:\d{2}:\d{2}$/)
  })
})

describe('formatDurationMs（cron-backup 4.2.4 耗时人性化「1m23s」）', () => {
  it('null/undefined 显示 —（running 派生缺失）', () => {
    expect(formatDurationMs(null)).toBe('—')
    expect(formatDurationMs(undefined)).toBe('—')
  })

  it('不足 1 分钟 → 秒级（45s）', () => {
    expect(formatDurationMs(45_000)).toBe('45s')
    expect(formatDurationMs(0)).toBe('0s')
  })

  it('分钟级 → 1m23s（PageSpec 示例形态）', () => {
    expect(formatDurationMs(83_000)).toBe('1m23s')
    expect(formatDurationMs(5 * 60_000)).toBe('5m00s')
  })

  it('小时级 → 1h02m03s（两位补零）', () => {
    expect(formatDurationMs(3_723_000)).toBe('1h02m03s')
  })

  it('负值钳制为 0（时钟偏差防御）', () => {
    expect(formatDurationMs(-5_000)).toBe('0s')
  })
})

describe('formatRelative（cron-backup 4.2.2 最近执行相对时间）', () => {
  it('null/undefined/空串显示 —（从未执行的 finishedAt 缺失场景）', () => {
    expect(formatRelative(null)).toBe('—')
    expect(formatRelative(undefined)).toBe('—')
    expect(formatRelative('')).toBe('—')
  })

  it('非法字符串原样透传（不虚构解析）', () => {
    expect(formatRelative('not-a-date')).toBe('not-a-date')
  })

  it('30 秒前 → 「N 秒前」', () => {
    expect(formatRelative(new Date(Date.now() - 30_000).toISOString())).toBe('30 秒前')
  })

  it('5 分钟前 → 「N 分钟前」', () => {
    expect(formatRelative(new Date(Date.now() - 5 * 60_000).toISOString())).toBe('5 分钟前')
  })

  it('3 小时前 → 「N 小时前」', () => {
    expect(formatRelative(new Date(Date.now() - 3 * 3_600_000).toISOString())).toBe('3 小时前')
  })

  it('2 天前 → 「N 天前」', () => {
    expect(formatRelative(new Date(Date.now() - 2 * 86_400_000).toISOString())).toBe('2 天前')
  })

  it('超 7 天回退绝对日期（formatDateTime 同源形态）', () => {
    const iso = new Date(Date.now() - 10 * 86_400_000).toISOString()
    expect(formatRelative(iso)).toBe(formatDateTime(iso))
  })

  it('未来时刻（时钟偏差）回退绝对日期', () => {
    const iso = new Date(Date.now() + 60_000).toISOString()
    expect(formatRelative(iso)).toBe(formatDateTime(iso))
  })
})
