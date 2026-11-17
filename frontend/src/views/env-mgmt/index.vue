<script setup lang="ts">
/**
 * 环境管理页（PageSpec env-mgmt v1.1，hybrid；路由 /env；admin+readonly 同路由
 * 按角色渲染子集——操作区整区不渲染，ADR-018 负空间）。
 * 页头通道状态条 + 环境卡片网格（七条目 D-02 v1.1 +MongoDB：目录字段+实况+
 * 在途标记；实况三态 ⚠️A9——通道级不可用归页头横幅、条目级真错误才显
 * 「探测不可用」角标、未装含包名不在包数据库形态）+ 任务 Drawer（历史筛选
 * 列表 / 详情输出尾部 running 5s 轮询终态即停）。安装/升级 NPopconfirm（刷新
 * 索引+第三方源代配提示 ⚠️A10）；卸载强确认 NModal 两段警示（⚠️A6：本域服务
 * 实况+关联模块静态影响）+ 输入环境名锁定；服务操作组（D-09，单元全名 ⚠️A8）。
 */
import { computed, h, onMounted, onUnmounted, ref } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import { NButton, NTag, NTooltip } from 'naive-ui'
import type { DataTableColumns, SelectOption } from 'naive-ui'
import { envInstall, envServiceOp, envUninstall } from '@/api/env'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { useEnvStore } from '@/stores/env'
import { BOOTSTRAP_STATUS, ENV_TASK_ACTION, ENV_TASK_STATUS, ENV_TYPE, PACKAGE_MANAGER_FAMILY, dictItem } from '@/types/enums'
import type { EnvItem } from '@/types/env'
import type { EnvServiceOp, EnvTaskItem } from '@/types/env'
import type { EnvTaskStatus, EnvType } from '@/types/enums'
import { formatDateTime, formatTime } from '@/utils/format'
import EnvTaskDrawer from './components/EnvTaskDrawer.vue'

const authStore = useAuthStore()
const store = useEnvStore()
const message = useMessage()
const dialog = useDialog()

/** ---- 任务 Drawer ---- */

const drawerShow = ref(false)

function openTasks(): void {
  drawerShow.value = true
  void store.loadTasks(1)
}

function onDrawerTaskClick(taskId: number): void {
  void store.loadTaskDetail(taskId)
}

/** 在途任务「查看」引导（卡片标记 → Drawer 定位任务） */
function viewRunningTask(taskId: number): void {
  drawerShow.value = true
  void store.loadTasks(1).then(() => store.loadTaskDetail(taskId))
}

/** ---- 安装/升级（NPopconfirm 文案含刷新索引与仓库代配提示，D-05/D-06） ---- */

const installing = ref<string | null>(null)

async function onInstall(item: EnvItem): Promise<void> {
  installing.value = item.envType
  try {
    const res = await envInstall(item.envType)
    message.success(
      res.action === 'upgrade'
        ? '升级任务已创建（先刷新包索引，可能需要数分钟）'
        : '安装任务已创建（先刷新包索引，可能需要数分钟）'
    )
    await store.refreshAfterWrite()
    viewRunningTask(res.taskId)
  } catch (e) {
    handleEnvError(e)
  } finally {
    installing.value = null
  }
}

/** ---- 卸载强确认（NModal 两段警示 + 输入环境名锁定，⚠️A6/D-07） ---- */

const uninstallTarget = ref<EnvItem | null>(null)
const uninstallConfirmName = ref('')
const uninstalling = ref(false)

const uninstallConfirmOk = computed(
  () => uninstallTarget.value !== null && uninstallConfirmName.value === uninstallTarget.value.displayName
)

function openUninstall(item: EnvItem): void {
  uninstallTarget.value = item
  uninstallConfirmName.value = ''
}

async function onUninstall(): Promise<void> {
  const t = uninstallTarget.value
  if (t === null) return
  uninstalling.value = true
  try {
    const res = await envUninstall(t.envType)
    message.success('卸载任务已创建（保留系统配置与数据目录）')
    uninstallTarget.value = null
    await store.refreshAfterWrite()
    viewRunningTask(res.taskId)
  } catch (e) {
    handleEnvError(e)
  } finally {
    uninstalling.value = false
  }
}

