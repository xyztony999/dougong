<script setup lang="ts">
/**
 * 任务 Drawer（PageSpec env-mgmt v1.0 4.2/4.3）：历史 Tab（筛选列表，进入拉取
 * 一次+手动刷新——列表页保守手动）+ 详情区（概要/失败原因/输出尾部等宽块；
 * running/pending 态 5s 轮询终态即停，轮询由 envStore 承载、页面 onUnmounted
 * 清理）。两角色可看（ADR-018），无操作按钮。
 */
import type { DataTableColumns, SelectOption } from 'naive-ui'
import type { EnvTaskItem } from '@/types/env'
import type { EnvTaskStatus, EnvType } from '@/types/enums'
import { useEnvStore } from '@/stores/env'
import { ENV_TASK_ACTION, ENV_TASK_STATUS, dictItem } from '@/types/enums'
import { formatDateTime } from '@/utils/format'

defineProps<{
  columns: DataTableColumns<EnvTaskItem>
  envTypeOptions: SelectOption[]
  statusOptions: SelectOption[]
}>()

const show = defineModel<boolean>('show', { default: false })
const store = useEnvStore()

function onFilterEnvType(v: string | null): void {
  store.setTaskFilter(v as EnvType | null, store.taskFilter.status)
}

function onFilterStatus(v: string | null): void {
  store.setTaskFilter(store.taskFilter.envType, v as EnvTaskStatus | null)
}
</script>

<template>
  <n-drawer v-model:show="show" :width="760" placement="right">
    <n-drawer-content closable title="环境变更任务">
      <!-- 筛选区 -->
      <div class="filters">
        <n-select
          class="filter"
          clearable
          :value="store.taskFilter.envType"
          :options="envTypeOptions"
          placeholder="环境（空=全部）"
          @update:value="onFilterEnvType"
        />
        <n-select
          class="filter"
          clearable
          :value="store.taskFilter.status"
          :options="statusOptions"
          placeholder="状态（空=全部）"
          @update:value="onFilterStatus"
        />
        <n-button size="small" :loading="store.tasksLoading" @click="store.loadTasks(1)">刷新</n-button>
      </div>

      <!-- 在途提示（手动刷新口径，D-03 待对齐-3 落定） -->
      <n-alert
        v-if="store.tasks.some((t) => t.status === 'pending' || t.status === 'running')"
        type="info"
        :show-icon="true"
        style="margin-bottom: 10px"
      >
        有任务进行中。点「详情」可看输出。
      </n-alert>

      <n-data-table
        :columns="columns"
        :data="store.tasks"
        :loading="store.tasksLoading"
        :row-key="(r: EnvTaskItem) => r.id"
        size="small"
        :max-height="240"
      >
        <template #empty>
          <n-empty description="暂无环境变更任务" size="small" />
        </template>
      </n-data-table>

      <!-- 详情区 -->
      <n-divider>任务详情</n-divider>
      <template v-if="store.currentTask !== null">
        <n-descriptions label-placement="left" :column="2" size="small" bordered>
          <n-descriptions-item label="任务">
            #{{ store.currentTask.id }}
            {{ dictItem(ENV_TASK_ACTION, store.currentTask.action).label }}
          </n-descriptions-item>
          <n-descriptions-item label="状态">
            <n-tag
              :type="dictItem(ENV_TASK_STATUS, store.currentTask.status).tag"
              size="small"
              :bordered="false"
            >
              {{ dictItem(ENV_TASK_STATUS, store.currentTask.status).label }}
            </n-tag>
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
        <n-alert
          v-if="store.currentTask.error !== null && store.currentTask.error !== ''"
          type="error"
          style="margin-top: 10px"
        >
          {{ store.currentTask.error }}
        </n-alert>
        <!-- 引导结果横幅（v1.2 安装后配置就绪，D-18/⚠️A14；bootstrapStatus 非空时渲染。
             任务 status 仍包操作结果——引导失败独立维度，不改 succeeded。两角色可看 ADR-018。） -->
        <n-alert
          v-if="store.currentTask.bootstrapStatus === 'succeeded'"
          type="success"
          title="自动配置完成"
          style="margin-top: 10px"
        >
          {{ store.currentTask.bootstrapNote || '已创建管理账号并添加到数据库管理，可以直接管理了。' }}
          <router-link to="/databases" class="boot-link">前往数据库管理</router-link>
        </n-alert>
        <n-alert
          v-else-if="store.currentTask.bootstrapStatus === 'failed'"
          type="warning"
          title="已安装 · 自动配置失败"
          style="margin-top: 10px"
        >
          {{ store.currentTask.bootstrapNote || '安装后的自动配置失败。' }}
          <div class="boot-hint">环境已装好，但面板还无法直接管理；可重新安装或升级再试一次（不会重复配置）。</div>
        </n-alert>
        <n-alert
          v-else-if="store.currentTask.bootstrapStatus === 'skipped'"
          type="info"
          title="已跳过自动配置"
          style="margin-top: 10px"
        >
          {{ store.currentTask.bootstrapNote || '检测到已有配置，已跳过自动配置（不会覆盖你的设置）。' }}
        </n-alert>
        <div class="tail-head">运行日志（最近 200 行，含自动配置步骤）</div>
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
      <n-empty v-else description="从上方任务列表点击「详情」查看" size="small" />
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
.boot-link {
  margin-left: 8px;
  font-weight: 500;
}
.boot-hint {
  margin-top: 4px;
  font-size: 12px;
  opacity: 0.8;
}
</style>
