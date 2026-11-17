/**
 * envStore —— 环境管理状态（PageSpec env-mgmt v1.0 第 1 节）：
 * 环境全景投影缓存（loadList 成败流/generatedAt/通道状态；D-01 无列表轮询——手动
 * 刷新+写后刷新）与任务 Drawer 态（清单筛选 + 当前任务详情 running 5s 轮询终态即停，
 * 终态跃迁触发环境全景与任务清单重拉——v0.1.4 反馈第 1 项 refresh 语义；
 * cron-backup 详情 Drawer 先例；onUnmounted 配对清理由页面承载）。
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { envList, envTaskDetail, envTaskList } from '@/api/env'
import type { EnvListData, EnvTaskDetail, EnvTaskItem } from '@/types/env'
import type { EnvTaskStatus, EnvType } from '@/types/enums'

export const useEnvStore = defineStore('env', () => {
  /** 环境全景投影 */
  const items = ref<EnvListData['items']>([])
  const channel = ref<EnvListData['channel']>({ family: null, available: false })
  const generatedAt = ref('')
  const listLoading = ref(false)
  const listError = ref(false)

  let listInFlight = false
  async function loadList(): Promise<void> {
    if (listInFlight) return // 刷新防抖（service-mgmt 先例）
    listInFlight = true
    listLoading.value = true
    listError.value = false
    try {
      const data = await envList()
      items.value = data.items
      channel.value = data.channel
      generatedAt.value = data.generatedAt
    } catch {
      listError.value = true // 如实降级：页面 NAlert+重试（loadError 规范）
    } finally {
      listLoading.value = false
      listInFlight = false
    }
  }

  /** 任务清单（Drawer 历史 Tab） */
  const tasks = ref<EnvTaskItem[]>([])
  const tasksTotal = ref(0)
  const tasksLoading = ref(false)
  const taskFilter = ref<{ envType: EnvType | null; status: EnvTaskStatus | null }>({
    envType: null,
    status: null
  })

  async function loadTasks(page = 1): Promise<void> {
    tasksLoading.value = true
    try {
      const data = await envTaskList({
        envType: taskFilter.value.envType ?? undefined,
        status: taskFilter.value.status ?? undefined,
        page,
        pageSize: 50
      })
      tasks.value = data.items
      tasksTotal.value = data.total
    } finally {
      tasksLoading.value = false
    }
  }

  function setTaskFilter(envType: EnvType | null, status: EnvTaskStatus | null): void {
    taskFilter.value = { envType, status }
  }

  /** 当前任务详情（running/pending 5s 轮询，终态即停） */
  const currentTask = ref<EnvTaskDetail | null>(null)
  const detailLoading = ref(false)
  let pollTimer: ReturnType<typeof setInterval> | null = null

  /** 在途态判定（pending/running=轮询继续；succeeded/failed=终态） */
  function isInFlight(s: EnvTaskStatus): boolean {
    return s === 'pending' || s === 'running'
  }

  /** 详情到达统一入口：在途→终态跃迁时重拉环境全景与任务清单
   *（v0.1.4 用户反馈第 1 项：任务完成后 installed/version/service 态与
   * 在途标记需落定——旧实现终态即停不刷新，用户看到旧环境状态；
   * 语义=「终态触发重拉」，保持 5s 轮询纪律，无 SSE/WebSocket） */
  function onDetailArrived(d: EnvTaskDetail): void {
    const prev = currentTask.value
    currentTask.value = d
    if (prev !== null && isInFlight(prev.status) && !isInFlight(d.status)) {
      void loadList()
      void loadTasks(1)
    }
    schedulePoll(d.status)
  }

  async function loadTaskDetail(taskId: number): Promise<void> {
    detailLoading.value = true
    try {
      const data = await envTaskDetail(taskId)
      onDetailArrived(data)
    } finally {
      detailLoading.value = false
    }
  }

  function schedulePoll(status: EnvTaskStatus): void {
    stopPoll()
    if (isInFlight(status)) {
      const id = currentTask.value?.id
      if (id == null) return
      pollTimer = setInterval(() => {
        const tid = currentTask.value?.id
        if (tid == null) {
          stopPoll()
          return
        }
        void envTaskDetail(tid)
          .then((d) => {
            onDetailArrived(d)
            if (!isInFlight(d.status)) stopPoll()
          })
          .catch(() => stopPoll()) // 轮询失败即停（手动刷新兜底）
      }, 5000)
    }
  }

  function stopPoll(): void {
    if (pollTimer !== null) {
      clearInterval(pollTimer)
      pollTimer = null
    }
  }

  function clearTaskDetail(): void {
    stopPoll()
    currentTask.value = null
  }

  /** 写后刷新组合（任务创建后：清单+全景在途标记） */
  async function refreshAfterWrite(): Promise<void> {
    await Promise.all([loadList(), loadTasks(1)])
  }

  return {
    items,
    channel,
    generatedAt,
    listLoading,
    listError,
    loadList,
    tasks,
    tasksTotal,
    tasksLoading,
    taskFilter,
    loadTasks,
    setTaskFilter,
    currentTask,
    detailLoading,
    loadTaskDetail,
    clearTaskDetail,
    stopPoll,
    refreshAfterWrite
  }
})
