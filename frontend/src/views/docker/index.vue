<script setup lang="ts">
/**
 * 容器管理页（PageSpec 第 4 节，路由 /docker；hybrid·五 Tab 布局）。
 * 结构：页头 daemon 接管状态 Card（4.0，恒为第一区块，含刷新）→ state=unavailable 整页
 * 引导态 NEmpty（不渲染五 Tab 与任何清单内容，不呈现空容器列表——未装≠无容器，I-01/FL S-45-02）
 * | 五 Tab（4.1 容器 / 4.3 Compose / 4.4 镜像 / 4.5 卷 / 4.6 网络，懒加载）+ 容器详情 Drawer
 * + Compose 日志 Drawer + 拉取镜像 Dialog。
 * readonly 子集（ADR-018 负空间 13 项）：容器/Compose/镜像/卷/网络操作列整列不渲染、
 * 无「拉取镜像」按钮；筛选/刷新/页头/引导态/观测 Drawer（stats+日志+WS 跟随）全保留（全可看）。
 * 刷新纪律（D-01 按需拉取）：status 先行 / Tab 懒加载 / 容器动作→重拉容器+compose 过期 /
 * 资源删除→重拉本清单+容器过期 / compose 动作→重拉 compose+容器 / 无轮询无定时器。
 * 操作列权威来源=服务端 supportedActions（4.2，前端不自行推断）；Compose locatable 驱动禁用（4.3）。
 */
