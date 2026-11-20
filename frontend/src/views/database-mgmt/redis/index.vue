<script setup lang="ts">
/**
 * Redis 治理页 redis-manage（PageSpec 6.2，路由 /databases/redis/:instanceId；
 * engineGuard 已在路由守卫校验 engine=redis）。hybrid：Tab1 状态概览 + Tab2 慢日志（两角色）；
 * Tab3 key 浏览 admin 专属——readonly 不渲染本 Tab（D0 口径：key 值属数据内容取出，
 * 负空间呈现=不渲染非禁用置灰）。页面头部：实例摘要条 + 设置密码（admin，5.7）。
 */
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useDialog, useMessage } from 'naive-ui'
import type { DataTableColumns, SelectOption } from 'naive-ui'
import { redisInfo, redisPassword, redisSlowlog } from '@/api/database'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { useDbStore } from '@/stores/database'
import type { RedisInfoData, RedisSlowlogItem } from '@/types/database'
import { REDIS_INFO_FIELD_TEXT, dbmErrorText } from '@/types/database'
import { formatDateTime } from '@/utils/format'
import InstanceCredentialsModal from '../components/InstanceCredentialsModal.vue'
import PasswordSetModal from '../components/PasswordSetModal.vue'
import { ConnStateTag, CredentialsTag, EngineTag, identityText } from '../render'
import KeyBrowse from './components/KeyBrowse.vue'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const dialog = useDialog()
const authStore = useAuthStore()
const dbStore = useDbStore()

const instanceId = computed(() => Number(route.params.instanceId))
const instance = computed(() => dbStore.findById(instanceId.value))

/** 治理/观测前置（I-05 前置链呈现层）：不可达或未配置账号密码 → 横幅 + Tab 数据不自动拉取 */
const governable = computed(
  () => instance.value !== null && instance.value.connState === 'reachable' && instance.value.credentialsSet
)

function toastDbm(e: unknown): void {
  if (!(e instanceof ApiError)) return
  const text = dbmErrorText(e.code, e.data)
  if (text !== null) message.error(text)
}

/** ---- DBM_019 强提示（api D-05，5.7 处置同 5.5）：直达 5.1 凭据弹窗恢复 ---- */

const credentialsShow = ref(false)

function handlePasswordSyncError(): void {
  dialog.error({
    title: '密码已修改，但面板保存的账号密码同步失败',
    content:
      'Redis 密码已修改成功，但面板保存的账号密码同步失败（数据库不会回滚）——请立即重新配置账号密码，否则后续管理将不可用。',
    positiveText: '重新配置账号密码',
    negativeText: '稍后处理',
    onPositiveClick: () => {
      credentialsShow.value = true
    }
  })
}

/** ---- 设置密码（5.7，admin） ---- */

const passwordShow = ref(false)
const passwordSubmitting = ref(false)

async function onSetPassword(newPassword: string): Promise<void> {
  passwordSubmitting.value = true
  try {
    await redisPassword({ instanceId: instanceId.value, newPassword })
    message.success('Redis 密码已设置，面板账号密码已同步')
    passwordShow.value = false
  } catch (e) {
    if (e instanceof ApiError && e.is('DBM_019')) {
      passwordShow.value = false
      handlePasswordSyncError()
      return
    }
    toastDbm(e)
  } finally {
    passwordSubmitting.value = false
  }
}

/** ---- Tab 框架（懒加载；readonly 仅两 Tab——key Tab 不渲染非禁用） ---- */

type TabName = 'info' | 'slowlog' | 'keys'
const tabValue = ref<TabName>('info')
const loadedTabs = ref<Set<string>>(new Set())

function ensureTabLoaded(name: TabName): void {
  if (loadedTabs.value.has(name) || !governable.value) return
  loadedTabs.value.add(name)
  if (name === 'info') void loadInfo()
  else if (name === 'slowlog') void loadSlowlog()
  // keys：组件内自带查询触发（不自动拉取，PageSpec 6.2 工具栏「查询」起始）
}

watch(tabValue, (v) => ensureTabLoaded(v))
watch(governable, (v) => {
  if (v) ensureTabLoaded(tabValue.value)
})

/** ---- Tab 1 状态概览（info 白名单分组 NDescriptions，D-08） ---- */

const info = ref<RedisInfoData | null>(null)
const infoLoading = ref(false)

async function loadInfo(): Promise<void> {
  infoLoading.value = true
  try {
    info.value = await redisInfo(instanceId.value)
  } catch (e) {
    toastDbm(e)
  } finally {
    infoLoading.value = false
  }
}

interface InfoRow {
  key: string
  label: string
  value: string
}

function sectionRows(
  sectionLabel: string,
  section: Record<string, string> | undefined,
  keys: readonly string[]
): { section: string; rows: InfoRow[] } {
  const entries =
    section === undefined
      ? []
      : keys.filter((k) => section[k] !== undefined).map((k) => ({ key: k, value: section[k] }))
  return {
    section: sectionLabel,
    rows: entries.map((e) => ({ key: e.key, label: REDIS_INFO_FIELD_TEXT[e.key] ?? e.key, value: e.value }))
  }
}

const infoGroups = computed<{ section: string; rows: InfoRow[] }[]>(() => {
  if (info.value === null) return []
  const s = info.value.sections
  return [
    sectionRows('server', s.server, ['version']),
    sectionRows('clients', s.clients, ['connectedClients']),
    sectionRows('memory', s.memory, ['usedMemoryHuman', 'maxMemoryHuman']),
    sectionRows('stats', s.stats, ['totalConnectionsReceived', 'keyspaceHits', 'keyspaceMisses'])
  ]
})

