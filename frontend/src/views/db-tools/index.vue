<script setup lang="ts">
/**
 * 数据库工具页 tool-list（PageSpec db-tools v1.1，hybrid；路由 /db-tools；
 * admin+readonly 同路由按角色渲染子集——操作区整区不渲染，ADR-018 + ⚠️A10 负空间）。
 * 页头说明条（gating 安全口径+轻量入口并存 Q3）+ 刷新 + generatedAt 探测时刻；
 * 工具卡片网格两条目（phpMyAdmin/mongo-express：部署状态/版本/运行态三态/依赖就绪度
 * depsMissing 清单+环境管理链接/在途标记/绑定实例呈现）；安装分工具形态（phpMyAdmin
 * NPopconfirm / mongo-express NModal 实例选择）；卸载强确认 NModal 两段警示+输入锁定
 * +不删数据文案（⚠️A5/D-15）；升级/轮换 NPopconfirm（轮换仅 mongo-express D-08）；
 * 打开分发=新标签页 window.open noopener（4.4 裁定，signon/gating）；任务 Drawer
 * 双区（历史筛选列表/详情输出尾部 running 5s 轮询终态即停+终态跃迁重拉全景）。
 * readonly：目录/任务观测全保留，变更与打开/轮换入口不渲染（6 负空间）；工具本体
 * 经 nginx gating 网络层闸门不可达（非仅前端隐藏）。
 */
import { computed, h, onMounted, onUnmounted, ref } from 'vue'
import { useMessage } from 'naive-ui'
import { NButton, NTag, NTooltip } from 'naive-ui'
import type { DataTableColumns, SelectOption } from 'naive-ui'
import { dbtoolInstall, dbtoolOpen, dbtoolRotate, dbtoolUninstall, dbtoolUpgrade } from '@/api/dbtool'
import { instanceList } from '@/api/database'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { useDbtoolStore } from '@/stores/dbtool'
import { DB_ENGINE, DB_TOOL_TASK_ACTION, DB_TOOL_TASK_STATUS, DB_TOOL_TYPE, dictItem } from '@/types/enums'
import type { DbToolTaskStatus, DbToolType } from '@/types/enums'
import type { DbToolItem, DbToolTaskItem } from '@/types/dbtool'
import {
  DBTOOL_PAGE_INTRO,
  depMissingText,
  depsMissingSummary,
  depsReadyTag,
  dbtErrorText,
  dbtMissingOf,
  dbtTaskIdOf,
  installedStateTag,
  instanceIdentityText,
  openToastText,
  runningStateTag
} from '@/types/dbtool'
import type { DbInstanceItem } from '@/types/database'
import { formatDateTime, formatTime } from '@/utils/format'
import DbtoolTaskDrawer from './components/DbtoolTaskDrawer.vue'
import ToolInstanceModal from './components/ToolInstanceModal.vue'

const message = useMessage()
const authStore = useAuthStore()
const store = useDbtoolStore()

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

/** ---- 绑定实例反查（mongo_express 已装时呈现 boundInstanceId → identityKey） ---- */

const instances = ref<DbInstanceItem[]>([])

async function loadInstances(): Promise<void> {
  try {
    const data = await instanceList()
    instances.value = data.items
  } catch {
    // 反查失败不阻断工具全景呈现——降级显示实例 id（不虚构标识）
  }
}

function boundInstanceText(item: DbToolItem): string | null {
  if (item.toolType !== 'mongo_express' || item.boundInstanceId === null) return null
  const inst = instances.value.find((it) => it.id === item.boundInstanceId)
  if (inst === undefined) return `实例 #${item.boundInstanceId}`
  return `${instanceIdentityText(inst)}（#${item.boundInstanceId}）`
}

/** ---- 错误分流（PageSpec 第 1 节：DBT 码页面级提示——拦截器不含 DBT 码集） ---- */