import { computed, h, onMounted, onUnmounted, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { NButton, NPopconfirm, NTag, NTooltip } from 'naive-ui'
import type { DataTableColumns, SelectOption } from 'naive-ui'
import type { VNode } from 'vue'
import { ApiError } from '@/api/http'
import {
  composeDown,
  composeRestart,
  composeUp,
  containerDelete,
  containerPause,
  containerRestart,
  containerStart,
  containerStop,
  containerUnpause,
  imageDelete,
  imagePull,
  networkDelete,
  volumeDelete
} from '@/api/docker'
import { useAuthStore } from '@/stores/auth'
import { useDockerStore } from '@/stores/docker'
import type { DockerTab } from '@/stores/docker'
import { CONTAINER_STATUS, DOCKER_DAEMON_STATE, dictItem } from '@/types/enums'
import type { ContainerAction, ContainerStatus, DictItem } from '@/types/enums'
import {
  COMPOSE_ACTION_TEXT,
  CONTAINER_ACTION_TEXT,
  DK_ERROR_TEXT,
  daemonDetailText,
  dockerErrorReason,
  isActionMismatchInfo,
  isComposeLocateFailedInfo
} from '@/types/docker'
import type {
  DockerComposeItem,
  DockerContainerItem,
  DockerImageItem,
  DockerNetworkItem,
  DockerVolumeItem
} from '@/types/docker'
import { formatBytes, formatDateTime } from '@/utils/format'
import ComposeLogsDrawer from './components/ComposeLogsDrawer.vue'
import ContainerDetailDrawer from './components/ContainerDetailDrawer.vue'
import PullImageModal from './components/PullImageModal.vue'

const store = useDockerStore()
const authStore = useAuthStore()
const message = useMessage()

/** ---- Tab 框架（懒加载：首次切 Tab 拉取对应清单，切回不重复拉取除非已标记过期） ---- */

const activeTab = ref<DockerTab>('containers')

watch(activeTab, (tab) => store.ensureTabLoaded(tab))

/** ---- 页头（4.0 daemon 接管状态 Card） ---- */

const daemonDict = computed<DictItem | null>(() =>
  store.daemon === null ? null : dictItem(DOCKER_DAEMON_STATE, store.daemon.state)
)
/** available 时「Docker {version}」；version=null →「—」不虚构 */
const versionText = computed(() => {
  const version = store.daemon?.version ?? null
  return version !== null ? `Docker ${version}` : '—'
})
const unavailableDetail = computed(() =>
  store.daemon?.state === 'unavailable' ? daemonDetailText(store.daemon.detail) : ''
)
const statusGeneratedAtText = computed(() => formatDateTime(store.daemon?.generatedAt ?? null))
/** 引导态文案（4.0：detail 区分未装/不可达；合成分辨时无 detail 走通用文案） */
const guideText = computed(() =>
  unavailableDetail.value !== ''
    ? `未检测到可用的 Docker daemon（${unavailableDetail.value}）。请安装 Docker 后刷新重试。`
    : '未检测到可用的 Docker daemon。请安装 Docker 后刷新重试。'
)

/** 页头刷新：重拉 daemonStatus + 当前 Tab 清单（loading 防抖在 store；引导态保留刷新=安装后重试入口） */
async function refreshAll(): Promise<void> {
  await store.loadStatus()
  if (store.daemon !== null && !store.daemonUnavailable) store.reloadTab(activeTab.value)
}

/** ---- Tab 1 容器工具条（4.1 参数变更即触发 containerList） ---- */

const stateOptions: SelectOption[] = (Object.keys(CONTAINER_STATUS) as ContainerStatus[]).map(
  (v) => ({ label: CONTAINER_STATUS[v].label, value: v })
)

const stateFilter = ref<ContainerStatus[]>([...store.listParams.state])
const composeProjectFilter = ref(store.listParams.composeProject)

watch(stateFilter, (v) => store.setStateFilter(v))

/** 文本筛选 300ms 去抖（精确匹配逐键请求的窗口合并；定时器 onUnmounted 配对清理） */
let composeProjectTimer: ReturnType<typeof setTimeout> | null = null

watch(composeProjectFilter, (v) => {
  if (composeProjectTimer !== null) clearTimeout(composeProjectTimer)
  composeProjectTimer = setTimeout(() => {
    composeProjectTimer = null
    store.setComposeProjectFilter(v)
  }, 300)
})

const containerGeneratedAtText = computed(() => formatDateTime(store.containerGeneratedAt))
const composeGeneratedAtText = computed(() => formatDateTime(store.composeGeneratedAt))
const imageGeneratedAtText = computed(() => formatDateTime(store.imageGeneratedAt))
const volumeGeneratedAtText = computed(() => formatDateTime(store.volumeGeneratedAt))
const networkGeneratedAtText = computed(() => formatDateTime(store.networkGeneratedAt))

/** ---- 容器六动作（4.2 supportedActions 权威渲染 + 在途互斥 + DK_003/004/017 分流） ---- */

const CONTAINER_ACTION_API: Readonly<
  Record<ContainerAction, (req: { containerId: string }) => Promise<unknown>>
> = {
  start: containerStart,
  stop: containerStop,
  restart: containerRestart,
  pause: containerPause,
  unpause: containerUnpause,
  delete: containerDelete
}

function containerActionLabel(action: string): string {
  const text = (CONTAINER_ACTION_TEXT as Readonly<Record<string, string>>)[action]
  return text ?? action
}

async function execContainerAction(row: DockerContainerItem, action: ContainerAction): Promise<void> {
  const key = `container:${row.containerId}`
  store.setInFlight(key) // 该行全部操作按钮 loading/禁用（防重复提交，PageSpec 4.2）
  try {
    await CONTAINER_ACTION_API[action]({ containerId: row.containerId })
    message.success(`${containerActionLabel(action)}指令已执行`)
    await store.loadContainers()
    store.markComposeStale() // compose 项目容器实况随容器操作变化（第 2 节纪律）
  } catch (e) {
    handleContainerActionError(e)
  } finally {
    store.releaseInFlight(key)
  }
}

function handleContainerActionError(e: unknown): void {
  if (!(e instanceof ApiError)) return
  if (e.is('DK_003')) {
    message.warning(DK_ERROR_TEXT['DK_003'])
    void store.loadContainers() // 实况与权威面同步（第 1 节）
    return
  }
  if (e.is('DK_004')) {
    // data={currentState, action, allowedActions}——服务端权威动作面（I-04/D-04）
    if (isActionMismatchInfo(e.data)) {
      const allowed = e.data.allowedActions.map(containerActionLabel).join('、')
      message.warning(`容器当前状态（${e.data.currentState}）不允许该操作，可用：${allowed}`)
    } else {
      message.error(DK_ERROR_TEXT['DK_004'])
    }
    void store.loadContainers()
    return
  }
  handleCommonWriteError(e)
}

/** 写动作公共分流：DK_002 兜底引导态 / DK_017 error+reason / DK_001 参数兜底 / 其余码文案 */
function handleCommonWriteError(e: ApiError): void {
  if (e.is('DK_002')) {
    store.markDaemonUnavailable()
    return
  }
  if (e.is('DK_017')) {
    message.error(`${DK_ERROR_TEXT['DK_017']}：${dockerErrorReason(e.data) ?? '未知原因'}`)
    return
  }
  const text = DK_ERROR_TEXT[e.code]
  if (text !== undefined) message.error(text) // DK_001 兜底（正常 UI 不可达，R-14）
}

/** ---- Compose 三动作（4.3 locatable 驱动 + 项目级在途互斥 + DK_014/015/016/018 分流） ---- */

type ComposeVerb = 'up' | 'down' | 'restart'

const COMPOSE_ACTION_API: Readonly<
  Record<ComposeVerb, (req: { project: string }) => Promise<unknown>>
> = {
  up: composeUp,
  down: composeDown,
  restart: composeRestart
}

async function execComposeAction(row: DockerComposeItem, verb: ComposeVerb): Promise<void> {
  const key = `compose:${row.project}`
  store.setInFlight(key) // 同项目按钮互斥禁用（DK_018 服务端兜底，前端同项目互斥）
  try {
    await COMPOSE_ACTION_API[verb]({ project: row.project })
    message.success(`${COMPOSE_ACTION_TEXT[verb]} 指令已执行`)
    await store.loadCompose()
    await store.loadContainers() // 项目容器集合与状态已变（第 2 节纪律）
  } catch (e) {
    handleComposeActionError(e)
  } finally {
    store.releaseInFlight(key)
  }
}

function handleComposeActionError(e: unknown): void {
  if (!(e instanceof ApiError)) return
  if (e.is('DK_014')) {
    message.warning(DK_ERROR_TEXT['DK_014'])
    void store.loadCompose()
    return
  }
  if (e.is('DK_015')) {
    // data={workingDir, configFile, missing}——呈现 label 记录路径 + 容器级兜底（第 1 节）
    if (isComposeLocateFailedInfo(e.data)) {
      message.warning(
        `${DK_ERROR_TEXT['DK_015']}（工作目录：${e.data.workingDir ?? '未知'}，配置文件：${e.data.configFile ?? '未知'}）——可经容器页逐个操作兜底`
      )
    } else {
      message.warning(`${DK_ERROR_TEXT['DK_015']}——可经容器页逐个操作兜底`)
    }
    return
  }
  if (e.is('DK_016')) {
    message.error(`${DK_ERROR_TEXT['DK_016']}：${dockerErrorReason(e.data) ?? '未知原因'}`)
    return
  }
  if (e.is('DK_018')) {
    message.warning(DK_ERROR_TEXT['DK_018'])
    return
  }
  handleCommonWriteError(e)
}

/** ---- 镜像/卷/网络删除（4.4/4.5/4.6：daemon 拒绝为最终权威 DK_006/008/010/011，D-08/I-05） ---- */

async function execImageDelete(row: DockerImageItem): Promise<void> {
  const key = `image:${row.imageId}`
  store.setInFlight(key)
  try {
    await imageDelete({ imageId: row.imageId })
    message.success('镜像已删除')
    await store.loadImages()
    store.markContainersStale()
  } catch (e) {
    handleResourceDeleteError(e, 'DK_005', 'DK_006', () => void store.loadImages())
  } finally {
    store.releaseInFlight(key)
  }
}

async function execVolumeDelete(row: DockerVolumeItem): Promise<void> {
  const key = `volume:${row.volumeName}`
  store.setInFlight(key)
  try {
    await volumeDelete({ volumeName: row.volumeName })
    message.success('卷已删除')
    await store.loadVolumes()
    store.markContainersStale()
  } catch (e) {
    handleResourceDeleteError(e, 'DK_007', 'DK_008', () => void store.loadVolumes())
  } finally {
    store.releaseInFlight(key)
  }
}

async function execNetworkDelete(row: DockerNetworkItem): Promise<void> {
  const key = `network:${row.networkId}`
  store.setInFlight(key)
  try {
    await networkDelete({ networkId: row.networkId })
    message.success('网络已删除')
    await store.loadNetworks()
    store.markContainersStale()
  } catch (e) {
    handleResourceDeleteError(e, 'DK_009', 'DK_010', () => void store.loadNetworks(), 'DK_011')
  } finally {
    store.releaseInFlight(key)
  }
}

/** 资源删除公共分流：notFound→warning+重拉清单；inUse→warning+data.reason；predefined（网络）→warning */
function handleResourceDeleteError(
  e: unknown,
  notFoundCode: string,
  inUseCode: string,
  reload: () => void,
  predefinedCode?: string
): void {
  if (!(e instanceof ApiError)) return
  if (e.is(notFoundCode)) {
    message.warning(DK_ERROR_TEXT[notFoundCode])
    reload()
    return
  }
  if (e.is(inUseCode)) {
    const reason = dockerErrorReason(e.data)
    message.warning(reason !== null ? `${DK_ERROR_TEXT[inUseCode]}：${reason}` : DK_ERROR_TEXT[inUseCode])
    return
  }
  if (predefinedCode !== undefined && e.is(predefinedCode)) {
    message.warning(DK_ERROR_TEXT[predefinedCode])
    return
  }
  handleCommonWriteError(e)
}

/** ---- 拉取镜像 Dialog 编排（5.4：DK_012 字段错误 / DK_013 Dialog 保持打开） ---- */

const pullServerError = ref<string | null>(null)

async function onPullSubmit(ref: string): Promise<void> {
  pullServerError.value = null
  store.pullInFlight = true
  try {
    await imagePull({ ref })
    message.success(`镜像已拉取：${ref}`)
    store.closePullDialog()
    await store.loadImages()
    store.markContainersStale()
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.is('DK_013')) {
        // 拉取失败：error + data.reason（Dialog 保持打开供修正重试，PageSpec 5.4）
        message.error(`${DK_ERROR_TEXT['DK_013']}：${dockerErrorReason(e.data) ?? '未知原因'}`)
        return
      }
      if (e.is('DK_012')) {
        pullServerError.value = DK_ERROR_TEXT['DK_012']
        return
      }
      if (e.is('DK_002')) {
        store.markDaemonUnavailable()
        store.closePullDialog()
        return
      }
      handleCommonWriteError(e)
    }
  } finally {
    store.pullInFlight = false
  }
}

