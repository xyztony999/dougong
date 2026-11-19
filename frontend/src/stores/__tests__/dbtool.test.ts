/**
 * dbtoolStore 测试（PageSpec db-tools v1.2 第 1 节）：loadList 成败流（失败→listError=true
 * 页面 NAlert+重试；成功→items/generatedAt 投影，api v1.3 载荷含 boundInstanceId/
 * running 三态 + landingPort/autoCreated additive 可空透传〔D-19/I-14〕）+ ensureLoaded
 * 懒加载（database-mgmt 高级入口 6.8 步骤 1）+ findByType + taskFilter 联动
 * + 终态跃迁重拉全景与清单（4.3）+ clearTaskDetail 轮询清理幂等。
 */
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useDbtoolStore } from '../dbtool'

const okPayload = {
  items: [
    {
      toolType: 'phpmyadmin',
      displayName: 'phpMyAdmin',
      description: 'MySQL/MariaDB 高级管理',
      targetEngines: ['mysql', 'mariadb'],
      catalogVersion: '5.2.2',
      installed: true,
      installedVersion: '5.2.2',
      boundInstanceId: null,
      running: true,
      depsReady: true,
      depsMissing: [],
      runningTaskId: null,
      landingPort: 18090,
      autoCreated: true
    },
    {
      toolType: 'mongo_express',
      displayName: 'mongo-express',
      description: 'MongoDB 高级管理',
      targetEngines: ['mongodb'],
      catalogVersion: '1.0.0',
      installed: false,
      installedVersion: null,
      boundInstanceId: null,
      running: null,
      depsReady: false,
      depsMissing: [{ kind: 'env', name: 'node' }],
      runningTaskId: 7,
      landingPort: null,
      autoCreated: null
    }
  ],
  generatedAt: '2026-11-19T00:00:00Z'
}

