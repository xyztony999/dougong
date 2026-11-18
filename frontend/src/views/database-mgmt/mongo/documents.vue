<script setup lang="ts">
/**
 * MongoDB 文档浏览页 mongo-documents（PageSpec v1.4 6.7，路由
 * /databases/mongo/:instanceId/browse/:database/:collection）。
 * admin 专属数据内容（Q6/I-11，mongoDocumentList admin GET）——readonly 负空间
 * NEmpty「数据内容仅管理员可见」，不请求 mongoDocumentList、无工具栏/分页器（readonly 文档 6.7）。
 * 文档 JSON 呈现：_id 置顶列（ObjectId {"$oid":"hex"} 缩写+tooltip 全文）+ JSON 摘要列
 * （单行截断 ellipsis+tooltip 格式化全文）；BSON→JSON 扩展形态透传（JSON null≠TS undefined）。
 * 分页 page/pageSize 默认 20 上限 100（DOC_PAGE_LIMIT，D-23）；total=CountDocuments。
 * 工具栏（admin）：插入文档（5.14 插入态）+刷新；行操作：编辑（5.14 编辑态）/删除（5.15 强确认）。
 * 错误分流：DBM_025 集合不存在 → toast + 回集合清单；DBM_027 文档不存在 → toast + 刷新。
 */
import { computed, h, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useMessage } from 'naive-ui'
import { NButton, NPopconfirm, NTooltip } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { mongoDocumentDelete, mongoDocumentList } from '@/api/database'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { useDbStore } from '@/stores/database'
import type { MongoDocument } from '@/types/database'
import {
  BROWSE_PAGE_SIZES,
  BROWSE_PAGE_SIZE_DEFAULT,
  dbmErrorText,
  docWithIdFirst,
  formatDocId
} from '@/types/database'
import { formatDateTime } from '@/utils/format'
import { ConnStateTag, EngineTag, identityText } from '../render'
import DocumentEditModal from './components/DocumentEditModal.vue'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const authStore = useAuthStore()
const dbStore = useDbStore()

const instanceId = computed(() => Number(route.params.instanceId))
/** route.params 原值（vue-router 已 URL decode；不二次解码——含 % 名称安全） */
const database = computed(() => String(route.params.database ?? ''))
const collection = computed(() => String(route.params.collection ?? ''))
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

/** ---- 文档分页（admin 专属；readonly 不请求——Q6 负空间） ---- */

