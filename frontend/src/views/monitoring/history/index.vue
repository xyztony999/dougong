<script setup lang="ts">
import { computed, onMounted, watch } from 'vue'
import { useMessage } from 'naive-ui'
import type { SelectOption } from 'naive-ui'
import { useMonitorStore } from '@/stores/monitoring'
import TrendChart from '../components/TrendChart.vue'
import type { MetricSeries } from '@/types/monitoring'
import { METRIC_DIMENSION, METRIC_SCOPE, METRIC_WINDOW } from '@/types/enums'
import type { MetricDimension, MetricScope, MetricWindow } from '@/types/enums'
import { formatDateTime } from '@/utils/format'

/**
 * 历史趋势（PageSpec 6.2，路由 /monitoring/history，admin+readonly 完全一致）：
 * 控件条为查询条件而非表单（无提交语义，切换即查）；趋势无自动轮询——
 * 仅进入页面/切参/「刷新」按钮触发（D-01）；刷新按钮 loading 防抖。
 */
const store = useMonitorStore()
const message = useMessage()

/** 控件条选项（字典单一事实源 enums.ts，PageSpec 第 3 节） */
const windowOptions: SelectOption[] = (Object.keys(METRIC_WINDOW) as MetricWindow[]).map(
  (value) => ({ label: METRIC_WINDOW[value].label, value })
)
const dimensionOptions: SelectOption[] = (Object.keys(METRIC_DIMENSION) as MetricDimension[]).map(
  (value) => ({ label: METRIC_DIMENSION[value].label, value })
)
const scopeOptions: SelectOption[] = (Object.keys(METRIC_SCOPE) as MetricScope[]).map((value) => ({
  label: METRIC_SCOPE[value].label,
  value
}))

/** v-model 桥接 store 参数（切换即查；scope 离开 cpu 时由 store 重置为 total，D-04） */
const windowModel = computed<MetricWindow>({
  get: () => store.trendParams.window,
  set: (v) => store.setTrendWindow(v)
})
const dimensionModel = computed<MetricDimension>({
  get: () => store.trendParams.dimension,
  set: (v) => store.setTrendDimension(v)
})
const scopeModel = computed<MetricScope>({
  get: () => store.trendParams.scope,
  set: (v) => store.setTrendScope(v)
})

/** 统计口径仅 dimension=cpu 时渲染（D-04 条件显隐） */
const isCpu = computed(() => store.trendParams.dimension === 'cpu')

const chartSeries = computed<MetricSeries[]>(() => store.trendData?.series ?? [])
const startTimeText = computed(() =>
  store.trendData !== null && store.trendData.startTime !== null
    ? formatDateTime(store.trendData.startTime)
    : null
)

/** MON_001~003 兜底提示（正常 UI 由选择器约束不可达，R-14 服务端兜底；store 无 Message 上下文） */
watch(
  () => store.trendError,
  (msg) => {
    if (msg !== null) message.error(msg)
  }
)

/** 进入页面拉取一次（D-01：趋势按需拉取，无自动轮询） */
onMounted(() => {
  void store.loadTrend()
})
</script>

<template>
  <div class="page">
    <!-- 控件条（查询条件：参数变更即触发 metricsHistory 拉取） -->
    <n-card size="small">
      <div class="controls-row">
        <span class="ctrl-label">时间窗口</span>
        <n-select v-model:value="windowModel" :options="windowOptions" class="ctrl" />
        <span class="ctrl-label">指标维度</span>
        <n-select v-model:value="dimensionModel" :options="dimensionOptions" class="ctrl" />
        <template v-if="isCpu">
          <span class="ctrl-label">统计口径</span>
          <n-select v-model:value="scopeModel" :options="scopeOptions" class="ctrl ctrl-scope" />
        </template>
        <n-button type="primary" :loading="store.trendLoading" @click="store.loadTrend()">
          刷新
        </n-button>
      </div>
    </n-card>

    <!-- 趋势图：断线=数据缺失（connectNulls=false）；partial/startTime 清零边界呈现 -->
    <n-card title="历史趋势" size="small" :loading="store.trendLoading && store.trendData === null">
      <template #header-extra>
        <n-tag v-if="store.trendData?.partial === true" type="warning" size="small" :bordered="false">
          窗口内存在缺失时段
        </n-tag>
      </template>
      <div class="chart-wrap">
        <TrendChart :series="chartSeries" />
        <!-- 空态：series=[] 或 startTime=null（MON_004 语义同空态处理） -->
        <div v-if="store.trendEmpty" class="empty-overlay">
          <n-empty description="重启后暂无历史数据" />
        </div>
      </div>
      <div v-if="startTimeText !== null" class="start-hint">
        数据自 {{ startTimeText }} 起（重启后历史清零，缓冲保留 24 小时）
      </div>
    </n-card>
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
}
.ctrl-label {
  font-size: 13px;
  opacity: 0.75;
}
.ctrl {
  width: 150px;
}
.ctrl-scope {
  width: 120px;
}
.chart-wrap {
  position: relative;
}
.empty-overlay {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}
.start-hint {
  margin-top: 8px;
  font-size: 12px;
  opacity: 0.6;
}
</style>
