<script setup lang="ts">
/**
 * 任务 Drawer（PageSpec db-tools 4.2/4.3 v1.2）：历史 Tab（toolType/status 筛选列表，
 * 进入拉取一次+手动刷新+写后刷新——无自动列表轮询）+ 详情区（概要/失败原因/
 * 输出尾部等宽块 ≤200 行；running/pending 态 5s 轮询终态即停，轮询由 dbtoolStore
 * 承载、页面 onUnmounted 清理；终态跃迁触发全景与清单重拉）。
 * v1.2 增量（api D-22/D-19）：失败原因 error 全文含 data.reason 具体原因如实呈现
 * （端口占用附释放引导/多候选附「到站点管理明确落点」/sites 链接）；install succeeded
 * 且 autoCreated=true → 详情区顶部安装成功放行提示（端口+访问地址，终态跃迁重拉判定）。
 * 两角色可看（ADR-018），无操作按钮；readonly 详情刷新与观测轮询同等可用，
 * reason/放行提示只读呈现同等（纯信息无操作，负空间 6 不变）。
 */
import { computed } from 'vue'
import type { DataTableColumns, SelectOption } from 'naive-ui'
import type { DbToolTaskItem } from '@/types/dbtool'
import type { DbToolTaskStatus, DbToolType } from '@/types/enums'
import { useDbtoolStore } from '@/stores/dbtool'
import { dbtFailureGuide, installReadyAlertText } from '@/types/dbtool'
import { DB_TOOL_TASK_ACTION, DB_TOOL_TASK_STATUS, DB_TOOL_TYPE, dictItem } from '@/types/enums'
import { formatDateTime } from '@/utils/format'

defineProps<{
  columns: DataTableColumns<DbToolTaskItem>
  toolTypeOptions: SelectOption[]
  statusOptions: SelectOption[]
}>()

const show = defineModel<boolean>('show', { default: false })
const store = useDbtoolStore()

/** 安装成功放行提示（v1.2，PageSpec 4.1/4.3）：install succeeded 且工具档案 autoCreated=true
 * （终态跃迁重拉 dbtoolList 后 findByType 取最新投影）；既有面板域块形态不渲染 */
const installReadyText = computed(() => {
  const t = store.currentTask
  if (t === null) return null
  return installReadyAlertText(t, store.findByType(t.toolType), window.location.hostname)
})

/** 失败原因引导（v1.2，api D-22 reason 细分）：端口占用→释放端口后重试；
 * 多候选→到站点管理明确落点（/sites）；error 全文本身已含 reason 如实呈现 */
const failureGuide = computed(() => dbtFailureGuide(store.currentTask?.error ?? null))

function onFilterToolType(v: string | null): void {
  store.setTaskFilter(v as DbToolType | null, store.taskFilter.status)
}

function onFilterStatus(v: string | null): void {
  store.setTaskFilter(store.taskFilter.toolType, v as DbToolTaskStatus | null)
}
</script>

