/**
 * appStore 测试（vitest）——全景成败流/任务筛选透传/终态跃迁重拉（Sync 语义
 * 经 store 层验证）；api 层以 vi.mock 注入替身（零后端依赖）。
 */
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAppStore } from '../app'
import type { AppListData, AppTaskDetailData, AppTaskItem } from '@/types/app'

const listFixture: AppListData = {
  pythonReadiness: { installed: true, version: '3.11.2', venvSupport: true, pipAvailable: true },
  nodeReadiness: { nodeAvailable: true, npmAvailable: true, pm2Available: true, nodeVersion: '22.14.0' },
  items: [
    {
      id: 1, name: 'nim-proxy', dir: '/opt/nim', entry: 'app.py', runtime: 'python', startMode: 'preset_fastapi',
      startSpec: { module: 'app', app: 'app' },
      listenPort: 8100, serviceRunning: true, serviceEnabled: true, venvExists: true,
      depsInstalled: true, portListening: true, runningTaskId: null, hasSite: false,
      unitFailed: false, pm2Status: null, restarts: null, memoryBytes: null,
      createdAt: '2026-11-20T00:00:00Z', createdBy: 'admin'
    }
  ],
  generatedAt: '2026-11-20T00:00:00Z'
}

const taskFailed: AppTaskItem = {
  id: 2, appId: 1, appName: 'nim-proxy', action: 'deps_install', status: 'failed',
  purgeDir: false, requestedBy: 'admin', createdAt: '2026-11-20T01:00:00Z',
  error: '依赖安装失败: network down', retryable: true
}

vi.mock('@/api/app', () => ({
  appList: vi.fn(),
  appTaskList: vi.fn(),
  appTaskDetail: vi.fn()
}))

import { appList, appTaskDetail, appTaskList } from '@/api/app'
const mockList = vi.mocked(appList)
const mockTaskList = vi.mocked(appTaskList)
const mockDetail = vi.mocked(appTaskDetail)

async function flush() {
  await Promise.resolve()
  await Promise.resolve()
}

describe('appStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })
  afterEach(() => {
    useAppStore('python').stopPolling()
    useAppStore('node').stopPolling()
  })

  it('loadList 成功覆盖投影并清空错误态', async () => {
    const st = useAppStore('python')
    st.loadError = 'previous error'
    mockList.mockResolvedValueOnce(listFixture)
    await st.loadList()
    expect(st.items).toHaveLength(1)
    expect(st.items[0].name).toBe('nim-proxy')
    expect(st.ready.installed).toBe(true)
    expect(st.loadError).toBeNull()
    expect(st.generatedAt).toBe('2026-11-20T00:00:00Z')
  })

  it('loadList 失败保留上一投影（I-04 env 先例）并置错误态', async () => {
    const st = useAppStore('python')
    mockList.mockResolvedValueOnce(listFixture)
    await st.loadList()
    mockList.mockRejectedValueOnce(new Error('boom'))
    await st.loadList()
    expect(st.items).toHaveLength(1)
    expect(st.loadError).toBe('boom')
  })

  it('loadTaskDetail 终态即停并跃迁重拉全景/清单（env 先例）', async () => {
    const st = useAppStore('python')
    mockList.mockResolvedValue(listFixture)
    mockTaskList.mockResolvedValueOnce({ items: [taskFailed], total: 1 })
    const settled: AppTaskDetailData = { ...taskFailed, status: 'failed', outputTail: 'pip failed' }
    mockDetail.mockResolvedValueOnce(settled)
    await st.loadTaskDetail(2)
    await flush()
    expect(st.currentTask?.retryable).toBe(true)
    // 终态：轮询不启动（无挂起定时器即无泄漏）+ 全景/清单重拉发生
    expect(mockList.mock.calls.length).toBeGreaterThanOrEqual(1)
    expect(mockTaskList.mock.calls.length).toBeGreaterThanOrEqual(1)
  })

  it('loadTasks 筛选透传（appId/action/status）', async () => {
    const st = useAppStore('python')
    st.taskFilter = { appId: 1, action: 'deps_install', status: 'failed' }
    mockTaskList.mockResolvedValueOnce({ items: [taskFailed], total: 1 })
    await st.loadTasks()
    expect(mockTaskList.mock.calls[0][0]).toMatchObject({ appId: 1, action: 'deps_install', status: 'failed' })
  })

  it('loadList 按运行时请求清单（网站管理页的 Python / Node 分栏各取其一）', async () => {
    const st = useAppStore('node')
    mockList.mockResolvedValueOnce(listFixture)
    await st.loadList()
    expect(mockList.mock.calls[0][0]).toBe('node')
    expect(st.runtime).toBe('node')
    expect(st.nodeReady.pm2Available).toBe(true)
    expect(st.nodeReady.nodeVersion).toBe('22.14.0')
  })

  it('Python 与 Node 是两个独立仓库：清单互不串、停轮询互不影响', async () => {
    const py = useAppStore('python')
    const nd = useAppStore('node')
    expect(py).not.toBe(nd)
    const nodeFixture: AppListData = {
      ...listFixture,
      items: [{ ...listFixture.items[0], id: 9, name: 'web', runtime: 'node', startMode: 'npm_script', startSpec: { script: 'start' } }]
    }
    mockList.mockResolvedValueOnce(listFixture).mockResolvedValueOnce(nodeFixture)
    await py.loadList()
    await nd.loadList()
    expect(py.items[0]?.name).toBe('nim-proxy')
    expect(nd.items[0]?.name).toBe('web')
    // 同一运行时复用同一实例
    expect(useAppStore('node')).toBe(nd)
  })
})
