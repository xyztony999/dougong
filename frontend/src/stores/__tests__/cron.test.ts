/**
 * cronStore 回归测试（Gate 2 W2 修复轮 2026-11-16——cron-backup 前端域此前仅
 * utils 层 cron.test.ts 15 用例，store 0 覆盖）：覆盖 loadJobs/loadLogTail 成败流
 * 与在途防抖、setLogLines 档位联动、loadRuns 筛选参数投影（null→undefined）与
 * setRunFilter page 复位、inFlight 前缀键互斥（PageSpec 4.1.3）、runDetail 迟到
 * 响应守卫与 CB_007 关 Drawer 重拉（api D-09 轮转消失）、转终态刷新历史一次、
 * markHighlight 10s 定时器与 resetBackupView 配对清理（第 2 节纪律表）。
 * 挂载级组件用例沿 panel-core W4 / service-mgmt W1 / database-mgmt W2 先例维持
 * （@vue/test-utils 与 DOM 测试环境不在 devDependencies）。
 */
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

// http.ts 模块级 createDiscreteApi 依赖 DOM——以最小 ApiError 假体替换（file.test.ts 先例）
vi.mock('@/api/http', () => {
  class ApiError extends Error {
    readonly code: string
    readonly data: unknown
    constructor(code: string, message: string, data: unknown) {
      super(message)
      this.name = 'ApiError'
      this.code = code
      this.data = data
    }
    is(code: string): boolean {
      return this.code === code
    }
  }
  return { ApiError }
})

vi.mock('@/api/cron', () => ({
  jobList: vi.fn(),
  logTail: vi.fn(),
  backupJobList: vi.fn(),
  backupRunList: vi.fn(),
  backupRunDetail: vi.fn()
}))

import { ApiError } from '@/api/http'
import {
  backupRunDetail,
  backupRunList,
  jobList,
  logTail
} from '@/api/cron'
import { useCronStore } from '@/stores/cron'
import { CB_ERROR_TEXT } from '@/types/cron'
import type {
  BackupRunDetailData,
  BackupRunListData,
  CronJobListData
} from '@/types/cron'

const mockJobList = vi.mocked(jobList)
const mockLogTail = vi.mocked(logTail)
const mockRunList = vi.mocked(backupRunList)
const mockRunDetail = vi.mocked(backupRunDetail)

function jobListData(): CronJobListData {
  return {
    items: [
      {
        jobKey: '0 2 * * *\t/legacy/nightly.sh',
        expr: '0 2 * * *',
        command: '/legacy/nightly.sh',
        remark: null,
        source: 'existing'
      }
    ],
    systemEntries: { cronDFiles: [], cronDirs: [] }
  }
}

function runListData(): BackupRunListData {
  return {
    items: [
      {
        id: 3,
        jobId: 1,
        jobName: 'nightly',
        trigger: 'manual',
        status: 'running',
        startedAt: '2026-11-16T00:03:00Z',
        finishedAt: null,
        durationMs: null,
        artifactSize: null,
        errorBrief: null
      }
    ],
    total: 1
  }
}

function detailData(overrides: Partial<BackupRunDetailData> = {}): BackupRunDetailData {
  return {
    id: 3,
    jobId: 1,
    jobName: 'nightly',
    trigger: 'manual',
    status: 'running',
    startedAt: '2026-11-16T00:03:00Z',
    finishedAt: null,
    durationMs: null,
    artifactPath: null,
    artifactSize: null,
    files: null,
    errorMessage: null,
    sources: [],
    ...overrides
  }
}

/** 手动 flush 微任务队列（store 内 void 调用不返回 Promise） */
async function flush(): Promise<void> {
  await new Promise((r) => setTimeout(r, 0))
}

beforeEach(() => {
  setActivePinia(createPinia())
  mockJobList.mockReset()
  mockLogTail.mockReset()
  mockRunList.mockReset()
  mockRunDetail.mockReset()
})

