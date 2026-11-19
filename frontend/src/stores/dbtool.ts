/**
 * dbtoolStore —— db-tools 跨页面共享状态（PageSpec db-tools 第 1 节声明 Pinia：
 * 工具投影缓存/generatedAt/刷新、任务列表/当前任务详情、在途标记）。
 *
 * 共享根因：工具管理页与 database-mgmt 高级管理入口（PageSpec 6.8）同源消费工具全景
 * ——入口首次点击懒加载（ensureLoaded），手动刷新语义同工具管理页 4.1（D-02 实时探测，
 * 无列表轮询）。任务 Drawer 态：清单筛选 + 当前任务详情 running/pending 5s 轮询终态即停，
 * 终态跃迁触发工具全景与任务清单重拉（installed/installedVersion/running 态与在途标记
 * 落定——env-mgmt v1.2 先例）；onUnmounted 配对清理由页面承载。
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { dbtoolList, dbtoolTaskDetail, dbtoolTaskList } from '@/api/dbtool'
import type { DbToolItem, DbToolTaskDetail, DbToolTaskItem } from '@/types/dbtool'
import { isDbToolTaskInFlight } from '@/types/dbtool'
import type { DbToolTaskStatus, DbToolType } from '@/types/enums'

export const useDbtoolStore = defineStore('dbtool', () => {
  /** ---- 工具全景投影（D-02 实时探测，手动刷新+写后刷新） ---- */
  const items = ref<DbToolItem[]>([])
  const generatedAt = ref('')
  const listLoading = ref(false)
  const listError = ref(false)
  /** 首次加载完成标志（database-mgmt 高级入口懒加载判定，PageSpec 6.8 步骤 1） */
  const loaded = ref(false)

  let listInFlight = false
  async function loadList(): Promise<void> {
    if (listInFlight) return // 刷新防抖（env-mgmt/service-mgmt 先例）
    listInFlight = true
    listLoading.value = true
    listError.value = false
    try {
      const data = await dbtoolList()
      items.value = data.items
      generatedAt.value = data.generatedAt
      loaded.value = true
    } catch {
      listError.value = true // 如实降级：页面 NAlert+重试（loadError 规范）
    } finally {
      listLoading.value = false
      listInFlight = false
    }
  }

  /** 高级入口懒加载入口：首次点击确保全景已加载（PageSpec 6.8 步骤 1） */
  async function ensureLoaded(): Promise<void> {
    if (!loaded.value) await loadList()
  }

  /** 按 toolType 取条目（引擎映射后定位目标工具；未命中返回 null 不虚构） */
  function findByType(toolType: DbToolType): DbToolItem | null {
    return items.value.find((it) => it.toolType === toolType) ?? null
  }

  /** ---- 任务清单（Drawer 历史 Tab；进入拉取一次+手动刷新+写后刷新） ---- */
  const tasks = ref<DbToolTaskItem[]>([])
  const tasksTotal = ref(0)
  const tasksLoading = ref(false)
  const taskFilter = ref<{ toolType: DbToolType | null; status: DbToolTaskStatus | null }>({
    toolType: null,
    status: null
  })

  async function loadTasks(page = 1): Promise<void> {
    tasksLoading.value = true
    try {
      const data = await dbtoolTaskList({
        toolType: taskFilter.value.toolType ?? undefined,
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

  function setTaskFilter(toolType: DbToolType | null, status: DbToolTaskStatus | null): void {
    taskFilter.value = { toolType, status }
  }

  /** ---- 当前任务详情（running/pending 5s 轮询，终态即停） ---- */
  const currentTask = ref<DbToolTaskDetail | null>(null)
  const detailLoading = ref(false)
  let pollTimer: ReturnType<typeof setInterval> | null = null

  /** 详情到达统一入口：在途→终态跃迁时重拉工具全景与任务清单（PageSpec 4.3——
   * 任务完成后 installed/installedVersion/running 态与在途标记落定；env-mgmt 先例） */
  function onDetailArrived(d: DbToolTaskDetail): void {
    const prev = currentTask.value
    currentTask.value = d
    if (prev !== null && isDbToolTaskInFlight(prev.status) && !isDbToolTaskInFlight(d.status)) {
      void loadList()
      void loadTasks(1)
    }
    schedulePoll(d.status)
  }

  async function loadTaskDetail(taskId: number): Promise<void> {
    detailLoading.value = true
    try {
      const data = await dbtoolTaskDetail(taskId)
      onDetailArrived(data)
    } finally {
      detailLoading.value = false
    }
  }

  function schedulePoll(status: DbToolTaskStatus): void {
    stopPoll()
    if (isDbToolTaskInFlight(status)) {
      const id = currentTask.value?.id
      if (id == null) return
      pollTimer = setInterval(() => {
        const tid = currentTask.value?.id
        if (tid == null) {
          stopPoll()
          return
        }
        void dbtoolTaskDetail(tid)
          .then((d) => {
            onDetailArrived(d)
            if (!isDbToolTaskInFlight(d.status)) stopPoll()
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
    generatedAt,
    listLoading,
    listError,
    loaded,
    loadList,
    ensureLoaded,
    findByType,
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
