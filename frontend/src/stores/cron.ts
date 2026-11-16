/**
 * cronStore —— PageSpec 第 1 节显式声明的状态面：
 * jobItems/systemEntries（cron 页 Tab1 含预置位分区）/ logTailText（Tab2）/
 * backupJobItems/runItems/runTotal/listParams（backup 页双 Tab）/
 * inFlight（行级与表单级在途互斥，前缀键覆盖 9 个写 Action）/
 * triggerModal（手动触发结果呈现态）/ runDrawer（执行详情 Drawer 态）/
 * backupFormDrawer（备份任务表单态）。
 *
 * 刷新纪律（PageSpec 第 2 节纪律表）：
 * - 进入页面当前 Tab 清单按需拉取（Tab 懒加载，切回不重拉除非过期）；
 * - jobCreate/Update/Delete 成功 → 重拉 jobList（crontab 实况已变）；
 *   jobTrigger 完成 → 不重拉 jobList（代执行不改 crontab），结果经 triggerResult 呈现；
 * - backupJobCreate/Update/Delete 成功 → 重拉 backupJobList（含派生列刷新）；
 *   backupJobRun 成功 → runId 反馈 + 历史 Tab 重拉 + 新记录行高亮 10s（页面编排）；
 * - 执行详情 Drawer：running 态 5s 轮询 backupRunDetail（定时器由 RunDetailDrawer
 *   组件持有并 onUnmounted 配对清理，store 提供数据通道）；转终态即停并刷新一次
 *   backupRunList；CB_007（轮转后记录消失）→ 关 Drawer + 重拉列表；
 * - 备份历史列表 running 态不自动轮询（克制原则）；
 * - 页面卸载 resetCronView/resetBackupView 清理 Dialog/Drawer/inFlight/高亮定时器。
 */
import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { ApiError } from '@/api/http'
import {
  backupJobList,
  backupRunDetail,
  backupRunList,
  jobList,
  logTail
} from '@/api/cron'
import type {
  BackupJobListItem,
  BackupRunDetailData,
  BackupRunListItem,
  CronJobItem,
  CronJobTriggerResult,
  CronSystemEntries
} from '@/types/cron'
import { CB_ERROR_TEXT } from '@/types/cron'
import type { BackupRunStatus, BackupTrigger } from '@/types/enums'

/** backup 页 Tab 2 查询条件（api list query 一一对应，PageSpec 4.2.4） */
export interface CronRunListParams {
  jobId: number | null
  status: BackupRunStatus | null
  trigger: BackupTrigger | null
  from: string | undefined
  to: string | undefined
  page: number
  pageSize: number
}

/** 手动触发结果呈现态（PageSpec 5.2；readonly 无触发入口不可达） */
export interface CronTriggerDisplay {
  expr: string
  result: CronJobTriggerResult
}

/** 立即备份后新记录行高亮时长（PageSpec 第 2 节纪律表：浅蓝高亮 10s） */
const HIGHLIGHT_MS = 10_000