describe('cronStore loadJobs / loadLogTail', () => {
  it('成功：items/systemEntries/loaded 置位；失败吞错保持上一投影且 loaded 不回退', async () => {
    const store = useCronStore()
    mockJobList.mockResolvedValue(jobListData())
    await store.loadJobs()
    expect(store.jobItems).toHaveLength(1)
    expect(store.systemEntries).toEqual({ cronDFiles: [], cronDirs: [] })
    expect(store.jobLoaded).toBe(true)
    const before = store.jobItems
    mockJobList.mockRejectedValue(new Error('network down'))
    await store.loadJobs() // 错误由 http 拦截器统一提示（store 吞错不 rethrow）
    expect(store.jobLoading).toBe(false) // finally 复位
    expect(store.jobItems).toBe(before) // 引用不变=上一投影未动
    expect(store.jobLoaded).toBe(true)
  })

  it('防抖：在途时二次 loadJobs 直接返回不重复请求', async () => {
    const store = useCronStore()
    let release!: (v: CronJobListData) => void
    mockJobList.mockReturnValue(
      new Promise<CronJobListData>((resolve) => {
        release = resolve
      })
    )
    const first = store.loadJobs()
    void store.loadJobs() // 在途防抖
    await flush()
    expect(mockJobList).toHaveBeenCalledTimes(1)
    release(jobListData())
    await first
    expect(store.jobLoaded).toBe(true)
  })

  it('logTail：setLogLines 档位变更触发带参重拉；失败保留上次尾部文本', async () => {
    const store = useCronStore()
    mockLogTail.mockResolvedValue('line-a')
    await store.loadLogTail()
    expect(store.logTailText).toBe('line-a')
    mockLogTail.mockResolvedValue('line-b')
    store.setLogLines(500) // void 调用
    await flush()
    expect(mockLogTail).toHaveBeenLastCalledWith(500)
    expect(store.logTailText).toBe('line-b')
    mockLogTail.mockRejectedValue(new Error('journalctl down'))
    await store.loadLogTail() // 同 loadJobs：拦截器提示，store 吞错
    expect(store.logLoading).toBe(false)
    expect(store.logTailText).toBe('line-b') // 刷新失败不静默清空已有内容
    expect(store.logLoaded).toBe(true)
  })
})

describe('cronStore loadRuns / 筛选分页', () => {
  it('loadRuns：listParams null 筛选投影为 undefined 查询参数', async () => {
    const store = useCronStore()
    mockRunList.mockResolvedValue(runListData())
    store.setRunFilter({ status: 'failure', from: '2026-11-16T00:00:00Z' })
    await flush()
    expect(mockRunList).toHaveBeenLastCalledWith({
      jobId: undefined,
      status: 'failure',
      trigger: undefined,
      from: '2026-11-16T00:00:00Z',
      to: undefined,
      page: 1,
      pageSize: 20
    })
    expect(store.runTotal).toBe(1)
    expect(store.runLoaded).toBe(true)
  })

  it('setRunFilter page 复位；setRunPage/setRunPageSize 联动重拉', async () => {
    const store = useCronStore()
    mockRunList.mockResolvedValue(runListData())
    store.listParams.page = 4
    store.setRunFilter({ trigger: 'cron' })
    await flush()
    expect(store.listParams.page).toBe(1) // 筛选变更复位页码
    store.setRunPage(3)
    await flush()
    expect(mockRunList).toHaveBeenLastCalledWith(
      expect.objectContaining({ trigger: 'cron', page: 3 })
    )
    store.setRunPageSize(50)
    await flush()
    expect(store.listParams).toMatchObject({ page: 1, pageSize: 50 })
  })

  it('在途防抖：runLoading 时二次 loadRuns 不重复请求', async () => {
    const store = useCronStore()
    let release!: (v: BackupRunListData) => void
    mockRunList.mockReturnValue(
      new Promise<BackupRunListData>((resolve) => {
        release = resolve
      })
    )
    const first = store.loadRuns()
    void store.loadRuns()
    await flush()
    expect(mockRunList).toHaveBeenCalledTimes(1)
    release(runListData())
    await first
  })
})

describe('cronStore inFlight 互斥（前缀键覆盖 9 写 Action）', () => {
  it('set/release/isInFlight/hasInFlight 状态一致；resetCronView 清空', () => {
    const store = useCronStore()
    store.setInFlight('job-del:k1')
    store.setInFlight('backup-run:1')
    expect(store.hasInFlight).toBe(true)
    expect(store.isInFlight('job-del:k1')).toBe(true)
    expect(store.isInFlight('job-trigger:k1')).toBe(false)
    store.releaseInFlight('job-del:k1')
    expect(store.isInFlight('job-del:k1')).toBe(false)
    expect(store.hasInFlight).toBe(true) // backup-run:1 仍在途
    store.resetCronView()
    expect(store.hasInFlight).toBe(false)
  })
})