function handleDbtError(e: unknown): void {
  if (!(e instanceof ApiError)) return
  if (e.is('DBT_001')) {
    // 依赖缺失内联清单（data.missing）+ 环境管理链接（卡片 depsMissing 已同源呈现）
    const missing = dbtMissingOf(e.data)
    message.error(
      missing.length > 0
        ? `依赖缺失：${depsMissingSummary(missing)}——请到环境管理安装`
        : '依赖缺失——请到环境管理安装'
    )
    void store.loadList()
    return
  }
  if (e.is('DBT_003')) {
    message.error('该工具有任务进行中')
    const taskId = dbtTaskIdOf(e.data)
    if (taskId !== null) viewRunningTask(taskId)
    return
  }
  if (e.is('DBT_004')) {
    message.error('工具未安装——请先安装')
    void store.loadList()
    return
  }
  if (e.is('DBT_006')) {
    message.error('实例未就绪——请到数据库管理配置凭据')
    return
  }
  if (e.is('DBT_011')) {
    message.info('当前已为目录版本，无需升级')
    return
  }
  const text = dbtErrorText(e.code, e.data)
  if (text !== null) message.error(text)
  // 其余错误（网络/AUTH 族）已由拦截器统一提示
}

/** ---- 安装（phpMyAdmin NPopconfirm / mongo-express NModal 实例选择，4.1） ---- */

const installing = ref<DbToolType | null>(null)
const installModalShow = ref(false)

async function doInstall(toolType: DbToolType, instanceId?: number): Promise<void> {
  installing.value = toolType
  try {
    const res = await dbtoolInstall(toolType, instanceId)
    message.success('任务已创建')
    installModalShow.value = false
    await store.refreshAfterWrite()
    viewRunningTask(res.taskId)
  } catch (e) {
    handleDbtError(e)
  } finally {
    installing.value = null
  }
}

function onInstallClick(item: DbToolItem): void {
  if (item.toolType === 'mongo_express') {
    installModalShow.value = true
    return
  }
  // phpmyadmin 经 NPopconfirm 触发（模板内 onPositiveClick）
}

const INSTALL_INTRO_PHPMYADMIN =
  '将下载 phpMyAdmin 官方包（sha256 校验，可能需要数分钟）并部署到面板管理目录，经面板域路径安全访问；需 PHP-FPM 环境与扩展就绪（缺失将拒绝并引导补装）。'

function installIntroMongo(catalogVersion: string): string {
  return `将 npm 安装 mongo-express ${catalogVersion} 并创建本机服务（仅监听 127.0.0.1:8081），注入所选实例凭据（加密存储不回显）与面板访问凭据；需 Node 环境就绪。`
}

const mongoCatalogVersion = computed(
  () => store.findByType('mongo_express')?.catalogVersion ?? ''
)

/** ---- 升级（NPopconfirm，4.1；DBT_011 → 已为目录版本提示） ---- */

const upgrading = ref<DbToolType | null>(null)

async function onUpgrade(item: DbToolItem): Promise<void> {
  upgrading.value = item.toolType
  try {
    const res = await dbtoolUpgrade(item.toolType)
    message.success('任务已创建')
    await store.refreshAfterWrite()
    viewRunningTask(res.taskId)
  } catch (e) {
    handleDbtError(e)
  } finally {
    upgrading.value = null
  }
}

function upgradeIntro(item: DbToolItem): string {
  return `将升级 ${item.displayName} 到目录版本 ${item.catalogVersion}（phpMyAdmin 保留配置与密钥；mongo-express 重装并重启服务，凭据与访问路径不变）。`
}

/** ---- 卸载强确认（NModal 两段警示 + 输入工具显示名锁定，⚠️A5/D-15） ---- */

const uninstallTarget = ref<DbToolItem | null>(null)
const uninstallConfirmName = ref('')
const uninstalling = ref(false)

