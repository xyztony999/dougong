<script setup lang="ts">
import { computed } from 'vue'
import { useThemeVars } from 'naive-ui'
import type { MetricsSnapshot } from '@/types/monitoring'
import { formatBps, formatBytes, formatLoad, formatPercent, usagePercent } from '@/utils/format'

/**
 * Card B 实时快照（PageSpec 6.1，3s 轮询同源刷新）：每次刷新整卡替换数据，
 * 无增量动画（数字直更即为实时感；echarts 不用于快照区避免重绘开销）。
 * D-08：失败字段 null → '—'（缺失项暂不显示，横幅由页面级 degraded 提示承担）。
 * 磁盘进度条 percentage = usagePercent(used/total×100)（Gate 2 E2：禁直传字节数）。
 */
const props = defineProps<{ snapshot: MetricsSnapshot | null }>()

/** ↑/↓ 双色用主题变量（🟡 禁硬编码十六进制色） */
const themeVars = useThemeVars()

const PLACEHOLDER = '—'

const cpuTotalText = computed(() =>
  formatPercent(props.snapshot?.cpu?.totalPercent ?? null)
)
/** 分核使用率（null 组 → 空数组不渲染迷你条组） */
const perCore = computed<number[]>(() => props.snapshot?.cpu?.perCore ?? [])

const memPercent = computed<number | null>(() => props.snapshot?.memory?.usedPercent ?? null)
const memUsedText = computed(() => formatBytes(props.snapshot?.memory?.usedBytes ?? null))
const memTotalText = computed(() => formatBytes(props.snapshot?.memory?.totalBytes ?? null))

/** 磁盘行视图模型（percent=null → 进度条不渲染，仅文本占位） */
interface DiskRowView {
  mount: string
  percent: number | null
  usageText: string
}

const diskRows = computed<DiskRowView[]>(() =>
  (props.snapshot?.disk?.capacity ?? []).map((row) => ({
    mount: row.mount,
    percent: usagePercent(row.usedBytes, row.totalBytes),
    usageText:
      row.usedBytes === null || row.totalBytes === null
        ? PLACEHOLDER
        : `${formatBytes(row.usedBytes)} / ${formatBytes(row.totalBytes)}`
  }))
)
const ioReadText = computed(() => formatBps(props.snapshot?.disk?.ioReadBps ?? null))
const ioWriteText = computed(() => formatBps(props.snapshot?.disk?.ioWriteBps ?? null))

const netUpText = computed(() => formatBps(props.snapshot?.net?.upBps ?? null))
const netDownText = computed(() => formatBps(props.snapshot?.net?.downBps ?? null))

const loadTexts = computed(() => ({
  l1: formatLoad(props.snapshot?.load?.l1 ?? null),
  l5: formatLoad(props.snapshot?.load?.l5 ?? null),
  l15: formatLoad(props.snapshot?.load?.l15 ?? null)
}))
</script>

