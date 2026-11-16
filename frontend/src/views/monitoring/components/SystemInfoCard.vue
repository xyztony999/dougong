<script setup lang="ts">
import { computed } from 'vue'
import type { SystemInfoData } from '@/types/monitoring'
import { formatUptime } from '@/utils/format'

/**
 * Card A 系统信息（PageSpec 6.1）：60s 慢轮询、无操作按钮；
 * D-08：失败字段 null 显示「暂不可用」，errors[] 内联列出失败原因，不虚构默认值。
 */
const props = defineProps<{ info: SystemInfoData | null }>()

const FIELD_LABELS: Readonly<Record<string, string>> = {
  distro: '发行版',
  kernel: '内核版本',
  hostname: '主机名',
  uptimeSeconds: '运行时长'
}

/** null → '—' 占位（值缺失与「暂不可用」并行表达：描述项占位 + 失败清单列原因） */
const distro = computed(() => props.info?.distro ?? null)
const kernel = computed(() => props.info?.kernel ?? null)
const hostname = computed(() => props.info?.hostname ?? null)

/** 运行时长按 D-02 格式化（「N 天 N 小时」≥2 级粒度） */
const uptimeText = computed(() =>
  props.info !== null && props.info.uptimeSeconds !== null
    ? formatUptime(props.info.uptimeSeconds)
    : null
)

const fieldErrors = computed(() => props.info?.errors ?? [])
</script>

<template>
  <n-card title="系统信息" size="small">
    <n-descriptions label-placement="left" :column="1" bordered size="small">
      <n-descriptions-item label="发行版">
        <span :class="{ muted: distro === null }">{{ distro ?? '暂不可用' }}</span>
      </n-descriptions-item>
      <n-descriptions-item label="内核版本">
        <span :class="{ muted: kernel === null }">{{ kernel ?? '暂不可用' }}</span>
      </n-descriptions-item>
      <n-descriptions-item label="主机名">
        <span :class="{ muted: hostname === null }">{{ hostname ?? '暂不可用' }}</span>
      </n-descriptions-item>
      <n-descriptions-item label="运行时长">
        <span :class="{ muted: uptimeText === null }">{{ uptimeText ?? '暂不可用' }}</span>
      </n-descriptions-item>
    </n-descriptions>
    <ul v-if="fieldErrors.length > 0" class="errors">
      <li v-for="e in fieldErrors" :key="e.field">
        {{ FIELD_LABELS[e.field] ?? e.field }}：{{ e.reason }}
      </li>
    </ul>
  </n-card>
</template>

<style scoped>
.muted {
  opacity: 0.5;
}
.errors {
  margin: 12px 0 0;
  padding-left: 18px;
  font-size: 12px;
  opacity: 0.65;
}
.errors li {
  line-height: 1.8;
}
</style>