describe('dbtoolStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('loadList 成功流：items/generatedAt 投影、listError=false（api v1.3 形状含 boundInstanceId/landingPort/autoCreated）', async () => {
    const dbtoolList = vi.fn().mockResolvedValue(okPayload)
    vi.doMock('@/api/dbtool', () => ({
      dbtoolList,
      dbtoolTaskList: vi.fn(),
      dbtoolTaskDetail: vi.fn()
    }))
    vi.resetModules()
    const { useDbtoolStore: useFresh } = await import('../dbtool')
    const store = useFresh()
    await store.loadList()
    expect(store.items).toHaveLength(2)
    expect(store.items[0].boundInstanceId).toBeNull()
    expect(store.items[1].running).toBeNull() // 三态不虚构（D-02）
    expect(store.items[1].depsMissing).toEqual([{ kind: 'env', name: 'node' }])
    // v1.3 additive 可空透传（D-19/I-14）：已装=siteRef 投影；未装=null（JSON null≠undefined）
    expect(store.items[0].landingPort).toBe(18090)
    expect(store.items[0].autoCreated).toBe(true)
    expect(store.items[1].landingPort).toBeNull()
    expect(store.items[1].autoCreated).toBeNull()
    expect(store.generatedAt).toBe('2026-11-19T00:00:00Z')
    expect(store.listError).toBe(false)
    expect(store.loaded).toBe(true)
  })

  it('loadList 失败流：listError=true 如实降级（页面 NAlert+重试）', async () => {
    vi.doMock('@/api/dbtool', () => ({
      dbtoolList: vi.fn().mockRejectedValue(new Error('boom')),
      dbtoolTaskList: vi.fn(),
      dbtoolTaskDetail: vi.fn()
    }))
    vi.resetModules()
    const { useDbtoolStore: useFresh } = await import('../dbtool')
    const store = useFresh()
    await store.loadList()
    expect(store.listError).toBe(true)
    expect(store.items).toHaveLength(0)
    expect(store.loaded).toBe(false)
  })

  it('ensureLoaded 懒加载仅首拉（高级入口 6.8 步骤 1：缓存后不重复请求）', async () => {
    const dbtoolList = vi.fn().mockResolvedValue(okPayload)
    vi.doMock('@/api/dbtool', () => ({
      dbtoolList,
      dbtoolTaskList: vi.fn(),
      dbtoolTaskDetail: vi.fn()
    }))
    vi.resetModules()
    const { useDbtoolStore: useFresh } = await import('../dbtool')
    const store = useFresh()
    await store.ensureLoaded()
    await store.ensureLoaded()
    expect(dbtoolList).toHaveBeenCalledTimes(1)
  })

  it('findByType 命中/未命中（未命中 null 不虚构）', async () => {
    vi.doMock('@/api/dbtool', () => ({
      dbtoolList: vi.fn().mockResolvedValue(okPayload),
      dbtoolTaskList: vi.fn(),
      dbtoolTaskDetail: vi.fn()
    }))
    vi.resetModules()
    const { useDbtoolStore: useFresh } = await import('../dbtool')
    const store = useFresh()
    await store.loadList()
    expect(store.findByType('phpmyadmin')?.catalogVersion).toBe('5.2.2')
    expect(store.findByType('mongo_express')?.installed).toBe(false)
  })

  it('setTaskFilter 联动 loadTasks 参数（toolType/status 筛选，pageSize=50 D-13）', async () => {
    const dbtoolTaskList = vi.fn().mockResolvedValue({ items: [], total: 0 })
    vi.doMock('@/api/dbtool', () => ({
      dbtoolList: vi.fn(),
      dbtoolTaskList,
      dbtoolTaskDetail: vi.fn()
    }))
    vi.resetModules()
    const { useDbtoolStore: useFresh } = await import('../dbtool')
    const store = useFresh()
    store.setTaskFilter('mongo_express', 'running')
    expect(store.taskFilter).toEqual({ toolType: 'mongo_express', status: 'running' })
    await store.loadTasks(2)
    expect(dbtoolTaskList).toHaveBeenCalledWith({
      toolType: 'mongo_express',
      status: 'running',
      page: 2,
      pageSize: 50
    })
  })

  it('clearTaskDetail 幂等清理（无任务详情时安全调用）', () => {
    const store = useDbtoolStore()
    expect(() => {
      store.stopPoll()
      store.clearTaskDetail()
    }).not.toThrow()
    expect(store.currentTask).toBeNull()
  })

  it('任务在途→终态跃迁触发工具全景与任务清单重拉（PageSpec 4.3，env-mgmt 先例）', async () => {
    const detail = (status: string) => ({
      id: 9,
      toolType: 'phpmyadmin',
      action: 'install',
      status,
      instanceId: null,
      requestedBy: 'admin',
      createdAt: '2026-11-19T00:00:00Z',
      startedAt: '2026-11-19T00:00:01Z',
      finishedAt: null,
      error: null,
      outputTail: ''
    })
    const dbtoolList = vi.fn().mockResolvedValue(okPayload)
    const dbtoolTaskList = vi.fn().mockResolvedValue({ items: [], total: 0 })
    const dbtoolTaskDetail = vi
      .fn()
      .mockResolvedValueOnce(detail('running'))
      .mockResolvedValueOnce(detail('succeeded'))
    vi.doMock('@/api/dbtool', () => ({ dbtoolList, dbtoolTaskList, dbtoolTaskDetail }))
    vi.resetModules()
    const { useDbtoolStore: useFresh } = await import('../dbtool')
    const store = useFresh()
    await store.loadTaskDetail(9) // 首拉=running（轮询启动）
    expect(dbtoolList).not.toHaveBeenCalled() // 在途未跃迁不重拉
    await store.loadTaskDetail(9) // 二拉=succeeded（终态跃迁）
    expect(dbtoolList).toHaveBeenCalledTimes(1) // 工具全景重拉
    expect(dbtoolTaskList).toHaveBeenCalledWith({
      toolType: undefined,
      status: undefined,
      page: 1,
      pageSize: 50
    })
    store.stopPoll()
  })

  it('首拉即终态不触发重拉（无跃迁；点开历史任务不受影响）', async () => {
    const dbtoolList = vi.fn().mockResolvedValue(okPayload)
    const dbtoolTaskDetail = vi.fn().mockResolvedValue({
      id: 3,
      toolType: 'mongo_express',
      action: 'uninstall',
      status: 'failed',
      instanceId: 2,
      requestedBy: 'admin',
      createdAt: '2026-11-18T00:00:00Z',
      startedAt: null,
      finishedAt: '2026-11-18T00:01:00Z',
      error: '面板重启中断',
      outputTail: ''
    })
    vi.doMock('@/api/dbtool', () => ({
      dbtoolList,
      dbtoolTaskList: vi.fn(),
      dbtoolTaskDetail
    }))
    vi.resetModules()
    const { useDbtoolStore: useFresh } = await import('../dbtool')
    const store = useFresh()
    await store.loadTaskDetail(3)
    expect(dbtoolList).not.toHaveBeenCalled()
    expect(store.currentTask?.error).toBe('面板重启中断') // I-09 如实呈现
  })

  it('refreshAfterWrite 组合刷新（任务创建后：全景+清单）', async () => {
    const dbtoolList = vi.fn().mockResolvedValue(okPayload)
    const dbtoolTaskList = vi.fn().mockResolvedValue({ items: [], total: 0 })
    vi.doMock('@/api/dbtool', () => ({
      dbtoolList,
      dbtoolTaskList,
      dbtoolTaskDetail: vi.fn()
    }))
    vi.resetModules()
    const { useDbtoolStore: useFresh } = await import('../dbtool')
    const store = useFresh()
    await store.refreshAfterWrite()
    expect(dbtoolList).toHaveBeenCalledTimes(1)
    expect(dbtoolTaskList).toHaveBeenCalledTimes(1)
  })
})
