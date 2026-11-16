<script setup lang="ts">
/**
 * 备份执行详情 Drawer（PageSpec 第 6 节；NDrawer 右侧宽 640px，两角色可见）。
 * 三 Card：执行信息（状态 Tag，running 附轮询提示）/ 备份源明细（类型 Tag+引用，
 * 失败源以 errorMessage 文本命中 red 标注——契约仅给文本摘要，前端按 ref 文本匹配）/
 * 产物清单（success 表+行内下载 admin 专属；failure NAlert；running NSpin）。
 *
 * 轮询纪律（PageSpec 第 2 节纪律表，api D-07 留 D4 落定）：Drawer 打开且
 * status=running → 每 5s 轮询 backupRunDetail，转终态即停（store 刷新一次列表）；
 * Drawer 关闭立即停；document.hidden 暂停、恢复可见且条件仍满足则恢复；
 * 组件卸载 onUnmounted 清理定时器与监听（frontend-rules §4.5 配对）。
 */
import { computed, h, onMounted, onUnmounted, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { NButton } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { backupRunDownload } from '@/api/cron'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { useCronStore } from '@/stores/cron'
import { BACKUP_RUN_STATUS, BACKUP_SOURCE_TYPE, BACKUP_TRIGGER, dictItem } from '@/types/enums'
import type { DictItem } from '@/types/enums'
import { CB_ERROR_TEXT } from '@/types/cron'
import type { BackupRunDetailData, BackupRunFile } from '@/types/cron'
import { formatBytes, formatDateTime, formatDurationMs } from '@/utils/format'

/** Drawer 轮询节奏（PageSpec 第 2 节纪律表：每 5s；转终态即停/关闭即停） */
const POLL_MS = 5_000

const store = useCronStore()
const authStore = useAuthStore()
const message = useMessage()

const show = defineModel<boolean>('show', { default: false })

const detail = computed<BackupRunDetailData | null>(() => store.runDetail)

const statusDict = computed(() =>
  detail.value === null ? null : dictItem(BACKUP_RUN_STATUS, detail.value.status)
)
const triggerDict = computed(() =>
  detail.value === null ? null : dictItem(BACKUP_TRIGGER, detail.value.trigger)
)

/**
 * 失败源标注：errorMessage 文本命中 source.ref → red（契约仅给文本摘要，无结构化失败源映射）。
 * Gate 2 E1 修复：ref 经 utils/cronback 归一恒为 string；''（不可推导）不参与文本命中
 * （includes('') 恒真会把全部源误标失败）。
 */
function sourceFailed(ref: string): boolean {
  if (ref === '') return false
  const err = detail.value?.errorMessage
  return err !== null && err !== undefined && err.includes(ref)
}

/** 源类型 dict：未知 type 走 dictItem 兜底原值灰字（⚠️A14 口径，直接索引会崩） */
function sourceDict(type: string): DictItem {
  return dictItem(BACKUP_SOURCE_TYPE, type)
}

/** ---- 轮询（组件持有定时器；store 提供数据通道与转终态列表刷新） ---- */
let pollTimer: ReturnType<typeof setInterval> | null = null

function pollShouldRun(): boolean {
  return show.value && detail.value?.status === 'running'
}

function startPolling(): void {
  if (pollTimer !== null) return
  pollTimer = setInterval(() => {
    void store.loadDetail()
  }, POLL_MS)
}

function stopPolling(): void {
  if (pollTimer !== null) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

watch(show, (v) => {
  if (!v) stopPolling() // Drawer 关闭立即停（无残留定时器）
})

watch(
  () => detail.value?.status,
  (status) => {
    if (status === 'running' && pollShouldRun()) startPolling()
    else stopPolling() // 转终态即停（store 已在 loadDetail 内刷新一次 backupRunList）
  }
)

/** 页面可见性：隐藏暂停、恢复且条件仍满足则恢复（PageSpec 第 2 节纪律表） */
function onVisibility(): void {
  if (document.hidden) stopPolling()
  else if (pollShouldRun()) startPolling()
}

onMounted(() => {
  document.addEventListener('visibilitychange', onVisibility)
})

onUnmounted(() => {
  stopPolling()
  document.removeEventListener('visibilitychange', onVisibility)
})

/** CB_007 文案提示（store 已关 Drawer+重拉列表；monitoring 先例——store 无 Message 上下文） */
watch(
  () => store.detailError,
  (text) => {
    if (text !== null) {
      message.warning(text)
      store.clearDetailError()
    }
  }
)

/** ---- 产物下载（admin 专属；CB_008/009 前置校验 warning，api D-10/D-16） ---- */
async function onDownload(file: BackupRunFile): Promise<void> {
  if (detail.value === null) return
  const key = `backup-download:${detail.value.id}:${file.name}`
  store.setInFlight(key)
  try {
    await backupRunDownload({ runId: detail.value.id, file: file.name })
  } catch (e) {
    if (e instanceof ApiError && (e.is('CB_008') || e.is('CB_009'))) {
      message.warning(CB_ERROR_TEXT[e.code])
    }
    // CB_007（轮转后记录消失）等其余错误拦截器统一提示
  } finally {
    store.releaseInFlight(key)
  }
}

const fileColumns = computed<DataTableColumns<BackupRunFile>>(() => {
  const cols: DataTableColumns<BackupRunFile> = [
    { title: '文件名', key: 'name', render: (row) => h('span', { class: 'mono' }, row.name) },
    {
      title: '大小',
      key: 'size',
      width: 110,
      align: 'right',
      render: (row) => formatBytes(row.size)
    }
  ]
  if (authStore.isAdmin) {
    cols.push({
      title: '操作',
      key: 'actions',
      width: 90,
      render: (row) =>
        h(
          NButton,
          {
            size: 'small',
            loading: store.isInFlight(`backup-download:${detail.value?.id}:${row.name}`),
            onClick: () => void onDownload(row)
          },
          { default: () => '下载' }
        )
    })
  }
  return cols
})

function fileRowKey(row: BackupRunFile): string {
  return row.name
}
</script>

<template>
  <n-drawer v-model:show="show" :width="640" placement="right">
    <n-drawer-content :title="`备份执行详情 #${detail?.id ?? '…'}`" closable>
      <template v-if="detail !== null">
        <!-- Card 1 执行信息（状态/任务名/触发/起止/耗时；running 附轮询提示） -->
        <n-card size="small" title="执行信息" class="detail-card">
          <div class="info-row">
            <span class="label">状态</span>
            <n-tag v-if="statusDict !== null" :type="statusDict.tag" size="small" :bordered="false">
              {{ statusDict.label }}
            </n-tag>
            <span v-if="detail.status === 'running'" class="meta">
              <n-spin :size="12" class="inline-spin" /> 每 5 秒自动刷新
            </span>
          </div>
          <div class="info-row">
            <span class="label">任务</span>
            <span>{{ detail.jobName }}</span>
            <span class="meta">（快照）</span>
          </div>
          <div class="info-row">
            <span class="label">触发方式</span>
            <n-tag
              v-if="triggerDict !== null"
              :type="triggerDict.tag"
              size="small"
              :bordered="false"
            >
              {{ triggerDict.label }}
            </n-tag>
          </div>
          <div class="info-row">
            <span class="label">开始时间</span>
            <span>{{ formatDateTime(detail.startedAt) }}</span>
          </div>
          <div class="info-row">
            <span class="label">结束时间</span>
            <span>{{ formatDateTime(detail.finishedAt) }}</span>
          </div>
          <div class="info-row">
            <span class="label">耗时</span>
            <span>{{ formatDurationMs(detail.durationMs) }}</span>
          </div>
        </n-card>

        <!-- Card 2 备份源明细（执行时源清单快照；失败源 red 标注） -->
        <n-card size="small" title="备份源明细" class="detail-card">
          <div v-for="(s, i) in detail.sources" :key="i" class="info-row" :class="{ 'failed-source': sourceFailed(s.ref) }">
            <n-tag :type="sourceDict(s.type).tag" size="small" :bordered="false">
              {{ sourceDict(s.type).label }}
            </n-tag>
            <span v-if="s.ref !== ''" class="mono">{{ s.ref }}</span>
            <span v-if="sourceFailed(s.ref)" class="failed-mark">失败源</span>
          </div>
          <n-empty v-if="detail.sources.length === 0" description="无源清单" class="empty" />
        </n-card>

        <!-- Card 3 产物清单（success 表+下载 / failure NAlert / running NSpin，I-04） -->
        <n-card size="small" title="产物清单" class="detail-card">
          <template v-if="detail.status === 'success'">
            <n-data-table
              size="small"
              :columns="fileColumns"
              :data="detail.files ?? []"
              :row-key="fileRowKey"
            />
            <div class="meta artifact-meta">
              产物总大小 {{ formatBytes(detail.artifactSize) }} · 产物路径
              <span class="mono">{{ detail.artifactPath ?? '—' }}</span>
            </div>
            <!-- readonly 负空间：下载按钮不渲染（ADR-018 禁上传下载），指引文案不虚构操作路径 -->
            <div v-if="!authStore.isAdmin" class="meta">产物下载需管理员权限</div>
          </template>
          <n-alert v-else-if="detail.status === 'failure'" type="error">
            执行失败：{{ detail.errorMessage ?? '未知原因' }}
          </n-alert>
          <div v-else class="running-block">
            <n-spin size="small" />
            <span class="meta">备份执行中，每 5 秒自动刷新</span>
          </div>
        </n-card>
      </template>
      <div v-else class="loading-block">
        <n-spin size="small" />
        <span class="meta">加载执行详情…</span>
      </div>
    </n-drawer-content>
  </n-drawer>
</template>

<style scoped>
.detail-card {
  margin-bottom: 12px;
}
.info-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 2px 0;
  flex-wrap: wrap;
}
.label {
  font-size: 12px;
  opacity: 0.6;
  min-width: 64px;
}
.meta {
  font-size: 12px;
  opacity: 0.6;
}
.mono {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', monospace;
  font-size: 12px;
  word-break: break-all;
}
.failed-source .mono {
  color: rgba(255, 80, 80, 0.9);
}
.failed-mark {
  font-size: 12px;
  color: rgba(255, 80, 80, 0.9);
}
.artifact-meta {
  margin-top: 8px;
}
.running-block,
.loading-block {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 0;
}
.inline-spin {
  display: inline-flex;
  vertical-align: middle;
}
.empty {
  padding: 12px 0;
}
</style>