/** ---- 服务操作（NPopconfirm；ENV_006 失败原因如实呈现，D-09） ---- */

const serviceBusy = ref<string | null>(null)

async function onServiceOp(item: EnvItem, op: EnvServiceOp): Promise<void> {
  serviceBusy.value = `${item.envType}:${op}`
  try {
    await envServiceOp(op, item.envType)
    message.success('操作已执行')
    void store.loadList()
  } catch (e) {
    handleEnvError(e)
  } finally {
    serviceBusy.value = null
  }
}

/** env 域错误分流（ENV_001~006；文本与 api 3.4 一一对应） */
function handleEnvError(e: unknown): void {
  if (!(e instanceof ApiError)) return
  if (e.is('ENV_001')) {
    message.error('包管理器通道不可用（apt/dnf·yum 均未检出）——变更功能不可用')
    return
  }
  if (e.is('ENV_003')) {
    const taskId = (e.data as Record<string, unknown> | null)?.taskId
    message.error('该环境已有任务进行中')
    if (typeof taskId === 'number') viewRunningTask(taskId)
    return
  }
  if (e.is('ENV_004')) {
    message.error('环境未安装')
    void store.loadList()
    return
  }
  if (e.is('ENV_006')) {
    const reason = (e.data as Record<string, unknown> | null)?.reason
    dialog.error({
      title: '服务操作失败',
      content: typeof reason === 'string' ? reason : '（服务端未返回原因）'
    })
    return
  }
  message.error(e.message)
}

/** ---- 通道状态条 ---- */

const channelTag = computed(() => {
  if (!store.channel.available) {
    return { text: '包管理通道不可用（变更功能已禁用）', type: 'warning' as const }
  }
  const label = store.channel.family != null ? dictItem(PACKAGE_MANAGER_FAMILY, store.channel.family).label : ''
  return { text: `包管理通道：${label}`, type: 'success' as const }
})

/** 卡片实况渲染辅助（v1.1 三态 ⚠️A9：installed=null 仅条目级真错误显橙——通道级
 * 不可用由页头横幅承载语义，角标降级灰「未探测」不重复渲染；false=未安装含
 * 包名不在包数据库形态） */
function installedText(item: EnvItem): { text: string; type: 'default' | 'success' | 'warning' } {
  if (item.installed === null) {
    return store.channel.available
      ? { text: '探测不可用', type: 'warning' }
      : { text: '未探测', type: 'default' }
  }
  if (!item.installed) return { text: '未安装', type: 'default' }
  return { text: `已安装 · ${item.version ?? ''}`, type: 'success' }
}

/** ---- 任务清单列（Drawer 历史 Tab 宿主列定义在此共享） ---- */

const envTypeOptions: SelectOption[] = (Object.keys(ENV_TYPE) as EnvType[]).map((v) => ({
  label: ENV_TYPE[v].label,
  value: v
}))
const statusOptions: SelectOption[] = (Object.keys(ENV_TASK_STATUS) as EnvTaskStatus[]).map((v) => ({
  label: ENV_TASK_STATUS[v].label,
  value: v
}))

const taskColumns = computed<DataTableColumns<EnvTaskItem>>(() => [
  { title: 'ID', key: 'id', width: 70 },
  {
    title: '环境',
    key: 'envType',
    width: 110,
    render: (row) => dictItem(ENV_TYPE, row.envType).label
  },
  {
    title: '动作',
    key: 'action',
    width: 90,
    render: (row) => dictItem(ENV_TASK_ACTION, row.action).label
  },
  {
    title: '状态',
    key: 'status',
    width: 110,
    render: (row) =>
      h(
        NTag,
        { type: dictItem(ENV_TASK_STATUS, row.status).tag, size: 'small', bordered: false },
        { default: () => dictItem(ENV_TASK_STATUS, row.status).label }
      )
  },
  {
    title: '引导',
    key: 'bootstrapStatus',
    width: 90,
    render: (row) => {
      // null=无引导步骤（非数据库环境/卸载）→「—」不渲染角标（D-18）。
      if (!row.bootstrapStatus) return '—'
      const d = dictItem(BOOTSTRAP_STATUS, row.bootstrapStatus)
      const tag = h(NTag, { type: d.tag, size: 'small', bordered: false }, { default: () => d.label })
      if (!row.bootstrapNote) return tag
      return h(NTooltip, { trigger: 'hover' }, { trigger: () => tag, default: () => row.bootstrapNote })
    }
  },
  { title: '发起者', key: 'requestedBy', width: 100 },
  { title: '发起时间', key: 'createdAt', width: 170, render: (row) => formatDateTime(row.createdAt) },
  { title: '结束时间', key: 'finishedAt', width: 170, render: (row) => formatDateTime(row.finishedAt) },
  {
    title: '操作',
    key: 'actions',
    width: 80,
    render: (row) =>
      h(
        NButton,
        { size: 'small', text: true, type: 'primary', onClick: () => onDrawerTaskClick(row.id) },
        { default: () => '详情' }
      )
  }
])