<template>
  <n-drawer v-model:show="show" :width="760" placement="right">
    <n-drawer-content closable title="工具部署任务">
      <!-- 筛选区（api task/list 真实参数 toolType/status，含「全部」=clearable） -->
      <div class="filters">
        <n-select
          class="filter"
          clearable
          :value="store.taskFilter.toolType"
          :options="toolTypeOptions"
          placeholder="全部工具"
          @update:value="onFilterToolType"
        />
        <n-select
          class="filter"
          clearable
          :value="store.taskFilter.status"
          :options="statusOptions"
          placeholder="全部状态"
          @update:value="onFilterStatus"
        />
        <n-button size="small" :loading="store.tasksLoading" @click="store.loadTasks(1)">刷新</n-button>
      </div>

      <!-- 在途提示（手动刷新口径，PageSpec 4.2） -->
      <n-alert
        v-if="store.tasks.some((t) => t.status === 'pending' || t.status === 'running')"
        type="info"
        :show-icon="true"
        style="margin-bottom: 10px"
      >
        有任务进行中，手动刷新查看进度（详情区 5s 自动刷新至终态）。
      </n-alert>

      <n-data-table
        :columns="columns"
        :data="store.tasks"
        :loading="store.tasksLoading"
        :row-key="(r: DbToolTaskItem) => r.id"
        size="small"
        :max-height="240"
      >
        <template #empty>
          <n-empty description="暂无工具部署任务" size="small" />
        </template>
      </n-data-table>

      <!-- 详情区 -->
      <n-divider>任务详情</n-divider>
      <template v-if="store.currentTask !== null">
        <!-- 安装成功放行提示（v1.2：install succeeded 且 autoCreated=true，详情区顶部；
             readonly 同等呈现——纯信息无操作；outputTail 内后端放行提示文本如实呈现 D-19⑦） -->
        <n-alert v-if="installReadyText !== null" type="success" style="margin-bottom: 10px">
          {{ installReadyText }}
        </n-alert>
        <n-descriptions label-placement="left" :column="2" size="small" bordered>
          <n-descriptions-item label="任务">
            #{{ store.currentTask.id }}
            {{ dictItem(DB_TOOL_TYPE, store.currentTask.toolType).label }}
            {{ dictItem(DB_TOOL_TASK_ACTION, store.currentTask.action).label }}
          </n-descriptions-item>
          <n-descriptions-item label="状态">
            <n-tag
              :type="dictItem(DB_TOOL_TASK_STATUS, store.currentTask.status).tag"
              size="small"
              :bordered="false"
            >
              {{ dictItem(DB_TOOL_TASK_STATUS, store.currentTask.status).label }}
            </n-tag>
            <n-spin
              v-if="store.currentTask.status === 'running' || store.currentTask.status === 'pending'"
              size="small"
              style="margin-left: 8px"
            />
          </n-descriptions-item>
          <n-descriptions-item label="实例">
            {{ store.currentTask.instanceId ?? '—' }}
          </n-descriptions-item>
          <n-descriptions-item label="发起者">{{ store.currentTask.requestedBy }}</n-descriptions-item>
          <n-descriptions-item label="发起时间">
            {{ formatDateTime(store.currentTask.createdAt) }}
          </n-descriptions-item>
          <n-descriptions-item label="开始时间">
            {{ formatDateTime(store.currentTask.startedAt) }}
          </n-descriptions-item>
          <n-descriptions-item label="结束时间">
            {{ formatDateTime(store.currentTask.finishedAt) }}
          </n-descriptions-item>
        </n-descriptions>
        <!-- 失败原因（v1.2：error 全文含 data.reason 具体原因如实呈现，api D-22——
             不再仅「部署操作失败」；端口占用/多候选附引导，PageSpec 4.3） -->
        <n-alert
          v-if="store.currentTask.error !== null && store.currentTask.error !== ''"
          type="error"
          style="margin-top: 10px"
        >
          {{ store.currentTask.error }}
          <div v-if="failureGuide === 'release-port'" class="error-guide">
            请释放端口后重试。
          </div>
          <div v-if="failureGuide === 'site-landing'" class="error-guide">
            请到站点管理调整域名配置后重试——
            <router-link to="/sites">前往站点管理</router-link>
          </div>
        </n-alert>
        <div class="tail-head">运行日志（最近 200 行，用于排查问题）</div>
        <pre class="tail">{{ store.currentTask.outputTail || '（暂无输出）' }}</pre>
        <div class="detail-actions">
          <n-button
            size="small"
            :loading="store.detailLoading"
            @click="store.currentTask && store.loadTaskDetail(store.currentTask.id)"
          >
            刷新详情
          </n-button>
        </div>
      </template>
      <n-empty v-else description="从上方任务列表点击 ID 查看" size="small" />
    </n-drawer-content>
  </n-drawer>
</template>

<style scoped>
.filters {
  display: flex;
  gap: 8px;
  margin-bottom: 10px;
}
.filter {
  width: 160px;
}
.tail-head {
  font-size: 12px;
  opacity: 0.65;
  margin: 10px 0 4px;
}
.tail {
  background: rgba(128, 128, 128, 0.08);
  border-radius: 4px;
  padding: 10px;
  font-size: 12px;
  max-height: 260px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
}
.detail-actions {
  margin-top: 8px;
}
.error-guide {
  margin-top: 6px;
  font-size: 12px;
}
</style>
