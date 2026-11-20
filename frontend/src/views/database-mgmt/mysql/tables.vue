<script setup lang="ts">
/**
 * MySQL 表清单页 mysql-tables（PageSpec v1.4 6.4，路由
 * /databases/mysql/:instanceId/browse/:database；engineGuard 同链校验 engine∈mysql/mariadb）。
 * 元数据 Q6：admin+readonly 全可看（tableList 两角色）；readonly 操作列整列不渲染
 * （浏览数据/结构入口经表名行点击进入表详情页；删除表负空间）。
 * 本轮无新建入口（OUT-12 不做建表）；刷新保留（查询参数非变更）。
 * 错误分流：DBM_022 表不存在 → toast + 刷新表清单（第 1 节）。
 */
import { computed, h, onMounted, ref } from 'vue'
import type { VNode } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useMessage } from 'naive-ui'
import { NButton, NPopconfirm, NTooltip } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { tableDrop, tableList } from '@/api/database'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { useDbStore } from '@/stores/database'
import type { DataTableItem } from '@/types/database'
import { dbmErrorText } from '@/types/database'
import { formatBytes, formatDateTime } from '@/utils/format'
import { ConnStateTag, EngineTag, identityText } from '../render'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const authStore = useAuthStore()
const dbStore = useDbStore()

const instanceId = computed(() => Number(route.params.instanceId))
/** route.params 原值（vue-router 已 URL decode；不二次解码——含 % 名称安全） */
const database = computed(() => String(route.params.database ?? ''))
const instance = computed(() => dbStore.findById(instanceId.value))

/** 治理前置（I-05 呈现层）：不可达/未配置账号密码 → 横幅前置，不拉取 */
const governable = computed(
  () =>
    instance.value !== null &&
    instance.value.connState === 'reachable' &&
    instance.value.credentialsSet
)

function toastDbm(e: unknown): void {
  if (!(e instanceof ApiError)) return
  const text = dbmErrorText(e.code, e.data)
  if (text !== null) message.error(text)
}

/** ---- 表清单（information_schema 实况投影 I-04；generatedAt 快照标注） ---- */

const tables = ref<DataTableItem[]>([])
const loading = ref(false)
const generatedAt = ref<string | null>(null)

async function loadTables(): Promise<void> {
  if (!governable.value) return
  loading.value = true
  try {
    const data = await tableList(instanceId.value, database.value)
    tables.value = data.items
    generatedAt.value = data.generatedAt
  } catch (e) {
    toastDbm(e)
  } finally {
    loading.value = false
  }
}

/** 行点击与行内按钮互不串扰（db-list stopAnd 先例） */
function stopAnd(fn: () => void): (e: MouseEvent) => void {
  return (e: MouseEvent) => {
    e.stopPropagation()
    fn()
  }
}

/** ---- 进入表详情页（6.5；query.tab 定位初始 Tab，默认数据浏览） ---- */

function navigateToTable(name: string, tab?: 'structure'): void {
  const base = `/databases/mysql/${instanceId.value}/browse/${encodeURIComponent(database.value)}/${encodeURIComponent(name)}`
  void router.push(tab === 'structure' ? `${base}?tab=structure` : base)
}

/** ---- 删除表（5.15：DROP TABLE 不可逆强确认；DBM_022 如实提示+刷新） ---- */

async function onDropTable(row: DataTableItem): Promise<void> {
  try {
    await tableDrop({
      instanceId: instanceId.value,
      database: database.value,
      table: row.name
    })
    message.success(`表 ${row.name} 已删除（不可逆）`)
    void loadTables()
  } catch (e) {
    toastDbm(e)
    if (e instanceof ApiError && e.is('DBM_022')) void loadTables()
  }
}

/** ---- 表格列（PageSpec 6.4；行数(估)列头 tooltip 估算语义） ---- */