const documents = ref<MongoDocument[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(BROWSE_PAGE_SIZE_DEFAULT)
const loading = ref(false)
const generatedAt = ref<string | null>(null)

async function loadDocuments(): Promise<void> {
  if (!authStore.isAdmin || !governable.value) return
  loading.value = true
  try {
    const data = await mongoDocumentList({
      instanceId: instanceId.value,
      database: database.value,
      collection: collection.value,
      page: page.value,
      pageSize: pageSize.value
    })
    documents.value = data.documents
    total.value = data.total
    page.value = data.page
    pageSize.value = data.pageSize
    generatedAt.value = data.generatedAt
  } catch (e) {
    toastDbm(e)
    if (e instanceof ApiError && e.is('DBM_025')) backToCollections()
  } finally {
    loading.value = false
  }
}

/** 集合不存在（DBM_025）→ toast + 回集合清单刷新（第 1 节分流） */
function backToCollections(): void {
  void router.replace(
    `/databases/mongo/${instanceId.value}/browse/${encodeURIComponent(database.value)}`
  )
}

function onPageChange(p: number): void {
  page.value = p
  void loadDocuments()
}

function onPageSizeChange(size: number): void {
  pageSize.value = size
  page.value = 1
  void loadDocuments()
}

/** ---- 插入/编辑（5.14 一弹窗两态）/删除（5.15 强确认） ---- */

const editMode = ref<'insert' | 'edit'>('insert')
const editTarget = ref<MongoDocument | null>(null)
const editShow = ref(false)

function openInsert(): void {
  editMode.value = 'insert'
  editTarget.value = null
  editShow.value = true
}

function openEdit(row: MongoDocument): void {
  editMode.value = 'edit'
  editTarget.value = row
  editShow.value = true
}

async function onDeleteDocument(row: MongoDocument): Promise<void> {
  try {
    await mongoDocumentDelete({
      instanceId: instanceId.value,
      database: database.value,
      collection: collection.value,
      id: row._id ?? null
    })
    message.success('文档已删除')
    void loadDocuments()
  } catch (e) {
    toastDbm(e)
    if (e instanceof ApiError && (e.is('DBM_027') || e.is('DBM_025'))) void loadDocuments()
  }
}

/** ---- 表格列（_id 置顶 + JSON 摘要 + 操作；admin only——readonly 整页负空间无表格） ---- */

const columns = computed<DataTableColumns<MongoDocument>>(() => [
  {
    title: '_id',
    key: '_id',
    width: 240,
    fixed: 'left',
    render: (row) =>
      h(
        NTooltip,
        null,
        {
          trigger: () => h('span', { class: 'id-cell' }, formatDocId(row._id ?? null)),
          default: () => JSON.stringify(row._id ?? null)
        }
      )
  },
  {
    title: '文档 JSON',
    key: 'json',
    ellipsis: { tooltip: false },
    render: (row) => {
      const ordered = docWithIdFirst(row)
      return h(
        NTooltip,
        { contentStyle: 'max-width: 560px;' },
        {
          trigger: () => h('div', { class: 'json-summary' }, JSON.stringify(ordered)),
          default: () => h('pre', { class: 'json-full' }, JSON.stringify(ordered, null, 2))
        }
      )
    }
  },
  {
    title: '操作',
    key: 'actions',
    width: 140,
    fixed: 'right',
    render: (row) => [
      h(
        NButton,
        { size: 'small', onClick: () => openEdit(row) },
        { default: () => '编辑' }
      ),
      h(
        NPopconfirm,
        { onPositiveClick: () => void onDeleteDocument(row) },
        {
          trigger: () =>
            h(
              NButton,
              { size: 'small', type: 'error', ghost: true },
              { default: () => '删除' }
            ),
          default: () => `确认删除该文档（_id=${formatDocId(row._id ?? null)}）？不可恢复。`
        }
      )
    ]
  }
])

function rowKey(row: MongoDocument): string {
  return JSON.stringify(row._id ?? null)
}

onMounted(() => {
  if (instance.value === null) {
    message.error('实例档案不存在（可能已被移除），已返回实例清单')
    void router.replace('/databases')
    return
  }
  void loadDocuments()
})
</script>

<template>
  <div class="page">
    <!-- 摘要条 + 库名/集合名标注（PageSpec 6.7；connState tooltip 同 4.1 助手） -->
    <n-card size="small">
      <div class="summary">
        <template v-if="instance !== null">
          <EngineTag :engine="instance.engine" />
          <span class="identity">{{ identityText(instance) }}</span>
          <ConnStateTag :conn-state="instance.connState" />
          <n-tag size="small" :bordered="false">库 {{ database }}</n-tag>
          <n-tag size="small" :bordered="false" type="info">集合 {{ collection }}</n-tag>
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

    <n-card size="small" :title="`文档浏览 · ${collection}`">
      <template #header-extra>
        <span v-if="authStore.isAdmin" class="meta">
          共 {{ total }} 篇文档 · 快照时间 {{ formatDateTime(generatedAt) }}
        </span>
      </template>
      <!-- admin 数据内容面；readonly 整区负空间（不请求 mongoDocumentList，Q6/I-11） -->
      <template v-if="authStore.isAdmin">
        <div class="toolbar">
          <n-button type="primary" size="small" :disabled="!governable" @click="openInsert">
            插入文档
          </n-button>
          <n-button size="small" :loading="loading" :disabled="!governable" @click="loadDocuments()">
            刷新
          </n-button>
        </div>
        <n-data-table
          :columns="columns"
          :data="documents"
          :loading="loading"
          :scroll-x="900"
          :row-key="rowKey"
          :max-height="480"
        >
          <template #empty>
            <n-empty description="该集合暂无文档" class="empty">
              <template #extra>
                <n-button size="small" :disabled="!governable" @click="openInsert">
                  插入文档
                </n-button>
              </template>
            </n-empty>
          </template>
        </n-data-table>
        <div class="pager">
          <n-pagination
            :page="page"
            :page-size="pageSize"
            :item-count="total"
            :page-sizes="BROWSE_PAGE_SIZES"
            show-size-picker
            @update:page="onPageChange"
            @update:page-size="onPageSizeChange"
          >
            <template #prefix>共 {{ total }} 篇文档</template>
          </n-pagination>
        </div>
      </template>
      <!-- readonly 数据内容负空间（NEmpty 呈现，非禁用置灰；无工具栏/分页器） -->
      <n-empty v-else description="数据内容仅管理员可见" class="empty" />
    </n-card>

    <!-- 5.14 文档插入/编辑弹窗（admin，一弹窗两态复用） -->
    <DocumentEditModal
      v-model:show="editShow"
      :instance-id="instanceId"
      :database="database"
      :collection="collection"
      :mode="editMode"
      :doc="editTarget"
      @saved="loadDocuments()"
      @stale="loadDocuments()"
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
.pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
}
.empty {
  padding: 24px 0;
}
.id-cell {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
}
.json-summary {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
}
</style>

<style>
/* tooltip 内格式化全文（非 scoped——NTooltip 渲染于 body 传送门） */
.json-full {
  margin: 0;
  max-height: 320px;
  overflow: auto;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
