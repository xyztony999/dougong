<script setup lang="ts">
import { computed } from 'vue'
import type { DataTableColumns } from 'naive-ui'
import type { ProcessItem } from '@/types/monitoring'
import { formatBytes, formatPercent } from '@/utils/format'

/**
 * Card C TOP 进程（PageSpec 6.1）：数据随快照同源同响应刷新（不同步二次请求）；
 * ≤10 行、无分页、无排序控件（服务端已排序 I-06）、无操作列（第 4 节负空间：
 * kill/优先级等进程操作不在产品范围，仅观测无操作入口）。
 * cpuPercent/memBytes 可空（无基线/单进程读取失败，D-08）——null 经
 * formatPercent/formatBytes 显示 '—'，禁止无守卫 .toFixed（Gate 2 E1 崩溃路径）。
 */
const props = defineProps<{ processes: ProcessItem[] | null }>()

const rows = computed<ProcessItem[]>(() => props.processes ?? [])

const columns: DataTableColumns<ProcessItem> = [
  { title: 'PID', key: 'pid', width: 100 },
  { title: '进程名', key: 'name', ellipsis: { tooltip: true } },
  {
    title: 'CPU %',
    key: 'cpuPercent',
    width: 110,
    align: 'right',
    render: (row) => formatPercent(row.cpuPercent)
  },
  {
    title: '内存',
    key: 'memBytes',
    width: 130,
    align: 'right',
    render: (row) => formatBytes(row.memBytes)
  }
]
</script>

<template>
  <n-card title="TOP 进程" size="small">
    <n-data-table
      :columns="columns"
      :data="rows"
      :pagination="false"
      :bordered="false"
      size="small"
    />
  </n-card>
</template>
