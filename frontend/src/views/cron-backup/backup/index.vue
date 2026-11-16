<script setup lang="ts">
/**
 * 备份页（PageSpec 4.2，路由 /cron/backup；hybrid·Tab：备份任务 / 备份历史）。
 * Tab1：工具条（新建备份任务 admin / 刷新）+ 任务表（名称/备份源 Tag 组/目标/保留份数/
 * 调度 tooltip 下次执行/最近执行派生/操作列 admin）。
 * Tab2：历史筛选（任务/状态/触发方式/时间范围 + 刷新，query 一一对应）+ remote 分页表格
 * （耗时与产物大小人性化、running loading 图标、任务已删灰字标——D-12 快照语义）。
 * readonly 子集（ADR-018 负空间）：无新建按钮、任务表操作列整列不渲染、
 * 详情 Drawer 保留（产物清单可见、下载按钮不渲染）。
 * 刷新纪律：backupJobRun 成功 → message「已开始备份（#runId）」+ 切历史 Tab 重拉 +
 * 新记录行浅蓝高亮 10s；历史列表不自动轮询（进度经详情 Drawer 5s 轮询承载）。
 */
import { computed, h, onMounted, onUnmounted, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { NButton, NPopconfirm, NSpin, NTag, NTooltip } from 'naive-ui'
import type { DataTableColumns, PaginationProps, SelectOption } from 'naive-ui'
import { backupJobDelete, backupJobRun } from '@/api/cron'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { useCronStore } from '@/stores/cron'
import {
  BACKUP_RUN_STATUS,
  BACKUP_SOURCE_TYPE,
  BACKUP_TARGET_TYPE,
  BACKUP_TRIGGER,
  dictItem
} from '@/types/enums'
import type { BackupRunStatus, BackupTrigger, DictItem } from '@/types/enums'
import { CB_ERROR_TEXT } from '@/types/cron'
import type { BackupJobListItem, BackupRunListItem } from '@/types/cron'
import { formatBytes, formatDateTime, formatDurationMs, formatRelative, toIsoUtc } from '@/utils/format'
import BackupJobDrawer from '../components/BackupJobDrawer.vue'
import RunDetailDrawer from '../components/RunDetailDrawer.vue'

const store = useCronStore()
const authStore = useAuthStore()
const message = useMessage()

/** ---- Tab 框架（懒加载：首次切历史 Tab 拉取；任务 Tab 由进入页面先行拉取） ---- */
const activeTab = ref<'jobs' | 'runs'>('jobs')

watch(activeTab, (tab) => {
  if (tab === 'runs' && !store.runLoaded) void store.loadRuns()
})

/** ---- Tab 1 表格列（4.2.2；readonly 操作列整列不渲染——负空间） ---- */

function statusTag(map: Readonly<Record<string, DictItem>>, value: string): ReturnType<typeof h> {
  const item = dictItem(map, value)
  return h(NTag, { type: item.tag, size: 'small', bordered: false }, { default: () => item.label })
}

/**
 * 备份源 Tag 组（sourceSummary 逐项：site_dir green / database blue→info；tooltip 全量 ref）。
 * Gate 2 E1 修复：s.ref 经 utils/cronback 归一恒为 string（后端未产出契约 ref——
 * path/instanceId·database 推导，无可推导 ''）；'' → 仅类型 Tag 不带引用文本；
 * type 未知值走 dictItem 兜底原值呈现（⚠️A14 口径，不崩）。
 */
function sourceCell(row: BackupJobListItem): ReturnType<typeof h> {
  return h(
    'div',
    { class: 'source-tags' },
    row.sourceSummary.map((s, i) => {
      const dict = dictItem(BACKUP_SOURCE_TYPE, s.type)
      const label =
        s.ref === ''
          ? dict.label
          : s.type === 'site_dir'
            ? `目录 ${s.ref.length > 18 ? `${s.ref.slice(0, 18)}…` : s.ref}`
            : `DB ${s.ref}`
      return h(
        NTooltip,
        { key: i },
        {
          trigger: () =>
            h(
              NTag,
              { type: dict.tag, size: 'small', bordered: false },
              { default: () => label }
            ),
          default: () => (s.ref === '' ? dict.label : s.ref)
        }
      )
    })
  )
}

/** 调度列：scheduleCron 等宽原值 + tooltip 下次执行（读时派生 D-11） */
function scheduleCell(row: BackupJobListItem): ReturnType<typeof h> {
  const inner = h('span', { class: 'mono' }, row.scheduleCron)
  if (row.nextFireAt === null) return inner
  return h(
    NTooltip,
    null,
    { trigger: () => inner, default: () => `下次执行：${formatDateTime(row.nextFireAt)}` }
  )
}

/** 最近执行列：状态 colorTag + finishedAt 相对时间灰字；null=从未执行（D-11 派生） */
function lastRunCell(row: BackupJobListItem): ReturnType<typeof h> {
  if (row.lastRun === null) return h('span', { class: 'muted' }, '从未执行')
  return h('div', { class: 'last-run' }, [
    statusTag(BACKUP_RUN_STATUS, row.lastRun.status),
    h('span', { class: 'muted meta-inline' }, formatRelative(row.lastRun.finishedAt))
  ])
}

const jobColumns = computed<DataTableColumns<BackupJobListItem>>(() => {
  const base: DataTableColumns<BackupJobListItem> = [
    { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
    { title: '名称', key: 'name', width: 160, ellipsis: { tooltip: true } },
    { title: '备份源', key: 'sourceSummary', width: 220, render: sourceCell },
    {
      title: '目标',
      key: 'target',
      width: 180,
      render: (row) =>
        h('div', { class: 'target-cell' }, [
          statusTag(BACKUP_TARGET_TYPE, row.targetType),
          h('span', { class: 'muted meta-inline mono' }, row.targetBrief)
        ])
    },
    {
      title: '保留份数',
      key: 'retentionCount',
      width: 90,
      render: (row) => `${row.retentionCount} 份`
    },
    { title: '调度', key: 'scheduleCron', width: 140, render: scheduleCell },
    { title: '最近执行', key: 'lastRun', width: 170, render: lastRunCell }
  ]
  if (authStore.isAdmin) {
    base.push({
      title: '操作',
      key: 'actions',
      width: 220,
      fixed: 'right',
      render: (row) => {
        const runKey = `backup-run:${row.id}`
        const delKey = `backup-del:${row.id}`
        const runInFlight = store.isInFlight(runKey)
        const delInFlight = store.isInFlight(delKey)
        // 前端预警：lastRun=running 禁用（服务端 I-01/CB_006 权威兜底）
        const running = row.lastRun?.status === 'running'
        const runBtn = running
          ? h(
              NTooltip,
              null,
              {
                trigger: () =>
                  h(NButton, { size: 'small', type: 'primary', ghost: true, disabled: true }, { default: () => '立即备份' }),
                default: () => '执行中'
              }
            )
          : h(
              NButton,
              {
                size: 'small',
                type: 'primary',
                ghost: true,
                loading: runInFlight,
                disabled: delInFlight,
                onClick: () => void onRunNow(row)
              },
              { default: () => '立即备份' }
            )
        return h('div', { class: 'row-actions' }, [
          runBtn,
          h(
            NButton,
            { size: 'small', disabled: runInFlight || delInFlight, onClick: () => store.openBackupForm(row) },
            { default: () => '编辑' }
          ),
          h(
            NPopconfirm,
            { onPositiveClick: () => void onDelete(row) },
            {
              trigger: () =>
                h(
                  NButton,
                  { size: 'small', type: 'error', ghost: true, loading: delInFlight, disabled: runInFlight },
                  { default: () => '删除' }
                ),
              // FR-09 不级联语义的用户可知会项（PageSpec 4.2.3 原文）
              default: () => '确认删除该备份任务？既有执行历史与产物将保留（按保留份数自然轮转）。'
            }
          )
        ])
      }
    })
  }
  return base
})

const jobScrollX = computed(() => (authStore.isAdmin ? 1240 : 1020))

function jobRowKey(row: BackupJobListItem): number {
  return row.id
}

/** ---- Tab 1 写动作（4.2.3 在途互斥；CB_006 执行互斥/CB_004 实况已删） ---- */

async function onRunNow(row: BackupJobListItem): Promise<void> {
  const key = `backup-run:${row.id}`
  store.setInFlight(key)
  try {
    const { runId } = await backupJobRun({ jobId: row.id })
    message.success(`已开始备份（执行记录 #${runId}）`)
    activeTab.value = 'runs' // 自动切换到备份历史 Tab（第 2 节纪律表）
    await store.loadRuns()
    store.markHighlight(runId) // 新记录行浅蓝高亮 10s
  } catch (e) {
    if (e instanceof ApiError && e.is('CB_006')) {
      message.warning(CB_ERROR_TEXT['CB_006']) // 执行互斥（I-01 手动侧）
      return
    }
    if (e instanceof ApiError && e.is('CB_004')) {
      message.warning(CB_ERROR_TEXT['CB_004'])
      void store.loadBackupJobs()
    }
  } finally {
    store.releaseInFlight(key)
  }
}

async function onDelete(row: BackupJobListItem): Promise<void> {
  const key = `backup-del:${row.id}`
  store.setInFlight(key)
  try {
    await backupJobDelete({ jobId: row.id })
    message.success('备份任务已删除（执行历史与产物保留至自身轮转）')
    await store.loadBackupJobs()
  } catch (e) {
    if (e instanceof ApiError && e.is('CB_004')) {
      message.warning(CB_ERROR_TEXT['CB_004'])
      void store.loadBackupJobs()
    }
  } finally {
    store.releaseInFlight(key)
  }
}

/** ---- Tab 2 备份历史（4.2.4：筛选 query 一一对应 + remote 分页） ---- */

const jobIdFilter = ref<number | null>(store.listParams.jobId)
const statusFilter = ref<BackupRunStatus | null>(store.listParams.status)
const triggerFilter = ref<BackupTrigger | null>(store.listParams.trigger)
const range = ref<[number, number] | null>(null)

const statusOptions: SelectOption[] = (Object.keys(BACKUP_RUN_STATUS) as BackupRunStatus[]).map(
  (v) => ({ label: BACKUP_RUN_STATUS[v].label, value: v })
)
const triggerOptions: SelectOption[] = (Object.keys(BACKUP_TRIGGER) as BackupTrigger[]).map(
  (v) => ({ label: BACKUP_TRIGGER[v].label, value: v })
)

/**
 * 任务下拉：options=backupJobList 动态装配；已删任务不可得——当前筛选值不在清单时
 * 以「#{id}」灰字 label 呈现（api D-12 弱引用如实）。
 */
const jobFilterOptions = computed<SelectOption[]>(() => {
  const options: SelectOption[] = store.backupJobItems.map((j) => ({
    label: j.name,
    value: j.id
  }))
  const current = jobIdFilter.value
  if (current !== null && !store.backupJobItems.some((j) => j.id === current)) {
    options.unshift({ label: `#${current}（任务已删）`, value: current, disabled: false })
  }
  return options
})

watch(jobIdFilter, (v) => store.setRunFilter({ jobId: v }))
watch(statusFilter, (v) => store.setRunFilter({ status: v }))
watch(triggerFilter, (v) => store.setRunFilter({ trigger: v }))
watch(range, (v) =>
  store.setRunFilter({
    from: v === null ? undefined : toIsoUtc(v[0]),
    to: v === null ? undefined : toIsoUtc(v[1], true)
  })
)

/** 任务列：jobName 快照 + 已删灰字小标（jobId 不在当前任务清单 → 任务已删，D-12） */
function jobDeleted(jobId: number): boolean {
  return store.backupJobItems.some((j) => j.id === jobId) === false
}

const runColumns: DataTableColumns<BackupRunListItem> = [
  { title: '序号', key: 'index', width: 60, render: (_row, index) => index + 1 },
  {
    title: '任务',
    key: 'jobName',
    width: 150,
    render: (row) =>
      h('div', { class: 'job-cell' }, [
        h('span', null, row.jobName),
        jobDeleted(row.jobId)
          ? h('span', { class: 'muted meta-inline' }, '任务已删')
          : null
      ])
  },
  {
    title: '触发方式',
    key: 'trigger',
    width: 100,
    render: (row) => statusTag(BACKUP_TRIGGER, row.trigger)
  },
  {
    title: '状态',
    key: 'status',
    width: 110,
    render: (row) =>
      h('div', { class: 'status-cell' }, [
        statusTag(BACKUP_RUN_STATUS, row.status),
        row.status === 'running' ? h(NSpin, { size: 12 }) : null // running 行附 loading 图标
      ])
  },
  { title: '开始时间', key: 'startedAt', width: 170, render: (row) => formatDateTime(row.startedAt) },
  {
    title: '耗时',
    key: 'durationMs',
    width: 100,
    render: (row) => (row.status === 'running' ? h('span', { class: 'muted' }, '—') : formatDurationMs(row.durationMs))
  },
  {
    title: '产物大小',
    key: 'artifactSize',
    width: 110,
    align: 'right',
    render: (row) => formatBytes(row.artifactSize)
  },
  {
    title: '操作',
    key: 'actions',
    width: 100,
    fixed: 'right',
    render: (row) =>
      h(
        NButton,
        {
          size: 'small',
          text: true,
          type: 'primary',
          onClick: () => store.openRunDetail(row.id)
        },
        { default: () => '详情' }
      )
  }
]

const runScrollX = 1000

function runRowKey(row: BackupRunListItem): number {
  return row.id
}

/** 立即备份新记录行浅蓝高亮（10s，PageSpec 第 2 节纪律表；rgba 随主题叠加） */
function runRowClass(row: BackupRunListItem): string {
  return store.highlightRunId === row.id ? 'fresh-row' : ''
}

const pagination = computed<PaginationProps>(() => ({
  page: store.listParams.page,
  pageSize: store.listParams.pageSize,
  itemCount: store.runTotal,
  showSizePicker: true,
  pageSizes: [20, 50, 100],
  onChange: (page: number) => store.setRunPage(page),
  onUpdatePageSize: (pageSize: number) => store.setRunPageSize(pageSize)
}))

/** ---- 生命周期（进入页面 backupJobList——历史筛选/已删判定亦依赖；卸载清 Drawer/表单/inFlight/高亮） ---- */

onMounted(() => {
  void store.loadBackupJobs()
})

onUnmounted(() => {
  store.resetBackupView()
})
</script>

<template>
  <div class="page">
    <n-card size="small">
      <n-tabs v-model:value="activeTab" type="line">
        <!-- Tab 1 备份任务（4.2.1 工具条 + 4.2.2 表格） -->
        <n-tab-pane name="jobs" tab="备份任务">
          <div class="controls-row">
            <!-- 新建入口 admin 专属（readonly 负空间：按钮不渲染） -->
            <n-button
              v-if="authStore.isAdmin"
              type="primary"
              :disabled="store.hasInFlight"
              @click="store.openBackupForm(null)"
            >
              新建备份任务
            </n-button>
            <n-button :loading="store.backupJobLoading" @click="store.loadBackupJobs()">刷新</n-button>
          </div>
          <n-data-table
            :columns="jobColumns"
            :data="store.backupJobItems"
            :loading="store.backupJobLoading"
            :scroll-x="jobScrollX"
            :row-key="jobRowKey"
          >
            <template #empty>
              <n-empty description="暂无备份任务" class="empty" />
            </template>
          </n-data-table>
        </n-tab-pane>

        <!-- Tab 2 备份历史（4.2.4：筛选 + remote 分页；无自动轮询） -->
        <n-tab-pane name="runs" tab="备份历史">
          <div class="controls-row">
            <n-select
              v-model:value="jobIdFilter"
              clearable
              filterable
              :options="jobFilterOptions"
              class="ctrl-filter"
              placeholder="任务（空=全部）"
            />
            <n-select
              v-model:value="statusFilter"
              clearable
              :options="statusOptions"
              class="ctrl-filter"
              placeholder="状态（空=全部）"
            />
            <n-select
              v-model:value="triggerFilter"
              clearable
              :options="triggerOptions"
              class="ctrl-filter"
              placeholder="触发方式（空=全部）"
            />
            <n-date-picker
              v-model:value="range"
              type="datetimerange"
              clearable
              class="ctrl-range"
            />
            <n-button :loading="store.runLoading" @click="store.loadRuns()">刷新</n-button>
          </div>
          <n-data-table
            remote
            :columns="runColumns"
            :data="store.runItems"
            :loading="store.runLoading"
            :scroll-x="runScrollX"
            :row-key="runRowKey"
            :row-class-name="runRowClass"
            :pagination="pagination"
          >
            <template #empty>
              <n-empty description="暂无备份执行记录" class="empty" />
            </template>
          </n-data-table>
        </n-tab-pane>
      </n-tabs>
    </n-card>

    <!-- 新建/编辑备份任务 Drawer（admin 专属；readonly 不渲染宿主——负空间第 2 节） -->
    <backup-job-drawer
      v-if="authStore.isAdmin"
      v-model:show="store.backupFormVisible"
      @changed="store.loadBackupJobs()"
    />

    <!-- 执行详情 Drawer（两角色可见；readonly 产物清单可见、下载按钮不渲染——负空间第 10 项） -->
    <run-detail-drawer v-model:show="store.runDrawerVisible" />
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.controls-row {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 8px;
}
.ctrl-filter {
  width: 180px;
}
.ctrl-range {
  width: 340px;
}
.muted {
  opacity: 0.55;
}
.mono {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', monospace;
  font-size: 12px;
}
.meta-inline {
  font-size: 12px;
  margin-left: 6px;
}
.source-tags {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
}
.target-cell,
.last-run,
.status-cell,
.job-cell {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
}
.row-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.empty {
  padding: 24px 0;
}
/* 立即备份新记录行浅蓝高亮（PageSpec 第 2 节；rgba 语义色随明暗主题叠加） */
:deep(.fresh-row td) {
  background: rgba(64, 128, 255, 0.12);
}
</style>
