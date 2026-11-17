/**
 * envStore 测试（PageSpec env-mgmt v1.1 第 1 节）：loadList 成败流（失败→listError=true
 * 页面 NAlert+重试；成功→items/channel/generatedAt 投影，v1.1 七条目载荷含
 * mongodb 与全名 unitName）+ taskFilter 联动 + clearTaskDetail 轮询清理（stopPoll 幂等）。
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
      installed: true,
      version: '1.24.0',
      serviceRunning: true,
      serviceEnabled: true,
      unitName: 'nginx.service',
      runningTaskId: null
    },
    {
      envType: 'docker',
      displayName: 'Docker',
      description: '容器运行时',
      installed: null,
      version: null,
      serviceRunning: null,
      serviceEnabled: null,
      unitName: 'docker.service',
      runningTaskId: 7
    },
    {
      envType: 'mongodb',
      displayName: 'MongoDB',
      description: '文档数据库',
      installed: false,
      version: null,
      serviceRunning: null,
      serviceEnabled: null,
      unitName: 'mongod.service',
      runningTaskId: null
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
    expect(store.items).toHaveLength(3)
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

  it('任务在途→终态跃迁触发环境全景与任务清单重拉（v0.1.4 反馈第 1 项）', async () => {
    const detail = (status: string) => ({
      id: 9,
      envType: 'mysql',
      action: 'install',
      status,
      requestedBy: 'admin',
      createdAt: '2026-11-17T00:00:00Z',
      startedAt: '2026-11-17T00:00:01Z',
      finishedAt: null,
      error: null,
      bootstrapStatus: null,
      bootstrapNote: null,
      outputTail: ''
    })
    const envList = vi.fn().mockResolvedValue(okPayload)
    const envTaskList = vi.fn().mockResolvedValue({ items: [], total: 0 })
    const envTaskDetail = vi.fn().mockResolvedValueOnce(detail('running')).mockResolvedValueOnce(detail('succeeded'))
    vi.doMock('@/api/env', () => ({ envList, envTaskList, envTaskDetail }))
    vi.resetModules()
    const { useEnvStore: useFresh } = await import('../env')
    const store = useFresh()
    await store.loadTaskDetail(9) // 首拉=running（轮询启动）
    expect(envList).not.toHaveBeenCalled() // 在途未跃迁不重拉
    await store.loadTaskDetail(9) // 二拉=succeeded（终态跃迁）
    expect(envList).toHaveBeenCalledTimes(1) // 环境全景重拉
    expect(envTaskList).toHaveBeenCalledWith({ envType: undefined, status: undefined, page: 1, pageSize: 50 })
    store.stopPoll()
  })

  it('首拉即终态不触发重拉（无跃迁；点开历史任务不受影响）', async () => {
    const envList = vi.fn().mockResolvedValue(okPayload)
    const envTaskDetail = vi.fn().mockResolvedValue({
      id: 3,
      envType: 'nginx',
      action: 'install',
      status: 'failed',
      requestedBy: 'admin',
      createdAt: '2026-11-16T00:00:00Z',
      startedAt: null,
      finishedAt: '2026-11-16T00:01:00Z',
      error: 'boom',
      bootstrapStatus: null,
      bootstrapNote: null,
      outputTail: ''
    })
    vi.doMock('@/api/env', () => ({ envList, envTaskList: vi.fn(), envTaskDetail }))
    vi.resetModules()
    const { useEnvStore: useFresh } = await import('../env')
    const store = useFresh()
    await store.loadTaskDetail(3)
    expect(envList).not.toHaveBeenCalled()
  })

  it('引导结果字段经 loadTaskDetail 透传（v1.2 安装后配置就绪，D-18）', async () => {
    const envTaskDetail = vi.fn().mockResolvedValue({
      id: 11,
      envType: 'mysql',
      action: 'install',
      status: 'succeeded', // 包安装成功
      requestedBy: 'admin',
      createdAt: '2026-11-17T00:00:00Z',
      startedAt: '2026-11-17T00:00:01Z',
      finishedAt: '2026-11-17T00:02:00Z',
      error: null,
      bootstrapStatus: 'failed', // 引导失败独立维度（不改任务 succeeded）
      bootstrapNote: '引导通道建连失败',
      outputTail: '[bootstrap] failed: 引导通道建连失败'
    })
    vi.doMock('@/api/env', () => ({ envList: vi.fn(), envTaskList: vi.fn(), envTaskDetail }))
    vi.resetModules()
    const { useEnvStore: useFresh } = await import('../env')
    const store = useFresh()
    await store.loadTaskDetail(11)
    expect(store.currentTask?.status).toBe('succeeded')
    expect(store.currentTask?.bootstrapStatus).toBe('failed')
    expect(store.currentTask?.bootstrapNote).toBe('引导通道建连失败')
    store.stopPoll()
  })
})
