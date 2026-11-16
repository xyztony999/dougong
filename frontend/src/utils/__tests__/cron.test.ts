import { describe, expect, it } from 'vitest'
import { CB_ERROR_TEXT, cronValidateErrors, isStringArray } from '@/types/cron'
import {
  normalizeBackupJobListData,
  normalizeBackupRunDetail,
  normalizeSourceRef,
  normalizeSourceRefList
} from '../cronback'

describe('CB_ERROR_TEXT（PageSpec 第 1 节错误处理 ↔ api 3.4 CB_001~009 一一对应）', () => {
  it('9 码全覆盖且键名逐字对齐', () => {
    const codes = Object.keys(CB_ERROR_TEXT)
    expect(codes).toEqual([
      'CB_001',
      'CB_002',
      'CB_003',
      'CB_004',
      'CB_005',
      'CB_006',
      'CB_007',
      'CB_008',
      'CB_009'
    ])
  })

  it('关键分流文案逐字（PageSpec 第 1 节）', () => {
    expect(CB_ERROR_TEXT['CB_003']).toBe('该任务已不存在（可能已在面板外修改）')
    expect(CB_ERROR_TEXT['CB_005']).toBe('名称已存在')
    expect(CB_ERROR_TEXT['CB_006']).toBe('该任务已有备份正在执行，请稍后再试')
    expect(CB_ERROR_TEXT['CB_008']).toBe('仅成功的执行记录可下载产物')
    expect(CB_ERROR_TEXT['CB_009']).toContain('产物文件已缺失')
  })
})

describe('cronValidateErrors（CB_002 data.errors[] 段位级错误提取）', () => {
  it('合法 errors 数组提取', () => {
    expect(cronValidateErrors({ errors: ['分钟段越界（0-59）', '小时段非法'] })).toEqual([
      '分钟段越界（0-59）',
      '小时段非法'
    ])
  })

  it('形状不符回退空数组（走兜底文案）', () => {
    expect(cronValidateErrors(null)).toEqual([])
    expect(cronValidateErrors('oops')).toEqual([])
    expect(cronValidateErrors({ errors: 'not-array' })).toEqual([])
    expect(cronValidateErrors({ fields: [{ field: 'expr', message: 'x' }] })).toEqual([])
  })
})

describe('isStringArray（unknown 收窄，禁 any 纪律的运行时通道）', () => {
  it('仅全 string 数组通过', () => {
    expect(isStringArray([])).toBe(true)
    expect(isStringArray(['a', 'b'])).toBe(true)
    expect(isStringArray(['a', 1])).toBe(false)
    expect(isStringArray(undefined)).toBe(false)
    expect(isStringArray(null)).toBe(false)
  })
})