/** ---- Tab 2 慢日志（条数选择 50/100/200，D-09 SLOWLOG_LIMIT 上限 200） ---- */

const slowlogItems = ref<RedisSlowlogItem[]>([])
const slowlogLoading = ref(false)
const slowlogGeneratedAt = ref<string | null>(null)
const slowlogLimit = ref(50)

const limitOptions: SelectOption[] = [50, 100, 200].map((n) => ({ label: `${n} 条`, value: n }))

async function loadSlowlog(): Promise<void> {
  slowlogLoading.value = true
  try {
    const data = await redisSlowlog(instanceId.value, slowlogLimit.value)
    slowlogItems.value = data.items
    slowlogGeneratedAt.value = data.generatedAt
  } catch (e) {
    toastDbm(e)
  } finally {
    slowlogLoading.value = false
  }
}

/** durationUs → ms 呈现（PageSpec 6.2：右对齐，2 位小数） */
function durationMs(durationUs: number): string {
  return `${(durationUs / 1000).toFixed(2)} ms`
}

const slowlogColumns: DataTableColumns<RedisSlowlogItem> = [
  { title: 'id', key: 'id', width: 100 },
  { title: '时间', key: 'timestamp', width: 180, render: (row) => formatDateTime(row.timestamp) },
  {
    title: '耗时',
    key: 'duration',
    width: 120,
    align: 'right',
    render: (row) => durationMs(row.durationUs)
  },
  { title: '命令', key: 'command', ellipsis: { tooltip: true } }
]

function slowlogRowKey(row: RedisSlowlogItem): number {
  return row.id
}

/** ---- 挂载（守卫已加载档案；Tab1 随 governable 就绪拉取） ---- */

onMounted(() => {
  if (instance.value === null) {
    message.error('实例档案不存在（可能已被移除），已返回实例清单')
    void router.replace('/databases')
    return
  }
  ensureTabLoaded(tabValue.value)
})
</script>

<template>
  <div class="page">
    <!-- 实例摘要条（PageSpec 6.2：同 6.1）+ 设置密码（admin，5.7） -->
    <n-card size="small">
      <div class="summary">
        <template v-if="instance !== null">
          <EngineTag :engine="instance.engine" />
          <span class="identity">{{ identityText(instance) }}</span>
          <ConnStateTag :conn-state="instance.connState" />
          <span class="muted">版本 {{ instance.version ?? '—' }}</span>
          <span class="muted">
            凭据
            <CredentialsTag :credentials-set="instance.credentialsSet" />
          </span>
        </template>
        <span class="spacer" />
        <n-button v-if="authStore.isAdmin" size="small" @click="passwordShow = true">
          设置密码
        </n-button>
      </div>
    </n-card>

    <!-- unreachable NAlert 同 6.1（DBM_003 语义前置提示） -->
    <n-alert
      v-if="instance !== null && instance.connState === 'unreachable'"
      type="warning"
      title="实例不可达——管理操作将无法执行（实例启停请前往服务管理）"
    />
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
        <!-- Tab 1 状态概览（info 白名单分组） -->
        <n-tab-pane name="info" tab="状态概览">
          <n-spin :show="infoLoading">
            <n-descriptions
              v-if="infoGroups.length > 0"
              :column="1"
              label-placement="left"
              bordered
              size="small"
            >
              <template v-for="group in infoGroups" :key="group.section">
                <n-descriptions-item :label="`# ${group.section}`">
                  <span class="group-divider" />
                </n-descriptions-item>
                <n-descriptions-item
                  v-for="row in group.rows"
                  :key="row.key"
                  :label="row.label"
                >
                  {{ row.value }}
                </n-descriptions-item>
              </template>
            </n-descriptions>
            <n-empty v-else-if="!infoLoading" description="暂无状态数据" class="empty" />
          </n-spin>
        </n-tab-pane>

        <!-- Tab 2 慢日志（条数选择 50/100/200 保留——查询参数非变更，readonly 文档 6.2） -->
        <n-tab-pane name="slowlog" tab="慢日志">
          <div class="toolbar">
            <n-select
              v-model:value="slowlogLimit"
              :options="limitOptions"
              class="limit-select"
              @update:value="loadSlowlog()"
            />
            <n-button size="small" :loading="slowlogLoading" @click="loadSlowlog()">
              刷新
            </n-button>
            <span class="meta">快照时间 {{ formatDateTime(slowlogGeneratedAt) }}</span>
          </div>
          <n-data-table
            :columns="slowlogColumns"
            :data="slowlogItems"
            :loading="slowlogLoading"
            :scroll-x="800"
            :row-key="slowlogRowKey"
            :max-height="480"
          >
            <template #empty>
              <n-empty description="暂无慢日志记录" class="empty" />
            </template>
          </n-data-table>
        </n-tab-pane>

        <!-- Tab 3 key 浏览（admin 专属；readonly 不渲染本 Tab——负空间「界面不提供」口径） -->
        <n-tab-pane v-if="authStore.isAdmin" name="keys" tab="key 浏览">
          <key-browse :instance-id="instanceId" />
        </n-tab-pane>
      </n-tabs>
    </n-card>

    <!-- 弹窗组（admin 治理动作 + DBM_019 恢复入口 5.1） -->
    <instance-credentials-modal
      v-model:show="credentialsShow"
      :instance="instance"
      @saved="dbStore.applyCredentials"
    />
    <password-set-modal
      v-model:show="passwordShow"
      title="设置 Redis 密码"
      :submitting="passwordSubmitting"
      @submit="onSetPassword"
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
.limit-select {
  width: 120px;
}
.meta {
  font-size: 12px;
  opacity: 0.6;
}
.group-divider {
  display: inline-block;
}
.empty {
  padding: 24px 0;
}
</style>
