<script setup lang="ts">
/**
 * MongoDB 治理页 mongo-manage（PageSpec v1.2 6.3，路由 /databases/mongo/:instanceId；
 * engineGuard 已在路由守卫校验 engine=mongodb）。hybrid 三 Tab：
 * 状态概览（两视角全等只读）/数据库（只读投影 D-18——隐式建库语义无建/删库，
 * admin 亦无工具栏与操作列）/用户（admin 工具栏+操作列）。
 * readonly 负空间（readonly 文档 6.3）：无「新建用户」按钮、无用户 Tab 工具栏与操作列。
 * 无页面级改密入口（MongoDB 改密经用户治理承载，MVP 不设面板建连账户改密同步）。
 * 刷新纪律（D-04/I-04）：进入/切 Tab 拉取，写动作成功重拉。
 */
import { computed, h, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useMessage } from 'naive-ui'
import { NButton, NPopconfirm, NTag } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { mongoDatabaseList, mongoStatus, mongoUserDelete, mongoUserList } from '@/api/database'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { useDbStore } from '@/stores/database'
import { MONGO_ROLE, dictItem } from '@/types/enums'
import type {
  MongoDatabaseItem,
  MongoStatusData,
  MongoUserItem
} from '@/types/database'
import { dbmErrorText } from '@/types/database'
import { formatBytes, formatDateTime } from '@/utils/format'
import { ConnStateTag, EngineTag, identityText } from '../render'
import MongoCreateUserModal from './components/MongoCreateUserModal.vue'
import MongoRoleManageModal from './components/MongoRoleManageModal.vue'

const route = useRoute()
const message = useMessage()
const authStore = useAuthStore()
const dbStore = useDbStore()

/** ---- 实例档案（守卫已确保加载） ---- */

const instanceId = computed(() => Number(route.params.instanceId))
const instance = computed(() => dbStore.findById(instanceId.value))

const governable = computed(
  () => instance.value !== null && instance.value.connState === 'reachable' && instance.value.credentialsSet
)

function toastDbm(e: unknown): void {
  if (!(e instanceof ApiError)) return
  const text = dbmErrorText(e.code, e.data)
  if (text !== null) message.error(text)
}

/** ---- Tab 框架（懒加载：进入 Tab 首次拉取；写动作成功重拉当前 Tab） ---- */

const tabValue = ref<'status' | 'databases' | 'users'>('status')
const loadedTabs = ref<Set<string>>(new Set())

function ensureTabLoaded(name: string): void {
  if (loadedTabs.value.has(name) || !governable.value) return
  loadedTabs.value.add(name)
  if (name === 'status') void loadStatus()
  else if (name === 'databases') void loadDatabases()
  else void loadUsers()
}

watch(tabValue, (v) => ensureTabLoaded(v))
watch(governable, (v) => {
  if (v) ensureTabLoaded(tabValue.value)
})

/** ---- Tab 1 状态概览（serverStatus 白名单 D-08 同构；两视角全等） ---- */

const status = ref<MongoStatusData | null>(null)
const statusLoading = ref(false)

async function loadStatus(): Promise<void> {
  statusLoading.value = true
  try {
    status.value = await mongoStatus(instanceId.value)
  } catch (e) {
    toastDbm(e) // DBM_022（权限不足）/DBM_003 等
  } finally {
    statusLoading.value = false
  }
}

const statusItems = computed(() => {
  const s = status.value
  if (s === null) return []
  return [
    { label: '版本', value: s.version },
    { label: '运行时长（秒）', value: String(s.uptimeSec) },
    { label: '当前连接数', value: String(s.connectionsCurrent) },
    { label: '可用连接数', value: String(s.connectionsAvailable) },
    { label: '常驻内存', value: `${s.memResidentMb} MB` },
    { label: 'insert 计数', value: String(s.opInsert) },
    { label: 'query 计数', value: String(s.opQuery) },
    { label: 'update 计数', value: String(s.opUpdate) },
    { label: 'delete 计数', value: String(s.opDelete) }
  ]
})

/** ---- Tab 2 数据库（只读投影 D-18：无工具栏/无操作列——admin 与 readonly 一致） ---- */

const databases = ref<MongoDatabaseItem[]>([])
const databasesLoading = ref(false)
const databasesGeneratedAt = ref<string | null>(null)

