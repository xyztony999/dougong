<script setup lang="ts">
/**
 * MySQL/MariaDB 治理页 mysql-manage（PageSpec 6.1，路由 /databases/mysql/:instanceId；
 * engineGuard 已在路由守卫校验 engine∈mysql/mariadb）。hybrid 三 Tab：
 * 库（admin 工具栏+操作列）/用户（同）/状态与进程（两视角全等只读观测）。
 * readonly 负空间（readonly 文档 6.1）：无「修改 root 密码」按钮、无两 Tab 工具栏；
 * 库 Tab 操作列仅「浏览」入口（v1.4 F3 两角色可见，元数据 Q6）；
 * unreachable NAlert 与状态观测全等。刷新纪律（D-04/I-04）：进入/切 Tab 拉取，写动作成功重拉。
 * v1.4：摘要条 connState/credentialsSet Tag 复用 render.ts tooltip 助手（F2 自动获益）。
 */
import { computed, h, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useDialog, useMessage } from 'naive-ui'
import { NButton, NPopconfirm, NTag } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import {
  backupExport,
  databaseDelete,
  databaseList,
  dbuserDelete,
  dbuserList,
  mysqlProcesslist,
  mysqlRootPassword,
  mysqlStatus
} from '@/api/database'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { useDbStore } from '@/stores/database'
import { GRANT_LEVEL, dictItem } from '@/types/enums'
import type {
  DbDatabaseItem,
  DbUserItem,
  MysqlProcessItem,
  MysqlStatusData
} from '@/types/database'
import { MYSQL_STATUS_FIELD_TEXT, dbmErrorText } from '@/types/database'
import { formatBytes, formatDateTime } from '@/utils/format'
import type { VNode } from 'vue'
import InstanceCredentialsModal from '../components/InstanceCredentialsModal.vue'
import PasswordSetModal from '../components/PasswordSetModal.vue'
import AdvancedManageEntry from '../components/AdvancedManageEntry.vue'
import { ConnStateTag, CredentialsTag, EngineTag, identityText } from '../render'
import CreateDatabaseModal from './components/CreateDatabaseModal.vue'
import CreateUserModal from './components/CreateUserModal.vue'
import GrantManageModal from './components/GrantManageModal.vue'
import ImportBackupModal from './components/ImportBackupModal.vue'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const dialog = useDialog()
const authStore = useAuthStore()
const dbStore = useDbStore()

/** ---- 实例档案（守卫已确保加载；行级直更凭据后摘要条随之刷新） ---- */

const instanceId = computed(() => Number(route.params.instanceId))
const instance = computed(() => dbStore.findById(instanceId.value))

/** 治理前置（I-05 前置链呈现层）：不可达/未配置账号密码 → 横幅前置 + Tab 数据不自动拉取 */
const governable = computed(
  () => instance.value !== null && instance.value.connState === 'reachable' && instance.value.credentialsSet
)

/** ---- 错误分流（第 1 节：DBM 页面级提示） ---- */

function toastDbm(e: unknown): void {
  if (!(e instanceof ApiError)) return
  const text = dbmErrorText(e.code, e.data)
  if (text !== null) message.error(text)
}

/** ---- DBM_019 强提示（api D-05）：改密成功但面板同步失败——引导直达 5.1 弹窗 ---- */

const credentialsShow = ref(false)

function handlePasswordSyncError(): void {
  dialog.error({
    title: '密码已修改，但面板保存的账号密码同步失败',
    content:
      '实例密码已修改成功，但面板保存的账号密码同步失败（数据库不会回滚）——请立即重新配置账号密码，否则后续管理将不可用。',
    positiveText: '重新配置账号密码',
    negativeText: '稍后处理',
    onPositiveClick: () => {
      credentialsShow.value = true
    }
  })
}

/** ---- 修改 root 密码（5.5，admin） ---- */

const passwordShow = ref(false)
const passwordSubmitting = ref(false)

async function onRootPassword(newPassword: string): Promise<void> {
  passwordSubmitting.value = true
  try {
    await mysqlRootPassword({ instanceId: instanceId.value, newPassword })
    message.success('root 密码已修改，面板账号密码已同步')
    passwordShow.value = false
  } catch (e) {
    if (e instanceof ApiError && e.is('DBM_019')) {
      passwordShow.value = false
      handlePasswordSyncError()
      return
    }
    toastDbm(e) // 其余失败弹窗保持打开（可重试）
  } finally {
    passwordSubmitting.value = false
  }
}

