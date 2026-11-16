<script setup lang="ts">
import { computed, h, onMounted, reactive, ref } from 'vue'
import { useMessage } from 'naive-ui'
import { NButton, NTag, NTooltip } from 'naive-ui'
import type { DataTableColumns, PaginationProps, SelectOption } from 'naive-ui'
import { auditList, exportAuditLog } from '@/api/auth'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import type { AuditCategory } from '@/types/enums'
import { AUDIT_ACTION_TEXT, AUDIT_CATEGORY, AUDIT_RESULT, AUDIT_SOURCE, dictItem } from '@/types/enums'
import type { AuditFilter, AuditLogItem } from '@/types/auth'
import { formatDateTime, toIsoUtc } from '@/utils/format'
import AuditDetailModal from './components/AuditDetailModal.vue'

const message = useMessage()
const authStore = useAuthStore()

const records = ref<AuditLogItem[]>([])
const total = ref(0)
const loading = ref(false)

/** 筛选条件（api 4.3.4 query：from/to/actorName/category/action/objectType/objectId） */
const range = ref<[number, number] | null>(null)
const actorName = ref('')
const category = ref<AuditCategory | null>(null)
const action = ref<string | null>(null)
const objectType = ref('')
const objectId = ref('')

const query = reactive({ page: 1, pageSize: 20 })

const detailItem = ref<AuditLogItem | null>(null)
const exporting = ref(false)

const categoryOptions: SelectOption[] = (Object.keys(AUDIT_CATEGORY) as AuditCategory[]).map(
  (value) => ({ label: AUDIT_CATEGORY[value].label, value })
)

const actionOptions: SelectOption[] = Object.entries(AUDIT_ACTION_TEXT).map(([value, label]) => ({
  label: `${label}（${value}）`,
  value
}))

/** 组装筛选参数（时间范围 ISO UTC 转换、to 闭合至当日末） */
function buildFilter(): AuditFilter {
  return {
    ...(range.value !== null
      ? { from: toIsoUtc(range.value[0]), to: toIsoUtc(range.value[1], true) }
      : {}),
    ...(actorName.value !== '' ? { actorName: actorName.value } : {}),
    ...(category.value !== null ? { category: category.value } : {}),
    ...(action.value !== null ? { action: action.value } : {}),
    ...(objectType.value !== '' ? { objectType: objectType.value } : {}),
    ...(objectId.value !== '' ? { objectId: objectId.value } : {})
  }
}