describe('cronStore runDetail 通道（Drawer 数据面）', () => {
  it('openRunDetail 拉取；迟到响应守卫：Drawer 关闭后返回数据不写入', async () => {
    const store = useCronStore()
    let release!: (v: BackupRunDetailData) => void
    mockRunDetail.mockReturnValue(
      new Promise<BackupRunDetailData>((resolve) => {
        release = resolve
      })
    )
    store.openRunDetail(3)
    expect(store.runDrawerVisible).toBe(true)
    expect(store.runDetail).toBeNull() // 切换目标清旧内容
    store.closeRunDrawer() // 轮询窗口关闭（迟到响应）
    release(detailData())
    await flush()
    expect(store.runDetail).toBeNull() // 守卫生效：迟到数据不写入
    expect(store.runDrawerVisible).toBe(false)
  })

  it('CB_007（轮转后记录消失）：关 Drawer + detailError 文案 + 重拉列表', async () => {
    const store = useCronStore()
    mockRunDetail.mockRejectedValue(new ApiError('CB_007', CB_ERROR_TEXT['CB_007'], null))
    mockRunList.mockResolvedValue(runListData())
    store.openRunDetail(99)
    await flush()
    expect(store.runDrawerVisible).toBe(false)
    expect(store.runDetail).toBeNull()
    expect(store.detailError).toBe(CB_ERROR_TEXT['CB_007'])
    expect(mockRunList).toHaveBeenCalledTimes(1) // 关 Drawer 后重拉历史
    store.clearDetailError()
    expect(store.detailError).toBeNull()
  })

  it('轮询观测转终态：running→success 刷新一次历史列表', async () => {
    const store = useCronStore()
    mockRunDetail.mockResolvedValueOnce(detailData({ status: 'running' }))
    store.openRunDetail(3)
    await flush()
    expect(store.runDetail?.status).toBe('running')
    mockRunList.mockResolvedValue(runListData())
    mockRunDetail.mockResolvedValueOnce(
      detailData({ status: 'success', finishedAt: '2026-11-16T00:04:00Z', durationMs: 60000 })
    )
    await store.loadDetail() // Drawer 组件 5s 轮询驱动
    expect(store.runDetail?.status).toBe('success')
    expect(mockRunList).toHaveBeenCalledTimes(1) // 转终态刷新一次
  })
})

describe('cronStore 高亮定时器与页面卸载清理', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('markHighlight 10s 后自动清除；resetBackupView 即时清并取消定时器', async () => {
    vi.useFakeTimers()
    const store = useCronStore()
    store.markHighlight(7)
    expect(store.highlightRunId).toBe(7)
    vi.advanceTimersByTime(10_000) // HIGHLIGHT_MS=10s
    expect(store.highlightRunId).toBeNull()

    store.markHighlight(8)
    store.resetBackupView() // backup 页卸载：清 Drawer/表单/inFlight/高亮定时器
    expect(store.highlightRunId).toBeNull()
    expect(store.hasInFlight).toBe(false)
    vi.advanceTimersByTime(10_000)
    expect(store.highlightRunId).toBeNull() // 定时器已取消不复活
  })
})

describe('cronStore Dialog/Drawer/触发结果态', () => {
  it('jobForm/backupForm/triggerResult 开关与 reset 双清', () => {
    const store = useCronStore()
    const job = jobListData().items[0]
    store.openJobForm(job)
    expect(store.jobFormVisible).toBe(true)
    expect(store.editingJob?.jobKey).toBe(job.jobKey)
    store.closeJobForm()
    expect(store.jobFormVisible).toBe(false)
    expect(store.editingJob).toBeNull()

    store.openBackupForm(null)
    expect(store.backupFormVisible).toBe(true)
    store.closeBackupForm()
    expect(store.backupFormVisible).toBe(false)

    store.showTriggerResult('0 2 * * *', {
      exitCode: 0,
      output: 'done',
      timedOut: false,
      durationMs: 12
    })
    expect(store.triggerResult?.expr).toBe('0 2 * * *')
    expect(store.triggerResult?.result.exitCode).toBe(0)
    store.closeTriggerResult()
    expect(store.triggerResult).toBeNull()
  })
})
