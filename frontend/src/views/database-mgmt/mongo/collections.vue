<script setup lang="ts">
/**
 * MongoDB 集合清单页 mongo-collections（PageSpec v1.4 6.6，路由
 * /databases/mongo/:instanceId/browse/:database；engineGuard 同链校验 engine=mongodb）。
 * 元数据 Q6：admin+readonly 全可看（mongoCollectionList 两角色）；readonly 工具栏无「新建集合」、
 * 操作列整列不渲染（浏览文档经集合名行点击进入文档浏览页；删除集合负空间）。
 * 工具栏（admin）：新建集合（5.13 弹窗，隐式建库说明）+刷新。
 * 错误分流：DBM_025 集合不存在 → toast + 刷新集合清单（第 1 节）。
 */
import { computed, h, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useMessage } from 'naive-ui'
import { NButton, NPopconfirm, NTooltip } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { mongoCollectionDrop, mongoCollectionList } from '@/api/database'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { useDbStore } from '@/stores/database'
import type { MongoCollectionItem } from '@/types/database'
import { dbmErrorText } from '@/types/database'
import { formatBytes, formatDateTime } from '@/utils/format'
import { ConnStateTag, EngineTag, identityText } from '../render'
import CreateCollectionModal from './components/CreateCollectionModal.vue'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const authStore = useAuthStore()
const dbStore = useDbStore()

const instanceId = computed(() => Number(route.params.instanceId))
/** route.params 原值（vue-router 已 URL decode；不二次解码——含 % 名称安全） */
const database = computed(() => String(route.params.database ?? ''))
const instance = computed(() => dbStore.findById(instanceId.value))

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

/** ---- 集合清单（实况投影 I-04；generatedAt 快照标注） ---- */

const collections = ref<MongoCollectionItem[]>([])
const loading = ref(false)
const generatedAt = ref<string | null>(null)

async function loadCollections(): Promise<void> {
  if (!governable.value) return
  loading.value = true
  try {
    const data = await mongoCollectionList(instanceId.value, database.value)
    collections.value = data.items
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

/** ---- 进入文档浏览页（6.7） ---- */

function navigateToCollection(name: string): void {
  void router.push(
    `/databases/mongo/${instanceId.value}/browse/${encodeURIComponent(database.value)}/${encodeURIComponent(name)}`
  )
}

/** ---- 删除集合（5.15：dropCollection 不可逆强确认；DBM_025 如实提示+刷新） ---- */

async function onDropCollection(row: MongoCollectionItem): Promise<void> {
  try {
    await mongoCollectionDrop({
      instanceId: instanceId.value,
      database: database.value,
      name: row.name
    })
    message.success(`集合 ${row.name} 已删除（不可逆）`)
    void loadCollections()
  } catch (e) {
    toastDbm(e)
    if (e instanceof ApiError && e.is('DBM_025')) void loadCollections()
  }
}

/** ---- 新建集合（5.13 弹窗，admin） ---- */

const createShow = ref(false)

/** ---- 表格列（PageSpec 6.6；文档数列头 tooltip 估算语义） ---- */

const columns = computed<DataTableColumns<MongoCollectionItem>>(() => {
  const cols: DataTableColumns<MongoCollectionItem> = [
    { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
    {
      title: '集合名',
      key: 'name',
      width: 280,
      fixed: 'left',
      render: (row) =>
        h(
          NButton,
          { text: true, type: 'primary', onClick: stopAnd(() => navigateToCollection(row.name)) },
          { default: () => row.name }
        )
    },
    {
      title: () =>
        h(
          NTooltip,
          null,
          {
            trigger: () => h('span', null, '文档数'),
            default: () => 'estimatedDocumentCount 估算值'
          }
        ),
      key: 'docCount',
      width: 120,
      align: 'right'
    },
    {
      title: '大小',
      key: 'sizeBytes',
      width: 120,
      align: 'right',
      render: (row) => formatBytes(row.sizeBytes)
    }
  ]
  // readonly 操作列整列不渲染（浏览文档经集合名行点击；删除集合负空间）
  if (authStore.isAdmin) {
    cols.push({
      title: '操作',
      key: 'actions',
      width: 200,
      fixed: 'right',
      render: (row) => [
        h(
          NButton,
          { size: 'small', onClick: stopAnd(() => navigateToCollection(row.name)) },
          { default: () => '浏览文档' }
        ),
        h(
          NPopconfirm,
          { onPositiveClick: () => void onDropCollection(row) },
          {
            trigger: () =>
              h(
                NButton,
                { size: 'small', type: 'error', ghost: true, onClick: stopAnd(() => undefined) },
                { default: () => '删除集合' }
              ),
            default: () =>
              `不可逆：连同集合内全部文档移除（dropCollection）${row.name}，确认？`
          }
        )
      ]
    })
  }
  return cols
})

const scrollX = computed(() => (authStore.isAdmin ? 780 : 580))

function rowProps(row: MongoCollectionItem): { onClick: () => void } {
  return { onClick: () => navigateToCollection(row.name) }
}

onMounted(() => {
  if (instance.value === null) {
    message.error('实例档案不存在（可能已被移除），已返回实例清单')
    void router.replace('/databases')
    return
  }
  void loadCollections()
})
</script>

<template>
  <div class="page">
    <!-- 实例摘要条 + 库名标注（PageSpec 6.6；connState tooltip 同 4.1 助手） -->
    <n-card size="small">
      <div class="summary">
        <template v-if="instance !== null">
          <EngineTag :engine="instance.engine" />
          <span class="identity">{{ identityText(instance) }}</span>
          <ConnStateTag :conn-state="instance.connState" />
          <n-tag size="small" :bordered="false">库 {{ database }}</n-tag>
        </template>
        <span class="spacer" />
        <n-button text size="small" @click="router.push('/databases')">
          返回实例清单
        </n-button>
      </div>
    </n-card>

    <n-alert v-if="instance !== null && !governable" type="warning" title="实例不可治理">
      {{
        instance.connState !== 'reachable'
          ? '实例连接不可达——治理与观测操作将无法执行（实例启停请前往服务管理）。'
          : '实例未配置凭据——请先在实例清单配置凭据后治理。'
      }}
    </n-alert>

    <n-card size="small" :title="`集合清单 · ${database}`">
      <template #header-extra>
        <span class="meta">
          共 {{ collections.length }} 个集合 · 快照时间 {{ formatDateTime(generatedAt) }}
        </span>
      </template>
      <div class="toolbar">
        <!-- 新建集合 admin only（readonly 负空间）；刷新两角色保留 -->
        <n-button
          v-if="authStore.isAdmin"
          type="primary"
          size="small"
          :disabled="!governable"
          @click="createShow = true"
        >
          新建集合
        </n-button>
        <n-button size="small" :loading="loading" :disabled="!governable" @click="loadCollections()">
          刷新
        </n-button>
      </div>
      <n-data-table
        :columns="columns"
        :data="collections"
        :loading="loading"
        :scroll-x="scrollX"
        :row-key="(row: MongoCollectionItem) => row.name"
        :row-props="rowProps"
      >
        <template #empty>
          <n-empty
            description="该库暂无集合——新建集合即完成建库（MongoDB 库随首集合写入产生）"
            class="empty"
          />
        </template>
      </n-data-table>
    </n-card>

    <!-- 5.13 新建集合弹窗（admin，含隐式建库说明文案） -->
    <CreateCollectionModal
      v-model:show="createShow"
      :instance-id="instanceId"
      :database="database"
      @created="loadCollections()"
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