const uninstallConfirmOk = computed(
  () =>
    uninstallTarget.value !== null &&
    uninstallConfirmName.value === uninstallTarget.value.displayName
)

function openUninstall(item: DbToolItem): void {
  uninstallTarget.value = item
  uninstallConfirmName.value = ''
}

async function onUninstall(): Promise<void> {
  const t = uninstallTarget.value
  if (t === null) return
  uninstalling.value = true
  try {
    const res = await dbtoolUninstall(t.toolType)
    message.success('卸载任务已创建（不删除数据库数据）')
    uninstallTarget.value = null
    await store.refreshAfterWrite()
    viewRunningTask(res.taskId)
  } catch (e) {
    handleDbtError(e)
  } finally {
    uninstalling.value = false
  }
}

/** ---- 轮换凭据（NPopconfirm，仅 mongo-express；响应无口令字段 I-02） ---- */

const rotating = ref(false)

async function onRotate(item: DbToolItem): Promise<void> {
  rotating.value = true
  try {
    const res = await dbtoolRotate(item.toolType)
    message.success(`凭据已轮换（${formatDateTime(res.rotatedAt)}）`)
    void store.loadList()
  } catch (e) {
    handleDbtError(e)
  } finally {
    rotating.value = false
  }
}

/** ---- 打开分发（4.4：新标签页 window.open noopener，非内嵌 iframe） ---- */

const opening = ref<DbToolType | null>(null)
const openModalShow = ref(false)
/** 打开弹窗目标工具（phpmyadmin 需实例选择；单实例自动选中免弹窗） */
const openTarget = ref<DbToolItem | null>(null)

async function doOpen(toolType: DbToolType, instanceId?: number): Promise<void> {
  opening.value = toolType
  try {
    const res = await dbtoolOpen(toolType, instanceId)
    window.open(res.url, '_blank', 'noopener')
    message.success(openToastText(res))
    openModalShow.value = false
  } catch (e) {
    handleDbtError(e)
  } finally {
    opening.value = null
  }
}

async function onOpenClick(item: DbToolItem): Promise<void> {
  if (item.toolType === 'mongo_express') {
    // 部署时已绑定实例，无需选择（4.4）
    await doOpen('mongo_express')
    return
  }
  // phpmyadmin：需 instanceId——单实例自动选中免弹窗（4.4）
  opening.value = item.toolType
  try {
    const data = await instanceList()
    instances.value = data.items
    const ready = data.items.filter(
      (it) => (it.engine === 'mysql' || it.engine === 'mariadb') && it.credentialsSet
    )
    if (ready.length === 1) {
      await doOpen('phpmyadmin', ready[0].id)
      return
    }
    openTarget.value = item
    openModalShow.value = true
  } catch (e) {
    handleDbtError(e)
  } finally {
    opening.value = null
  }
}

/** ---- 卡片呈现助手（第 3 节颜色方案；纯函数 types/dbtool.ts 承载） ---- */

function engineTags(item: DbToolItem): { label: string; tag: 'default' | 'info' | 'success' }[] {
  return item.targetEngines.map((eng) => {
    const d = dictItem(DB_ENGINE, eng)
    return { label: d.label, tag: d.tag === 'info' ? 'info' : d.tag === 'success' ? 'success' : 'default' }
  })
}

/** 变更按钮在途禁用（I-06：runningTaskId 非空时安装/升级/卸载/轮换禁用） */
function changeDisabled(item: DbToolItem): boolean {
  return item.runningTaskId !== null
}

/** ---- 任务清单列（Drawer 历史 Tab 宿主列定义，4.2） ---- */

const toolTypeOptions: SelectOption[] = (Object.keys(DB_TOOL_TYPE) as DbToolType[]).map((v) => ({
  label: DB_TOOL_TYPE[v].label,
  value: v
}))
const statusOptions: SelectOption[] = (Object.keys(DB_TOOL_TASK_STATUS) as DbToolTaskStatus[]).map(
  (v) => ({ label: DB_TOOL_TASK_STATUS[v].label, value: v })
)

