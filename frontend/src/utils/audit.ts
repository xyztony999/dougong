/**
 * 审计清单条目键归一（Gate 2 E2 修复，2026-11-14）。
 *
 * 契约基准：api-account-auth.md §4.3.4——items 条目键为 camelCase（id/actorName/…/createdAt），
 * 前端类型 src/types/auth.ts AuditLogItem 与契约一致（单一来源，不改）。
 * 后端现状（E2 双侧修复后，2026-11-14）：internal/store/auth/models.go AuditRecord
 * 已带全量 camelCase json tag（ActorID json:"-" 隐藏），条目键即契约 camelCase 直通。
 *
 * 决策：前端在 api 层保留容错归一垫片——优先读契约 camelCase 键，缺省回退 PascalCase 键
 * （E2 修复前历史通道，防旧形态响应击穿）；类型与页面字段全部保持契约 camelCase。
 * 枚举域（category/result/source）由 DDL CHECK 约束（api §5.3）保证，dictItem 对未知值原值展示。
 */
import type { ListResult } from '@/types/api'
import type { AuditCategory, AuditResult, AuditSource } from '@/types/enums'
import type { AuditLogItem } from '@/types/auth'

/** 后端 Go 字段名中连续大写缩写的特例（id→ID、objectId→ObjectID），其余为常规首字母大写 */
const PASCAL_ALIASES: Readonly<Record<string, string>> = {
  id: 'ID',
  objectId: 'ObjectID'
}

/** 读契约 camelCase 键，缺失时回退后端现状 PascalCase 键 */
function fieldOf(rec: Record<string, unknown>, camel: string): unknown {
  if (rec[camel] !== undefined) return rec[camel]
  const alias = PASCAL_ALIASES[camel] ?? camel.charAt(0).toUpperCase() + camel.slice(1)
  return rec[alias]
}

export function normalizeAuditItem(raw: unknown): AuditLogItem | null {
  if (typeof raw !== 'object' || raw === null) return null
  const rec = raw as Record<string, unknown>
  const id = fieldOf(rec, 'id')
  const actorName = fieldOf(rec, 'actorName')
  const category = fieldOf(rec, 'category')
  const action = fieldOf(rec, 'action')
  const objectType = fieldOf(rec, 'objectType')
  const objectId = fieldOf(rec, 'objectId')
  const detail = fieldOf(rec, 'detail')
  const result = fieldOf(rec, 'result')
  const source = fieldOf(rec, 'source')
  const createdAt = fieldOf(rec, 'createdAt')
  if (typeof id !== 'number') return null
  if (
    typeof actorName !== 'string' ||
    typeof category !== 'string' ||
    typeof action !== 'string' ||
    typeof objectType !== 'string' ||
    typeof result !== 'string' ||
    typeof source !== 'string' ||
    typeof createdAt !== 'string'
  ) {
    return null
  }
  return {
    id,
    actorName,
    category: category as AuditCategory,
    action,
    objectType,
    objectId: typeof objectId === 'string' ? objectId : null,
    detail: typeof detail === 'string' ? detail : null,
    result: result as AuditResult,
    source: source as AuditSource,
    createdAt
  }
}

/** data={items,total}（items/total 键双侧一致，仅条目内键需归一） */
export function normalizeAuditListResult(raw: unknown): ListResult<AuditLogItem> {
  const items: AuditLogItem[] = []
  let total = 0
  if (typeof raw === 'object' && raw !== null) {
    const rec = raw as Record<string, unknown>
    if (Array.isArray(rec.items)) {
      for (const it of rec.items) {
        const normalized = normalizeAuditItem(it)
        if (normalized !== null) items.push(normalized)
      }
    }
    if (typeof rec.total === 'number') total = rec.total
    else total = items.length
  }
  return { items, total }
}