onMounted(() => {
  void store.loadList()
})

onUnmounted(() => {
  store.stopPoll()
  store.clearTaskDetail()
})
</script>

<template>
  <div class="page">
    <!-- 页头：通道状态条 + 刷新 + 任务入口 -->
    <n-card size="small">
      <div class="head-row">
        <n-tag :type="channelTag.type" size="small">{{ channelTag.text }}</n-tag>
        <span class="meta">探测于 {{ formatTime(store.generatedAt) }}</span>
        <div class="spacer"></div>
        <n-button size="small" :loading="store.listLoading" @click="store.loadList()">刷新</n-button>
        <n-button size="small" @click="openTasks">任务记录</n-button>
      </div>
      <n-alert
        v-if="store.listError"
        type="error"
        closable
        style="margin-top: 8px"
        title="环境实况加载失败"
      >
        加载失败（{{ store.listError ? '列表请求未成功' : '' }}）。
        <n-button size="tiny" @click="store.loadList()">重试</n-button>
      </n-alert>
    </n-card>

    <!-- 环境卡片网格（七条目 v1.1 +MongoDB；readonly 操作区整区不渲染——ADR-018 负空间） -->
    <div class="grid">
      <n-card v-for="item in store.items" :key="item.envType" size="small" class="env-card">
        <template #header>
          <span class="env-title">{{ item.displayName }}</span>
          <n-tag v-if="item.runningTaskId !== null" type="info" size="small" class="env-task-tag">
            任务进行中
          </n-tag>
        </template>
        <template #header-extra>
          <n-tag :type="installedText(item).type" size="small" :bordered="false">
            {{ installedText(item).text }}
          </n-tag>
        </template>
        <div class="env-desc">{{ item.description }}</div>
        <!-- 未安装环境不渲染服务态行（v0.1.4：曾显示「已停止/自启关」误导） -->
        <div v-if="item.installed !== false" class="env-service">
          <template v-if="item.serviceRunning !== null">
            <n-tag :type="item.serviceRunning ? 'success' : 'default'" size="small" :bordered="false">
              {{ item.serviceRunning ? '运行中' : '已停止' }}
            </n-tag>
            <n-tag :type="item.serviceEnabled ? 'success' : 'default'" size="small" :bordered="false">
              自启{{ item.serviceEnabled ? '开' : '关' }}
            </n-tag>
          </template>
          <span v-else class="env-unit-none">服务态未探测</span>
        </div>
        <template v-if="item.runningTaskId !== null" #action>
          <n-button size="tiny" text type="primary" @click="viewRunningTask(item.runningTaskId ?? 0)">
            查看任务
          </n-button>
        </template>
        <template v-if="authStore.isAdmin && store.channel.available" #action>
          <div class="env-actions">
            <template v-if="item.installed === true">
              <n-popconfirm @positive-click="onInstall(item)">
                <template #trigger>
                  <n-button size="small" :loading="installing === item.envType">升级</n-button>
                </template>
                将通过 {{ dictItem(PACKAGE_MANAGER_FAMILY, store.channel.family ?? 'apt').label }} 升级到仓库最新版（先刷新包索引，可能需要数分钟；需第三方官方源的环境——MongoDB、RHEL 系 Docker CE——将自动写入源配置并留痕）。
              </n-popconfirm>
              <n-button size="small" type="error" ghost @click="openUninstall(item)">卸载</n-button>
              <n-popconfirm @positive-click="onServiceOp(item, 'start')">
                <template #trigger><n-button size="small" :disabled="serviceBusy === `${item.envType}:start`">启动</n-button></template>
                启动 {{ item.unitName }}
              </n-popconfirm>
              <n-popconfirm @positive-click="onServiceOp(item, 'stop')">
                <template #trigger><n-button size="small" :disabled="serviceBusy === `${item.envType}:stop`">停止</n-button></template>
                停止 {{ item.unitName }}
              </n-popconfirm>
              <n-popconfirm @positive-click="onServiceOp(item, 'restart')">
                <template #trigger><n-button size="small" :disabled="serviceBusy === `${item.envType}:restart`">重启</n-button></template>
                重启 {{ item.unitName }}
              </n-popconfirm>
              <n-switch
                :value="item.serviceEnabled === true"
                :loading="serviceBusy === `${item.envType}:enable` || serviceBusy === `${item.envType}:disable`"
                @update:value="(v: boolean) => onServiceOp(item, v ? 'enable' : 'disable')"
              />
            </template>
            <template v-else-if="item.installed === false">
              <n-popconfirm @positive-click="onInstall(item)">
                <template #trigger>
                  <n-button size="small" type="primary" :loading="installing === item.envType">安装</n-button>
                </template>
                将通过包管理器安装 {{ item.displayName }} 到仓库最新版（先刷新包索引，可能需要数分钟；需第三方官方源的环境——MongoDB、RHEL 系 Docker CE——将自动写入源配置并留痕）。
              </n-popconfirm>
            </template>
          </div>
        </template>
      </n-card>
    </div>

    <!-- 任务 Drawer -->
    <env-task-drawer
      v-model:show="drawerShow"
      :columns="taskColumns"
      :env-type-options="envTypeOptions"
      :status-options="statusOptions"
      @refresh="store.loadTasks(1)"
    />

    <!-- 卸载强确认（两段警示 + 输入环境名锁定，⚠️A6/D-07） -->
    <n-modal
      :show="uninstallTarget !== null"
      preset="card"
      title="卸载环境（高危操作）"
      style="width: 540px"
      :mask-closable="false"
      @update:show="(v: boolean) => { if (!v) uninstallTarget = null }"
    >
      <n-alert
        v-if="uninstallTarget?.serviceRunning === true"
        type="warning"
        title="服务正在运行"
        style="margin-bottom: 10px"
      >
        {{ uninstallTarget.displayName }} 服务当前正在运行，卸载将使其立即不可用。
      </n-alert>
      <n-alert type="error" title="关联模块影响" style="margin-bottom: 10px">
        卸载后对应功能将失去运行环境——Docker：容器管理；MySQL/MongoDB：数据库管理；nginx/PHP-FPM：网站管理。
        请先到对应模块确认数据已处置（面板不做跨模块数据检查）。
      </n-alert>
      <n-form label-placement="left" label-width="110">
        <n-form-item label="确认输入">
          <n-input
            v-model:value="uninstallConfirmName"
            :placeholder="`输入环境名 ${uninstallTarget?.displayName ?? ''} 确认`"
          />
        </n-form-item>
      </n-form>
      <n-alert type="info" :show-icon="true">
        将执行包卸载（保留系统配置与数据目录；已配第三方官方源文件保留不回收——重装即用）；任务异步执行，输出可在任务记录中查看。
      </n-alert>
      <template #footer>
        <div class="footer">
          <n-button :disabled="uninstalling" @click="uninstallTarget = null">取消</n-button>
          <n-button type="error" :disabled="!uninstallConfirmOk" :loading="uninstalling" @click="onUninstall">
            确认卸载
          </n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.head-row {
  display: flex;
  align-items: center;
  gap: 12px;
}
.spacer {
  flex: 1;
}
.meta {
  font-size: 12px;
  opacity: 0.65;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 16px;
}
.env-card :deep(.n-card__content) {
  min-height: 150px;
}
.env-title {
  font-weight: 600;
}
.env-task-tag {
  margin-left: 8px;
}
.env-desc {
  font-size: 13px;
  opacity: 0.85;
}
.env-service {
  margin-top: 10px;
  display: flex;
  gap: 8px;
  align-items: center;
}
.env-unit-none {
  font-size: 12px;
  opacity: 0.55;
}
.env-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  align-items: center;
}
.footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
