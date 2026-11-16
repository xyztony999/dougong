<script setup lang="ts">
/**
 * 表达式校验预览区（PageSpec 5.1：表达式字段下方恒占位区）——
 * 任务 Dialog 与备份 Drawer 调度字段共用（D2 口径-2 单一表达式语义双消费）。
 * idle=灰字提示 / checking=校验中 / valid=未来 5 次（本地时区）/ invalid=段位级错误红字。
 */
import { computed } from 'vue'
import { formatDateTime } from '@/utils/format'
import type { ExprValidateState } from '../expr-validate'

const props = defineProps<{
  state: ExprValidateState
  nextFires: string[]
  errorText: string | null
}>()

const previewText = computed(() =>
  props.nextFires.map((iso) => formatDateTime(iso)).join('、')
)
</script>

<template>
  <div class="expr-preview">
    <span v-if="props.state === 'idle'" class="hint">
      输入表达式后自动校验（失焦/防抖 500ms），并预览未来 5 次执行时间
    </span>
    <span v-else-if="props.state === 'checking'" class="hint">校验中…</span>
    <span v-else-if="props.state === 'valid'" class="hint">
      未来 5 次：{{ previewText }}
    </span>
    <span v-else class="error">{{ props.errorText ?? '表达式未通过校验' }}</span>
  </div>
</template>

<style scoped>
.expr-preview {
  min-height: 20px;
  font-size: 12px;
  line-height: 20px;
  margin-top: 4px;
  word-break: break-all;
}
.hint {
  opacity: 0.6;
}
.error {
  color: rgba(255, 80, 80, 0.9);
}
</style>
