/**
 * serviceStore 回归测试（Gate 2 W1 修复轮 2026-11-16——serviceStore 此前 0 用例）：
 * 覆盖 loadList 成败流（SVC_008 横幅清数据 I-01 / SVC_002 兜底文案 / 在途防抖）、
 * 参数联动（state 逗号拼接 query）、行级 inFlight 互斥、applySnapshot 直更 +
 * applyActiveStateRaw 三态防御收窄、Drawer 成败流与迟到响应守卫（关闭后到达不写入）、
 * resetView 清理。刷新纪律（D-03）：无轮询，store 不持定时器。
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

// http.ts 模块级 createDiscreteApi 依赖 DOM——以最小 ApiError 假体替换（store 仅用 instanceof + is()，file.test.ts 先例）
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

vi.mock('@/api/service', () => ({
  unitList: vi.fn(),
  unitDetail: vi.fn()
}))

import { ApiError } from '@/api/http'
import { unitDetail, unitList } from '@/api/service'
import { useServiceStore } from '@/stores/service'
import type { ServiceUnitDetail, ServiceUnitItem, ServiceUnitListData } from '@/types/service'

const mockUnitList = vi.mocked(unitList)
const mockUnitDetail = vi.mocked(unitDetail)

function makeItem(overrides: Partial<ServiceUnitItem> = {}): ServiceUnitItem {
  return {
    name: 'nginx.service',
    unitType: 'service',
    activeState: 'active',
    subState: 'running',
    enabledState: 'enabled',
    canOperate: true,
    canToggleAutostart: true,
    ...overrides
  }
}

const LIST_DATA: ServiceUnitListData = {
  items: [
    makeItem({ name: 'nginx.service', activeState: 'active' }),
    makeItem({ name: 'ssh.service', activeState: 'inactive', subState: 'dead', enabledState: 'disabled' })
  ],
  total: 2,
  generatedAt: '2026-11-16T00:00:00Z'
}

const DETAIL: ServiceUnitDetail = {
  ...makeItem(),
  description: 'nginx web server',
  loadState: 'loaded'
}

/** 每次返回深拷贝：store 直存 items 引用且 applySnapshot 原地改写行，防跨用例污染共享 fixture */
function freshList(): ServiceUnitListData {
  return structuredClone(LIST_DATA)
}

/** 手动 flush 微任务队列（store 内 void 调用不返回 Promise） */
async function flush(): Promise<void> {
  await new Promise((r) => setTimeout(r, 0))
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
})

describe('serviceStore · loadList 加载流', () => {
  it('成功：units/total/generatedAt 落位、loading 复位、无横幅；空过滤 query 不带 state', async () => {
    mockUnitList.mockResolvedValueOnce(freshList())
    const store = useServiceStore()
    await store.loadList()

    expect(mockUnitList).toHaveBeenCalledWith({ state: undefined, search: '' })
    expect(store.units).toEqual(LIST_DATA.items)
    expect(store.total).toBe(2)
    expect(store.generatedAt).toBe('2026-11-16T00:00:00Z')
    expect(store.listLoading).toBe(false)
    expect(store.listUnavailable).toBe(false)
    expect(store.listBizError).toBeNull()
  })

  it('SVC_008：页面级横幅置位 + 既有数据清空（I-01 不显示过期数据）', async () => {
    mockUnitList.mockResolvedValueOnce(freshList())
    const store = useServiceStore()
    await store.loadList()
    expect(store.units).toHaveLength(2)

    mockUnitList.mockRejectedValueOnce(new ApiError('SVC_008', 'unavailable', null))
    await store.loadList()
    expect(store.listUnavailable).toBe(true)
    expect(store.units).toEqual([])
    expect(store.total).toBe(0)
    expect(store.generatedAt).toBeNull()
  })

  it('SVC_002：兜底文案置位（页面 watch 提示）、既有数据保留', async () => {
    mockUnitList.mockResolvedValueOnce(freshList())
    const store = useServiceStore()
    await store.loadList()

    mockUnitList.mockRejectedValueOnce(new ApiError('SVC_002', 'bad param', null))
    await store.loadList()
    expect(store.listBizError).toBe('参数非法')
    expect(store.units).toEqual(LIST_DATA.items)
    expect(store.listUnavailable).toBe(false)
  })

  it('在途防抖：listLoading 期间二次 loadList 早退，不重复请求', async () => {
    let resolveList!: (v: ServiceUnitListData) => void
    mockUnitList.mockReturnValueOnce(
      new Promise<ServiceUnitListData>((r) => {
        resolveList = r
      })
    )
    const store = useServiceStore()
    const first = store.loadList() // 同步段即置 listLoading=true
    void store.loadList() // 刷新按钮防抖：早退
    expect(mockUnitList).toHaveBeenCalledTimes(1)

    resolveList(freshList())
    await first
    expect(store.units).toEqual(LIST_DATA.items)
    expect(store.listLoading).toBe(false)
  })
})

describe('serviceStore · 参数联动（参数变更即触发 unitList，PageSpec 4.1）', () => {
  it('setStateFilter：多选逗号拼接为 state query 并重拉', async () => {
    mockUnitList.mockResolvedValue(freshList())
    const store = useServiceStore()
    store.setStateFilter(['active', 'failed'])
    await flush()
    expect(mockUnitList).toHaveBeenLastCalledWith({ state: 'active,failed', search: '' })
  })

  it('setSearch：搜索词更新并重拉', async () => {
    mockUnitList.mockResolvedValue(freshList())
    const store = useServiceStore()
    store.setSearch('nginx')
    await flush()
    expect(mockUnitList).toHaveBeenLastCalledWith({ state: undefined, search: 'nginx' })
  })
})

