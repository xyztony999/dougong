<script setup lang="ts">
/**
 * MySQL 表详情页 mysql-table-detail（PageSpec v1.4 6.5，路由
 * /databases/mysql/:instanceId/browse/:database/:table；query.tab 定位初始 Tab，默认数据浏览）。
 * Tab 1 数据浏览：admin 专属数据内容（Q6/I-11，rowList admin GET）——readonly 负空间
 * NEmpty「数据内容仅管理员可见」，不请求 rowList、无分页器（readonly 文档 6.5）。
 * 列头=结构列序动态渲染；null 显式 NTag「NULL」（JSON null≠TS undefined）；长文本截断+tooltip。
 * 行写仅单列主键表开放（singlePk 派生 I-10；false 时编辑/删除禁用态+tooltip，DBM_023 服务端兜底）。
 * Tab 2 结构：元数据两角色全可看（tableStructure）；只读呈现无操作列。
 * 分页 page/pageSize 默认 20 上限 100（TABLE_PAGE_LIMIT，D-22）；本轮无插入行入口（OUT-12）。
 */
import { computed, h, onMounted, ref, watch } from 'vue'
import type { VNode } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useMessage } from 'naive-ui'
import { NButton, NPopconfirm, NTag, NTooltip } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { rowDelete, rowList, tableStructure } from '@/api/database'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { useDbStore } from '@/stores/database'
import type { RowCellValue, TableRow, TableStructureData } from '@/types/database'
import {
  BROWSE_PAGE_SIZES,
  BROWSE_PAGE_SIZE_DEFAULT,
  NO_SINGLE_PK_TOOLTIP,
  dbmErrorText
} from '@/types/database'
import { formatDateTime } from '@/utils/format'
import { ConnStateTag, EngineTag, identityText } from '../render'
import RowEditModal from './components/RowEditModal.vue'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const authStore = useAuthStore()
const dbStore = useDbStore()

const instanceId = computed(() => Number(route.params.instanceId))
/** route.params 原值（vue-router 已 URL decode；不二次解码——含 % 名称安全） */
const database = computed(() => String(route.params.database ?? ''))
const tableName = computed(() => String(route.params.table ?? ''))
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

/** ---- Tab 框架（query.tab 定位初始 Tab；readonly 数据浏览 Tab 可达呈负空间） ---- */

/** query.tab 定位初始 Tab；admin 默认数据浏览，readonly 默认结构（数据区负空间，readonly 文档 6.5） */
const tabValue = ref<'data' | 'structure'>(
  route.query.tab === 'structure'
    ? 'structure'
    : route.query.tab === 'data'
      ? 'data'
      : authStore.isAdmin
        ? 'data'
        : 'structure'
)

/** ---- 结构（元数据两角色；singlePk/pkColumn 行写禁用判定同源 I-10） ---- */

const structure = ref<TableStructureData | null>(null)
const structureLoading = ref(false)

async function loadStructure(): Promise<void> {
  if (!governable.value) return
  structureLoading.value = true
  try {
    structure.value = await tableStructure(instanceId.value, database.value, tableName.value)
  } catch (e) {
    toastDbm(e)
    if (e instanceof ApiError && e.is('DBM_022')) backToTables()
  } finally {
    structureLoading.value = false
  }
}

/** 表不存在（DBM_022）→ toast + 回表清单刷新（第 1 节分流） */
function backToTables(): void {
  void router.replace(
    `/databases/mysql/${instanceId.value}/browse/${encodeURIComponent(database.value)}`
  )
}

const singlePk = computed(() => structure.value?.singlePk === true)
const pkColumn = computed(() => structure.value?.pkColumn ?? null)

/** ---- Tab 1 数据浏览（admin 专属；readonly 不请求 rowList——Q6 负空间） ---- */

