/**
 * envStore 测试（PageSpec env-mgmt v1.0）：loadList 成败流（失败→listError=true
 * 页面 NAlert+重试；成功→items/channel/generatedAt 投影）+ taskFilter 联动
 * + clearTaskDetail 轮询清理（stopPoll 幂等）。
 */
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useEnvStore } from '../env'

const okPayload = {
  channel: { family: 'apt', available: true },
  items: [
    {
      envType: 'nginx',
      displayName: 'nginx',
      description: 'Web 服务器',
      relatedModule: '网站管理',
      installed: true,
      version: '1.24.0',
      serviceRunning: true,
      serviceEnabled: true,
      unitName: 'nginx',
      runningTaskId: null
    },
    {
      envType: 'docker',
      displayName: 'Docker',
      description: '容器运行时',
      relatedModule: '容器管理',
      installed: null,
      version: null,
      serviceRunning: null,
      serviceEnabled: null,
      unitName: 'docker',
      runningTaskId: 7
    }
  ],
  generatedAt: '2026-11-17T00:00:00Z'
}

describe('envStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('loadList 成功流：items/channel/generatedAt 投影、listError=false', async () => {
    const envList = vi.fn().mockResolvedValue(okPayload)
    vi.doMock('@/api/env', () => ({ envList, envTaskList: vi.fn(), envTaskDetail: vi.fn() }))
    vi.resetModules()
    const { useEnvStore: useFresh } = await import('../env')
    const store = useFresh()
    await store.loadList()
    expect(store.items).toHaveLength(2)
    expect(store.channel.available).toBe(true)
    expect(store.channel.family).toBe('apt')
    expect(store.generatedAt).toBe('2026-11-17T00:00:00Z')
    expect(store.listError).toBe(false)
  })

  it('loadList 失败流：listError=true 如实降级（页面 NAlert+重试）', async () => {
    vi.doMock('@/api/env', () => ({
      envList: vi.fn().mockRejectedValue(new Error('boom')),
      envTaskList: vi.fn(),
      envTaskDetail: vi.fn()
    }))
    vi.resetModules()
    const { useEnvStore: useFresh } = await import('../env')
    const store = useFresh()
    await store.loadList()
    expect(store.listError).toBe(true)
    expect(store.items).toHaveLength(0)
  })

  it('setTaskFilter 联动 loadTasks 参数（envType/status 筛选）', async () => {
    const envTaskList = vi.fn().mockResolvedValue({ items: [], total: 0 })
    vi.doMock('@/api/env', () => ({ envList: vi.fn(), envTaskList, envTaskDetail: vi.fn() }))
    vi.resetModules()
    const { useEnvStore: useFresh } = await import('../env')
    const store = useFresh()
    store.setTaskFilter('nginx', 'running')
    expect(store.taskFilter).toEqual({ envType: 'nginx', status: 'running' })
    await store.loadTasks(2)
    expect(envTaskList).toHaveBeenCalledWith({ envType: 'nginx', status: 'running', page: 2, pageSize: 50 })
  })

  it('clearTaskDetail 幂等清理（无任务详情时安全调用）', () => {
    const store = useEnvStore()
    expect(() => {
      store.stopPoll()
      store.clearTaskDetail()
    }).not.toThrow()
    expect(store.currentTask).toBeNull()
  })
})
