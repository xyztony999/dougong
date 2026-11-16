/**
 * monitorStore —— PageSpec 第 1 节声明的跨页面共享状态：
 * latestSnapshot（概览页 + 后续 panel-core 仪表盘复用）/ trendData + trendParams（历史趋势页，
 * 离开后参数与数据保留）/ systemInfo / pollTimers（轮询生命周期集中管理）。
 *
 * 轮询纪律（api D-01 + PageSpec 第 2 节）：
 * - 快照 3s 固定间隔（=采样周期）；document.hidden 暂停、恢复可见立即拉取一次；
 * - 系统信息进入页面一次 + 60s 慢轮询（前端节流决策）；
 * - 趋势无自动轮询——仅进入页面/切参/刷新按钮触发；
 * - 页面 onUnmounted 调 stopRealtimePolling()：定时器与 visibilitychange 监听全部清理。
 * MON_004 空态「监控数据初始化中」轮询不停止；网络失败保留上次数据 + 连接中断角标。
 */
import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { ApiError } from '@/api/http'
import {
  metricsHistory,
  metricsSnapshot,
  systemInfo as fetchSystemInfo
} from '@/api/monitoring'
import { MON_ERROR_TEXT } from '@/types/monitoring'
import type {
  MetricsHistoryData,
  MetricsSnapshot,
  SystemInfoData
} from '@/types/monitoring'
import type { MetricDimension, MetricScope, MetricWindow } from '@/types/enums'

/** 轮询间隔常量集中管理（frontend-03 §5.1；快照 3s = 采样周期，D-01/⚠️A12） */
export const SNAPSHOT_POLL_INTERVAL = 3000
export const SYSTEM_INFO_POLL_INTERVAL = 60000
/** 快照滞后阈值 = 2×采样周期（PageSpec 6.1 页面级横幅） */
export const SNAPSHOT_STALE_MS = 6000

/** 趋势查询条件（非表单字段：无提交、无持久化，切换即查——PageSpec 第 5 节） */
export interface TrendParams {
  window: MetricWindow
  dimension: MetricDimension
  scope: MetricScope
}

