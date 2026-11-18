/**
 * dbStore 回归测试（Gate 2 W2 修复轮 2026-11-16——database-mgmt 前端域此前 0 用例）：
 * 覆盖 loadList 成败流（I-04 清单保持上一投影 / listLoading 在途防抖）、engine 筛选
 * 参数联动（setEngineFilter→loadList(engine)）、ensureLoaded 守卫入口（engineGuard 直达
 * 治理页判定，D-07）、findById 未命中不虚构、applyCredentials 行级直更（D-03 恒
 * reachable+credentialsSet 派生置位；version 空串跳过——I1 消费侧防御）、governableIds
 * 治理前置（I-05：reachable 且凭据已配置）。刷新纪律（D-04）：无轮询，store 不持定时器。
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'
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

vi.mock('@/api/database', () => ({
  instanceList: vi.fn()
}))

import { instanceList } from '@/api/database'
import { useDbStore } from '@/stores/database'
import type { CredentialsResult, DbInstanceItem, InstanceListData } from '@/types/database'

const mockList = vi.mocked(instanceList)

function makeItem(overrides: Partial<DbInstanceItem> = {}): DbInstanceItem {
  return {
    id: 1,
    engine: 'mysql',
    connMode: 'tcp',
    host: 'localhost',
    port: 3306,
    connState: 'reachable',
    credentialsSet: true,
    credUsername: 'root',
    discoveredAt: '2026-11-16T00:00:00Z',
    redundantSocket: false, // v2.3 D-21 additive 字段（默认非冗余）
    ...overrides
  }
}

/** 深拷贝防跨用例污染（store 直存 items 引用且 applyCredentials 原地改写行） */
function freshList(): InstanceListData {
  return {
    items: [
      makeItem({ id: 1, engine: 'mysql', connState: 'reachable', credentialsSet: true }),
      makeItem({ id: 2, engine: 'redis', port: 6379, connState: 'unreachable', credentialsSet: false, credUsername: undefined }),
      makeItem({ id: 3, engine: 'mysql', port: 3307, connState: 'reachable', credentialsSet: false, credUsername: undefined })
    ],
    generatedAt: '2026-11-16T01:00:00Z'
  }
}

/** 手动 flush 微任务队列（store 内 void 调用不返回 Promise） */
async function flush(): Promise<void> {
  await new Promise((r) => setTimeout(r, 0))
}

beforeEach(() => {
  setActivePinia(createPinia())
  mockList.mockReset()
})

describe('dbStore loadList', () => {
  it('成功：items/generatedAt/loaded 置位，listLoading 先真后假', async () => {
    const store = useDbStore()
    mockList.mockResolvedValue(structuredClone(freshList()))
    expect(store.loaded).toBe(false)
    const p = store.loadList()
    expect(store.listLoading).toBe(true) // 在途标记（按钮 loading 呈现）
    await p
    expect(store.listLoading).toBe(false)
    expect(store.loaded).toBe(true)
    expect(store.items).toHaveLength(3)
    expect(store.generatedAt).toBe('2026-11-16T01:00:00Z')
    expect(mockList).toHaveBeenCalledWith({ engine: undefined }) // 默认无筛选
  })

  it('防抖：在途时二次 loadList 直接返回，不重复请求', async () => {
    const store = useDbStore()
    let release!: (v: InstanceListData) => void
    mockList.mockReturnValue(
      new Promise<InstanceListData>((resolve) => {
        release = resolve
      })
    )
    const first = store.loadList()
    const second = store.loadList() // 在途防抖（service-mgmt 先例）
    await flush()
    expect(mockList).toHaveBeenCalledTimes(1)
    release(structuredClone(freshList()))
    await Promise.all([first, second])
    expect(store.loaded).toBe(true)
  })

  it('失败：错误传播（拦截器提示），清单保持上一投影（I-04）', async () => {
    const store = useDbStore()
    mockList.mockResolvedValue(structuredClone(freshList()))
    await store.loadList()
    const before = store.items
    mockList.mockRejectedValue(new Error('network down'))
    await expect(store.loadList()).rejects.toThrow('network down')
    expect(store.listLoading).toBe(false) // finally 复位
    expect(store.items).toBe(before) // 引用不变=上一投影未动
    expect(store.loaded).toBe(true) // 已加载态不回退（engineGuard 不误判）
  })

  it('engine 筛选：setEngineFilter 更新筛选并触发带参重拉', async () => {
    const store = useDbStore()
    mockList.mockResolvedValue(structuredClone(freshList()))
    store.setEngineFilter('redis')
    await flush()
    expect(store.engineFilter).toBe('redis')
    expect(mockList).toHaveBeenCalledWith({ engine: 'redis' })
    // 清空筛选 → engine 参数回 undefined
    store.setEngineFilter(null)
    await flush()
    expect(mockList).toHaveBeenLastCalledWith({ engine: undefined })
  })
})