/** ---- 表格列（4.1/4.3/4.4/4.5/4.6；readonly 操作列整列不渲染——负空间） ---- */

function dictTag(map: Readonly<Record<string, DictItem>>, value: string): VNode {
  const item = dictItem(map, value)
  return h(NTag, { type: item.tag, size: 'small', bordered: false }, { default: () => item.label })
}

function withTooltip(node: VNode, text: string): VNode {
  return h(NTooltip, null, { trigger: () => node, default: () => text })
}

/** 容器操作列：supportedActions 服务端权威面驱动（4.2——六按钮照此渲染，前端不自行推断） */
function renderContainerActions(row: DockerContainerItem): VNode {
  const inFlight = store.isInFlight(`container:${row.containerId}`)
  const buttons: VNode[] = []
  const actionDefs: ReadonlyArray<{ action: ContainerAction; confirm: string | null; danger?: boolean }> = [
    { action: 'start', confirm: null },
    { action: 'stop', confirm: `确认停止容器 ${row.name}？` },
    { action: 'restart', confirm: `确认重启容器 ${row.name}？` },
    { action: 'pause', confirm: `确认暂停容器 ${row.name}？暂停后进程冻结、资源保留。` },
    { action: 'unpause', confirm: null },
    {
      action: 'delete',
      // 加强文案（含镜像信息；运行中按钮不渲染即不可达——先停止路径自然引导）
      confirm: `确认删除容器 ${row.name}（镜像 ${row.image}）？该操作不可恢复。`,
      danger: true
    }
  ]
  for (const def of actionDefs) {
    if (!row.supportedActions.includes(def.action)) continue
    const btn = h(
      NButton,
      {
        size: 'small',
        type: def.danger === true ? 'error' : 'default',
        ghost: def.danger === true,
        loading: inFlight,
        disabled: inFlight,
        onClick: () => {
          void execContainerAction(row, def.action)
        }
      },
      { default: () => CONTAINER_ACTION_TEXT[def.action] }
    )
    buttons.push(
      def.confirm === null
        ? btn
        : h(
            NPopconfirm,
            {
              onPositiveClick: () => {
                void execContainerAction(row, def.action)
              }
            },
            { trigger: () => btn, default: () => def.confirm }
          )
    )
  }
  // 过渡态 supportedActions=[] → 仅灰字「执行中」（4.2）
  if (buttons.length === 0) return h('span', { class: 'muted' }, '执行中')
  return h('div', { class: 'row-actions' }, buttons)
}