export const useMonitorStore = defineStore('monitor', () => {
  /** ---- 实时快照（3s 轮询） ---- */
  const latestSnapshot = ref<MetricsSnapshot | null>(null)
  /** MON_004：启动后尚无成功采样（空态「监控数据初始化中」，轮询不停止） */
  const initializing = ref(false)
  /** 网络失败角标（保留上次数据，恢复后自动消失） */
  const connectionLost = ref(false)
  /** ts 滞后 > 6s 角标（每次刷新后计算） */
  const stale = ref(false)

  /** ---- 系统信息（进入一次 + 60s 慢轮询） ---- */
  const systemInfoData = ref<SystemInfoData | null>(null)

  /** ---- 历史趋势（按需拉取，无自动轮询） ---- */
  const trendParams = ref<TrendParams>({ window: 'last_1h', dimension: 'cpu', scope: 'total' })
  const trendData = ref<MetricsHistoryData | null>(null)
  const trendLoading = ref(false)
  /** MON_001~003 兜底文案（正常 UI 由选择器约束不可达；history 页 watch 后提示） */
  const trendError = ref<string | null>(null)

  /** ---- 轮询生命周期（store 内单点持有，页面挂载/卸载配对起停） ---- */
  let snapshotTimer: ReturnType<typeof setInterval> | null = null
  let systemTimer: ReturnType<typeof setInterval> | null = null
  let visibilityBound = false
  let snapshotInFlight = false

  async function fetchSnapshot(): Promise<void> {
    if (snapshotInFlight) return // 慢响应不叠加（3s 节拍防重入）
    snapshotInFlight = true
    try {
      const data = await metricsSnapshot()
      latestSnapshot.value = data
      initializing.value = false
      connectionLost.value = false
      // 滞后判定：ts 为 ISO-8601 UTC，解析失败按未滞后处理（不虚构中断）
      const tsMs = Date.parse(data.ts)
      stale.value = Number.isNaN(tsMs) ? false : Date.now() - tsMs > SNAPSHOT_STALE_MS
    } catch (e) {
      if (e instanceof ApiError && e.is('MON_004')) {
        // 启动后首轮采样未完成：空态语义，轮询不停止（PageSpec 第 1 节）
        initializing.value = true
        latestSnapshot.value = null
        connectionLost.value = false
        stale.value = false
      } else {
        // 网络失败/服务错误：保留上次数据 + 连接中断角标（恢复后自动消失）
        connectionLost.value = true
      }
    } finally {
      snapshotInFlight = false
    }
  }

  async function loadSystemInfo(): Promise<void> {
    try {
      systemInfoData.value = await fetchSystemInfo()
    } catch {
      // 错误已由拦截器统一提示；准静态信息保留上次数据（I-07 无缓存语义，下轮覆盖）
    }
  }

  /** document.hidden 暂停 / 恢复可见立即拉取一次（D-01；配对移除见 unbindVisibility） */
  function handleVisibility(): void {
    if (document.hidden) {
      clearTimers()
    } else {
      void fetchSnapshot()
      startTimers()
    }
  }

  function startTimers(): void {
    if (snapshotTimer === null) {
      snapshotTimer = setInterval(() => {
        void fetchSnapshot()
      }, SNAPSHOT_POLL_INTERVAL)
    }
    if (systemTimer === null) {
      systemTimer = setInterval(() => {
        void loadSystemInfo()
      }, SYSTEM_INFO_POLL_INTERVAL)
    }
  }

  function clearTimers(): void {
    if (snapshotTimer !== null) {
      clearInterval(snapshotTimer)
      snapshotTimer = null
    }
    if (systemTimer !== null) {
      clearInterval(systemTimer)
      systemTimer = null
    }
  }

  /** 概览页 onMounted：进入即拉取 + 起轮询（含 visibilitychange 监听） */
  function startRealtimePolling(): void {
    void fetchSnapshot()
    void loadSystemInfo()
    startTimers()
    if (!visibilityBound) {
      document.addEventListener('visibilitychange', handleVisibility)
      visibilityBound = true
    }
  }

  /** 概览页 onUnmounted：定时器与监听全部清理（frontend-rules 🔴 配对清理） */
  function stopRealtimePolling(): void {
    clearTimers()
    if (visibilityBound) {
      document.removeEventListener('visibilitychange', handleVisibility)
      visibilityBound = false
    }
  }

  /** ---- 趋势（history 页；参数变更即拉取） ---- */

  async function loadTrend(): Promise<void> {
    if (trendLoading.value) return // 刷新按钮 loading 防抖（PageSpec 6.2）
    trendLoading.value = true
    trendError.value = null
    try {
      trendData.value = await metricsHistory(trendParams.value)
    } catch (e) {
      if (e instanceof ApiError && MON_ERROR_TEXT[e.code] !== undefined) {
        trendError.value = MON_ERROR_TEXT[e.code]
      }
      // 网络错误已由拦截器提示；保留上次数据（MON_004 语义同空态，series=[] 表达）
    } finally {
      trendLoading.value = false
    }
  }

  function setTrendWindow(w: MetricWindow): void {
    trendParams.value.window = w
    void loadTrend()
  }

  function setTrendDimension(d: MetricDimension): void {
    trendParams.value.dimension = d
    if (d !== 'cpu') trendParams.value.scope = 'total' // 离开 cpu 重置口径（PageSpec 6.2 显隐规则）
    void loadTrend()
  }

  function setTrendScope(s: MetricScope): void {
    trendParams.value.scope = s
    void loadTrend()
  }

  /** 趋势空态：series=[] 或 startTime=null（重启后暂无历史数据，PageSpec 6.2） */
  const trendEmpty = computed(
    () => trendData.value === null || trendData.value.series.length === 0 || trendData.value.startTime === null
  )

  return {
    latestSnapshot,
    initializing,
    connectionLost,
    stale,
    systemInfoData,
    trendParams,
    trendData,
    trendLoading,
    trendError,
    trendEmpty,
    startRealtimePolling,
    stopRealtimePolling,
    loadTrend,
    setTrendWindow,
    setTrendDimension,
    setTrendScope
  }
})
