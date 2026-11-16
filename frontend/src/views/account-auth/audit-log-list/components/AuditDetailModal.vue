<script setup lang="ts">
import { computed } from 'vue'
import type { AuditLogItem } from '@/types/auth'
import { AUDIT_ACTION_TEXT, AUDIT_CATEGORY, AUDIT_RESULT, AUDIT_SOURCE, dictItem } from '@/types/enums'
import { formatDateTime } from '@/utils/format'

const props = defineProps<{
  item: AuditLogItem | null
}>()

const emit = defineEmits<{
  close: []
}>()

function onUpdateShow(v: boolean): void {
  if (!v) emit('close')
}

/** detail 为 JSON 字符串（api 4.3.1 值对象）；解析失败原文展示 */
const detailText = computed<string>(() => {
  if (props.item === null || props.item.detail === null) return ''
  try {
    const parsed: unknown = JSON.parse(props.item.detail)
    return JSON.stringify(parsed, null, 2)
  } catch {
    return props.item.detail
  }
})

const categoryLabel = computed(() =>
  props.item === null ? '' : dictItem(AUDIT_CATEGORY, props.item.category).label
)
const resultLabel = computed(() =>
  props.item === null ? '' : dictItem(AUDIT_RESULT, props.item.result).label
)
const sourceLabel = computed(() =>
  props.item === null ? '' : dictItem(AUDIT_SOURCE, props.item.source).label
)
</script>

<template>
  <n-modal
    :show="item !== null"
    preset="card"
    title="审计详情"
    style="width: 640px"
    @update:show="onUpdateShow"
  >
    <n-descriptions v-if="item !== null" :column="2" bordered size="small">
      <n-descriptions-item label="时间">{{ formatDateTime(item.createdAt) }}</n-descriptions-item>
      <n-descriptions-item label="操作者">{{ item.actorName }}</n-descriptions-item>
      <n-descriptions-item label="类别">{{ categoryLabel }}</n-descriptions-item>
      <n-descriptions-item label="动作">
        {{ AUDIT_ACTION_TEXT[item.action] ?? item.action }}（{{ item.action }}）
      </n-descriptions-item>
      <n-descriptions-item label="对象">
        {{ item.objectId !== null ? `${item.objectType}:${item.objectId}` : item.objectType }}
      </n-descriptions-item>
      <n-descriptions-item label="结果">{{ resultLabel }}</n-descriptions-item>
      <n-descriptions-item label="来源">{{ sourceLabel }}</n-descriptions-item>
      <n-descriptions-item label="记录 ID">{{ item.id }}</n-descriptions-item>
    </n-descriptions>
    <div v-if="detailText !== ''" class="detail">
      <n-code :code="detailText" language="json" word-wrap />
    </div>
    <template #footer>
      <div class="footer">
        <n-button @click="onUpdateShow(false)">关闭</n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.detail {
  margin-top: 12px;
  max-height: 320px;
  overflow: auto;
}
.footer {
  display: flex;
  justify-content: flex-end;
}
</style>
