/**
 * dockerStore —— PageSpec 第 1 节显式声明的状态面：
 * daemonStatus / 五类清单（containerItems/composeItems/imageItems/volumeItems/networkItems
 * + generatedAt×5）/ listParams / inFlight / 拉取 Dialog 态 / logDrawer 态与 WS 订阅句柄。
 *
 * 刷新纪律（api D-01 按需拉取 + PageSpec 第 2 节纪律表）：
 * - 进入页面 daemonStatus 先行 → state=available 激活五 Tab 懒加载（首次切 Tab 拉取，
 *   切回不重复拉取除非已标记过期）；
 * - 容器六动作成功 → 重拉 containerList + composeList 标记过期；imagePull/imageDelete/
 *   volumeDelete/networkDelete 成功 → 重拉对应清单 + containerList 标记过期（保守仅 delete）；
 *   composeUp/Down/Restart 成功 → 重拉 composeList + containerList；
 * - Drawer 打开：containerStats + containerLogs（tail 缺省 500）按需拉取；「跟随」开启建立
 *   WS 订阅，关闭 Drawer 即断开（无残留订阅）；end 帧 → 停止跟随按 reason 提示、不自动重连；
 * - 无自动轮询（store 不持定时器，stats 为打开 Drawer 单次快照 D-07）；
 * - 页面卸载 resetView() 清理 Dialog/inFlight/过期标记/全部 WS 订阅。
 * DK_002（正常流程不可达，status 先行）→ 兜底合成 unavailable 降级引导态（不返回空清单——未装≠无容器，I-01）。
 */
import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { ApiError } from '@/api/http'
import {
  composeList as fetchComposeList,
  composeLogs as fetchComposeLogs,
  connectComposeLogsFollow,
  connectContainerLogsFollow,
  containerList as fetchContainerList,
  containerLogs as fetchContainerLogs,
  containerStats as fetchContainerStats,
  daemonStatus as fetchDaemonStatus,
  imageList as fetchImageList,
  networkList as fetchNetworkList,
  volumeList as fetchVolumeList
} from '@/api/docker'
import type { DockerLogSocket } from '@/api/docker'
import type { ContainerStatus, DockerLogEndReason } from '@/types/enums'
import type {
  DockerComposeItem,
  DockerContainerItem,
  DockerDaemonStatusData,
  DockerImageItem,
  DockerNetworkItem,
  DockerStatsData,
  DockerVolumeItem
} from '@/types/docker'

/** 五 Tab 标识（懒加载粒度） */
export type DockerTab = 'containers' | 'compose' | 'images' | 'volumes' | 'networks'

/** Tab 1 查询条件（PageSpec 4.1：参数变更即触发 containerList） */
export interface DockerListParams {
  /** container_status 多选，空=全部（逗号拼接 query state） */
  state: ContainerStatus[]
  /** Compose 项目名等值；空串=全部 */
  composeProject: string
}

/** 观测 Drawer 目标（容器详情 / Compose 日志，同一时刻至多一个） */
export type DockerLogTarget =
  | { kind: 'container'; containerId: string; name: string }
  | {
      kind: 'compose'
      project: string
      workingDir: string | null
      configFile: string | null
      locatable: boolean
    }

/** 跟随日志行数上限（防长跟随内存膨胀；超出丢最旧行——UI 呈现窗口，非持久化审计） */
const MAX_LOG_LINES = 5000

/** WS 订阅句柄（非响应式运行时资源，不入 Pinia state——terminal 先例） */
let followSocket: DockerLogSocket | null = null