const taskColumns = computed<DataTableColumns<DbToolTaskItem>>(() => [
  { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
  {
    title: 'ID',
    key: 'id',
    width: 70,
    render: (row) =>
      h(
        NButton,
        { size: 'small', text: true, type: 'primary', onClick: () => onDrawerTaskClick(row.id) },
        { default: () => `#${row.id}` }
      )
  },
  {
    title: '工具',
    key: 'toolType',
    width: 130,
    render: (row) => dictItem(DB_TOOL_TYPE, row.toolType).label
  },
  {
    title: '动作',
    key: 'action',
    width: 90,
    render: (row) => dictItem(DB_TOOL_TASK_ACTION, row.action).label
  },
  {
    title: '状态',
    key: 'status',
    width: 110,
    render: (row) => {
      const d = dictItem(DB_TOOL_TASK_STATUS, row.status)
      const tag = h(NTag, { type: d.tag, size: 'small', bordered: false }, { default: () => d.label })
      // failed 行附 error NTooltip（4.2）
      if (row.status === 'failed' && row.error !== null && row.error !== '') {
        return h(NTooltip, null, { trigger: () => tag, default: () => row.error })
      }
      return tag
    }
  },
  {
    title: '实例',
    key: 'instanceId',
    width: 90,
    render: (row) => (row.instanceId === null ? '—' : String(row.instanceId))
  },
  { title: '发起者', key: 'requestedBy', width: 100 },
  { title: '发起时间', key: 'createdAt', width: 170, render: (row) => formatDateTime(row.createdAt) },
  { title: '结束时间', key: 'finishedAt', width: 170, render: (row) => formatDateTime(row.finishedAt) }
])

onMounted(() => {
  void store.loadList()
  void loadInstances()
})

onUnmounted(() => {
  store.stopPoll()
  store.clearTaskDetail()
})
</script>

<template>
  <div class="page">
    <!-- 页头：说明条 + 刷新 + generatedAt + 任务入口（两角色同等，观测能力） -->
    <n-card size="small">
      <n-alert type="info" :show-icon="true" style="margin-bottom: 10px">
        {{ DBTOOL_PAGE_INTRO }}
      </n-alert>
      <div class="head-row">
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
        title="工具实况加载失败"
      >
        <n-button size="tiny" @click="store.loadList()">重试</n-button>
      </n-alert>
    </n-card>

    <!-- 工具卡片网格（两条目 D-13；readonly 操作区整区不渲染——ADR-018 + ⚠️A10 负空间） -->
    <div class="grid">
      <n-card v-for="item in store.items" :key="item.toolType" size="small" class="tool-card">
        <template #header>
          <span class="tool-title">{{ item.displayName }}</span>
          <n-tag size="small" :bordered="false" style="margin-left: 8px">
            {{ dictItem(DB_TOOL_TYPE, item.toolType).label }}
          </n-tag>
          <n-tag
            v-for="eng in engineTags(item)"
            :key="eng.label"
            :type="eng.tag"
            size="small"
            :bordered="false"
            style="margin-left: 6px"
          >
            {{ eng.label }}
          </n-tag>
        </template>
        <template #header-extra>
          <n-tag
            :type="installedStateTag(item.installed, item.installedVersion).tag"
            size="small"
            :bordered="false"
          >
            {{ installedStateTag(item.installed, item.installedVersion).label }}
          </n-tag>
        </template>

        <div class="tool-desc">{{ item.description }}</div>
        <div class="tool-catalog">目录版本 {{ item.catalogVersion }}</div>

        <!-- 运行态（已装才呈现——未装无运行态语义，env-mgmt v0.1.4 先例；三态不虚构 D-02） -->
        <div v-if="item.installed" class="tool-row">
          <n-tag
            :type="runningStateTag(item.running).tag"
            size="small"
            :bordered="false"
          >
            {{ runningStateTag(item.running).label }}
          </n-tag>
          <span v-if="boundInstanceText(item) !== null" class="bound-instance">
            绑定实例 {{ boundInstanceText(item) }}
          </span>
        </div>

        <!-- 依赖就绪度（DBT_001 前置数据源；缺失清单+引导链接，readonly 链接可达——观测引导） -->
        <div class="tool-row">
          <n-tag :type="depsReadyTag(item.depsReady).tag" size="small" :bordered="false">
            {{ depsReadyTag(item.depsReady).label }}
          </n-tag>
        </div>
        <ul v-if="!item.depsReady && item.depsMissing.length > 0" class="deps-list">
          <li v-for="dep in item.depsMissing" :key="`${dep.kind}:${dep.name}`">
            <template v-if="dep.kind === 'env'">
              <span>{{ depMissingText(dep) }}</span>
              <router-link to="/env" class="deps-link">到环境管理安装</router-link>
            </template>
            <template v-else>
              <span>{{ depMissingText(dep) }}</span>
              <span class="deps-hint">（经环境管理 PHP-FPM 补装扩展后刷新）</span>
            </template>
          </li>
        </ul>

        <!-- 在途标记（I-06） -->
        <div v-if="item.runningTaskId !== null" class="tool-row">
          <n-tag type="info" size="small" :bordered="false">任务进行中</n-tag>
          <n-button size="tiny" text type="primary" @click="viewRunningTask(item.runningTaskId ?? 0)">
            查看
          </n-button>
        </div>

        <!-- 操作区（admin only；readonly 整区不渲染——6 负空间之前端隐藏层） -->
        <template v-if="authStore.isAdmin" #action>
          <div class="tool-actions">
            <template v-if="!item.installed">
              <!-- 安装：depsReady=false 禁用 + tooltip 缺失清单 + 环境管理链接（DBT_001 前置） -->
              <n-tooltip v-if="!item.depsReady" :show-arrow="true">
                <template #trigger>
                  <span>
                    <n-button size="small" type="primary" disabled>安装</n-button>
                  </span>
                </template>
                依赖缺失：{{ depsMissingSummary(item.depsMissing) }}
                <router-link to="/env" class="deps-link">到环境管理安装</router-link>
              </n-tooltip>
              <n-popconfirm
                v-else-if="item.toolType === 'phpmyadmin'"
                @positive-click="doInstall('phpmyadmin')"
              >
                <template #trigger>
                  <n-button
                    size="small"
                    type="primary"
                    :loading="installing === 'phpmyadmin'"
                    :disabled="changeDisabled(item)"
                  >
                    安装
                  </n-button>
                </template>
                {{ INSTALL_INTRO_PHPMYADMIN }}
              </n-popconfirm>
              <n-button
                v-else
                size="small"
                type="primary"
                :loading="installing === 'mongo_express'"
                :disabled="changeDisabled(item)"
                @click="onInstallClick(item)"
              >
                安装
              </n-button>
            </template>
            <template v-else>
              <n-button
                size="small"
                type="primary"
                :loading="opening === item.toolType"
                @click="onOpenClick(item)"
              >
                打开
              </n-button>
              <n-popconfirm @positive-click="onUpgrade(item)">
                <template #trigger>
                  <n-button
                    size="small"
                    :loading="upgrading === item.toolType"
                    :disabled="changeDisabled(item)"
                  >
                    升级
                  </n-button>
                </template>
                {{ upgradeIntro(item) }}
              </n-popconfirm>
              <n-button
                size="small"
                type="error"
                ghost
                :disabled="changeDisabled(item)"
                @click="openUninstall(item)"
              >
                卸载
              </n-button>
              <!-- 轮换凭据仅 mongo_express 渲染（phpmyadmin 无轮换入口 D-08/DBT_009） -->
              <n-popconfirm
                v-if="item.toolType === 'mongo_express'"
                @positive-click="onRotate(item)"
              >
                <template #trigger>
                  <n-button size="small" :loading="rotating" :disabled="changeDisabled(item)">
                    轮换凭据
                  </n-button>
                </template>
                将重新生成 mongo-express 访问口令（旧口令即时失效）；浏览器无感知（服务端注入），无需重新登录。
              </n-popconfirm>
            </template>
          </div>
        </template>
      </n-card>
    </div>

    <!-- 任务 Drawer（两角色可看，ADR-018） -->
    <dbtool-task-drawer
      v-model:show="drawerShow"
      :columns="taskColumns"
      :tool-type-options="toolTypeOptions"
      :status-options="statusOptions"
    />

    <!-- mongo-express 安装实例选择弹窗（4.1） -->
    <tool-instance-modal
      v-model:show="installModalShow"
      :engines="['mongodb']"
      title="安装 mongo-express"
      :intro="installIntroMongo(mongoCatalogVersion)"
      ok-text="安装"
      @select="(id: number) => doInstall('mongo_express', id)"
    />

    <!-- phpmyadmin 打开实例选择弹窗（4.4；单实例自动选中免弹窗） -->
    <tool-instance-modal
      v-model:show="openModalShow"
      :engines="['mysql', 'mariadb']"
      title="打开 phpMyAdmin"
      intro="选择目标 MySQL/MariaDB 实例——signon 令牌绑定所选实例（60 秒时效，一次性），经面板会话安全访问进入工具后自选库。"
      ok-text="打开"
      @select="(id: number) => doOpen('phpmyadmin', id)"
    />

    <!-- 卸载强确认（两段警示 + 输入工具显示名锁定 + 不删数据文案，⚠️A5/D-15） -->
    <n-modal
      :show="uninstallTarget !== null"
      preset="card"
      title="卸载工具（高危操作）"
      style="width: 560px"
      :mask-closable="false"
      @update:show="(v: boolean) => { if (!v) uninstallTarget = null }"
    >
      <n-alert type="warning" title="功能影响" style="margin-bottom: 10px">
        卸载后 {{ uninstallTarget?.displayName ?? '' }} 高级管理入口将不可用；面板内表/文档浏览（轻量入口）不受影响。
      </n-alert>
      <n-alert type="info" title="数据保全" style="margin-bottom: 10px">
        卸载仅解除工具部署（程序/服务/站点配置/访问凭据），绝不删除数据库数据（沿实例移除哲学）。
      </n-alert>
      <n-form label-placement="left" label-width="110">
        <n-form-item label="确认输入">
          <n-input
            v-model:value="uninstallConfirmName"
            :placeholder="`输入工具名 ${uninstallTarget?.displayName ?? ''} 确认`"
          />
        </n-form-item>
      </n-form>
      <template #footer>
        <div class="footer">
          <n-button :disabled="uninstalling" @click="uninstallTarget = null">取消</n-button>
          <n-button
            type="error"
            :disabled="!uninstallConfirmOk"
            :loading="uninstalling"
            @click="onUninstall"
          >
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
  grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
  gap: 16px;
}
.tool-card :deep(.n-card__content) {
  min-height: 170px;
}
.tool-title {
  font-weight: 600;
}
.tool-desc {
  font-size: 13px;
  opacity: 0.85;
}
.tool-catalog {
  font-size: 12px;
  opacity: 0.6;
  margin-top: 4px;
}
.tool-row {
  margin-top: 10px;
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
}
.bound-instance {
  font-size: 12px;
  opacity: 0.75;
}
.deps-list {
  margin: 6px 0 0;
  padding-left: 18px;
  font-size: 12px;
}
.deps-link {
  margin-left: 8px;
  font-weight: 500;
}
.deps-hint {
  opacity: 0.65;
}
.tool-actions {
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
