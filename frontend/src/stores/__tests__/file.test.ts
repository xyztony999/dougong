/**
 * fileStore 回归测试（Gate 2 第 1 轮 E1 前端侧用例——报告 §5「修复时应补双侧用例：
 * 前端：首屏渲染 data.path」）：
 * - root-default 契约（后端 A-F20：entry/list 缺省 path=受管根）：初始浏览省略 path，
 *   响应 data.path 同时作为首屏面包屑基线（resolvedPath）、目录规范化（currentPath）
 *   与写操作寻址根（rootPath——后端仅 list 有缺省语义，写操作 dir/targetDir 空串即 FILE_001）；
 * - FILE_002 页面横幅清数据 / FILE_012 回受管根重拉 / 陈旧选择修剪 / 排序参数直传 / picker 同规范。
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { entryList } from '@/api/file'
import { useFileStore } from '@/stores/file'
import type { EntryListData, FileEntry } from '@/types/file'

// http.ts 模块级 createDiscreteApi 依赖 DOM——以最小 ApiError 假体替换（store 仅用 instanceof + is()）
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
vi.mock('@/api/file', () => ({ entryList: vi.fn() }))

const mockEntryList = vi.mocked(entryList)
// 置于 vi.mock 之后仅为可读性（vitest 会提升 vi.mock 到文件首）
import { ApiError } from '@/api/http'

function makeEntry(overrides: Partial<FileEntry> = {}): FileEntry {
  return {
    name: 'a.txt',
    path: '/srv/a.txt',
    entryType: 'file',
    size: 12,
    mode: '0644',
    ownerUser: 'root',
    ownerGroup: 'root',
    mtime: '2026-11-14T00:00:00Z',
    ...overrides
  }
}

const ROOT_DATA: EntryListData = {
  path: '/srv',
  items: [
    makeEntry(),
    makeEntry({ name: 'sub', path: '/srv/sub', entryType: 'directory', size: 0 })
  ],
  total: 2,
  generatedAt: '2026-11-14T00:00:00Z'
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
})

describe('fileStore · Gate 2 E1 回归（root-default 契约 A-F20）', () => {
  it('初始浏览省略 path（缺省=受管根），data.path 首屏渲染 + currentPath/rootPath 规范化', async () => {
    mockEntryList.mockResolvedValue(ROOT_DATA)
    const store = useFileStore()
    await store.loadList()

    expect(mockEntryList).toHaveBeenCalledTimes(1)
    expect(mockEntryList).toHaveBeenCalledWith({ path: undefined, sort: 'name', order: 'asc' })
    // 首屏渲染基线：面包屑用 resolvedPath；写操作寻址用 rootPath/currentPath（具体绝对路径）
    expect(store.resolvedPath).toBe('/srv')
    expect(store.currentPath).toBe('/srv')
    expect(store.rootPath).toBe('/srv')
    expect(store.total).toBe(2)
    expect(store.entries).toHaveLength(2)
  })

  it('受管根下的写寻址 writeDir() 返回 rootPath 具体路径（upload/create/move 空串即 FILE_001）', async () => {
    const store = useFileStore()
    expect(store.writeDir()).toBe('/') // 列表未加载 rootPath 未知：'/' 兜底（越界由服务端 FILE_002 拦截）

    mockEntryList.mockResolvedValue(ROOT_DATA)
    await store.loadList()
    expect(store.writeDir()).toBe('/srv')

    mockEntryList.mockResolvedValue({ ...ROOT_DATA, path: '/srv/www' })
    await store.navigate('/srv/www')
    expect(store.writeDir()).toBe('/srv/www')
  })

  it('排序参数变更即以具体 path 重拉（服务端排序 D-03）', async () => {
    mockEntryList.mockResolvedValue(ROOT_DATA)
    const store = useFileStore()
    await store.loadList()

    store.setSort('size', 'desc')
    expect(mockEntryList).toHaveBeenLastCalledWith({ path: '/srv', sort: 'size', order: 'desc' })
  })

  it('FILE_002 → 页面横幅态 + 不显示过期数据', async () => {
    mockEntryList.mockRejectedValue(new ApiError('FILE_002', 'out of scope', { reason: 'excluded' }))
    const store = useFileStore()
    await store.loadList()

    expect(store.scopeError).not.toBeNull()
    expect(store.entries).toHaveLength(0)
    expect(store.total).toBe(0)
    expect(store.generatedAt).toBeNull()
  })

  it('FILE_012（目标目录消失）→ 回受管根 root-default 重拉一次（防死循环）', async () => {
    mockEntryList.mockRejectedValueOnce(new ApiError('FILE_012', 'not a directory', null))
    mockEntryList.mockResolvedValue(ROOT_DATA)
    const store = useFileStore()
    store.currentPath = '/gone'
    await store.loadList()

    expect(mockEntryList).toHaveBeenCalledTimes(2)
    expect(mockEntryList).toHaveBeenLastCalledWith({ path: undefined, sort: 'name', order: 'asc' })
    expect(store.listBizError).toBe('目标目录不存在')
    expect(store.currentPath).toBe('/srv') // 重拉后规范化
  })

  it('列表重拉修剪指向已消失条目的陈旧选择', async () => {
    mockEntryList.mockResolvedValue(ROOT_DATA)
    const store = useFileStore()
    await store.loadList()

    store.setSelection(['/srv/a.txt', '/srv/vanished'])
    await store.loadList()
    expect(store.selection).toEqual(['/srv/a.txt'])
  })
})

describe('fileStore · 目录选择器（4.4.3/4.4.6——targetDir 必为具体路径）', () => {
  it('openPicker 起始受管根：root-default 请求 + pickerPath 规范化为 data.path + 仅列子目录', async () => {
    mockEntryList.mockResolvedValue(ROOT_DATA)
    const store = useFileStore()
    store.openPicker('move', '')
    await vi.waitFor(() => {
      expect(store.pickerPath).toBe('/srv')
    })

    expect(mockEntryList).toHaveBeenLastCalledWith({ path: undefined, sort: 'name', order: 'asc' })
    expect(store.pickerMode).toBe('move')
    expect(store.pickerDirs).toHaveLength(1)
    expect(store.pickerDirs[0]?.entryType).toBe('directory')
  })
})
