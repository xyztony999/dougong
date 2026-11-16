/**
 * trashStore 回归测试（Gate 2 第 1 轮 W1——后端 A-F11 增补 data.warning 此前未声明/未消费，
 * 孤儿对账剔除事实用户不可见；另覆盖行级在途互斥与错误静默保留数据语义）。
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { trashList } from '@/api/file'
import { useTrashStore } from '@/stores/trash'
import type { TrashItem, TrashListData } from '@/types/file'

vi.mock('@/api/http', () => {
  class ApiError extends Error {
    readonly code: string
    readonly data: unknown
    constructor(code: string, message: string, data: unknown) {
      super(message)
      this.code = code
      this.data = data
    }
    is(code: string): boolean {
      return this.code === code
    }
  }
  return { ApiError }
})
vi.mock('@/api/file', () => ({ trashList: vi.fn() }))

const mockTrashList = vi.mocked(trashList)
// 置于 vi.mock 之后仅为可读性（vitest 会提升 vi.mock 到文件首）
import { ApiError } from '@/api/http'

const ITEM: TrashItem = {
  id: 7,
  originalPath: '/srv/old.conf',
  entryType: 'file',
  size: 128,
  operatorName: 'admin',
  deletedAt: '2026-11-14T00:00:00Z'
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
})

describe('trashStore · W1 回归（data.warning 消费）', () => {
  it('孤儿剔除 warning 非空时捕获（页面 NAlert 呈现）', async () => {
    const data: TrashListData = {
      items: [ITEM],
      total: 1,
      warning: '2 条记录因回收站内容缺失已剔除（可能被面板外改动）'
    }
    mockTrashList.mockResolvedValue(data)
    const store = useTrashStore()
    await store.loadList()

    expect(store.items).toEqual([ITEM])
    expect(store.total).toBe(1)
    expect(store.listWarning).toBe(data.warning)
  })

  it('无孤儿时 warning 归位 null（omitempty 不下发）', async () => {
    mockTrashList.mockResolvedValueOnce({ items: [ITEM], total: 1, warning: '有一条' })
    mockTrashList.mockResolvedValue({ items: [ITEM], total: 1 })
    const store = useTrashStore()
    await store.loadList()
    expect(store.listWarning).toBe('有一条')

    await store.loadList()
    expect(store.listWarning).toBeNull()
  })
})

describe('trashStore · 在途互斥与错误语义', () => {
  it('还原行在途登记/解除', () => {
    const store = useTrashStore()
    store.setInFlight(7)
    expect(store.isInFlight(7)).toBe(true)
    store.releaseInFlight(7)
    expect(store.isInFlight(7)).toBe(false)
  })

  it('拉取失败静默（拦截器统一提示），既有数据保留', async () => {
    mockTrashList.mockResolvedValueOnce({ items: [ITEM], total: 1 })
    mockTrashList.mockRejectedValueOnce(new ApiError('FILE_013', 'fs error', { reason: 'io' }))
    const store = useTrashStore()
    await store.loadList()
    await store.loadList()

    expect(store.items).toEqual([ITEM])
    expect(store.listLoading).toBe(false)
  })
})