const columns: DataTableColumns<AuditLogItem> = [
  { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
  {
    title: '时间',
    key: 'createdAt',
    width: 180,
    fixed: 'left',
    render: (row) => formatDateTime(row.createdAt)
  },
  { title: '操作者', key: 'actorName', width: 120, ellipsis: { tooltip: true } },
  {
    title: '类别',
    key: 'category',
    width: 100,
    render: (row) => {
      const item = dictItem(AUDIT_CATEGORY, row.category)
      return h(NTag, { type: item.tag, size: 'small', bordered: false }, { default: () => item.label })
    }
  },
  {
    title: '动作',
    key: 'action',
    width: 140,
    render: (row) =>
      h(
        NTooltip,
        null,
        {
          trigger: () => h('span', { class: 'mono' }, row.action),
          default: () => AUDIT_ACTION_TEXT[row.action] ?? row.action
        }
      )
  },
  {
    title: '对象',
    key: 'object',
    width: 180,
    ellipsis: { tooltip: true },
    render: (row) => (row.objectId !== null ? `${row.objectType}:${row.objectId}` : row.objectType)
  },
  {
    title: '结果',
    key: 'result',
    width: 90,
    render: (row) => {
      const item = dictItem(AUDIT_RESULT, row.result)
      return h(NTag, { type: item.tag, size: 'small', bordered: false }, { default: () => item.label })
    }
  },
  {
    title: '来源',
    key: 'source',
    width: 90,
    render: (row) => dictItem(AUDIT_SOURCE, row.source).label
  },
  {
    title: '详情',
    key: 'detail',
    width: 80,
    render: (row) =>
      h(
        NButton,
        {
          size: 'small',
          text: true,
          type: 'primary',
          // detail 为 null 时禁用（PageSpec 4.3）
          disabled: row.detail === null,
          onClick: () => {
            detailItem.value = row
          }
        },
        { default: () => '查看' }
      )
  }
]

/** 排序固定 createdAt desc（API 无排序参数）；remote 分页 page 从 1 起 */
async function load(): Promise<void> {
  loading.value = true
  try {
    const data = await auditList({ ...buildFilter(), page: query.page, pageSize: query.pageSize })
    records.value = data.items
    total.value = data.total
  } catch {
    // 错误已由拦截器统一提示
  } finally {
    loading.value = false
  }
}

function onSearch(): void {
  query.page = 1
  void load()
}

function onReset(): void {
  range.value = null
  actorName.value = ''
  category.value = null
  action.value = null
  objectType.value = ''
  objectId.value = ''
  onSearch()
}

const pagination = computed<PaginationProps>(() => ({
  page: query.page,
  pageSize: query.pageSize,
  itemCount: total.value,
  showSizePicker: true,
  pageSizes: [20, 50, 100],
  onChange: (page: number) => {
    query.page = page
    void load()
  },
  onUpdatePageSize: (pageSize: number) => {
    query.pageSize = pageSize
    query.page = 1
    void load()
  }
}))

/** 导出 CSV（admin only）：按当前筛选触发下载；超限（AUTH_012）提示缩小时间范围 */
async function onExport(): Promise<void> {
  exporting.value = true
  try {
    await exportAuditLog(buildFilter())
    message.success('导出成功，导出动作已记入审计')
  } catch (e) {
    if (e instanceof ApiError && e.is('AUTH_012')) {
      message.warning('导出行数超限（上限 100,000 行），请缩小时间范围后重试')
    }
    // 其余错误已由拦截器统一提示
  } finally {
    exporting.value = false
  }
}

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="page">
    <n-card title="筛选">
      <n-form
        label-placement="left"
        :show-feedback="false"
        inline
        @keyup.enter="onSearch"
      >
        <n-form-item label="时间范围">
          <n-date-picker
            v-model:value="range"
            type="daterange"
            clearable
            style="width: 260px"
          />
        </n-form-item>
        <n-form-item label="操作者">
          <n-input v-model:value="actorName" placeholder="操作者用户名" clearable style="width: 160px" />
        </n-form-item>
        <n-form-item label="类别">
          <n-select
            v-model:value="category"
            :options="categoryOptions"
            clearable
            placeholder="全部"
            style="width: 120px"
          />
        </n-form-item>
        <n-form-item label="动作">
          <n-select
            v-model:value="action"
            :options="actionOptions"
            filterable
            clearable
            placeholder="全部"
            style="width: 200px"
          />
        </n-form-item>
        <n-form-item label="对象类型">
          <n-input v-model:value="objectType" placeholder="如 user" clearable style="width: 140px" />
        </n-form-item>
        <n-form-item label="对象标识">
          <n-input v-model:value="objectId" placeholder="如 3" clearable style="width: 140px" />
        </n-form-item>
        <n-form-item>
          <n-button type="primary" @click="onSearch">查询</n-button>
        </n-form-item>
        <n-form-item>
          <n-button @click="onReset">重置</n-button>
        </n-form-item>
      </n-form>
    </n-card>

    <n-card title="审计日志">
      <template #header-extra>
        <!-- 导出 CSV（admin only；readonly 无工具栏——负空间） -->
        <n-button v-if="authStore.isAdmin" :loading="exporting" @click="onExport">
          导出 CSV
        </n-button>
      </template>
      <n-data-table
        :columns="columns"
        :data="records"
        :loading="loading"
        remote
        :pagination="pagination"
        :scroll-x="1140"
        :row-key="(row: AuditLogItem) => row.id"
      />
    </n-card>

    <audit-detail-modal
      :item="detailItem"
      @close="detailItem = null"
    />
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.mono {
  font-family: var(--n-font-family-mono, ui-monospace, monospace);
  font-size: 12px;
}
</style>