export const useDockerStore = defineStore('docker', () => {
  /** ---- 页头 daemon 接管结论（4.0，恒为页面第一数据源） ---- */
  const daemon = ref<DockerDaemonStatusData | null>(null)
  const statusLoading = ref(false)
  /** 首拉网络级失败（拦截器已提示；页头呈重试入口） */
  const statusFailed = ref(false)
  /** 引导态判定（state=unavailable：整页降级，不呈现空清单，I-01/FL S-45-02） */
  const daemonUnavailable = computed(() => daemon.value?.state === 'unavailable')

  /** ---- Tab 1 容器 ---- */
  const containerItems = ref<DockerContainerItem[]>([])
  const containerTotal = ref(0)
  const containerGeneratedAt = ref<string | null>(null)
  const containerLoading = ref(false)
  const containerLoaded = ref(false)
  const listParams = ref<DockerListParams>({ state: [], composeProject: '' })

  /** ---- Tab 2 Compose ---- */
  const composeItems = ref<DockerComposeItem[]>([])
  const composeTotal = ref(0)
  const composeGeneratedAt = ref<string | null>(null)
  const composeLoading = ref(false)
  const composeLoaded = ref(false)
  /** 写操作成功后的过期标记（切回 Tab 时重拉） */
  const composeStale = ref(false)

  /** ---- Tab 3 镜像 ---- */
  const imageItems = ref<DockerImageItem[]>([])
  const imageTotal = ref(0)
  const imageGeneratedAt = ref<string | null>(null)
  const imageLoading = ref(false)
  const imageLoaded = ref(false)

  /** ---- Tab 4 卷 ---- */
  const volumeItems = ref<DockerVolumeItem[]>([])
  const volumeTotal = ref(0)
  const volumeGeneratedAt = ref<string | null>(null)
  const volumeLoading = ref(false)
  const volumeLoaded = ref(false)

  /** ---- Tab 5 网络 ---- */
  const networkItems = ref<DockerNetworkItem[]>([])
  const networkTotal = ref(0)
  const networkGeneratedAt = ref<string | null>(null)
  const networkLoading = ref(false)
  const networkLoaded = ref(false)

  /** 容器清单过期标记（资源 delete 成功的保守标记，PageSpec 第 2 节纪律） */
  const containerStale = ref(false)

  /** ---- 行级在途互斥（4.2/4.3：该行操作按钮 loading/禁用 + Compose 同项目互斥） ---- */
  const inFlightKeys = ref<ReadonlySet<string>>(new Set<string>())

  /** ---- 拉取镜像 Dialog（5.4，admin 专属） ---- */
  const pullDialogVisible = ref(false)
  /** 同步拉取在途（D-05 ≤600s；Dialog 按钮 loading + 「拉取中…」文案） */
  const pullInFlight = ref(false)

  /** ---- 观测 Drawer（5.1 容器详情 / 5.2 Compose 日志共用态面） ---- */
  const logTarget = ref<DockerLogTarget | null>(null)
  /** 日志区行（尾部读取 + 跟随追加共用缓冲） */
  const logLines = ref<string[]>([])
  /** tail 档位（200/500/1000/2000，缺省 500——1-2000 域的档位化，D-03） */
  const logTail = ref(500)
  const logLoading = ref(false)
  const logGeneratedAt = ref<string | null>(null)
  /** 跟随开关状态（WS 已建立） */
  const followActive = ref(false)
  /** end 帧终止原因（按 reason 分级提示，5.1/5.3；null=未收到） */
  const followEnded = ref<DockerLogEndReason | null>(null)
  /** 异常断流（握手拒绝/网络断开/协议违规——end 帧未达即关闭；无自动重连） */
  const followBroken = ref(false)
  /** 容器资源占用快照（D-07 单次读取） */
  const stats = ref<DockerStatsData | null>(null)
  const statsLoading = ref(false)

  /** DK_002 兜底：合成 unavailable 结论降级引导态（正常流程不可达——status 先行判定） */
  function markDaemonUnavailable(): void {
    daemon.value = {
      state: 'unavailable',
      version: null,
      detail: null,
      generatedAt: new Date().toISOString()
    }
    containerItems.value = []
    containerTotal.value = 0
    containerGeneratedAt.value = null
    composeItems.value = []
    composeTotal.value = 0
    composeGeneratedAt.value = null
    imageItems.value = []
    imageTotal.value = 0
    imageGeneratedAt.value = null
    volumeItems.value = []
    volumeTotal.value = 0
    volumeGeneratedAt.value = null
    networkItems.value = []
    networkTotal.value = 0
    networkGeneratedAt.value = null
  }

  async function loadStatus(): Promise<void> {
    if (statusLoading.value) return // 刷新按钮 loading 防抖
    statusLoading.value = true
    statusFailed.value = false
    try {
      daemon.value = await fetchDaemonStatus()
    } catch {
      // 网络/会话级错误已由 http 拦截器统一提示；首拉失败时页头呈重试入口
      if (daemon.value === null) statusFailed.value = true
    } finally {
      statusLoading.value = false
    }
  }

  /** daemon 可用性守卫（status 先行，PageSpec 第 2 节纪律；引导态不触清单） */
  function listAllowed(): boolean {
    return daemon.value !== null && daemon.value.state === 'available'
  }

  /** DK_002 通用分流：业务面前置复核失败 → 降级引导态（其余错误交调用方/拦截器） */
  function isDaemonUnavailableError(e: unknown): boolean {
    return e instanceof ApiError && e.is('DK_002')
  }

  async function loadContainers(): Promise<void> {
    if (!listAllowed() || containerLoading.value) return
    containerLoading.value = true
    try {
      const data = await fetchContainerList({
        state:
          listParams.value.state.length > 0 ? listParams.value.state.join(',') : undefined,
        composeProject:
          listParams.value.composeProject !== '' ? listParams.value.composeProject : undefined
      })
      containerItems.value = data.items
      containerTotal.value = data.total
      containerGeneratedAt.value = data.generatedAt
      containerLoaded.value = true
      containerStale.value = false
    } catch (e) {
      if (isDaemonUnavailableError(e)) markDaemonUnavailable()
      // DK_001（筛选值非法）正常 UI 下拉不可达（PageSpec 第 1 节）；其余由拦截器提示
    } finally {
      containerLoading.value = false
    }
  }

  async function loadCompose(): Promise<void> {
    if (!listAllowed() || composeLoading.value) return
    composeLoading.value = true
    try {
      const data = await fetchComposeList()
      composeItems.value = data.items
      composeTotal.value = data.total
      composeGeneratedAt.value = data.generatedAt
      composeLoaded.value = true
      composeStale.value = false
    } catch (e) {
      if (isDaemonUnavailableError(e)) markDaemonUnavailable()
    } finally {
      composeLoading.value = false
    }
  }

  async function loadImages(): Promise<void> {
    if (!listAllowed() || imageLoading.value) return
    imageLoading.value = true
    try {
      const data = await fetchImageList()
      imageItems.value = data.items
      imageTotal.value = data.total
      imageGeneratedAt.value = data.generatedAt
      imageLoaded.value = true
    } catch (e) {
      if (isDaemonUnavailableError(e)) markDaemonUnavailable()
    } finally {
      imageLoading.value = false
    }
  }

  async function loadVolumes(): Promise<void> {
    if (!listAllowed() || volumeLoading.value) return
    volumeLoading.value = true
    try {
      const data = await fetchVolumeList()
      volumeItems.value = data.items
      volumeTotal.value = data.total
      volumeGeneratedAt.value = data.generatedAt
      volumeLoaded.value = true
    } catch (e) {
      if (isDaemonUnavailableError(e)) markDaemonUnavailable()
    } finally {
      volumeLoading.value = false
    }
  }

  async function loadNetworks(): Promise<void> {
    if (!listAllowed() || networkLoading.value) return
    networkLoading.value = true
    try {
      const data = await fetchNetworkList()
      networkItems.value = data.items
      networkTotal.value = data.total
      networkGeneratedAt.value = data.generatedAt
      networkLoaded.value = true
    } catch (e) {
      if (isDaemonUnavailableError(e)) markDaemonUnavailable()
    } finally {
      networkLoading.value = false
    }
  }

  /** Tab 懒加载（首次进入拉取；切回仅在已标记过期时重拉，PageSpec 第 2 节纪律） */
  function ensureTabLoaded(tab: DockerTab): void {
    if (!listAllowed()) return
    switch (tab) {
      case 'containers':
        if (!containerLoaded.value || containerStale.value) void loadContainers()
        break
      case 'compose':
        if (!composeLoaded.value || composeStale.value) void loadCompose()
        break
      case 'images':
        if (!imageLoaded.value) void loadImages()
        break
      case 'volumes':
        if (!volumeLoaded.value) void loadVolumes()
        break
      case 'networks':
        if (!networkLoaded.value) void loadNetworks()
        break
    }
  }

  /** 当前 Tab 清单刷新（页头/Tab 内刷新按钮共用） */
  function reloadTab(tab: DockerTab): void {
    switch (tab) {
      case 'containers':
        void loadContainers()
        break
      case 'compose':
        void loadCompose()
        break
      case 'images':
        void loadImages()
        break
      case 'volumes':
        void loadVolumes()
        break
      case 'networks':
        void loadNetworks()
        break
    }
  }

  function setStateFilter(states: ContainerStatus[]): void {
    listParams.value.state = [...states]
    void loadContainers()
  }

  function setComposeProjectFilter(project: string): void {
    listParams.value.composeProject = project
    void loadContainers()
  }

  /** 写后关联清单标记（PageSpec 第 2 节纪律） */
  function markComposeStale(): void {
    composeStale.value = true
  }

  function markContainersStale(): void {
    containerStale.value = true
  }

  /** ---- 在途互斥（键形如 `container:{id}` / `compose:{project}` / `image:{id}` …） ---- */

  function setInFlight(key: string): void {
    const next = new Set(inFlightKeys.value)
    next.add(key)
    inFlightKeys.value = next
  }

  function releaseInFlight(key: string): void {
    const next = new Set(inFlightKeys.value)
    next.delete(key)
    inFlightKeys.value = next
  }

  function isInFlight(key: string): boolean {
    return inFlightKeys.value.has(key)
  }

  /** ---- 拉取镜像 Dialog ---- */

  function openPullDialog(): void {
    pullDialogVisible.value = true
  }

  function closePullDialog(): void {
    pullDialogVisible.value = false
  }

  /** ---- 观测 Drawer（stats/logs/follow，5.1/5.2） ---- */

  function stopFollow(): void {
    followSocket?.close()
    followSocket = null
    followActive.value = false
  }

  /** 打开容器详情 Drawer：复位观测态（stats/logs 由 Drawer 组件按需拉取并按码分流） */
  function openContainerDrawer(item: DockerContainerItem): void {
    stopFollow()
    logTarget.value = { kind: 'container', containerId: item.containerId, name: item.name }
    logLines.value = []
    logTail.value = 500
    followEnded.value = null
    followBroken.value = false
    stats.value = null
  }

  /** 打开 Compose 日志 Drawer：复位观测态（logs 由 Drawer 组件按需拉取；locatable=false 不触日志端点，DK_015 同判） */
  function openComposeDrawer(item: DockerComposeItem): void {
    stopFollow()
    logTarget.value = {
      kind: 'compose',
      project: item.project,
      workingDir: item.workingDir,
      configFile: item.configFile,
      locatable: item.locatable
    }
    logLines.value = []
    logTail.value = 500
    followEnded.value = null
    followBroken.value = false
  }

  /** 关闭 Drawer：即断 WS（无残留订阅，PageSpec 5.1） */
  function closeLogDrawer(): void {
    stopFollow()
    logTarget.value = null
    logLines.value = []
    logGeneratedAt.value = null
    followEnded.value = null
    followBroken.value = false
    stats.value = null
  }

  /** 资源占用快照（D-07 单次；非 DK_002 错误向调用方抛出按码分流） */
  async function loadStats(): Promise<void> {
    const target = logTarget.value
    if (target === null || target.kind !== 'container') return
    statsLoading.value = true
    try {
      stats.value = await fetchContainerStats(target.containerId)
    } catch (e) {
      if (isDaemonUnavailableError(e)) {
        markDaemonUnavailable()
        return
      }
      throw e
    } finally {
      statsLoading.value = false
    }
  }

  /** 日志尾部读取（tail 档位变更/Drawer 打开/手动刷新共用；非 DK_002 错误向调用方抛出） */
  async function loadLogs(): Promise<void> {
    const target = logTarget.value
    if (target === null) return
    if (target.kind === 'compose' && !target.locatable) return
    logLoading.value = true
    try {
      const data =
        target.kind === 'container'
          ? await fetchContainerLogs(target.containerId, { tail: logTail.value })
          : await fetchComposeLogs(target.project, { tail: logTail.value })
      logLines.value = data.lines.slice(-MAX_LOG_LINES)
      logGeneratedAt.value = data.generatedAt
    } catch (e) {
      if (isDaemonUnavailableError(e)) {
        markDaemonUnavailable()
        return
      }
      throw e
    } finally {
      logLoading.value = false
    }
  }

  /** tail 档位变更：以新档位重拉尾部（PageSpec 5.1 行数控件） */
  function setLogTail(tail: number): void {
    logTail.value = tail
    void loadLogs().catch(() => {})
  }

  function appendLine(line: string): void {
    logLines.value.push(line)
    if (logLines.value.length > MAX_LOG_LINES) {
      logLines.value = logLines.value.slice(-MAX_LOG_LINES)
    }
  }

  /** 开启跟随（重开=重试入口，PageSpec 第 2 节纪律；end 后不自动重连） */
  function startFollow(): void {
    const target = logTarget.value
    if (target === null || followSocket !== null) return
    if (target.kind === 'compose' && !target.locatable) return
    followEnded.value = null
    followBroken.value = false
    // D-03：握手 query 携 tail，服务端先回放尾部 tail 行再流式推送——
    // 清空 GET 尾部缓冲由 WS 回放重建，避免同段日志双份
    logLines.value = []
    logGeneratedAt.value = null
    const events = {
      onLog(line: string): void {
        appendLine(line)
      },
      onEnd(reason: DockerLogEndReason): void {
        followActive.value = false
        followSocket = null
        followEnded.value = reason
      },
      onInvalidFrame(): void {
        // 协议违规（D-03 帧收窄失败）：本地收口断开，按异常断流提示
        followActive.value = false
        followSocket?.close()
        followSocket = null
        followBroken.value = true
      },
      onClose(info: { endReceived: boolean; code: number }): void {
        if (info.endReceived) return // end 帧已处理
        followActive.value = false
        followSocket = null
        followBroken.value = true // 握手拒绝/网络断开（浏览器不区分；无自动重连）
      }
    }
    followActive.value = true
    followSocket =
      target.kind === 'container'
        ? connectContainerLogsFollow(
            { containerId: target.containerId, tail: logTail.value },
            events
          )
        : connectComposeLogsFollow({ project: target.project, tail: logTail.value }, events)
  }

  /** 关闭跟随（开关关闭=用户主动断开，非异常） */
  function stopFollowByUser(): void {
    stopFollow()
    followBroken.value = false
  }

  /** 页面卸载：清理 Dialog/inFlight/过期标记/全部 WS 订阅（PageSpec 第 2 节纪律末行） */
  function resetView(): void {
    closeLogDrawer()
    pullDialogVisible.value = false
    pullInFlight.value = false
    inFlightKeys.value = new Set<string>()
    composeStale.value = false
    containerStale.value = false
  }

  return {
    daemon,
    statusLoading,
    statusFailed,
    daemonUnavailable,
    containerItems,
    containerTotal,
    containerGeneratedAt,
    containerLoading,
    containerLoaded,
    containerStale,
    listParams,
    composeItems,
    composeTotal,
    composeGeneratedAt,
    composeLoading,
    composeLoaded,
    composeStale,
    imageItems,
    imageTotal,
    imageGeneratedAt,
    imageLoading,
    imageLoaded,
    volumeItems,
    volumeTotal,
    volumeGeneratedAt,
    volumeLoading,
    volumeLoaded,
    networkItems,
    networkTotal,
    networkGeneratedAt,
    networkLoading,
    networkLoaded,
    inFlightKeys,
    pullDialogVisible,
    pullInFlight,
    logTarget,
    logLines,
    logTail,
    logLoading,
    logGeneratedAt,
    followActive,
    followEnded,
    followBroken,
    stats,
    statsLoading,
    markDaemonUnavailable,
    loadStatus,
    loadContainers,
    loadCompose,
    loadImages,
    loadVolumes,
    loadNetworks,
    ensureTabLoaded,
    reloadTab,
    setStateFilter,
    setComposeProjectFilter,
    markComposeStale,
    markContainersStale,
    setInFlight,
    releaseInFlight,
    isInFlight,
    openPullDialog,
    closePullDialog,
    openContainerDrawer,
    openComposeDrawer,
    closeLogDrawer,
    loadStats,
    loadLogs,
    setLogTail,
    startFollow,
    stopFollowByUser,
    resetView
  }
})
