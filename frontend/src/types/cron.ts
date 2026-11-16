/**
 * cron-backup API 类型 —— 与 api-cron-backup.md v1.0 §4 对齐（唯一事实源）。
 * 三线并存一模块（PageSpec 第 1 节）：
 * - cron_job 通道投影型（零台账 D2 口径-1）：jobList 为 crontab 实况投影，
 *   jobKey=D-01 内容寻址规范串，前端作不透明引用透传（不解析不展示）；
 * - backup_job 主数据类：list 不下发 sourceItems/targetConfig 结构化定义
 *   （仅 sourceSummary/targetBrief 概要，api 4.1.4）——编辑 Drawer 回填受限为
 *   已声明的实现偏差（见 frontend/tasks.md）；
 * - backup_run 业务流程类：running→success/failure（ADR-016），jobId 弱引用 +
 *   jobName 快照（D-12，任务已删历史仍可读）。
 * 错误码 CB_001~009 文案逐条落 PageSpec 第 1 节错误处理行。
 */
import type { ListResult } from './api'
import type {
  BackupRunStatus,
  BackupSourceType,
  BackupTargetType,
  BackupTrigger,
  CronSource
} from './enums'

/** ---- cron_job（通道投影型，api 4.3） ---- */

/** root crontab 任务条目（行序如实呈现不重排，D-02） */
export interface CronJobItem {
  /** 内容寻址规范串（表达式+\t+命令原文，D-01）——update/delete/trigger 寻址的不透明引用 */
  jobKey: string
  /** 五段表达式原值（宏已归一化，D-03） */
  expr: string
  /** 命令原文（原样透传，等宽呈现） */
  command: string
  /** `# dugong: remark:` 注释行文本；null=无备注注释行（D-02） */
  remark: string | null
  /** 来源：标记注释存在⇔panel / 缺失⇔existing（D-02） */
  source: CronSource
}

/** /etc/cron.d/ 文件内任务行（文件粒度呈现，D-02） */
export interface CronDFileEntry {
  expr: string
  command: string
}

export interface CronDFile {
  file: string
  entries: CronDFileEntry[]
}

/** cron.hourly/daily/weekly/monthly 目录（脚本文件名清单） */
export interface CronDir {
  dir: string
  scripts: string[]
}

/** 系统预置位只读分区（D1 口径-1：仅展示无任何写操作） */
export interface CronSystemEntries {
  cronDFiles: CronDFile[]
  cronDirs: CronDir[]
}

/** GET /cron/job/list 响应 data（无参数全量，crontab 行序） */
export interface CronJobListData {
  items: CronJobItem[]
  systemEntries: CronSystemEntries
}

/** POST /cron/job/create req */
export interface CronJobCreateReq {
  expr: string
  command: string
  /** 空=不写备注注释行（D-02） */
  remark?: string
}

/** create 成功 data：写入后定位引用 */
export interface CronJobCreateResult {
  jobKey: string
}

/** POST /cron/job/update req（jobKey=编辑前原引用；执行=删旧行+写新行，jobKey 随内容变更） */
export interface CronJobUpdateReq extends CronJobCreateReq {
  jobKey: string
}

/** update 成功 data：新 jobKey */
export interface CronJobUpdateResult {
  jobKey: string
}

/** POST /cron/job/delete req */
export interface CronJobDeleteReq {
  jobKey: string
}

/** POST /cron/job/validate req（D-03：POST 计算 服务，表达式串不以查询串承载） */
export interface CronJobValidateReq {
  expr: string
}

/** validate 成功 data：未来 5 次执行时间（PREVIEW_COUNT=5，ISO-8601） */
export interface CronJobValidateResult {
  nextFires: string[]
}

/** POST /cron/job/trigger req（I-08：命令原文取自 crontab 不经请求） */
export interface CronJobTriggerReq {
  jobKey: string
}

/** trigger 成功 data（同步 120s，D-05；执行失败/超时为业务结果非请求错误） */
export interface CronJobTriggerResult {
  /** null=未正常退出（异常终止） */
  exitCode: number | null
  /** stdout+stderr 合并，尾部 64KB 截断（超限保留尾部+头部省略标记） */
  output: string
  timedOut: boolean
  durationMs: number
}

/** ---- backup_job（主数据类，api 4.1） ---- */

/** 备份源清单项（I-09 判别联合；database 项 database 缺省=整实例全库） */
export type BackupSourceItem =
  | { type: 'site_dir'; path: string }
  | { type: 'database'; instanceId: number; database?: string }

/** 备份目标配置（I-10 与 targetType 匹配；rsync 无凭据字段，D-06） */
export type BackupTargetConfig =
  | { path: string }
  | { host: string; user: string; port?: number; path: string }

/**
 * 概要引用（list sourceSummary 与 run detail sources 共用；ref=目录路径 / 实例·库名）。
 * type 为契约枚举 ∪ 后端透传 string（docker ContainerStatus|string 先例）——
 * Gate 2 E1：后端 dto.go SourceItemDTO 未产出契约 ref 字段（实际 {type,path?,instanceId?,database?}），
 * 响应经 utils/cronback.ts 归一后 ref 恒为 string（不可推导时 ''，渲染层降级不崩溃）；
 * type 未知值渲染走 dictItem 兜底原值灰字（⚠️A14 口径）。
 */
export interface BackupSourceRef {
  type: BackupSourceType | string
  ref: string
}