describe('normalizeSourceRef（Gate 2 E1：契约 ref 直通 / 后端现状 {type,path?,instanceId?,database?} 推导）', () => {
  it('契约形状 {type, ref} 直通（后端补 ref 后零改动接管）', () => {
    expect(normalizeSourceRef({ type: 'site_dir', ref: '/var/www/a' })).toEqual({
      type: 'site_dir',
      ref: '/var/www/a'
    })
    expect(normalizeSourceRef({ type: 'database', ref: 'mysql 127.0.0.1:3306·mydb' })).toEqual({
      type: 'database',
      ref: 'mysql 127.0.0.1:3306·mydb'
    })
  })

  it('后端现状 site_dir → ref=path（dto.go SourceItemDTO 实际下发形状）', () => {
    expect(normalizeSourceRef({ type: 'site_dir', path: '/var/www/a' })).toEqual({
      type: 'site_dir',
      ref: '/var/www/a'
    })
  })

  it('后端现状 database → 实例·库；缺库=整实例（instanceId 为快照事实，D-12 同判）', () => {
    expect(normalizeSourceRef({ type: 'database', instanceId: 3, database: 'mydb' })).toEqual({
      type: 'database',
      ref: '实例 3·mydb'
    })
    expect(normalizeSourceRef({ type: 'database', instanceId: 3 })).toEqual({
      type: 'database',
      ref: '实例 3'
    })
  })

  it('无可推导字段 → ref 空串降级（渲染层降级不崩溃，不虚构）', () => {
    expect(normalizeSourceRef({ type: 'site_dir' })).toEqual({ type: 'site_dir', ref: '' })
    expect(normalizeSourceRef({ type: 'database' })).toEqual({ type: 'database', ref: '' })
    expect(normalizeSourceRef({ type: 'database', database: 'mydb' })).toEqual({
      type: 'database',
      ref: ''
    })
  })

  it('非对象/缺 type → null 剔除（协议违规不构造行）', () => {
    expect(normalizeSourceRef(null)).toBeNull()
    expect(normalizeSourceRef('oops')).toBeNull()
    expect(normalizeSourceRef({ ref: '/x' })).toBeNull()
  })

  it('归一输出恒满足契约形状（type: string、ref: string——消费端 s.ref.length 不再对 undefined 崩）', () => {
    const inputs: unknown[] = [
      { type: 'site_dir', ref: '/a' },
      { type: 'site_dir', path: '/b' },
      { type: 'database', instanceId: 1, database: 'd' },
      { type: 'site_dir' },
      { type: 'weird', path: '/c' }
    ]
    for (const n of normalizeSourceRefList(inputs)) {
      expect(typeof n.type).toBe('string')
      expect(typeof n.ref).toBe('string')
    }
    expect(normalizeSourceRefList(inputs)).toHaveLength(5)
  })

  it('normalizeSourceRefList 非数组容忍为空清单', () => {
    expect(normalizeSourceRefList(undefined)).toEqual([])
    expect(normalizeSourceRefList(null)).toEqual([])
    expect(normalizeSourceRefList([null, { type: 'site_dir', path: '/a' }])).toEqual([
      { type: 'site_dir', ref: '/a' }
    ])
  })
})

describe('normalizeBackupJobListData / normalizeBackupRunDetail（api 层接线管线断言）', () => {
  it('job list items[].sourceSummary 逐项归一（后端现状形状入 → 契约形状出）', () => {
    const raw = {
      items: [
        {
          id: 1,
          name: 'j',
          sourceSummary: [
            { type: 'site_dir', path: '/var/www/a' },
            { type: 'database', instanceId: 2, database: 'mydb' }
          ],
          targetType: 'local',
          targetBrief: '/backups',
          retentionCount: 7,
          scheduleCron: '0 3 * * *',
          nextFireAt: null,
          lastRun: null
        }
      ]
    }
    const data = normalizeBackupJobListData(raw)
    expect(data.items[0].sourceSummary).toEqual([
      { type: 'site_dir', ref: '/var/www/a' },
      { type: 'database', ref: '实例 2·mydb' }
    ])
  })

  it('run detail sources 逐项归一；契约 ref 已有则直通不改写', () => {
    const raw = {
      id: 9,
      jobId: 1,
      jobName: 'j',
      trigger: 'manual',
      status: 'success',
      startedAt: '2026-11-14T00:00:00Z',
      finishedAt: '2026-11-14T00:01:00Z',
      durationMs: 60000,
      artifactPath: '/backups/j/x',
      artifactSize: 1024,
      files: [{ name: 'site-1.tar.gz', size: 1024 }],
      errorMessage: null,
      sources: [
        { type: 'site_dir', ref: '/原生 ref 直通' },
        { type: 'database', instanceId: 5 }
      ]
    }
    const data = normalizeBackupRunDetail(raw)
    expect(data.sources).toEqual([
      { type: 'site_dir', ref: '/原生 ref 直通' },
      { type: 'database', ref: '实例 5' }
    ])
  })

  it('坏形状容忍：非对象直通不构造（后续渲染层空态/拦截器暴露，不在归一器吞错）', () => {
    expect(normalizeBackupJobListData(null)).toBe(null)
    expect(normalizeBackupRunDetail(undefined)).toBe(undefined)
  })
})
