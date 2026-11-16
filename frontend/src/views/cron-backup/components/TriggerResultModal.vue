<script setup lang="ts">
/**
 * 手动触发结果 Modal（PageSpec 5.2；jobTrigger 同步 120s 语义落法，api D-05）。
 * admin 专属（readonly 无触发按钮不可达）；执行失败/超时为业务结果非请求错误——
 * 结果 Tag 四态 + 耗时灰字 + 输出等宽滚动区（尾部 64KB，超限头部省略标注）。
 * 无重试按钮（重试=再次手动触发，操作路径文案明确）。
 */
import { computed } from 'vue'
import { useCronStore } from '@/stores/cron'
import { formatDurationMs } from '@/utils/format'

/** OUTPUT_MAX=64KB（api 3.4 业务常量；超限保留尾部+头部省略标记，D-05） */
const OUTPUT_MAX = 64 * 1024

const store = useCronStore()

const display = computed(() => store.triggerResult)

const title = computed(() => {
  const expr = display.value?.expr ?? ''
  return expr.length > 24 ? `手动触发结果 — ${expr.slice(0, 24)}…` : `手动触发结果 — ${expr}`
})

/** 结果 Tag（PageSpec 5.2 四态）：超时 > 成功(0) > 失败(n) > 异常终止(null) */
const resultTag = computed<{ type: 'success' | 'error'; label: string } | null>(() => {
  const r = display.value?.result
  if (r === undefined || r === null) return null
  if (r.timedOut) return { type: 'error', label: '超时终止' }
  if (r.exitCode === 0) return { type: 'success', label: '成功（退出码 0）' }
  if (r.exitCode !== null) return { type: 'error', label: `失败（退出码 ${r.exitCode}）` }
  return { type: 'error', label: '异常终止' }
})

/** 头部省略标注：输出达到 64KB 截断上限（唯一可观测信号）时前置标注行 */
const truncated = computed(() => (display.value?.result.output.length ?? 0) >= OUTPUT_MAX)
</script>

<template>
  <n-modal
    :show="display !== null"
    preset="card"
    :title="title"
    style="width: 640px"
    :mask-closable="false"
    @update:show="(v: boolean) => { if (!v) store.closeTriggerResult() }"
  >
    <template v-if="display !== null">
      <div class="result-row">
        <n-tag v-if="resultTag !== null" :type="resultTag.type" size="small" :bordered="false">
          {{ resultTag.label }}
        </n-tag>
        <span class="meta">耗时 {{ formatDurationMs(display.result.durationMs) }}</span>
      </div>
      <div class="meta output-label">输出（stdout + stderr 合并，尾部最多 64KB）</div>
      <div v-if="truncated" class="truncated-mark">…（输出过长，已截断保留尾部）</div>
      <pre class="output mono">{{ display.result.output }}</pre>
    </template>
    <template #footer>
      <div class="modal-footer">
        <span class="meta retry-hint">如需重试请再次手动触发</span>
        <n-button @click="store.closeTriggerResult()">关闭</n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.result-row {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 8px;
}
.meta {
  font-size: 12px;
  opacity: 0.6;
}
.output-label {
  margin-bottom: 4px;
}
.truncated-mark {
  font-size: 12px;
  color: rgba(240, 160, 32, 0.9);
  margin-bottom: 4px;
}
.output {
  max-height: 320px;
  overflow: auto;
  margin: 0;
  padding: 8px;
  border-radius: 4px;
  background: rgba(128, 128, 128, 0.08);
  white-space: pre-wrap;
  word-break: break-all;
}
.mono {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', monospace;
  font-size: 12px;
}
.modal-footer {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 12px;
}
.retry-hint {
  margin-right: auto;
}
</style>
