import { describe, expect, it } from 'vitest'
import { normalizeAuditItem, normalizeAuditListResult } from '../audit'

describe('normalizeAuditItem（E2 审计条目键归一）', () => {
  it('后端现状 PascalCase 键归一为契约 camelCase', () => {
    const raw = {
      ID: 7,
      ActorID: 3,
      ActorName: 'alice',
      Category: 'security',
      Action: 'login_success',
      ObjectType: 'session',
      ObjectID: 'a1b2c3d4',
      Detail: '{"k":"v"}',
      Result: 'success',
      Source: 'web',
      CreatedAt: '2026-11-14T08:30:00Z'
    }
    expect(normalizeAuditItem(raw)).toEqual({
      id: 7,
      actorName: 'alice',
      category: 'security',
      action: 'login_success',
      objectType: 'session',
      objectId: 'a1b2c3d4',
      detail: '{"k":"v"}',
      result: 'success',
      source: 'web',
      createdAt: '2026-11-14T08:30:00Z'
    })
  })

  it('契约 camelCase 键直通（后端补 json tag 后不受影响）', () => {
    const raw = {
      id: 9,
      actorName: 'bob',
      category: 'change',
      action: 'user_created',
      objectType: 'user',
      objectId: null,
      detail: null,
      result: 'failure',
      source: 'cli',
      createdAt: '2026-11-14T09:00:00Z'
    }
    expect(normalizeAuditItem(raw)).toEqual(raw)
  })

  it('非对象/缺必填字段返回 null（不虚构行）', () => {
    expect(normalizeAuditItem(null)).toBeNull()
    expect(normalizeAuditItem('x')).toBeNull()
    expect(normalizeAuditItem({ ActorName: 'noid' })).toBeNull()
    expect(normalizeAuditItem({ ID: 'not-number' })).toBeNull()
  })
})

describe('normalizeAuditListResult', () => {
  it('items 键归一 + total 透传', () => {
    const raw = {
      items: [
        {
          ID: 1,
          ActorName: 'a',
          Category: 'change',
          Action: 'user_created',
          ObjectType: 'user',
          ObjectID: null,
          Detail: null,
          Result: 'success',
          Source: 'web',
          CreatedAt: '2026-11-14T08:30:00Z'
        }
      ],
      total: 42
    }
    const out = normalizeAuditListResult(raw)
    expect(out.total).toBe(42)
    expect(out.items).toHaveLength(1)
    expect(out.items[0]?.actorName).toBe('a')
    expect(out.items[0]?.createdAt).toBe('2026-11-14T08:30:00Z')
  })

  it('total 缺失时回退条目数；坏输入返回空结构', () => {
    expect(normalizeAuditListResult({ items: [] })).toEqual({ items: [], total: 0 })
    expect(normalizeAuditListResult(null)).toEqual({ items: [], total: 0 })
  })
})