const columns = ref<string[]>([])
const rows = ref<TableRow[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(BROWSE_PAGE_SIZE_DEFAULT)
const rowsLoading = ref(false)
const rowsGeneratedAt = ref<string | null>(null)

async function loadRows(): Promise<void> {
  // readonly 数据内容负空间：不请求 rowList（沿 redis key 先例，I-11/D-24）
  if (!authStore.isAdmin || !governable.value) return
  rowsLoading.value = true
  try {
    const data = await rowList({
      instanceId: instanceId.value,
      database: database.value,
      table: tableName.value,
      page: page.value,
      pageSize: pageSize.value
    })
    columns.value = data.columns
    rows.value = data.rows
    total.value = data.total
    page.value = data.page
    pageSize.value = data.pageSize
    rowsGeneratedAt.value = data.generatedAt
  } catch (e) {
    toastDbm(e)
    if (e instanceof ApiError && e.is('DBM_022')) backToTables()
  } finally {
    rowsLoading.value = false
  }
}

function onPageChange(p: number): void {
  page.value = p
  void loadRows()
}

function onPageSizeChange(size: number): void {
  pageSize.value = size
  page.value = 1
  void loadRows()
}

/** 单元格呈现：null 显式 NTag(gray)「NULL」（JSON null≠TS undefined）；其余文本形态 */
function cellNode(value: RowCellValue): VNode | string {
  if (value === null) {
    return h(NTag, { size: 'small', bordered: false }, { default: () => 'NULL' })
  }
  return String(value)
}

/** 数值列右对齐（结构类型派生；字符串列左对齐） */
function isNumericColumn(col: string): boolean {
  const def = structure.value?.columns.find((c) => c.column === col)
  if (def === undefined) return false
  return /^(tinyint|smallint|mediumint|int|integer|bigint|decimal|numeric|float|double|real|bit)/i.test(
    def.type
  )
}

/** 行主键定位值（单列主键恒非空；缺失兜底 null 禁用行写不虚构） */
function pkValueOf(row: TableRow): string | null {
  if (pkColumn.value === null) return null
  const v = row[pkColumn.value]
  return v === null || v === undefined ? null : String(v)
}

/** ---- 行编辑（5.12 弹窗）/行删除（5.15 强确认）——singlePk=false 禁用态+tooltip ---- */

const editTarget = ref<TableRow | null>(null)
const editShow = ref(false)

function openEdit(row: TableRow): void {
  editTarget.value = row
  editShow.value = true
}

async function onDeleteRow(row: TableRow): Promise<void> {
  const pk = pkValueOf(row)
  if (pk === null) return
  try {
    await rowDelete({
      instanceId: instanceId.value,
      database: database.value,
      table: tableName.value,
      pkValue: pk
    })
    message.success('行已删除')
    void loadRows()
  } catch (e) {
    toastDbm(e)
    if (e instanceof ApiError && (e.is('DBM_024') || e.is('DBM_022'))) void loadRows()
  }
}

/** 禁用态按钮包 tooltip（如实提示无单列主键原因，I-10/DBM_023 同源） */
function disabledWithTip(label: string): VNode {
  return h(
    NTooltip,
    null,
    {
      trigger: () =>
        h(
          'span',
          null,
          h(NButton, { size: 'small', disabled: true }, { default: () => label })
        ),
      default: () => NO_SINGLE_PK_TOOLTIP
    }
  )
}

const dataColumns = computed<DataTableColumns<TableRow>>(() => {
  const cols: DataTableColumns<TableRow> = columns.value.map((col) => ({
    title: col,
    key: col,
    width: 160,
    align: isNumericColumn(col) ? ('right' as const) : ('left' as const),
    ellipsis: { tooltip: true },
    render: (row: TableRow) => cellNode(row[col] ?? null)
  }))
  if (authStore.isAdmin) {
    cols.push({
      title: '操作',
      key: 'actions',
      width: 140,
      fixed: 'right',
      render: (row) => {
        if (!singlePk.value || pkValueOf(row) === null) {
          return [disabledWithTip('编辑'), disabledWithTip('删除')]
        }
        return [
          h(
            NButton,
            { size: 'small', onClick: () => openEdit(row) },
            { default: () => '编辑' }
          ),
          h(
            NPopconfirm,
            { onPositiveClick: () => void onDeleteRow(row) },
            {
              trigger: () =>
                h(
                  NButton,
                  { size: 'small', type: 'error', ghost: true },
                  { default: () => '删除' }
                ),
              default: () => `确认删除该行（${pkColumn.value}=${pkValueOf(row)}）？不可恢复。`
            }
          )
        ]
      }
    })
  }
  return cols
})

const dataScrollX = computed(() => columns.value.length * 160 + (authStore.isAdmin ? 140 : 0))

/** 行键=当前页内引用序号（无单列主键表亦可呈现；页重拉即重建） */
function rowKey(row: TableRow): number {
  return rows.value.indexOf(row)
}

/** ---- Tab 2 结构（元数据两角色全等；key=引擎原值开放字符串，PRI 高亮） ---- */

const structureColumns: DataTableColumns<TableStructureData['columns'][number]> = [
  { title: '列名', key: 'column', width: 180 },
  { title: '类型', key: 'type', width: 180 },
  {
    title: '可空',
    key: 'nullable',
    width: 80,
    render: (row) =>
      h(
        NTag,
        { size: 'small', bordered: false, type: row.nullable ? 'default' : 'info' },
        { default: () => (row.nullable ? 'YES' : 'NO') }
      )
  },
  {
    title: '键',
    key: 'key',
    width: 80,
    render: (row) =>
      row.key === ''
        ? '—'
        : h(
            NTag,
            { size: 'small', bordered: false, type: row.key === 'PRI' ? 'warning' : 'default' },
            { default: () => row.key }
          )
  },
  { title: '默认', key: 'default', width: 140, render: (row) => row.default ?? '—' },
  { title: '额外', key: 'extra', width: 140, render: (row) => (row.extra === '' ? '—' : row.extra) },
  {
    title: '注释',
    key: 'comment',
    width: 200,
    ellipsis: { tooltip: true },
    render: (row) => row.comment ?? '—'
  }
]

/** ---- Tab 切换懒加载（结构首次进入拉取；数据浏览 admin 首次进入拉取） ---- */

const structureLoaded = ref(false)
const dataLoaded = ref(false)

function ensureTabLoaded(name: 'data' | 'structure'): void {
  if (!governable.value) return
  if (name === 'structure' && !structureLoaded.value) {
    structureLoaded.value = true
    void loadStructure()
  } else if (name === 'data' && authStore.isAdmin && !dataLoaded.value) {
    dataLoaded.value = true
    void loadRows()
  }
}

watch(tabValue, (v) => ensureTabLoaded(v))
watch(governable, (v) => {
  if (v) ensureTabLoaded(tabValue.value)
})

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
    <!-- 摘要条 + 库名/表名标注 + singlePk 状态标注（PageSpec 6.5） -->
    <n-card size="small">
      <div class="summary">
        <template v-if="instance !== null">
          <EngineTag :engine="instance.engine" />
          <span class="identity">{{ identityText(instance) }}</span>
          <ConnStateTag :conn-state="instance.connState" />
          <n-tag size="small" :bordered="false">库 {{ database }}</n-tag>
          <n-tag size="small" :bordered="false" type="info">表 {{ tableName }}</n-tag>
          <n-tooltip v-if="structure !== null && !structure.singlePk">
            <template #trigger>
              <n-tag size="small" :bordered="false">无单列主键——行级编辑/删除不可用</n-tag>
            </template>
            {{ NO_SINGLE_PK_TOOLTIP }}
          </n-tooltip>
        </template>
        <span class="spacer" />
        <n-button text size="small" @click="router.push('/databases')">
          返回实例清单
        </n-button>
      </div>
    </n-card>

    <n-alert v-if="instance !== null && !governable" type="warning" title="实例无法管理">
      {{
        instance.connState !== 'reachable'
          ? '实例连接不可达——管理和查看操作将无法执行（实例启停请前往服务管理）。'
          : '实例未配置账号密码——请先在实例清单配置账号密码后再管理。'
      }}
    </n-alert>

    <n-card size="small">
      <n-tabs v-model:value="tabValue" type="line">
        <!-- Tab 1 数据浏览（admin 专属数据内容；readonly 负空间不请求 rowList） -->
        <n-tab-pane name="data" tab="数据浏览">
          <template v-if="authStore.isAdmin">
            <div class="toolbar">
              <n-button size="small" :loading="rowsLoading" @click="loadRows()">刷新</n-button>
              <!-- 本轮无「插入行」入口（OUT-12） -->
              <span class="meta">快照时间 {{ formatDateTime(rowsGeneratedAt) }}</span>
            </div>
            <n-data-table
              :columns="dataColumns"
              :data="rows"
              :loading="rowsLoading"
              :scroll-x="dataScrollX"
              :row-key="rowKey"
              :max-height="480"
            >
              <template #empty>
                <n-empty description="该表暂无数据" class="empty" />
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
                <template #prefix>共 {{ total }} 行</template>
              </n-pagination>
            </div>
          </template>
          <!-- readonly 数据内容负空间（Q6/I-11：NEmpty 呈现，非禁用置灰；无分页器） -->
          <n-empty v-else description="数据内容仅管理员可见" class="empty" />
        </n-tab-pane>

        <!-- Tab 2 结构（元数据两角色全等，只读呈现无操作列） -->
        <n-tab-pane name="structure" tab="结构">
          <n-spin :show="structureLoading">
            <div class="meta-line">
              快照时间 {{ formatDateTime(structure?.generatedAt ?? null) }}
            </div>
            <n-data-table
              :columns="structureColumns"
              :data="structure?.columns ?? []"
              :scroll-x="1100"
              :row-key="(row: TableStructureData['columns'][number]) => row.column"
              size="small"
            >
              <template #empty>
                <n-empty description="暂无结构数据" class="empty" />
              </template>
            </n-data-table>
          </n-spin>
        </n-tab-pane>
      </n-tabs>
    </n-card>

    <!-- 5.12 行编辑弹窗（admin，singlePk=true 入口） -->
    <RowEditModal
      v-model:show="editShow"
      :instance-id="instanceId"
      :database="database"
      :table="tableName"
      :structure="structure"
      :row="editTarget"
      @updated="loadRows()"
      @stale="loadRows()"
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
.meta-line {
  font-size: 12px;
  opacity: 0.6;
  margin-bottom: 8px;
  text-align: right;
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
</style>
