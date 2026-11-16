<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import { useThemeVars } from 'naive-ui'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import type { LineSeriesOption } from 'echarts/charts'
import {
  GridComponent,
  LegendComponent,
  TooltipComponent
} from 'echarts/components'
import type {
  GridComponentOption,
  LegendComponentOption,
  TooltipComponentOption
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { ComposeOption } from 'echarts/core'
import type { MetricSeries, MetricUnit } from '@/types/monitoring'
import { formatBps, formatBytes } from '@/utils/format'

/**
 * 历史趋势折线图（PageSpec 6.2；echarts 为 idea.md §5.3 材料给定选型）：
 * - v=null 点断线（connectNulls=false，D-03/ADR-014——断开=数据缺失，不插值补线）；
 * - y 轴/tooltip 单位按 series.unit 自适应（同一 dimension 内 unit 一致，api series 契约）；
 * - 主题色取 Naive UI 主题变量（useThemeVars，明暗随 n-config-provider——不硬编码色值；
 *   v6 ESM 入口不内置 'dark' 主题名，故以主题对象注入文字色 + 透明背景叠加卡片）；
 * - 按需注册（echarts/core + Line/Grid/Legend/Tooltip + Canvas），只打进用到的模块；
 * - resize 监听与图表实例在 onUnmounted 全部配对清理（frontend-rules 🔴）。
 */
echarts.use([LineChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer])

type TrendOption = ComposeOption<
  LineSeriesOption | GridComponentOption | LegendComponentOption | TooltipComponentOption
>

const props = defineProps<{ series: MetricSeries[] }>()

const themeVars = useThemeVars()
const el = ref<HTMLElement | null>(null)
let chart: echarts.ECharts | null = null

/** 按 series.unit 格式化数值（D-02 前端落法） */
function formatByUnit(unit: MetricUnit, value: number): string {
  switch (unit) {
    case 'percent':
      return `${value.toFixed(1)}%`
    case 'bytes':
      return formatBytes(value)
    case 'bps':
      return formatBps(value)
    case 'value':
      return value.toFixed(2)
  }
}

function buildOption(): TrendOption {
  const unit: MetricUnit = props.series[0]?.unit ?? 'value'
  return {
    backgroundColor: 'transparent',
    grid: { left: 64, right: 24, top: 32, bottom: 56 },
    legend: { type: 'scroll', bottom: 0 },
    tooltip: {
      trigger: 'axis',
      valueFormatter: (value: unknown): string => {
        if (typeof value === 'number') return formatByUnit(unit, value)
        if (value === null || value === '-') return '缺失'
        return String(value)
      }
    },
    xAxis: { type: 'time' },
    yAxis: {
      type: 'value',
      scale: true,
      axisLabel: { formatter: (value: number) => formatByUnit(unit, value) }
    },
    series: props.series.map((s) => ({
      name: s.label,
      type: 'line' as const,
      showSymbol: false,
      connectNulls: false,
      emphasis: { focus: 'series' },
      data: s.points.map((p) => [Date.parse(p.ts), p.v] as [number, number | null])
    }))
  }
}

function initChart(): void {
  if (el.value === null) return
  // 主题对象注入（init 第二参）：文字色随 Naive 明暗主题，背景透明叠加卡片
  chart = echarts.init(el.value, {
    backgroundColor: 'transparent',
    textStyle: { color: themeVars.value.textColorBase }
  })
  chart.setOption(buildOption())
}

function handleResize(): void {
  chart?.resize()
}

/** 数据替换：notMerge 全量覆盖（维度切换 series 条数变化，避免残留旧序列） */
watch(
  () => props.series,
  () => {
    chart?.setOption(buildOption(), { notMerge: true })
  }
)

onMounted(() => {
  initChart()
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
  chart?.dispose()
  chart = null
})
</script>

<template>
  <div ref="el" class="trend-chart" />
</template>

<style scoped>
.trend-chart {
  width: 100%;
  height: 380px;
}
</style>
