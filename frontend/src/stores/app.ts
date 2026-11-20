/**
 * appStore —— 应用管理状态（全景成败流/就绪度/任务筛选/详情 5s 轮询终态即停，
 * env-mgmt 先例；终态跃迁重拉全景清单）。
 */
import { defineStore } from 'pinia'
import * as api from '@/api/app'
import type { AppItem, AppListData, AppTaskDetailData, AppTaskItem } from '@/types/app'
import type { PythonReadiness } from '@/types/app'
import type { AppTaskAction, AppTaskStatus } from '@/types/enums'

let pollTimer: ReturnType<typeof setInterval> | null = null

export const useAppStore = defineStore('app', {
  state: () => ({
    ready: { installed: false, version: null, venvSupport: null, pipAvailable: null } as PythonReadiness,
    items: [] as AppItem[],
    generatedAt: '',
    loadError: null as string | null,
    tasks: [] as AppTaskItem[],
    tasksTotal: 0,
    taskFilter: { appId: undefined as number | undefined, action: undefined as AppTaskAction | undefined, status: undefined as AppTaskStatus | undefined },
    currentTask: null as AppTaskDetailData | null,
    pollTimerId: null as ReturnType<typeof setInterval> | null
  }),
  getters: {
    byId: (state) => (id: number) => state.items.find((i) => i.id === id) ?? null
  },
  actions: {
    /** loadList 全景成败流：成功覆盖投影；失败保留上一投影（I-04 env 先例）+错误态 */
    async loadList() {
      try {
        const data: AppListData = await api.appList()
        this.ready = data.pythonReadiness
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