export const useCronStore = defineStore('cron', () => {
  /** ---- cron 页 Tab 1 计划任务（crontab 实况投影，D-02） ---- */
  const jobItems = ref<CronJobItem[]>([])
  const systemEntries = ref<CronSystemEntries | null>(null)
  const jobLoading = ref(false)
  const jobLoaded = ref(false)

  /** ---- cron 页 Tab 2 执行日志（text/plain 尾部，D-04） ---- */
  const logTailText = ref<string | null>(null)
  const logLines = ref(200)
  const logLoading = ref(false)
  const logLoaded = ref(false)

  /** ---- 手动触发结果 Modal（admin 专属数据，readonly 无入口） ---- */
  const triggerResult = ref<CronTriggerDisplay | null>(null)

  /** ---- cron 任务 Dialog（admin 专属；表单字段在组件内） ---- */
  const jobFormVisible = ref(false)
  const editingJob = ref<CronJobItem | null>(null)

  /** ---- backup 页 Tab 1 备份任务 ---- */
  const backupJobItems = ref<BackupJobListItem[]>([])
  const backupJobLoading = ref(false)
  const backupJobLoaded = ref(false)

  /** ---- 备份任务表单 Drawer（admin 专属；表单字段在组件内） ---- */
  const backupFormVisible = ref(false)
  const editingBackupJob = ref<BackupJobListItem | null>(null)

  /** ---- backup 页 Tab 2 备份历史（remote 分页 + 筛选） ---- */
  const runItems = ref<BackupRunListItem[]>([])
  const runTotal = ref(0)
  const runLoading = ref(false)
  const runLoaded = ref(false)
  const listParams = ref<CronRunListParams>({
    jobId: null,
    status: null,
    trigger: null,
    from: undefined,
    to: undefined,
    page: 1,
    pageSize: 20
  })

  /** ---- 执行详情 Drawer（backupRunDetail；轮询定时器在 RunDetailDrawer 组件内） ---- */
  const runDrawerVisible = ref(false)
  const runDetailId = ref<number | null>(null)
  const runDetail = ref<BackupRunDetailData | null>(null)
  const runDetailLoading = ref(false)
  /** CB_007 等需 Drawer 组件（有 Message 上下文）提示的错误文案（store 无 Message，monitoring 先例） */
  const detailError = ref<string | null>(null)

  /** ---- 立即备份新记录行高亮（10s 定时器 store 持有，resetBackupView 配对清理） ---- */
  const highlightRunId = ref<number | null>(null)
  let highlightTimer: ReturnType<typeof setTimeout> | null = null

  /** ---- 在途互斥（PageSpec 4.1.3/4.2.3；前缀键覆盖 9 写 Action：
   * job-form / job-del:{jobKey} / job-trigger:{jobKey} / backup-form /
   * backup-del:{id} / backup-run:{id} / backup-download:{runId}:{file}；validate 为机制型在组件 checking 态承载） ---- */
  const inFlightKeys = ref<ReadonlySet<string>>(new Set<string>())
  const hasInFlight = computed(() => inFlightKeys.value.size > 0)

  function setInFlight(key: string): void {
    const next = new Set(inFlightKeys.value)
    next.add(key)
    inFlightKeys.value = next
  }

  function releaseInFlight(key: string): void {
    const next = new Set(inFlightKeys.value)
    next.delete(key)
    inFlightKeys.value = next
  }

  function isInFlight(key: string): boolean {
    return inFlightKeys.value.has(key)
  }

  /** ---- cron 页数据加载 ---- */

  async function loadJobs(): Promise<void> {
    if (jobLoading.value) return // 刷新按钮 loading 防抖
    jobLoading.value = true
    try {
      const data = await jobList()
      jobItems.value = data.items
      systemEntries.value = data.systemEntries
      jobLoaded.value = true
    } catch {
      // 错误已由 http 拦截器统一提示
    } finally {
      jobLoading.value = false
    }
  }

  async function loadLogTail(): Promise<void> {
    if (logLoading.value) return
    logLoading.value = true
    try {
      logTailText.value = await logTail(logLines.value)
      logLoaded.value = true
    } catch {
      // 拦截器统一提示；保留上次尾部文本（刷新失败不静默清空已有内容）
    } finally {
      logLoading.value = false
    }
  }

  /** 行数档位变更（[50,100,200,500,1000]，api D-04 越档服务端归 200） */
  function setLogLines(lines: number): void {
    logLines.value = lines
    void loadLogTail()
  }

  /** ---- backup 页数据加载 ---- */

  async function loadBackupJobs(): Promise<void> {
    if (backupJobLoading.value) return
    backupJobLoading.value = true
    try {
      const data = await backupJobList()
      backupJobItems.value = data.items
      backupJobLoaded.value = true
    } catch {
      // 拦截器统一提示
    } finally {
      backupJobLoading.value = false
    }
  }

  async function loadRuns(): Promise<void> {
    if (runLoading.value) return
    runLoading.value = true
    try {
      const p = listParams.value
      const data = await backupRunList({
        jobId: p.jobId ?? undefined,
        status: p.status ?? undefined,
        trigger: p.trigger ?? undefined,
        from: p.from,
        to: p.to,
        page: p.page,
        pageSize: p.pageSize
      })
      runItems.value = data.items
      runTotal.value = data.total
      runLoaded.value = true
    } catch {
      // 拦截器统一提示
    } finally {
      runLoading.value = false
    }
  }

  /** 筛选变更（时间范围已由页面转为 ISO UTC 串）；page 复位后重拉 */
  function setRunFilter(patch: Partial<Omit<CronRunListParams, 'page' | 'pageSize'>>): void {
    listParams.value = { ...listParams.value, ...patch, page: 1 }
    void loadRuns()
  }

  function setRunPage(page: number): void {
    listParams.value.page = page
    void loadRuns()
  }

  function setRunPageSize(pageSize: number): void {
    listParams.value.pageSize = pageSize
    listParams.value.page = 1
    void loadRuns()
  }

  /** ---- 执行详情 Drawer（数据通道；5s 轮询节奏由 RunDetailDrawer 组件驱动） ---- */

  function openRunDetail(runId: number): void {
    runDetailId.value = runId
    runDetail.value = null // 切换目标：清旧内容避免闪烁
    detailError.value = null
    runDrawerVisible.value = true
    void loadDetail()
  }

  function closeRunDrawer(): void {
    runDrawerVisible.value = false
    runDetailId.value = null
    runDetail.value = null
    detailError.value = null
  }

  async function loadDetail(): Promise<void> {
    const id = runDetailId.value
    if (id === null || runDetailLoading.value) return
    runDetailLoading.value = true
    const wasRunning = runDetail.value?.status === 'running'
    try {
      const data = await backupRunDetail(id)
      // 迟到响应守卫（Drawer 已关闭或已切换目标）
      if (!runDrawerVisible.value || runDetailId.value !== data.id) return
      runDetail.value = data
      // 轮询观测到转终态：刷新一次 backupRunList（第 2 节纪律表）
      if (wasRunning && data.status !== 'running') void loadRuns()
    } catch (e) {
      if (e instanceof ApiError && e.is('CB_007')) {
        // 轮转清理后记录消失（api D-09 自然结果）：关 Drawer + 重拉列表；文案由 Drawer 组件提示
        detailError.value = CB_ERROR_TEXT['CB_007']
        runDrawerVisible.value = false
        runDetailId.value = null
        runDetail.value = null
        void loadRuns()
      }
      // 其余错误拦截器统一提示（轮询下一轮自然重试）
    } finally {
      runDetailLoading.value = false
    }
  }

  function clearDetailError(): void {
    detailError.value = null
  }

  /** ---- 立即备份高亮（10s 后自动清除） ---- */

  function markHighlight(runId: number): void {
    if (highlightTimer !== null) clearTimeout(highlightTimer)
    highlightRunId.value = runId
    highlightTimer = setTimeout(() => {
      highlightRunId.value = null
      highlightTimer = null
    }, HIGHLIGHT_MS)
  }

  /** ---- Dialog/Drawer 态 ---- */

  function openJobForm(job: CronJobItem | null): void {
    editingJob.value = job
    jobFormVisible.value = true
  }

  function closeJobForm(): void {
    jobFormVisible.value = false
    editingJob.value = null
  }

  function openBackupForm(job: BackupJobListItem | null): void {
    editingBackupJob.value = job
    backupFormVisible.value = true
  }

  function closeBackupForm(): void {
    backupFormVisible.value = false
    editingBackupJob.value = null
  }

  function showTriggerResult(expr: string, result: CronJobTriggerResult): void {
    triggerResult.value = { expr, result }
  }

  function closeTriggerResult(): void {
    triggerResult.value = null
  }

  /** ---- 页面卸载清理（PageSpec 第 2 节纪律表末行） ---- */

  /** cron 页卸载：清 Dialog/触发 Modal/inFlight（无 store 定时器） */
  function resetCronView(): void {
    jobFormVisible.value = false
    editingJob.value = null
    triggerResult.value = null
    inFlightKeys.value = new Set<string>()
  }

  /** backup 页卸载：清 Drawer/表单态/inFlight/高亮定时器（轮询定时器由 Drawer 组件自清） */
  function resetBackupView(): void {
    closeRunDrawer()
    backupFormVisible.value = false
    editingBackupJob.value = null
    inFlightKeys.value = new Set<string>()
    if (highlightTimer !== null) {
      clearTimeout(highlightTimer)
      highlightTimer = null
    }
    highlightRunId.value = null
  }

  return {
    jobItems,
    systemEntries,
    jobLoading,
    jobLoaded,
    logTailText,
    logLines,
    logLoading,
    logLoaded,
    triggerResult,
    jobFormVisible,
    editingJob,
    backupFormVisible,
    editingBackupJob,
    backupJobItems,
    backupJobLoading,
    backupJobLoaded,
    runItems,
    runTotal,
    runLoading,
    runLoaded,
    listParams,
    runDrawerVisible,
    runDetail,
    runDetailLoading,
    detailError,
    highlightRunId,
    hasInFlight,
    loadJobs,
    loadLogTail,
    setLogLines,
    loadBackupJobs,
    loadRuns,
    setRunFilter,
    setRunPage,
    setRunPageSize,
    openRunDetail,
    closeRunDrawer,
    loadDetail,
    clearDetailError,
    markHighlight,
    openJobForm,
    closeJobForm,
    openBackupForm,
    closeBackupForm,
    showTriggerResult,
    closeTriggerResult,
    setInFlight,
    releaseInFlight,
    isInFlight,
    resetCronView,
    resetBackupView
  }
})
