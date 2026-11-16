/**
 * cron-backup 源清单投影归一器（Gate 2 E1 前端侧修复，2026-11-14 报告）。
 *
 * 断裂事实（报告 E1）：api-cron-backup.md §4.1.4 契约 sourceSummary[{type, ref}] /
 * detail sources[]，而后端 service/cronback/dto.go SourceItemDTO 实际下发
 * {type, path?, instanceId?, database?}——全链无 ref 字段；前端 BackupSourceRef{type,ref}
 * 消费处（备份源 Tag/失败源标注）对 ref undefined 崩溃或失效。
 *
 * 修复口径（account-auth Gate 2 E1/E2 同判——api 层归一、契约类型单一来源、
 * 后端补 ref 后契约分支直通零改动）：
 * - ① 契约分支：ref 字符串直通（后端修复后自然接管）；
 * - ② 后端现状分支：site_dir→ref=path；database→ref=`实例 {id}·{库}`（缺库=整实例全库；
 *   instanceId 为快照事实，实例名跨域不可得——已删实例如实以 id 呈现，D-12 同判）；
 * - 无可推导字段 → ref=''（渲染层降级「—」不崩溃，不虚构）；
 * - 非对象/缺 type → null 剔除（协议违规不构造行）；
 * - type 保持契约枚举 ∪ 后端透传 string（docker ContainerStatus|string 先例），
 *   渲染层走 dictItem 兜底原值灰字（service-mgmt ⚠️A14 同口径）。
 */
import type { BackupJobListData, BackupRunDetailData, BackupSourceRef } from '@/types/cron'

/** 单项归一：契约 ref 直通 / 现状字段推导 / 空串降级；非对象缺 type 剔除 */
export function normalizeSourceRef(raw: unknown): BackupSourceRef | null {
  if (typeof raw !== 'object' || raw === null) return null
  const rec = raw as Record<string, unknown>
  const type = typeof rec.type === 'string' ? rec.type : ''
  if (type === '') return null
  // ① 契约分支（api §4.1.4：{type, ref}——后端投影层补齐 ref 后零改动接管）
  if (typeof rec.ref === 'string' && rec.ref !== '') {
    return { type, ref: rec.ref }
  }
  // ② 后端现状分支（dto.go SourceItemDTO：{type, path?, instanceId?, database?}）
  if (type === 'site_dir') {
    return { type, ref: typeof rec.path === 'string' ? rec.path : '' }
  }
  if (type === 'database') {
    const instanceId = typeof rec.instanceId === 'number' ? rec.instanceId : null
    const database =
      typeof rec.database === 'string' && rec.database !== '' ? rec.database : null
    if (instanceId === null) return { type, ref: '' }
    return { type, ref: database === null ? `实例 ${instanceId}` : `实例 ${instanceId}·${database}` }
  }
  // 未知 type（I-09 服务端值集校验下不可达）：ref 空串降级，dict 兜底原值呈现
  return { type, ref: '' }
}

/** 清单归一：非数组容忍为空清单（协议违规不构造行） */
export function normalizeSourceRefList(raw: unknown): BackupSourceRef[] {
  if (!Array.isArray(raw)) return []
  const out: BackupSourceRef[] = []
  for (const it of raw) {
    const n = normalizeSourceRef(it)
    if (n !== null) out.push(n)
  }
  return out
}

/** backup-job/list 响应归一：items[].sourceSummary 逐项过归一器（其余字段后端已对齐，直通） */
export function normalizeBackupJobListData(raw: unknown): BackupJobListData {
  const data = raw as BackupJobListData
  if (typeof data === 'object' && data !== null && Array.isArray(data.items)) {
    for (const item of data.items) {
      item.sourceSummary = normalizeSourceRefList(item.sourceSummary)
    }
  }
  return data
}

/** backup-run/detail 响应归一：sources 逐项过归一器（其余字段后端已对齐，直通） */
export function normalizeBackupRunDetail(raw: unknown): BackupRunDetailData {
  const data = raw as BackupRunDetailData
  if (typeof data === 'object' && data !== null) {
    data.sources = normalizeSourceRefList(data.sources)
  }
  return data
}