describe('dbStore ensureLoaded / findById', () => {
  it('ensureLoaded：未加载时拉取一次；已加载不再拉取（engineGuard 入口）', async () => {
    const store = useDbStore()
    mockList.mockResolvedValue(structuredClone(freshList()))
    await store.ensureLoaded()
    expect(mockList).toHaveBeenCalledTimes(1)
    expect(store.loaded).toBe(true)
    await store.ensureLoaded()
    expect(mockList).toHaveBeenCalledTimes(1) // 不重复拉取
  })

  it('findById：命中返回行；未命中返回 null 不虚构', async () => {
    const store = useDbStore()
    mockList.mockResolvedValue(structuredClone(freshList()))
    await store.loadList()
    expect(store.findById(2)?.engine).toBe('redis')
    expect(store.findById(999)).toBeNull()
  })
})

describe('dbStore applyCredentials / governableIds', () => {
  it('applyCredentials：恒 reachable + credentialsSet 置位；version 空串跳过、非空刷新（D-03/D-04）', async () => {
    const store = useDbStore()
    mockList.mockResolvedValue(structuredClone(freshList()))
    await store.loadList()

    // 行 3：未配置凭据 + 无 version → 直更后可达且已配置；空串 version 不写入（I1 防御）。
    const emptyVersion: CredentialsResult = { instanceId: 3, connState: 'reachable', version: '' }
    store.applyCredentials(emptyVersion)
    const row3 = store.findById(3)
    expect(row3?.connState).toBe('reachable')
    expect(row3?.credentialsSet).toBe(true)
    expect(row3?.version).toBeUndefined()

    // 行 2：version 非空 → 实况刷新。
    store.applyCredentials({ instanceId: 2, connState: 'reachable', version: '7.2.4' })
    expect(store.findById(2)?.version).toBe('7.2.4')

    // 未命中 id：no-op 不抛错、不虚构行。
    store.applyCredentials({ instanceId: 999, connState: 'reachable', version: 'x' })
    expect(store.findById(999)).toBeNull()
    expect(store.items).toHaveLength(3)
  })

  it('governableIds：仅 reachable 且 credentialsSet 的实例入集合（I-05 前置）', async () => {
    const store = useDbStore()
    mockList.mockResolvedValue(structuredClone(freshList()))
    await store.loadList()
    expect([...store.governableIds]).toEqual([1]) // 行 2 unreachable、行 3 未配置凭据
    // 凭据配置成功后行 3 转可治理。
    store.applyCredentials({ instanceId: 3, connState: 'reachable', version: '' })
    expect(store.governableIds.has(3)).toBe(true)
  })
})

describe('dbStore redundantSocket 透传（v1.4 F1/D-21）', () => {
  it('已配凭据冗余 socket 行标注原样入清单（保守清理口径——保留不自动迁移）', async () => {
    const store = useDbStore()
    const data = freshList()
    data.items.push(
      makeItem({
        id: 4,
        engine: 'mysql',
        connMode: 'socket',
        socketPath: '/var/run/mysqld/mysqld.sock',
        host: undefined,
        port: undefined,
        connState: 'reachable',
        credentialsSet: true,
        redundantSocket: true
      })
    )
    mockList.mockResolvedValue(data)
    await store.loadList()
    const row = store.findById(4)
    expect(row?.redundantSocket).toBe(true)
    expect(row?.connMode).toBe('socket')
    // 冗余标注不影响治理前置判定（reachable+credentialsSet 仍可治理）
    expect(store.governableIds.has(4)).toBe(true)
    // TCP 行 redundantSocket 默认 false（不误标注）
    expect(store.findById(1)?.redundantSocket).toBe(false)
  })
})