const containerColumns = computed<DataTableColumns<DockerContainerItem>>(() => {
  const base: DataTableColumns<DockerContainerItem> = [
    { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
    {
      title: '名称',
      key: 'name',
      width: 200,
      render: (row) =>
        // 详情跳转经名称列 link（分册 3.1；观测 Drawer 按需拉取）
        h(
          NButton,
          { text: true, type: 'primary', onClick: () => store.openContainerDrawer(row) },
          { default: () => row.name }
        )
    },
    { title: '镜像', key: 'image', width: 220, render: (row) => h('span', { class: 'mono' }, row.image) },
    {
      title: '状态',
      key: 'state',
      width: 110,
      render: (row) => {
        if ((CONTAINER_STATUS as Readonly<Record<string, DictItem>>)[row.state] !== undefined) {
          return dictTag(CONTAINER_STATUS, row.state)
        }
        // 过渡态透传原值 Tag blue + Tooltip（4.1 颜色方案；操作入口由 supportedActions=[] 禁用）
        return withTooltip(
          h(NTag, { type: 'info', size: 'small', bordered: false }, { default: () => row.state }),
          '操作进行中，完成后自动回落'
        )
      }
    },
    {
      title: 'Compose 项目',
      key: 'composeProject',
      width: 160,
      render: (row) =>
        row.composeProject === null
          ? withTooltip(h('span', { class: 'muted' }, '—'), '独立容器')
          : h('span', null, row.composeProject)
    }
  ]
  if (authStore.isAdmin) {
    base.push({
      title: '操作',
      key: 'actions',
      width: 300,
      fixed: 'right',
      render: renderContainerActions
    })
  }
  return base
})

/** readonly 视角无 fixed:right 操作列（负空间：fixed:right 列消失） */
const containerScrollX = computed(() => (authStore.isAdmin ? 1050 : 750))

/** Compose 操作列：locatable 驱动（false 全禁用 + Tooltip 兜底口径）+ 项目级在途互斥 */
function renderComposeActions(row: DockerComposeItem): VNode {
  const inFlight = store.isInFlight(`compose:${row.project}`)
  const locateTip = '工作目录或配置文件缺失，无法执行项目级操作；可在容器页逐个操作兜底'
  const verbDefs: ReadonlyArray<{ verb: ComposeVerb; confirm: string; danger?: boolean }> = [
    { verb: 'up', confirm: '确认执行 up？将按配置文件创建/启动项目全部服务。' },
    { verb: 'restart', confirm: `确认重启项目 ${row.project} 全部服务？` },
    {
      verb: 'down',
      // 加强文案：down 影响面=项目全容器（4.3）
      confirm: `确认执行 down？将停止并移除项目 ${row.project} 的全部容器。`,
      danger: true
    }
  ]
  const nodes: VNode[] = []
  for (const def of verbDefs) {
    const disabled = !row.locatable || inFlight
    const btn = h(
      NButton,
      {
        size: 'small',
        type: def.danger === true ? 'error' : 'default',
        ghost: def.danger === true,
        disabled,
        onClick: () => {
          void execComposeAction(row, def.verb)
        }
      },
      { default: () => COMPOSE_ACTION_TEXT[def.verb] }
    )
    // 三动词均为 Popconfirm 二次确认（4.3；down 加强文案）
    nodes.push(
      h(
        NPopconfirm,
        {
          onPositiveClick: () => {
            void execComposeAction(row, def.verb)
          }
        },
        { trigger: () => btn, default: () => def.confirm }
      )
    )
  }
  // 日志入口：locatable=false 同判禁用（logs/logs-follow 同经 CLI 定位，DK_015，PageSpec 4.3）
  const logsDisabled = !row.locatable
  const logsBtn = h(
    NButton,
    {
      size: 'small',
      disabled: logsDisabled,
      onClick: () => store.openComposeDrawer(row)
    },
    { default: () => COMPOSE_ACTION_TEXT['logs'] }
  )
  nodes.push(logsDisabled ? withTooltip(logsBtn, locateTip) : logsBtn)
  return h('div', { class: 'row-actions' }, nodes)
}

const composeColumns = computed<DataTableColumns<DockerComposeItem>>(() => {
  const base: DataTableColumns<DockerComposeItem> = [
    { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
    {
      title: '项目名',
      key: 'project',
      width: 180,
      render: (row) =>
        // 项目名 link → Compose 日志 Drawer（查看类，两角色保留）
        h(
          NButton,
          { text: true, type: 'primary', onClick: () => store.openComposeDrawer(row) },
          { default: () => row.project }
        )
    },
    {
      title: '工作目录',
      key: 'workingDir',
      width: 260,
      render: (row) => h('span', { class: 'mono' }, row.workingDir ?? '—') // label 透传（R-13）
    },
    {
      title: '配置文件',
      key: 'configFile',
      width: 200,
      render: (row) => h('span', { class: 'mono' }, row.configFile ?? '—')
    },
    {
      title: '容器',
      key: 'containers',
      width: 200,
      render: (row) =>
        withTooltip(
          h('span', null, `${row.containers.length} 个容器`),
          row.containers.map((c) => `${c.name}（${c.state}）`).join('\n')
        )
    },
    {
      title: '定位',
      key: 'locatable',
      width: 110,
      render: (row) =>
        row.locatable
          ? h(NTag, { type: 'success', size: 'small', bordered: false }, { default: () => '可操作' })
          : withTooltip(
              h(NTag, { type: 'error', size: 'small', bordered: false }, { default: () => '定位失败' }),
              '工作目录或配置文件缺失'
            )
    }
  ]
  if (authStore.isAdmin) {
    base.push({
      title: '操作',
      key: 'actions',
      width: 240,
      fixed: 'right',
      render: renderComposeActions
    })
  }
  return base
})

const composeScrollX = computed(() => (authStore.isAdmin ? 1250 : 1010))

const imageColumns = computed<DataTableColumns<DockerImageItem>>(() => {
  const base: DataTableColumns<DockerImageItem> = [
    { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
    { title: '镜像', key: 'ref', width: 300, render: (row) => h('span', { class: 'mono' }, row.ref) },
    {
      title: '大小',
      key: 'sizeBytes',
      width: 120,
      align: 'right',
      render: (row) => formatBytes(row.sizeBytes)
    }
  ]
  if (authStore.isAdmin) {
    base.push({
      title: '操作',
      key: 'actions',
      width: 100,
      fixed: 'right',
      render: (row) => {
        const inFlight = store.isInFlight(`image:${row.imageId}`)
        return h(
          NPopconfirm,
          { onPositiveClick: () => void execImageDelete(row) },
          {
            trigger: () =>
              h(
                NButton,
                {
                  size: 'small',
                  type: 'error',
                  ghost: true,
                  loading: inFlight,
                  disabled: inFlight
                },
                { default: () => '删除' }
              ),
            default: () => `确认删除镜像 ${row.ref}？被容器引用时将被拒绝。`
          }
        )
      }
    })
  }
  return base
})

const imageScrollX = computed(() => (authStore.isAdmin ? 580 : 480))

const volumeColumns = computed<DataTableColumns<DockerVolumeItem>>(() => {
  const base: DataTableColumns<DockerVolumeItem> = [
    { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
    {
      title: '卷名',
      key: 'volumeName',
      width: 260,
      render: (row) => h('span', { class: 'mono' }, row.volumeName)
    },
    { title: '挂载点', key: 'mountpoint', width: 300, render: (row) => row.mountpoint },
    {
      title: '大小',
      key: 'sizeBytes',
      width: 120,
      align: 'right',
      render: (row) => formatBytes(row.sizeBytes)
    }
  ]
  if (authStore.isAdmin) {
    base.push({
      title: '操作',
      key: 'actions',
      width: 100,
      fixed: 'right',
      render: (row) => {
        const inFlight = store.isInFlight(`volume:${row.volumeName}`)
        return h(
          NPopconfirm,
          { onPositiveClick: () => void execVolumeDelete(row) },
          {
            trigger: () =>
              h(
                NButton,
                {
                  size: 'small',
                  type: 'error',
                  ghost: true,
                  loading: inFlight,
                  disabled: inFlight
                },
                { default: () => '删除' }
              ),
            // 加强文案：数据销毁不可恢复（PRD FR-13）
            default: () =>
              `确认删除卷 ${row.volumeName}？卷内数据将随删除永久销毁、不可恢复。`
          }
        )
      }
    })
  }
  return base
})

const volumeScrollX = computed(() => (authStore.isAdmin ? 840 : 740))

const networkColumns = computed<DataTableColumns<DockerNetworkItem>>(() => {
  const base: DataTableColumns<DockerNetworkItem> = [
    { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
    { title: '名称', key: 'name', width: 220, render: (row) => row.name },
    { title: '驱动', key: 'driver', width: 140, render: (row) => row.driver }, // 开放集合原值透传
    {
      title: '子网',
      key: 'subnet',
      width: 200,
      render: (row) => (!row.subnet ? h('span', { class: 'muted' }, '—') : row.subnet)
    }
  ]
  if (authStore.isAdmin) {
    base.push({
      title: '操作',
      key: 'actions',
      width: 100,
      fixed: 'right',
      render: (row) => {
        const inFlight = store.isInFlight(`network:${row.networkId}`)
        return h(
          NPopconfirm,
          { onPositiveClick: () => void execNetworkDelete(row) },
          {
            trigger: () =>
              h(
                NButton,
                {
                  size: 'small',
                  type: 'error',
                  ghost: true,
                  loading: inFlight,
                  disabled: inFlight
                },
                { default: () => '删除' }
              ),
            default: () => `确认删除网络 ${row.name}？`
          }
        )
      }
    })
  }
  return base
})

const networkScrollX = computed(() => (authStore.isAdmin ? 720 : 620))

function containerRowKey(row: DockerContainerItem): string {
  return row.containerId
}
function composeRowKey(row: DockerComposeItem): string {
  return row.project
}
function imageRowKey(row: DockerImageItem): string {
  return row.imageId
}
function volumeRowKey(row: DockerVolumeItem): string {
  return row.volumeName
}
function networkRowKey(row: DockerNetworkItem): string {
  return row.networkId
}

/** ---- 生命周期（进入页面 status 先行；卸载清理 Dialog/inFlight/过期标记/全部 WS 订阅） ---- */

onMounted(() => {
  void store.loadStatus().then(() => {
    if (store.daemon !== null && !store.daemonUnavailable) store.ensureTabLoaded('containers')
  })
})

onUnmounted(() => {
  if (composeProjectTimer !== null) {
    clearTimeout(composeProjectTimer)
    composeProjectTimer = null
  }
  store.resetView()
})
</script>

<template>
  <div class="page">
    <!-- 4.0 页头：daemon 接管状态 Card（恒为第一区块；引导态下保留刷新=安装后重试入口） -->
    <n-card size="small" title="Docker 接管状态">
      <template #header-extra>
        <n-button size="small" :loading="store.statusLoading" @click="refreshAll()">刷新</n-button>
      </template>
      <n-spin v-if="store.statusLoading && store.daemon === null" size="small" class="header-state" />
      <n-alert v-else-if="store.statusFailed" type="error" title="接管状态获取失败">
        <n-button size="small" @click="refreshAll()">重试</n-button>
      </n-alert>
      <template v-else-if="store.daemon !== null && daemonDict !== null">
        <div class="status-row">
          <span class="status-label">接管状态</span>
          <n-tag :type="daemonDict.tag" size="small" :bordered="false">{{ daemonDict.label }}</n-tag>
          <span class="status-label">版本</span>
          <span class="meta-inline">{{ versionText }}</span>
          <template v-if="unavailableDetail !== ''">
            <span class="status-label">不可用原因</span>
            <span class="meta-inline">{{ unavailableDetail }}</span>
          </template>
        </div>
        <div class="meta">数据生成于 {{ statusGeneratedAtText }}</div>
      </template>
    </n-card>

    <!-- 引导态（state=unavailable）：整页降级 NEmpty，不渲染五 Tab 与任何清单内容（FL S-45-02） -->
    <n-card v-if="store.daemonUnavailable" size="small">
      <n-empty :description="guideText" class="empty" />
    </n-card>

    <!-- 五 Tab：容器 / Compose / 镜像 / 卷 / 网络（懒加载） -->
    <n-card v-else-if="store.daemon !== null" size="small">
      <n-tabs v-model:value="activeTab" type="line">
        <!-- Tab 1 容器（4.1 工具条 + 表格；无新增按钮——容器创建不在面板动作集，PRD OUT-7） -->
        <n-tab-pane name="containers" tab="容器">
          <div class="controls-row">
            <n-select
              v-model:value="stateFilter"
              multiple
              clearable
              :options="stateOptions"
              class="ctrl-filter"
              placeholder="状态（空=全部）"
            />
            <n-input
              v-model:value="composeProjectFilter"
              clearable
              class="ctrl-input"
              placeholder="Compose 项目（精确匹配）"
            />
            <n-button :loading="store.containerLoading" @click="store.loadContainers()">刷新</n-button>
          </div>
          <div class="meta">
            数据生成于 {{ containerGeneratedAtText }} · 共 {{ store.containerTotal }} 个容器
          </div>
          <n-data-table
            :columns="containerColumns"
            :data="store.containerItems"
            :loading="store.containerLoading"
            :scroll-x="containerScrollX"
            :row-key="containerRowKey"
          >
            <template #empty>
              <n-empty description="当前无容器" class="empty" />
            </template>
          </n-data-table>
        </n-tab-pane>

        <!-- Tab 2 Compose（4.3：无筛选参数全量视图；locatable 预警 + 定位失败兜底口径） -->
        <n-tab-pane name="compose" tab="Compose">
          <div class="controls-row">
            <n-button :loading="store.composeLoading" @click="store.loadCompose()">刷新</n-button>
          </div>
          <div class="meta">
            数据生成于 {{ composeGeneratedAtText }} · 共 {{ store.composeTotal }} 个项目
          </div>
          <!-- readonly 处置线索（无写入口，不虚构操作路径） -->
          <div v-if="!authStore.isAdmin" class="meta">
            如需处置，请联系管理员或经服务器本地操作
          </div>
          <n-data-table
            :columns="composeColumns"
            :data="store.composeItems"
            :loading="store.composeLoading"
            :scroll-x="composeScrollX"
            :row-key="composeRowKey"
          >
            <template #empty>
              <n-empty description="当前无项目" class="empty" />
            </template>
          </n-data-table>
        </n-tab-pane>

        <!-- Tab 3 镜像（4.4：拉取按钮 admin 专属——readonly 不渲染非禁用，负空间） -->
        <n-tab-pane name="images" tab="镜像">
          <div class="controls-row">
            <n-button
              v-if="authStore.isAdmin"
              type="primary"
              :disabled="store.pullInFlight"
              @click="store.openPullDialog()"
            >
              拉取镜像
            </n-button>
            <n-button :loading="store.imageLoading" @click="store.loadImages()">刷新</n-button>
          </div>
          <div class="meta">
            数据生成于 {{ imageGeneratedAtText }} · 共 {{ store.imageTotal }} 个镜像
          </div>
          <n-data-table
            :columns="imageColumns"
            :data="store.imageItems"
            :loading="store.imageLoading"
            :scroll-x="imageScrollX"
            :row-key="imageRowKey"
          >
            <template #empty>
              <n-empty description="当前无镜像" class="empty" />
            </template>
          </n-data-table>
        </n-tab-pane>

        <!-- Tab 4 卷（4.5：数据销毁确认随操作列，readonly 整列不渲染） -->
        <n-tab-pane name="volumes" tab="卷">
          <div class="controls-row">
            <n-button :loading="store.volumeLoading" @click="store.loadVolumes()">刷新</n-button>
          </div>
          <div class="meta">
            数据生成于 {{ volumeGeneratedAtText }} · 共 {{ store.volumeTotal }} 个卷
          </div>
          <n-data-table
            :columns="volumeColumns"
            :data="store.volumeItems"
            :loading="store.volumeLoading"
            :scroll-x="volumeScrollX"
            :row-key="volumeRowKey"
          >
            <template #empty>
              <n-empty description="当前无卷" class="empty" />
            </template>
          </n-data-table>
        </n-tab-pane>

        <!-- Tab 5 网络（4.6：预定义网络如实呈现，删除以服务端权威拒绝为准） -->
        <n-tab-pane name="networks" tab="网络">
          <div class="controls-row">
            <n-button :loading="store.networkLoading" @click="store.loadNetworks()">刷新</n-button>
          </div>
          <div class="meta">
            数据生成于 {{ networkGeneratedAtText }} · 共 {{ store.networkTotal }} 个网络
          </div>
          <n-data-table
            :columns="networkColumns"
            :data="store.networkItems"
            :loading="store.networkLoading"
            :scroll-x="networkScrollX"
            :row-key="networkRowKey"
          >
            <template #empty>
              <n-empty description="当前无网络" class="empty" />
            </template>
          </n-data-table>
        </n-tab-pane>
      </n-tabs>
    </n-card>

    <!-- 容器详情 Drawer（5.1：stats 快照 + 日志双 Card；关闭即断 WS） -->
    <container-detail-drawer />
    <!-- Compose 日志 Drawer（5.2：单日志 Card + label 透传 + locatable 降级） -->
    <compose-logs-drawer />
    <!-- 拉取镜像 Dialog（5.4，admin 专属；readonly 不渲染整组件——无入口即无表单形态） -->
    <pull-image-modal
      v-if="authStore.isAdmin"
      v-model:show="store.pullDialogVisible"
      :submitting="store.pullInFlight"
      :server-error="pullServerError"
      @submit="onPullSubmit"
    />
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.status-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.status-label {
  font-size: 12px;
  opacity: 0.6;
}
.meta-inline {
  font-size: 13px;
}
.header-state {
  display: inline-flex;
}
.controls-row {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 8px;
}
.ctrl-filter {
  width: 220px;
}
.ctrl-input {
  width: 240px;
}
.meta {
  font-size: 12px;
  opacity: 0.6;
  margin-bottom: 8px;
}
.muted {
  opacity: 0.55;
}
.mono {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', monospace;
  font-size: 12px;
  word-break: break-all;
}
.empty {
  padding: 24px 0;
}
.row-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
</style>