const columns = computed<DataTableColumns<DataTableItem>>(() => {
  const cols: DataTableColumns<DataTableItem> = [
    { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
    {
      title: '表名',
      key: 'name',
      width: 240,
      fixed: 'left',
      render: (row) =>
        h(
          NButton,
          { text: true, type: 'primary', onClick: stopAnd(() => navigateToTable(row.name)) },
          { default: () => row.name }
        )
    },
    { title: '存储引擎', key: 'engine', width: 110 },
    {
      title: () =>
        h(
          NTooltip,
          null,
          {
            trigger: () => h('span', null, '行数(估)'),
            default: () => 'InnoDB 统计估算值，非精确计数'
          }
        ),
      key: 'rowEstimate',
      width: 110,
      align: 'right',
      render: (row) => (row.rowEstimate === null ? '—' : String(row.rowEstimate))
    },
    {
      title: '数据大小',
      key: 'dataSizeBytes',
      width: 120,
      align: 'right',
      render: (row) => formatBytes(row.dataSizeBytes)
    },
    {
      title: '排序规则',
      key: 'collation',
      width: 160,
      render: (row) => row.collation ?? '—'
    },
    {
      title: '注释',
      key: 'comment',
      width: 200,
      ellipsis: { tooltip: true },
      render: (row) => row.comment ?? '—'
    }
  ]
  // readonly 操作列整列不渲染（浏览数据/结构经表名行点击；删除表负空间）
  if (authStore.isAdmin) {
    cols.push({
      title: '操作',
      key: 'actions',
      width: 240,
      fixed: 'right',
      render: (row): VNode[] => [
        h(
          NButton,
          { size: 'small', onClick: stopAnd(() => navigateToTable(row.name)) },
          { default: () => '浏览数据' }
        ),
        h(
          NButton,
          { size: 'small', onClick: stopAnd(() => navigateToTable(row.name, 'structure')) },
          { default: () => '结构' }
        ),
        h(
          NPopconfirm,
          { onPositiveClick: () => void onDropTable(row) },
          {
            trigger: () =>
              h(
                NButton,
                { size: 'small', type: 'error', ghost: true, onClick: stopAnd(() => undefined) },
                { default: () => '删除表' }
              ),
            default: () =>
              `不可逆：连同表内数据与结构一并删除（DROP TABLE）${row.name}，确认？`
          }
        )
      ]
    })
  }
  return cols
})

const scrollX = computed(() => (authStore.isAdmin ? 1240 : 1000))

function rowProps(row: DataTableItem): { onClick: () => void } {
  return { onClick: () => navigateToTable(row.name) }
}

onMounted(() => {
  if (instance.value === null) {
    message.error('实例档案不存在（可能已被移除），已返回实例清单')
    void router.replace('/databases')
    return
  }
  void loadTables()
})
</script>

<template>
  <div class="page">
    <!-- 实例摘要条 + 库名标注（PageSpec 6.4；connState tooltip 同 4.1 助手） -->
    <n-card size="small">
      <div class="summary">
        <template v-if="instance !== null">
          <EngineTag :engine="instance.engine" />
          <span class="identity">{{ identityText(instance) }}</span>
          <ConnStateTag :conn-state="instance.connState" />
          <n-tag size="small" :bordered="false">库 {{ database }}</n-tag>
        </template>
        <span class="spacer" />
        <!-- 冗余返回入口（7.2：面包屑为主，文字按钮可选保留） -->
        <n-button text size="small" @click="router.push('/databases')">
          返回实例清单
        </n-button>
      </div>
    </n-card>

    <n-alert
      v-if="instance !== null && !governable"
      type="warning"
      title="实例无法管理"
    >
      {{
        instance.connState !== 'reachable'
          ? '实例连接不可达——管理和查看操作将无法执行（实例启停请前往服务管理）。'
          : '实例未配置账号密码——请先在实例清单配置账号密码后再管理。'
      }}
    </n-alert>

    <n-card size="small" :title="`表清单 · ${database}`">
      <template #header-extra>
        <span class="meta">
          共 {{ tables.length }} 张表 · 快照时间 {{ formatDateTime(generatedAt) }}
        </span>
      </template>
      <!-- 工具栏：无新建入口（OUT-12 本轮不做建表）；刷新两角色保留 -->
      <div class="toolbar">
        <n-button size="small" :loading="loading" :disabled="!governable" @click="loadTables()">
          刷新
        </n-button>
      </div>
      <n-data-table
        :columns="columns"
        :data="tables"
        :loading="loading"
        :scroll-x="scrollX"
        :row-key="(row: DataTableItem) => row.name"
        :row-props="rowProps"
      >
        <template #empty>
          <n-empty description="该库暂无表" class="empty" />
        </template>
      </n-data-table>
    </n-card>
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
.meta {
  font-size: 12px;
  opacity: 0.6;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}
.empty {
  padding: 24px 0;
}
</style>
