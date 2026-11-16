/**
 * cron-backup api 层 —— 15 Action 与 api-cron-backup.md §4 一一对应（S7，无虚构接口）。
 * 读 6 GET（admin+readonly 5 + admin 专属 download 1）；写 9 POST（admin 专属 + CSRF——
 * http.ts 统一注入 X-CSRF-Token）。函数名 = PageSpec 第 2 节 Action 名。
 * 特殊通道：logTail 为 200+text/plain（requestText，D-04）；backupRunDownload 为
 * GET 文件流（downloadFile + 浏览器原生保存，D-10/D-16）。
 */
import type {
  BackupJobCreateReq,
  BackupJobCreateResult,
  BackupJobDeleteReq,
  BackupJobListData,
  BackupJobRunReq,
  BackupJobRunResult,
  BackupJobUpdateReq,
  BackupRunDetailData,
  BackupRunDownloadQuery,
  BackupRunListData,
  BackupRunListQuery,
  CronJobCreateReq,
  CronJobCreateResult,
  CronJobDeleteReq,
  CronJobListData,
  CronJobTriggerReq,
  CronJobTriggerResult,
  CronJobUpdateReq,
  CronJobUpdateResult,
  CronJobValidateReq,
  CronJobValidateResult
} from '@/types/cron'
import { downloadFile, request, requestText } from './http'
import {
  normalizeBackupJobListData,
  normalizeBackupRunDetail
} from '@/utils/cronback'

/** ---- cron_job 通道投影（api 4.3） ---- */

/** jobList：root 任务投影 + 系统预置位只读分区（无参数，crontab 行序如实呈现 D-02） */
export function jobList(): Promise<CronJobListData> {
  return request<CronJobListData>('/cron/job/list')
}

/** jobCreate：写入 root crontab（含标记注释两行，D-02）；审计 cron_job_created */
export function jobCreate(req: CronJobCreateReq): Promise<CronJobCreateResult> {
  return request<CronJobCreateResult>('/cron/job/create', { method: 'POST', body: req })
}

/** jobUpdate：删旧行+写新行（jobKey 随内容变更，D-01）；审计 cron_job_updated */
export function jobUpdate(req: CronJobUpdateReq): Promise<CronJobUpdateResult> {
  return request<CronJobUpdateResult>('/cron/job/update', { method: 'POST', body: req })
}

/**
 * jobDelete：删除任务行+紧邻标记注释行；审计 cron_job_deleted。
 * 成功 data 形状契约未声明（api 4.3.4 仅记审计），前端不消费。
 */
export function jobDelete(req: CronJobDeleteReq): Promise<unknown> {
  return request<unknown>('/cron/job/delete', { method: 'POST', body: req })
}

/** jobValidate：表达式校验+未来 5 次预览（纯计算无副作用，D-03；机制型内嵌消费） */
export function jobValidate(req: CronJobValidateReq): Promise<CronJobValidateResult> {
  return request<CronJobValidateResult>('/cron/job/validate', { method: 'POST', body: req })
}

/** jobTrigger：面板代执行（同步 120s/64KB 尾部截断，D-05）；审计 cron_job_triggered */
export function jobTrigger(req: CronJobTriggerReq): Promise<CronJobTriggerResult> {
  return request<CronJobTriggerResult>('/cron/job/trigger', { method: 'POST', body: req })
}

/** logTail：cron 执行日志尾部（200+纯文本，D-04；lines 档位越档服务端归 200） */
export function logTail(lines?: number): Promise<string> {
  return requestText('/cron/log/tail', { lines })
}

/** ---- backup_job 主数据（api 4.1） ---- */

/**
 * backupJobList：任务清单（含 nextFireAt/lastRun 读时派生回显，D-11；无分页）。
 * 响应经 cronback 归一：sourceSummary 逐项补 ref（Gate 2 E1——后端 dto.go 未产出契约 ref，
 * {type,path?,instanceId?,database?} 现状推导；后端补齐后契约分支直通零改动）。
 */
export function backupJobList(): Promise<BackupJobListData> {
  return request<unknown>('/cron/backup-job/list').then(normalizeBackupJobListData)
}

/** backupJobCreate：创建任务（四要素；I-06/I-07/I-09/I-10 校验服务端终审）；审计 backup_job_created */
export function backupJobCreate(req: BackupJobCreateReq): Promise<BackupJobCreateResult> {
  return request<BackupJobCreateResult>('/cron/backup-job/create', { method: 'POST', body: req })
}

/**
 * backupJobUpdate：整体替换定义（值对象替换语义，编辑=全量提交）。
 * 成功 data 形状契约未声明（api 4.1.4），前端不消费。
 */
export function backupJobUpdate(req: BackupJobUpdateReq): Promise<unknown> {
  return request<unknown>('/cron/backup-job/update', { method: 'POST', body: req })
}

/**
 * backupJobDelete：删除任务（run/产物不级联留存至自身轮转，FR-09）；审计 backup_job_deleted。
 * 成功 data 形状契约未声明（api 4.1.4），前端不消费。
 */
export function backupJobDelete(req: BackupJobDeleteReq): Promise<unknown> {
  return request<unknown>('/cron/backup-job/delete', { method: 'POST', body: req })
}

/** backupJobRun：立即执行（异步返回 runId，D-07）；审计 backup_run_triggered */
export function backupJobRun(req: BackupJobRunReq): Promise<BackupJobRunResult> {
  return request<BackupJobRunResult>('/cron/backup-job/run', { method: 'POST', body: req })
}

/** ---- backup_run 业务流程（api 4.2） ---- */

/** backupRunList：历史分页检索（筛选 jobId/status/trigger/from/to + 公共分页，D-11） */
export function backupRunList(q: BackupRunListQuery): Promise<BackupRunListData> {
  return request<BackupRunListData>('/cron/backup-run/list', {
    query: {
      jobId: q.jobId,
      status: q.status,
      trigger: q.trigger,
      from: q.from,
      to: q.to,
      page: q.page,
      pageSize: q.pageSize
    }
  })
}

/**
 * backupRunDetail：单条完整信息（running 态产物字段 null；Drawer 5s 轮询承载，D-07）。
 * 响应经 cronback 归一：sources 逐项补 ref（Gate 2 E1，同 backupJobList 口径）。
 */
export function backupRunDetail(runId: number): Promise<BackupRunDetailData> {
  return request<unknown>('/cron/backup-run/detail', { query: { runId } }).then(
    normalizeBackupRunDetail
  )
}

/**
 * backupRunDownload：产物单文件流式下载（admin 专属 GET；两段校验 CB_008/009 后才开流，
 * D-10/D-16）。api 层触发浏览器原生保存（file-manager entryDownload 先例）；
 * 下载文件名=Content-Disposition 原名，缺失回退 manifest 内文件名。
 */
export async function backupRunDownload(q: BackupRunDownloadQuery): Promise<void> {
  const { blob, filename } = await downloadFile('/cron/backup-run/download', q)
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename ?? q.file
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}