/** ---- Tab 框架（懒加载：进入 Tab 首次拉取，此后保留；写动作成功重拉当前 Tab） ---- */

const tabValue = ref<'databases' | 'users' | 'observe'>('databases')
const loadedTabs = ref<Set<string>>(new Set())

function ensureTabLoaded(name: string): void {
  if (loadedTabs.value.has(name) || !governable.value) return
  loadedTabs.value.add(name)
  if (name === 'databases') void loadDatabases()
  else if (name === 'users') void loadUsers()
  else {
    void loadStatus()
    void loadProcesslist()
  }
}

watch(tabValue, (v) => ensureTabLoaded(v))
watch(governable, (v) => {
  if (v) ensureTabLoaded(tabValue.value)
})

/** ---- Tab 1 数据库（admin 工具栏：新建数据库；操作列：导出/导入/删除） ---- */

const databases = ref<DbDatabaseItem[]>([])
const databasesLoading = ref(false)
const databasesGeneratedAt = ref<string | null>(null)

async function loadDatabases(): Promise<void> {
  databasesLoading.value = true
  try {
    const data = await databaseList(instanceId.value)
    databases.value = data.items
    databasesGeneratedAt.value = data.generatedAt
  } catch (e) {
    toastDbm(e)
  } finally {
    databasesLoading.value = false
  }
}

const createDbShow = ref(false)

/** 「浏览」入口（v1.4 F3，PageSpec 6.1）：库行 → mysql-tables 表清单页（admin+readonly 均可见，Q6 元数据） */
function navigateToBrowse(row: DbDatabaseItem): void {
  void router.push(
    `/databases/mysql/${instanceId.value}/browse/${encodeURIComponent(row.name)}`
  )
}

/** 导出（5.8：POST 文件流直接触发；在途禁用 DBM_016；失败 DBM_012 展示 reason） */
const exportingDb = ref<string | null>(null)

async function onExport(row: DbDatabaseItem): Promise<void> {
  exportingDb.value = row.name
  try {
    await backupExport(instanceId.value, row.name)
    message.success('导出已开始下载，动作已记入审计')
  } catch (e) {
    toastDbm(e)
  } finally {
    exportingDb.value = null
  }
}

/** 导入（5.6：行内入口预选目标库） */
const importTarget = ref<string | null>(null)
const importShow = ref(false)

function onImport(row: DbDatabaseItem): void {
  importTarget.value = row.name
  importShow.value = true
}

/** 删除库（5.8：不可逆强确认） */
async function onDeleteDatabase(row: DbDatabaseItem): Promise<void> {
  try {
    await databaseDelete(instanceId.value, row.name)
    message.success(`数据库 ${row.name} 已删除（不可逆）`)
    void loadDatabases()
  } catch (e) {
    toastDbm(e)
  }
}

