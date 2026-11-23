/**
 * appStore —— 运行时项目状态（Python / Node 各一个仓库实例，ADR-033）：全景成败流 /
 * 就绪度 / 任务筛选 / 详情 5s 轮询终态即停，env-mgmt 先例；终态跃迁重拉全景清单。
 *
 * 网站管理页的「Python 项目」「Node 项目」分栏各挂一个面板，两个面板不能共用同一份
 * 清单——按运行时各建一个仓库（store id 带 runtime 后缀），轮询定时器 likewise 各自独立。
 */
import { defineStore } from 'pinia'
import * as api from '@/api/app'
import type { AppItem, AppListData, AppTaskDetailData, AppTaskItem, NodeReadiness, PythonReadiness } from '@/types/app'
import type { AppRuntime, AppTaskAction, AppTaskStatus } from '@/types/enums'

interface AppState {
  /** 当前仓库对应的运行时（清单只含这一类项目） */
  runtime: AppRuntime
  ready: PythonReadiness
  nodeReady: NodeReadiness
  items: AppItem[]
  generatedAt: string
  loadError: string | null
  tasks: AppTaskItem[]
  tasksTotal: number
  taskFilter: { appId: number | undefined; action: AppTaskAction | undefined; status: AppTaskStatus | undefined }
  currentTask: AppTaskDetailData | null
}

function build(runtime: AppRuntime) {
  // 轮询定时器是每个仓库实例私有的（不能放模块级共享——两个分栏会互相清掉对方的轮询）。
  let pollTimer: ReturnType<typeof setInterval> | null = null
  return defineStore(`app-${runtime}`, {
    state: (): AppState => ({
      runtime,
      ready: { installed: false, version: null, venvSupport: null, pipAvailable: null },
      nodeReady: { nodeAvailable: false, npmAvailable: false, pm2Available: false, nodeVersion: null },
      items: [],
      generatedAt: '',
      loadError: null,
      tasks: [],
      tasksTotal: 0,
      taskFilter: { appId: undefined, action: undefined, status: undefined },
      currentTask: null
    }),
    getters: {
      byId: (state) => (id: number) => state.items.find((i) => i.id === id) ?? null
    },
    actions: {
      /** loadList 全景成败流：成功覆盖投影；失败保留上一投影（I-04 env 先例）+错误态 */
      async loadList() {
        try {
          const data: AppListData = await api.appList(this.runtime)
          this.ready = data.pythonReadiness
          this.nodeReady = data.nodeReadiness
          this.items = data.items
          this.generatedAt = data.generatedAt
          this.loadError = null
        } catch (e) {
          this.loadError = e instanceof Error ? e.message : String(e)
        }
      },
      /** loadTasks 任务清单（筛选） */
      async loadTasks() {
        const data = await api.appTaskList({
          appId: this.taskFilter.appId,
          action: this.taskFilter.action,
          status: this.taskFilter.status
        })
        this.tasks = data.items
        this.tasksTotal = data.total
      },
      /** loadTaskDetail 单任务详情 + running/pending 态启动 5s 轮询（终态即停+跃迁重拉全景） */
      async loadTaskDetail(taskId: number) {
        const data = await api.appTaskDetail(taskId)
        this.currentTask = data
        if (data.status === 'running' || data.status === 'pending') {
          this.startPolling(taskId)
        } else {
          this.stopPolling()
          await this.loadList()
          await this.loadTasks()
        }
      },
      startPolling(taskId: number) {
        this.stopPolling()
        pollTimer = setInterval(async () => {
          const data = await api.appTaskDetail(taskId)
          this.currentTask = data
          if (data.status !== 'running' && data.status !== 'pending') {
            this.stopPolling()
            await this.loadList()
            await this.loadTasks()
          }
        }, 5000)
      },
      stopPolling() {
        if (pollTimer !== null) {
          clearInterval(pollTimer)
          pollTimer = null
        }
      }
    }
  })
}

const builders: Partial<Record<AppRuntime, ReturnType<typeof build>>> = {}

/** useAppStore 取指定运行时的项目仓库（同一运行时复用同一实例） */
export function useAppStore(runtime: AppRuntime) {
  return (builders[runtime] ??= build(runtime))()
}
