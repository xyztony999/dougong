<script setup lang="ts">
/**
 * 容器详情 Drawer（PageSpec 5.1；宽 720px，标题「容器 {name}」；名称列 link 点击打开）。
 * Card A 资源占用快照（containerStats，D-07 单次快照——无自动轮询，「刷新快照」手动重拉）+
 * Card B 日志（LogFollowCard：尾部 tail=500 + WS 跟随，D-03 双通道）。
 * 打开时按需拉取 stats+logs（stats 失败不阻断日志）；关闭即断 WS（store.closeLogDrawer）。
 * DK_003（面板外已删）→ warning + 自动重拉容器清单（PageSpec 第 1 节）。
 */
import { computed, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { ApiError } from '@/api/http'
import { useDockerStore } from '@/stores/docker'
import { DK_ERROR_TEXT } from '@/types/docker'
import { formatBytes, formatDateTime, formatPercent } from '@/utils/format'
import LogFollowCard from './LogFollowCard.vue'

const store = useDockerStore()
const message = useMessage()

const show = computed(() => store.logTarget?.kind === 'container')
/** 收窄目标（show=true 时恒非空 container 形态） */
const target = computed(() => (store.logTarget?.kind === 'container' ? store.logTarget : null))

/** Drawer 打开 → stats + logs 按需拉取（PageSpec 第 2 节纪律表；错误按码分流） */
watch(show, (visible) => {
  if (!visible) return
  void store.loadStats().catch((e: unknown) => handleObsError(e))
  void store.loadLogs().catch((e: unknown) => handleObsError(e))
})

/** 观测面错误分流（stats/logs 共用）：DK_003→warning+重拉容器清单；DK_002 已由 store 降级引导态 */
function handleObsError(e: unknown): void {
  if (!(e instanceof ApiError)) return
  if (e.is('DK_003')) {
    message.warning(DK_ERROR_TEXT['DK_003'])
    void store.loadContainers()
    return
  }
  const text = DK_ERROR_TEXT[e.code]
  if (text !== undefined) message.error(text)
}

function onRefreshStats(): void {
  void store.loadStats().catch((e: unknown) => handleObsError(e))
}

function onUpdateShow(v: boolean): void {
  if (!v) store.closeLogDrawer()
}
</script>

<template>
  <n-drawer :show="show" :width="720" @update:show="onUpdateShow">
    <n-drawer-content :title="`容器 ${target?.name ?? ''}`" closable>
      <!-- Card A 资源占用快照（D-07：单次读取，无轮询） -->
      <n-card size="small" title="资源占用" class="drawer-card">
        <template #header-extra>
          <n-button size="small" :loading="store.statsLoading" @click="onRefreshStats()">
            刷新快照
          </n-button>
        </template>
        <n-spin v-if="store.statsLoading && store.stats === null" size="small" />
        <template v-else-if="store.stats !== null">
          <div class="stats-row">
            <span class="stats-label">CPU 占用</span>
            <span>{{ formatPercent(store.stats.cpuPercent) }}</span>
          </div>
          <div class="stats-row">
            <span class="stats-label">内存</span>
            <span>
              {{ formatBytes(store.stats.memUsageBytes) }} / {{ formatBytes(store.stats.memLimitBytes) }}
              （{{ formatPercent(store.stats.memPercent) }}）
            </span>
          </div>
          <div class="meta">数据生成于 {{ formatDateTime(store.stats.generatedAt) }}</div>
        </template>
        <div v-else class="meta">暂无快照数据</div>
      </n-card>
      <!-- Card B 日志（tail 档位 + WS 跟随，与 Compose 日志 Drawer 同组件） -->
      <n-card size="small" title="日志" class="drawer-card">
        <log-follow-card />
      </n-card>
    </n-drawer-content>
  </n-drawer>
</template>

<style scoped>
.drawer-card {
  margin-bottom: 16px;
}
.stats-row {
  display: flex;
  align-items: baseline;
  gap: 10px;
  margin-bottom: 6px;
}
.stats-label {
  font-size: 12px;
  opacity: 0.6;
  min-width: 64px;
}
.meta {
  font-size: 12px;
  opacity: 0.6;
  margin-top: 6px;
}
</style>