const dbColumns = computed<DataTableColumns<DbDatabaseItem>>(() => {
  const cols: DataTableColumns<DbDatabaseItem> = [
    { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
    { title: '库名', key: 'name', width: 240 },
    { title: '字符集', key: 'charset', width: 140 }
  ]
  // v1.4 F3：操作列两角色均渲染——「浏览」入口可见（Q6 元数据）；
  // readonly 仅浏览（导出/导入/删除负空间，readonly 文档 6.1）
  const browseBtn = (row: DbDatabaseItem): VNode =>
    h(
      NButton,
      { size: 'small', onClick: () => navigateToBrowse(row) },
      { default: () => '浏览' }
    )
  if (authStore.isAdmin) {
    cols.push({
      title: '操作',
      key: 'actions',
      width: 320,
      fixed: 'right',
      render: (row) => {
        const busy = exportingDb.value !== null
        return [
          browseBtn(row),
          // v1.5 高级管理入口（PageSpec 6.1/6.8，admin only）：phpMyAdmin 打开分发
          // （库名不参与 signon 绑定——进入工具后自选库，工具内全库可达 admin 专属）
          h(AdvancedManageEntry, { instanceId: instanceId.value, engine: instance.value?.engine ?? 'mysql' }),
          h(
            NButton,
            {
              size: 'small',
              disabled: busy,
              loading: exportingDb.value === row.name,
              onClick: () => void onExport(row)
            },
            { default: () => '导出' }
          ),
          h(
            NButton,
            {
              size: 'small',
              disabled: busy,
              onClick: () => onImport(row)
            },
            { default: () => '导入' }
          ),
          h(
            NPopconfirm,
            { onPositiveClick: () => void onDeleteDatabase(row) },
            {
              trigger: () =>
                h(
                  NButton,
                  { size: 'small', type: 'error', ghost: true, disabled: busy },
                  { default: () => '删除' }
                ),
              default: () => `不可逆：${row.name} 连同库内数据一并删除，确认？`
            }
          )
        ]
      }
    })
  } else {
    cols.push({
      title: '操作',
      key: 'actions',
      width: 100,
      fixed: 'right',
      render: (row) => browseBtn(row)
    })
  }
  return cols
})

/** ---- Tab 2 用户（admin 工具栏：新建用户；操作列：授权管理/删除；无任何密码字段 I-02） ---- */

const users = ref<DbUserItem[]>([])
const usersLoading = ref(false)
const usersGeneratedAt = ref<string | null>(null)

async function loadUsers(): Promise<void> {
  usersLoading.value = true
  try {
    const data = await dbuserList(instanceId.value)
    users.value = data.items
    usersGeneratedAt.value = data.generatedAt
  } catch (e) {
    toastDbm(e)
  } finally {
    usersLoading.value = false
  }
}

const createUserShow = ref(false)

/** 授权概况列：grants → NTag 列表（`库名:档位`，三色；无授权显示 —） */
function grantsCell(row: DbUserItem): VNode[] {
  if (row.grants.length === 0) return [h('span', { class: 'muted' }, '—')]
  return row.grants.map((g) => {
    const d = dictItem(GRANT_LEVEL, g.level)
    return h(
      NTag,
      { type: d.tag, size: 'small', bordered: false, class: 'grant-tag' },
      { default: () => `${g.database}:${d.label}` }
    )
  })
}

/** 授权管理（5.4） */
const grantTarget = ref<DbUserItem | null>(null)
const grantShow = ref(false)

/** 删除用户（5.8：文案含 username@host + 既有授权一并回收） */
async function onDeleteUser(row: DbUserItem): Promise<void> {
  try {
    await dbuserDelete(instanceId.value, row.username, row.host)
    message.success(`用户 ${row.username}@${row.host} 已删除（既有授权一并回收）`)
    void loadUsers()
  } catch (e) {
    toastDbm(e)
  }
}

const userColumns = computed<DataTableColumns<DbUserItem>>(() => {
  const cols: DataTableColumns<DbUserItem> = [
    { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
    { title: '用户名', key: 'username', width: 180 },
    { title: 'host', key: 'host', width: 140 },
    { title: '授权概况', key: 'grants', render: (row) => grantsCell(row) }
  ]
  if (authStore.isAdmin) {
    cols.push({
      title: '操作',
      key: 'actions',
      width: 160,
      fixed: 'right',
      render: (row) => [
        h(
          NButton,
          {
            size: 'small',
            onClick: () => {
              grantTarget.value = row
              grantShow.value = true
            }
          },
          { default: () => '授权管理' }
        ),
        h(
          NPopconfirm,
          { onPositiveClick: () => void onDeleteUser(row) },
          {
            trigger: () =>
              h(
                NButton,
                { size: 'small', type: 'error', ghost: true },
                { default: () => '删除' }
              ),
            default: () =>
              `确认删除用户 ${row.username}@${row.host}？其既有授权将一并回收。`
          }
        )
      ]
    })
  }
  return cols
})

/** ---- Tab 3 状态与进程（两视角全等，只读观测无操作列） ---- */

const status = ref<MysqlStatusData | null>(null)
const statusLoading = ref(false)
const processes = ref<MysqlProcessItem[]>([])
const processesLoading = ref(false)
const processesGeneratedAt = ref<string | null>(null)

async function loadStatus(): Promise<void> {
  statusLoading.value = true
  try {
    status.value = await mysqlStatus(instanceId.value)
  } catch (e) {
    toastDbm(e)
  } finally {
    statusLoading.value = false
  }
}

async function loadProcesslist(): Promise<void> {
  processesLoading.value = true
  try {
    const data = await mysqlProcesslist(instanceId.value)
    processes.value = data.items
    processesGeneratedAt.value = data.generatedAt
  } catch (e) {
    toastDbm(e)
  } finally {
    processesLoading.value = false
  }
}

/** 状态概览行（D-08 白名单 ≤10 项；innodb 缓冲池字节数值转人类可读，其余原样） */
interface StatusRow {
  key: string
  label: string
  value: string
}

const statusRows = computed<StatusRow[]>(() => {
  if (status.value === null) return []
  return Object.entries(status.value.variables).map(([k, v]) => ({
    key: k,
    label: MYSQL_STATUS_FIELD_TEXT[k] ?? k,
    value:
      k === 'innodb_buffer_pool_size' && /^\d+$/.test(v) ? formatBytes(Number(v)) : v
  }))
})

const processColumns: DataTableColumns<MysqlProcessItem> = [
  { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
  { title: 'id', key: 'id', width: 90 },
  { title: 'user', key: 'user', width: 110 },
  { title: 'host', key: 'host', width: 170, ellipsis: { tooltip: true } },
  { title: 'db', key: 'db', width: 120, render: (row) => row.db ?? '—' },
  { title: 'command', key: 'command', width: 110 },
  { title: 'time(s)', key: 'timeSec', width: 90 },
  { title: 'state', key: 'state', width: 140, ellipsis: { tooltip: true }, render: (row) => row.state ?? '—' },
  { title: 'info', key: 'info', ellipsis: { tooltip: true }, render: (row) => row.info ?? '—' }
]

function processRowKey(row: MysqlProcessItem): number {
  return row.id
}

/** ---- 挂载（守卫已加载档案；Tab1 随 governable 就绪拉取） ---- */

onMounted(() => {
  if (instance.value === null) {
    // 守卫正常不可达兜底（如档案在页面驻留期间被移除）：返回列表 + toast（DBM_002 语义）
    message.error('实例档案不存在（可能已被移除），已返回实例清单')
    void router.replace('/databases')
    return
  }
  ensureTabLoaded(tabValue.value)
})
</script>

<template>
  <div class="page">
    <!-- 实例摘要条（PageSpec 6.1：identityKey + engine tag + connState tag + version） -->
    <n-card size="small">
      <div class="summary">
        <template v-if="instance !== null">
          <EngineTag :engine="instance.engine" />
          <span class="identity">{{ identityText(instance) }}</span>
          <ConnStateTag :conn-state="instance.connState" />
          <span class="muted">版本 {{ instance.version ?? '—' }}</span>
          <span class="muted">
            账号密码
            <CredentialsTag :credentials-set="instance.credentialsSet" />
          </span>
        </template>
        <span class="spacer" />
        <n-button
          v-if="authStore.isAdmin"
          size="small"
          @click="passwordShow = true"
        >
          修改 root 密码
        </n-button>
      </div>
    </n-card>

    <!-- DBM_003 语义前置提示（api 1.5：启停归 service-mgmt） -->
    <n-alert
      v-if="instance !== null && instance.connState === 'unreachable'"
      type="warning"
      title="实例不可达——管理操作将无法执行（实例启停请前往服务管理）"
    />
    <!-- DBM_004 引导（admin：直达 5.1 弹窗；readonly：待管理员配置标注） -->
    <n-alert
      v-else-if="instance !== null && !instance.credentialsSet"
      type="info"
      :title="authStore.isAdmin ? '实例未配置账号密码——管理和查看暂不可用' : '待管理员配置账号密码（配置后可管理和查看）'"
    >
      <template v-if="authStore.isAdmin" #default>
        <n-button size="small" type="primary" @click="credentialsShow = true">
          配置账号密码
        </n-button>
      </template>
    </n-alert>

    <n-card size="small">
      <n-tabs v-model:value="tabValue" type="line">
        <!-- Tab 1 数据库（readonly 无工具栏与操作列） -->
        <n-tab-pane name="databases" tab="数据库">
          <div v-if="authStore.isAdmin" class="toolbar">
            <n-button type="primary" size="small" @click="createDbShow = true">
              新建数据库
            </n-button>
            <n-button size="small" :loading="databasesLoading" @click="loadDatabases()">
              刷新
            </n-button>
          </div>
          <div v-else class="meta-line">
            快照时间 {{ formatDateTime(databasesGeneratedAt) }}
          </div>
          <n-data-table
            :columns="dbColumns"
            :data="databases"
            :loading="databasesLoading"
            :scroll-x="authStore.isAdmin ? 760 : 540"
            :row-key="(row: DbDatabaseItem) => row.name"
          >
            <template #empty>
              <n-empty description="实例内暂无数据库" class="empty" />
            </template>
          </n-data-table>
          <div v-if="authStore.isAdmin" class="meta-line">
            快照时间 {{ formatDateTime(databasesGeneratedAt) }}
          </div>
        </n-tab-pane>

        <!-- Tab 2 用户（readonly 无工具栏与操作列；无密码字段 I-02） -->
        <n-tab-pane name="users" tab="用户">
          <div v-if="authStore.isAdmin" class="toolbar">
            <n-button type="primary" size="small" @click="createUserShow = true">
              新建用户
            </n-button>
            <n-button size="small" :loading="usersLoading" @click="loadUsers()">刷新</n-button>
          </div>
          <n-data-table
            :columns="userColumns"
            :data="users"
            :loading="usersLoading"
            :scroll-x="authStore.isAdmin ? 840 : 680"
            :row-key="(row: DbUserItem) => `${row.username}@${row.host}`"
          >
            <template #empty>
              <n-empty description="实例内暂无数据库用户" class="empty" />
            </template>
          </n-data-table>
          <div class="meta-line">
            快照时间 {{ formatDateTime(usersGeneratedAt) }}
          </div>
        </n-tab-pane>

        <!-- Tab 3 状态与进程（两视角全等，只读观测） -->
        <n-tab-pane name="observe" tab="状态与进程">
          <n-spin :show="statusLoading">
            <n-card size="small" title="状态概览" class="observe-card">
              <n-descriptions
                v-if="status !== null"
                :column="2"
                label-placement="left"
                bordered
                size="small"
              >
                <n-descriptions-item label="版本">
                  {{ status.version }}
                </n-descriptions-item>
                <n-descriptions-item v-for="row in statusRows" :key="row.key" :label="row.label">
                  {{ row.value }}
                </n-descriptions-item>
              </n-descriptions>
              <n-empty v-else-if="!statusLoading" description="暂无状态数据" class="empty" />
            </n-card>
          </n-spin>
          <n-card size="small" title="进程列表" class="observe-card">
            <template #header-extra>
              <span class="meta">
                共 {{ processes.length }} 条 · 快照时间 {{ formatDateTime(processesGeneratedAt) }}
              </span>
            </template>
            <div class="toolbar">
              <n-button
                size="small"
                :loading="processesLoading"
                @click="loadProcesslist()"
              >
                刷新
              </n-button>
            </div>
            <n-data-table
              :columns="processColumns"
              :data="processes"
              :loading="processesLoading"
              :scroll-x="1090"
              :row-key="processRowKey"
              :max-height="420"
            >
              <template #empty>
                <n-empty description="当前无连接进程" class="empty" />
              </template>
            </n-data-table>
          </n-card>
        </n-tab-pane>
      </n-tabs>
    </n-card>

    <!-- 弹窗组（admin 治理动作） -->
    <instance-credentials-modal
      v-model:show="credentialsShow"
      :instance="instance"
      @saved="dbStore.applyCredentials"
    />
    <password-set-modal
      v-model:show="passwordShow"
      title="修改 root 密码"
      :submitting="passwordSubmitting"
      @submit="onRootPassword"
    />
    <create-database-modal
      v-model:show="createDbShow"
      :instance-id="instanceId"
      @created="loadDatabases()"
    />
    <create-user-modal
      v-model:show="createUserShow"
      :instance-id="instanceId"
      @created="loadUsers()"
    />
    <grant-manage-modal
      v-model:show="grantShow"
      :instance-id="instanceId"
      :user="grantTarget"
      @changed="loadUsers()"
    />
    <import-backup-modal
      v-model:show="importShow"
      :instance-id="instanceId"
      :database="importTarget"
      @imported="loadDatabases()"
    />
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.summary {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.identity {
  font-weight: 500;
}
.spacer {
  flex: 1;
}
.muted {
  font-size: 12px;
  opacity: 0.65;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}
.meta-line {
  font-size: 12px;
  opacity: 0.6;
  margin: 4px 0 8px;
  text-align: right;
}
.observe-card {
  margin-bottom: 16px;
}
.empty {
  padding: 24px 0;
}
.grant-tag {
  margin-right: 6px;
}
</style>