/** 最近执行结论（读时子查询派生，D-11，不落冗余列） */
export interface BackupJobLastRun {
  id: number
  status: BackupRunStatus
  trigger: BackupTrigger
  startedAt: string
  finishedAt: string | null
}

/** backup-job/list 条目 */
export interface BackupJobListItem {
  id: number
  name: string
  sourceSummary: BackupSourceRef[]
  targetType: BackupTargetType
  /** 目标概要灰字（非结构化定义，不可回填表单） */
  targetBrief: string
  retentionCount: number
  scheduleCron: string
  /** 调度器推算下次执行（读时派生 D-11）；null=不可推算 */
  nextFireAt: string | null
  lastRun: BackupJobLastRun | null
}

/** GET /cron/backup-job/list 响应 data（无分页，任务个位数~十余，按 id 升序） */
export interface BackupJobListData {
  items: BackupJobListItem[]
}

/** POST /cron/backup-job/create req（四要素） */
export interface BackupJobCreateReq {
  /** ≤64 字符（R-14）；唯一性服务端 CB_005 */
  name: string
  sourceItems: BackupSourceItem[]
  targetType: BackupTargetType
  targetConfig: BackupTargetConfig
  /** 1-100（I-05 水位） */
  retentionCount: number
  /** 五段表达式（I-07/CB_002） */
  scheduleCron: string
}

/** create 成功 data */
export interface BackupJobCreateResult {
  id: number
}

/** POST /cron/backup-job/update req（整体替换语义：值对象替换） */
export interface BackupJobUpdateReq extends BackupJobCreateReq {
  jobId: number
}

/** POST /cron/backup-job/delete req（run/产物不级联，FR-09） */
export interface BackupJobDeleteReq {
  jobId: number
}

/** POST /cron/backup-job/run req（异步提交，D-07） */
export interface BackupJobRunReq {
  jobId: number
}

/** run 成功 data：立即返回（执行后台进行，进度经 detail 轮询） */
export interface BackupJobRunResult {
  runId: number
}

/** ---- backup_run（业务流程类，api 4.2） ---- */

/** backup-run/list 条目（按 id 倒序） */
export interface BackupRunListItem {
  id: number
  jobId: number
  /** 快照（D-12：任务删除/改名后历史仍可读） */
  jobName: string
  trigger: BackupTrigger
  status: BackupRunStatus
  startedAt: string
  finishedAt: string | null
  /** 派生耗时（running 时 null） */
  durationMs: number | null
  artifactSize: number | null
  errorBrief: string | null
}

/** backup-run/list query（公共分页 page/pageSize） */
export interface BackupRunListQuery {
  jobId?: number
  status?: BackupRunStatus
  trigger?: BackupTrigger
  from?: string
  to?: string
  page: number
  pageSize: number
}

/** GET /cron/backup-run/list 响应 data */
export interface BackupRunListData extends ListResult<BackupRunListItem> {}

/** 产物清单文件（manifest files 快照，success 时写——download 枚举依据） */
export interface BackupRunFile {
  name: string
  size: number
}

/** GET /cron/backup-run/detail 响应 data（running 态 finishedAt 及产物字段为 null） */
export interface BackupRunDetailData {
  id: number
  jobId: number
  jobName: string
  trigger: BackupTrigger
  status: BackupRunStatus
  startedAt: string
  finishedAt: string | null
  durationMs: number | null
  artifactPath: string | null
  artifactSize: number | null
  files: BackupRunFile[] | null
  /** 失败原因（failure 时写；含失败源标识与超时/传输/写入错误摘要） */
  errorMessage: string | null
  /** 执行时源清单快照（manifest 语义） */
  sources: BackupSourceRef[]
}

/** GET /cron/backup-run/download query（file ∈ run.files 清单；流式下载 D-10/D-16） */
export interface BackupRunDownloadQuery {
  runId: number
  file: string
}

/** ---- 错误文案与 data 收窄（PageSpec 第 1 节错误处理，逐条对应 api 3.4） ---- */

/**
 * CB_001~009（AUTH 码族走全局拦截器不复述）。
 * 001=Dialog/Drawer 字段级（data.fields[]）；002=表达式段位级（data.errors[]）；
 * 003/004/007=实况投影 warning+自动重拉；005=Drawer 名称字段错误；006=执行互斥 warning；
 * 008/009=下载前置校验 warning。
 */
export const CB_ERROR_TEXT: Readonly<Record<string, string>> = {
  CB_001: '参数校验失败',
  CB_002: '表达式解析失败',
  CB_003: '该任务已不存在（可能已在面板外修改）',
  CB_004: '备份任务不存在',
  CB_005: '名称已存在',
  CB_006: '该任务已有备份正在执行，请稍后再试',
  CB_007: '执行记录不存在',
  CB_008: '仅成功的执行记录可下载产物',
  CB_009: '产物文件已缺失（可能在目标位置被删除），执行记录保留为历史事实'
}

/** CB_002 data.errors[]（段位级错误文案串）收窄 */
export function isStringArray(v: unknown): v is string[] {
  return (
    Array.isArray(v) && v.every((it) => typeof it === 'string')
  )
}

/** CB_002 data.errors[] 提取（形状不符回退空数组，由调用方走兜底文案） */
export function cronValidateErrors(data: unknown): string[] {
  if (typeof data === 'object' && data !== null) {
    const errors = (data as Record<string, unknown>).errors
    if (isStringArray(errors)) return errors
  }
  return []
}
