/**
 * monitoring 业务类型 —— 与 api-monitoring.md v1.0 请求/响应字段逐字对齐（F02）。
 * D-08（采集失败不虚构数据）：快照/系统信息失败字段 null + degraded/errors 标注，
 * 指标组与叶子字段均可空；进程清单整体失败走 MON_004，而单项 cpuPercent/memBytes
 * 亦可空（无基线/单进程读取失败，backend *float64/*uint64）——两层降级并存。
 */
import type { MetricDimension, MetricScope, MetricWindow } from './enums'

/** 错误码标识（api 3.4；MON_001~004，AUTH_* 复用 account-auth 码族全局处理） */
export type MonitorErrorCode = 'MON_001' | 'MON_002' | 'MON_003' | 'MON_004'

/**
 * 页面级兜底文案（PageSpec 第 1 节错误处理行）：
 * MON_001/002/003 正常 UI 由选择器约束不会触发（R-14 服务端兜底）；MON_004 为空态非提示流。
 */
export const MON_ERROR_TEXT: Readonly<Record<string, string>> = {
  MON_001: '查询参数非法：时间窗口不在支持范围内',
  MON_002: '查询参数非法：指标维度不在支持范围内',
  MON_003: '查询参数非法：统计口径仅 CPU 维度可用'
}

/** ---- GET /monitoring/metrics/snapshot（api 4.1.4） ---- */

/**
 * TOP 进程项（I-06：≤10、按 cpuPercent 降序）。cpuPercent/memBytes 可空——
 * null = 该进程本轮读取失败或无基线（新进程首轮无 CPU 基线必 null，常态可达；
 * backend service/monitoring/types.go ProcessInfo 为 *float64/*uint64，D-08 不虚构默认值）；
 * 进程清单整体采集失败走 MON_004（区别于字段级降级）。
 */
export interface ProcessItem {
  pid: number
  name: string
  cpuPercent: number | null
  memBytes: number | null
}

export interface DiskCapacityItem {
  mount: string
  totalBytes: number | null
  usedBytes: number | null
}

export interface MetricsSnapshot {
  /** 采样时间戳（ISO-8601 UTC，本地时区展示不改写原值） */
  ts: string
  /** 本轮存在采集失败指标（D-08；页面横幅） */
  degraded: boolean
  cpu: { totalPercent: number | null; perCore: number[] | null } | null
  memory: { usedBytes: number | null; totalBytes: number | null; usedPercent: number | null } | null
  disk: {
    capacity: DiskCapacityItem[] | null
    ioReadBps: number | null
    ioWriteBps: number | null
  } | null
  net: { upBps: number | null; downBps: number | null } | null
  load: { l1: number | null; l5: number | null; l15: number | null } | null
  /** 最新 TOP 清单（快照页一次取全，避免快照与 TOP 两次轮询错拍，api 4.1.4） */
  processes: ProcessItem[] | null
}

/** ---- GET /monitoring/metrics/history（api 4.1.4；D-03 统一形状） ---- */

export interface MetricsHistoryQuery {
  window: MetricWindow
  dimension: MetricDimension
  /** 省略时默认 total；仅 dimension=cpu 合法（D-04，api 层按此裁剪发送） */
  scope?: MetricScope
}

/** 序列点：v=null 表达该分钟缺失（采样失败/未覆盖，D-03；前端断线不插值） */
export interface MetricPoint {
  ts: string
  v: number | null
}

/** series 单位（api series 契约；同一 dimension 内 unit 一致） */
export type MetricUnit = 'percent' | 'bytes' | 'bps' | 'value'

export interface MetricSeries {
  /** key 规范：cpu_total / cpu_core_{n} / memory_used / disk_capacity:{mount} / disk_io_read|write / network_up|down / load_1m|5m|15m */
  key: string
  label: string
  unit: MetricUnit
  points: MetricPoint[]
}

export interface MetricsHistoryData {
  window: MetricWindow
  dimension: MetricDimension
  scope: MetricScope
  /** 窗口内最早有效点（重启清零边界，ADR-014；无数据 null） */
  startTime: string | null
  /** 窗口内存在 v=null 点或未被数据完整覆盖 */
  partial: boolean
  series: MetricSeries[]
}

/** ---- GET /monitoring/processes/top（api 4.1.4；独立契约，概览页默认不调用） ---- */

export interface ProcessesTopData {
  ts: string
  processes: ProcessItem[]
}

/** ---- GET /monitoring/system/info（api 4.2.4；准静态实时投影） ---- */

export interface SystemInfoFieldError {
  field: string
  reason: string
}

export interface SystemInfoData {
  distro: string | null
  kernel: string | null
  hostname: string | null
  uptimeSeconds: number | null
  /** 部分获取失败清单（D-08：失败字段 null + errors，不虚构默认值） */
  errors: SystemInfoFieldError[] | null
}