async function loadDatabases(): Promise<void> {
  databasesLoading.value = true
  try {
    const data = await mongoDatabaseList(instanceId.value)
    databases.value = data.items
    databasesGeneratedAt.value = data.generatedAt
  } catch (e) {
    toastDbm(e)
  } finally {
    databasesLoading.value = false
  }
}

const dbColumns: DataTableColumns<MongoDatabaseItem> = [
  { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
  { title: '库名', key: 'name', width: 260 },
  {
    title: '大小',
    key: 'sizeBytes',
    width: 140,
    align: 'right',
    render: (row) => formatBytes(row.sizeBytes)
  },
  { title: '集合数', key: 'collections', width: 120, align: 'right' }
]

/** ---- Tab 3 用户（admin 工具栏：新建用户；操作列：角色管理/删除；无密码字段 I-02） ---- */

const users = ref<MongoUserItem[]>([])
const usersLoading = ref(false)
const usersGeneratedAt = ref<string | null>(null)

async function loadUsers(): Promise<void> {
  usersLoading.value = true
  try {
    const data = await mongoUserList(instanceId.value)
    users.value = data.items
    usersGeneratedAt.value = data.generatedAt
  } catch (e) {
    toastDbm(e)
  } finally {
    usersLoading.value = false
  }
}

const createUserShow = ref(false)

/** 角色管理（5.10：一弹窗两区，复用 users 响应 roles 数据；DBM_011 如实提示） */
const roleTarget = ref<MongoUserItem | null>(null)
const roleShow = ref(false)

function openRoleManage(row: MongoUserItem): void {
  roleTarget.value = row
  roleShow.value = true
}

function onRoleChanged(): void {
  void loadUsers() // 授予/回收成功重拉（D-04）
}

/** 删除用户（5.11：连带回收全部角色，引擎语义） */
const deletingUser = ref<string | null>(null)

async function onDeleteUser(row: MongoUserItem): Promise<void> {
  deletingUser.value = row.username
  try {
    await mongoUserDelete({ instanceId: instanceId.value, username: row.username })
    message.success(`用户 ${row.username} 已删除（其全部库级角色一并回收）`)
    void loadUsers()
  } catch (e) {
    toastDbm(e) // DBM_009 不存在如实提示
  } finally {
    deletingUser.value = null
  }
}

/** 角色概况 Tag 列表（`库名:角色`；非三档形态原样呈现不映射——「未识别不呈现不回收」判据不适用，如实展示 raw） */
function roleTags(roles: MongoUserItem['roles']): import('vue').VNode[] {
  if (roles.length === 0) return [h('span', { class: 'meta' }, '—')]
  return roles.map((r) => {
    const d = dictItem(MONGO_ROLE, r.role as keyof typeof MONGO_ROLE)
    return h(
      NTag,
      {
        size: 'small',
        bordered: false,
        type: d !== undefined ? d.tag : 'default',
        class: 'role-tag'
      },
      { default: () => `${r.db}:${d !== undefined ? d.label : r.role}` }
    )
  })
}

const userColumns = computed<DataTableColumns<MongoUserItem>>(() => {
  const cols: DataTableColumns<MongoUserItem> = [
    { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
    { title: '用户名', key: 'username', width: 200 },
    {
      title: '认证机制',
      key: 'mechanisms',
      width: 150,
      render: (row) =>
        row.mechanisms.length === 0
          ? h('span', { class: 'meta' }, '—')
          : row.mechanisms.join(' / ')
    },
    {
      title: '角色概况',
      key: 'roles',
      render: (row) => h('div', { class: 'role-list' }, roleTags(row.roles))
    }
  ]
  if (authStore.isAdmin) {
    cols.push({
      title: '操作',
      key: 'actions',
      width: 200,
      fixed: 'right',
      render: (row) => [
        h(
          NButton,
          { size: 'small', onClick: () => openRoleManage(row) },
          { default: () => '角色管理' }
        ),
        h(
          NPopconfirm,
          { onPositiveClick: () => void onDeleteUser(row) },
          {
            trigger: () =>
              h(
                NButton,
                {
                  size: 'small',
                  type: 'error',
                  ghost: true,
                  disabled: deletingUser.value !== null,
                  loading: deletingUser.value === row.username
                },
                { default: () => '删除' }
              ),
            default: () =>
              `删除用户 ${row.username}？其全部库级角色将一并回收（引擎语义）。`
          }
        )
      ]
    })
  }
  return cols
})

onMounted(() => {
  ensureTabLoaded(tabValue.value)
})
</script>

<template>
  <div class="page" v-if="instance !== null">
    <!-- 摘要条（engine tag green；无页面级动作——无改密入口） -->
    <n-card size="small">
      <div class="head-row">
        <EngineTag :engine="instance.engine" />
        <span class="identity">{{ identityText(instance) }}</span>
        <ConnStateTag :connState="instance.connState" />
        <span class="meta">版本 {{ instance.version ?? '—' }}</span>
      </div>
    </n-card>

    <!-- unreachable / 未配置凭据前置横幅（I-05 前置链呈现层；DBM_003/004 语义） -->
    <n-alert v-if="!governable" type="warning" title="实例不可治理">
      {{
        instance.connState !== 'reachable'
          ? '实例连接不可达——治理与观测操作将无法执行（实例启停请前往服务管理）。'
          : '实例未配置凭据——请先在实例清单配置凭据后治理。'
      }}
    </n-alert>

    <n-card size="small">
      <n-tabs v-model:value="tabValue" type="line" :disabled="!governable">
        <!-- Tab 1 状态概览（两视角全等） -->
        <n-tab-pane name="status" tab="状态概览">
          <n-spin :show="statusLoading">
            <n-descriptions
              v-if="statusItems.length > 0"
              label-placement="left"
              :column="3"
              size="small"
              bordered
            >
              <n-descriptions-item v-for="it in statusItems" :key="it.label" :label="it.label">
                {{ it.value }}
              </n-descriptions-item>
            </n-descriptions>
            <n-empty v-else description="暂无状态数据" size="small" style="padding: 24px 0" />
          </n-spin>
        </n-tab-pane>

        <!-- Tab 2 数据库（只读投影 D-18：无工具栏/操作列，admin 与 readonly 一致） -->
        <n-tab-pane name="databases" tab="数据库">
          <div class="meta-row">
            <span class="meta" v-if="databasesGeneratedAt !== null">
              共 {{ databases.length }} 个库 · 快照 {{ formatDateTime(databasesGeneratedAt) }} ·
              只读投影（MongoDB 隐式建库，无建/删库操作）
            </span>
          </div>
          <n-data-table
            :columns="dbColumns"
            :data="databases"
            :loading="databasesLoading"
            :row-key="(r: MongoDatabaseItem) => r.name"
            size="small"
          >
            <template #empty>
              <n-empty description="实例内暂无数据库（MongoDB 库随首文档写入产生）" size="small" />
            </template>
          </n-data-table>
        </n-tab-pane>

        <!-- Tab 3 用户（admin 工具栏+操作列；readonly 负空间） -->
        <n-tab-pane name="users" tab="用户">
          <div class="meta-row">
            <span class="meta" v-if="usersGeneratedAt !== null">
              共 {{ users.length }} 个用户 · 快照 {{ formatDateTime(usersGeneratedAt) }}
            </span>
            <div class="spacer"></div>
            <n-button
              v-if="authStore.isAdmin"
              size="small"
              type="primary"
              @click="createUserShow = true"
            >
              新建用户
            </n-button>
          </div>
          <n-data-table
            :columns="userColumns"
            :data="users"
            :loading="usersLoading"
            :row-key="(r: MongoUserItem) => r.username"
            size="small"
          >
            <template #empty>
              <n-empty description="实例内暂无用户" size="small" />
            </template>
          </n-data-table>
        </n-tab-pane>
      </n-tabs>
    </n-card>

    <!-- 5.9 建用户弹窗（admin） -->
    <MongoCreateUserModal
      v-model:show="createUserShow"
      :instance-id="instanceId"
      :databases="databases"
      @created="loadUsers"
    />

    <!-- 5.10 角色管理弹窗（admin） -->
    <MongoRoleManageModal
      v-model:show="roleShow"
      :instance-id="instanceId"
      :user="roleTarget"
      :databases="databases"
      @changed="onRoleChanged"
    />
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
  flex-wrap: wrap;
}
.identity {
  font-weight: 600;
}
.meta {
  font-size: 12px;
  opacity: 0.65;
}
.meta-row {
  display: flex;
  align-items: center;
  margin-bottom: 10px;
  gap: 12px;
}
.spacer {
  flex: 1;
}
.role-list {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
.role-tag {
  margin: 0;
}
</style>
