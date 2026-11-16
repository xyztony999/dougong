/**
 * monitoring api 层 —— 4 Action 与 api-monitoring.md 接口一一对应（S7，无虚构接口）。
 * 全部 GET 只读（无 CSRF 注入需求）；命名遵循 PageSpec 第 2 节 Action 定义。
 */
import type {
  MetricsHistoryData,
  MetricsHistoryQuery,
  MetricsSnapshot,
  ProcessesTopData,
  SystemInfoData
} from '@/types/monitoring'
import { request } from './http'

/** 实时快照（最新采样条目，含同源 processes；概览页 3s 轮询唯一数据源） */
export function metricsSnapshot(): Promise<MetricsSnapshot> {
  return request<MetricsSnapshot>('/monitoring/metrics/snapshot')
}

/** 历史趋势（窗口 × 维度 × 口径；仅进入/切换/刷新时拉取，无自动轮询——D-01） */
export function metricsHistory(q: MetricsHistoryQuery): Promise<MetricsHistoryData> {
  return request<MetricsHistoryData>('/monitoring/metrics/history', {
    query: {
      window: q.window,
      dimension: q.dimension,
      // scope 仅 dimension=cpu 时携带（D-04；其余维度携带即 MON_003）
      scope: q.dimension === 'cpu' ? q.scope : undefined
    }
  })
}

/**
 * TOP 进程单取（独立契约保留：概览页默认用快照内 processes 同源同响应；
 * panel-core 仪表盘/后续页面单取时经此 Action——PageSpec 第 2 节轮询纪律）。
 */
export function processesTop(): Promise<ProcessesTopData> {
  return request<ProcessesTopData>('/monitoring/processes/top')
}

/** 系统信息（静态 + 运行时长单响应；进入页面一次 + 60s 慢轮询） */
export function systemInfo(): Promise<SystemInfoData> {
  return request<SystemInfoData>('/monitoring/system/info')
}