<template>
  <n-card title="实时快照" size="small">
    <div class="snapshot-grid">
      <!-- CPU：大数字 + 分核迷你条组 -->
      <section class="block">
        <h4 class="block-title">CPU</h4>
        <div class="cpu-main">
          <span class="big">{{ cpuTotalText }}</span>
        </div>
        <div v-if="perCore.length > 0" class="cores">
          <div v-for="(p, i) in perCore" :key="i" class="core-row">
            <span class="core-label">核 {{ i }}</span>
            <n-progress
              type="line"
              :percentage="p"
              :height="8"
              :border-radius="4"
              :show-indicator="false"
            />
            <span class="core-value">{{ formatPercent(p) }}</span>
          </div>
        </div>
        <div v-else-if="snapshot?.cpu?.perCore === null" class="muted">{{ PLACEHOLDER }}</div>
      </section>

      <!-- 内存：环形进度 + 已用/总量 -->
      <section class="block">
        <h4 class="block-title">内存</h4>
        <div class="mem">
          <n-progress
            v-if="memPercent !== null"
            type="circle"
            :percentage="memPercent"
            :offset-degree="0"
          >
            <span class="mem-num">{{ formatPercent(memPercent) }}</span>
          </n-progress>
          <span v-else class="big">{{ PLACEHOLDER }}</span>
          <div class="mem-text">
            <div>已用 {{ memUsedText }}</div>
            <div>总量 {{ memTotalText }}</div>
          </div>
        </div>
      </section>

      <!-- 磁盘容量：每挂载点一行 -->
      <section class="block">
        <h4 class="block-title">磁盘容量</h4>
        <div v-if="diskRows.length > 0" class="disk-rows">
          <div v-for="row in diskRows" :key="row.mount" class="disk-row">
            <span class="disk-mount" :title="row.mount">{{ row.mount }}</span>
            <n-progress
              v-if="row.percent !== null"
              type="line"
              :percentage="row.percent ?? 0"
              :height="8"
              :border-radius="4"
              :show-indicator="false"
            />
            <span class="disk-usage">{{ row.usageText }}</span>
          </div>
        </div>
        <div v-else class="muted">{{ PLACEHOLDER }}</div>
      </section>

      <!-- 磁盘 IO：读/写速率（↑read ↓write 双色） -->
      <section class="block">
        <h4 class="block-title">磁盘 IO</h4>
        <div class="rate-row">
          <span class="rate up" :style="{ color: themeVars.successColor }">↑ 读 {{ ioReadText }}</span>
          <span class="rate down" :style="{ color: themeVars.infoColor }">↓ 写 {{ ioWriteText }}</span>
        </div>
      </section>

      <!-- 网络：上行/下行速率（↑up ↓down 双色） -->
      <section class="block">
        <h4 class="block-title">网络</h4>
        <div class="rate-row">
          <span class="rate up" :style="{ color: themeVars.successColor }">↑ 上行 {{ netUpText }}</span>
          <span class="rate down" :style="{ color: themeVars.infoColor }">↓ 下行 {{ netDownText }}</span>
        </div>
      </section>

      <!-- 负载：1m/5m/15m 三数字 -->
      <section class="block">
        <h4 class="block-title">负载</h4>
        <div class="load-row">
          <div class="load-item">
            <span class="load-value">{{ loadTexts.l1 }}</span>
            <span class="load-label">1m</span>
          </div>
          <div class="load-item">
            <span class="load-value">{{ loadTexts.l5 }}</span>
            <span class="load-label">5m</span>
          </div>
          <div class="load-item">
            <span class="load-value">{{ loadTexts.l15 }}</span>
            <span class="load-label">15m</span>
          </div>
        </div>
      </section>
    </div>
  </n-card>
</template>

<style scoped>
.snapshot-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 16px 28px;
}
.block-title {
  margin: 0 0 10px;
  font-size: 13px;
  font-weight: 500;
  opacity: 0.75;
}
.muted {
  opacity: 0.5;
}
.cpu-main {
  margin-bottom: 10px;
}
.big {
  font-size: 30px;
  font-weight: 600;
  line-height: 1.2;
}
.cores {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 168px;
  overflow-y: auto;
}
.core-row {
  display: grid;
  grid-template-columns: 42px 1fr 52px;
  align-items: center;
  gap: 8px;
}
.core-label,
.core-value {
  font-size: 12px;
  opacity: 0.7;
}
.core-value {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.mem {
  display: flex;
  align-items: center;
  gap: 20px;
}
.mem-num {
  font-size: 16px;
  font-weight: 600;
}
.mem-text {
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 13px;
  opacity: 0.8;
}
.disk-rows {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.disk-row {
  display: grid;
  grid-template-columns: minmax(72px, 150px) 1fr 110px;
  align-items: center;
  gap: 8px;
}
.disk-mount {
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  opacity: 0.8;
}
.disk-usage {
  font-size: 12px;
  text-align: right;
  font-variant-numeric: tabular-nums;
  opacity: 0.8;
}
.rate-row {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.rate {
  font-size: 16px;
  font-weight: 500;
  font-variant-numeric: tabular-nums;
}
.load-row {
  display: flex;
  gap: 28px;
}
.load-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.load-value {
  font-size: 20px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.load-label {
  font-size: 12px;
  opacity: 0.6;
}
</style>