describe('serviceStore · 行级 inFlight 互斥（PageSpec 4.3）', () => {
  it('set/release 互斥且不同行互不影响', () => {
    const store = useServiceStore()
    store.setInFlight('nginx.service')
    expect(store.isInFlight('nginx.service')).toBe(true)
    expect(store.isInFlight('ssh.service')).toBe(false)

    store.releaseInFlight('nginx.service')
    expect(store.isInFlight('nginx.service')).toBe(false)
  })
})

describe('serviceStore · 动作成功快照直更（D-03 不整表重拉）', () => {
  it('applySnapshot：命中行三字段直更 + generatedAt 更新为响应时刻', async () => {
    mockUnitList.mockResolvedValueOnce(freshList())
    const store = useServiceStore()
    await store.loadList()

    store.applySnapshot({ name: 'nginx.service', activeState: 'inactive', subState: 'dead', enabledState: 'disabled' })
    const row = store.units.find((u) => u.name === 'nginx.service')
    expect(row?.activeState).toBe('inactive')
    expect(row?.subState).toBe('dead')
    expect(row?.enabledState).toBe('disabled')
    expect(store.generatedAt).not.toBe('2026-11-16T00:00:00Z')
    expect(Number.isNaN(Date.parse(store.generatedAt ?? ''))).toBe(false)
    expect(mockUnitList).toHaveBeenCalledTimes(1) // 未重拉
  })

  it('applySnapshot：未命中行数据不动、generatedAt 仍更新', async () => {
    mockUnitList.mockResolvedValueOnce(freshList())
    const store = useServiceStore()
    await store.loadList()

    store.applySnapshot({ name: 'ghost.service', activeState: 'active' })
    expect(store.units).toHaveLength(2)
    expect(store.units.map((u) => u.name)).toEqual(['nginx.service', 'ssh.service'])
    expect(store.generatedAt).not.toBe('2026-11-16T00:00:00Z')
  })

  it('applyActiveStateRaw：非三态（如 activating）拒绝直更（防御收窄），三态生效', async () => {
    mockUnitList.mockResolvedValueOnce(freshList())
    const store = useServiceStore()
    await store.loadList()

    store.applyActiveStateRaw('nginx.service', 'activating')
    expect(store.units.find((u) => u.name === 'nginx.service')?.activeState).toBe('active')

    store.applyActiveStateRaw('nginx.service', 'failed')
    expect(store.units.find((u) => u.name === 'nginx.service')?.activeState).toBe('failed')
  })

  it('markListUnavailable：动作路径 SVC_008 归一为横幅 + 清数据（I-01）', async () => {
    mockUnitList.mockResolvedValueOnce(freshList())
    const store = useServiceStore()
    await store.loadList()

    store.markListUnavailable()
    expect(store.listUnavailable).toBe(true)
    expect(store.units).toEqual([])
    expect(store.generatedAt).toBeNull()
  })
})

describe('serviceStore · 属性 Drawer（打开时刻快照，不自动刷新）', () => {
  it('openDrawer 成功：drawerOpen/drawerName 置位、unitDetail 落位', async () => {
    mockUnitDetail.mockResolvedValueOnce(DETAIL)
    const store = useServiceStore()
    store.openDrawer('nginx.service')
    expect(store.drawerOpen).toBe(true)
    expect(store.drawerName).toBe('nginx.service')
    await flush()
    expect(store.unitDetail).toEqual(DETAIL)
    expect(store.detailLoading).toBe(false)
    expect(store.detailError).toBeNull()
  })

  it('SVC_003 → notfound；SVC_008 → unavailable；unitDetail 保持 null', async () => {
    const store = useServiceStore()
    mockUnitDetail.mockRejectedValueOnce(new ApiError('SVC_003', 'gone', null))
    store.openDrawer('gone.service')
    await flush()
    expect(store.detailError).toBe('notfound')
    expect(store.unitDetail).toBeNull()

    mockUnitDetail.mockRejectedValueOnce(new ApiError('SVC_008', 'unavailable', null))
    store.openDrawer('nginx.service')
    await flush()
    expect(store.detailError).toBe('unavailable')
    expect(store.unitDetail).toBeNull()
  })

  it('迟到响应守卫：关闭 Drawer 后到达的响应不写入（drawerOpen 检查）', async () => {
    let resolveDetail!: (v: ServiceUnitDetail) => void
    mockUnitDetail.mockReturnValueOnce(
      new Promise<ServiceUnitDetail>((r) => {
        resolveDetail = r
      })
    )
    const store = useServiceStore()
    store.openDrawer('nginx.service')
    store.closeDrawer()
    resolveDetail(DETAIL)
    await flush()
    expect(store.unitDetail).toBeNull()
    expect(store.drawerOpen).toBe(false)
  })

  it('refreshDetail：按当前 drawerName 重拉', async () => {
    mockUnitDetail.mockResolvedValue(DETAIL)
    const store = useServiceStore()
    store.openDrawer('nginx.service')
    await flush()
    store.refreshDetail()
    await flush()
    expect(mockUnitDetail).toHaveBeenCalledTimes(2)
    expect(mockUnitDetail).toHaveBeenLastCalledWith('nginx.service')
  })

  it('resetView：页面卸载清理 Drawer 与 inFlight 态', async () => {
    mockUnitDetail.mockResolvedValue(DETAIL)
    const store = useServiceStore()
    store.setInFlight('nginx.service')
    store.openDrawer('nginx.service')
    await flush()

    store.resetView()
    expect(store.drawerOpen).toBe(false)
    expect(store.drawerName).toBeNull()
    expect(store.unitDetail).toBeNull()
    expect(store.detailError).toBeNull()
    expect(store.detailLoading).toBe(false)
    expect(store.isInFlight('nginx.service')).toBe(false)
  })
})
