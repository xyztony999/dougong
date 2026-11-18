<script setup lang="ts">
/**
 * 文档插入/编辑弹窗（PageSpec v1.4 5.14；一弹窗两态复用，JSON 编辑形态）。
 * 前端 JSON.parse 校验先行拦截（parseDocumentJson 纯函数，非法即时内联错误不提交）；
 * 服务端 DBM_026 兜底（api D-23 双层校验）。
 * 插入态：document._id 可选（缺省引擎自动生成 ObjectId），默认 {} 模板。
 * 编辑态：_id 只读展示（定位键不可变更——JSON 内 _id 与原值不一致前端拦截，服务端 AUTH_012）；
 * id=行文档 _id 原值透传（ObjectId {"$oid":"hex"} 形态如实）；ReplaceOne 整体替换语义。
 * DBM_027 内联「文档已不存在」+刷新；_id 冲突 DBM_008 语义 data.reason 内联。
 */
import { computed, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { mongoDocumentInsert, mongoDocumentUpdate } from '@/api/database'
import { ApiError } from '@/api/http'
import type { MongoDocValue, MongoDocument } from '@/types/database'
import { dbmErrorText, docWithIdFirst, formatDocId, parseDocumentJson, reasonOf } from '@/types/database'

const show = defineModel<boolean>('show', { default: false })

const props = defineProps<{
  instanceId: number
  database: string
  collection: string
  /** insert=插入态（默认 {} 模板）；edit=编辑态（行文档 JSON 格式化，_id 置顶） */
  mode: 'insert' | 'edit'
  doc: MongoDocument | null
}>()

const emit = defineEmits<{
  /** 插入/编辑成功（页面重拉当前页） */
  saved: []
  /** DBM_027 文档定位失败（页面刷新文档页，弹窗内联提示保持打开） */
  stale: []
}>()

const message = useMessage()
const saving = ref(false)
const inlineError = ref<string | null>(null)
const jsonText = ref('{}')

/** 编辑态定位键（行文档 _id 原值透传，ObjectId 扩展 JSON 形态如实） */
const originalId = computed<MongoDocValue>(() => props.doc?._id ?? null)

watch(show, (v) => {
  if (!v) return
  inlineError.value = null
  if (props.mode === 'edit' && props.doc !== null) {
    jsonText.value = JSON.stringify(docWithIdFirst(props.doc), null, 2)
  } else {
    jsonText.value = '{}'
  }
})

async function onSubmit(): Promise<void> {
  // 前端拦截为先（非法 JSON 即时内联错误，不提交——D-23 双层校验）
  const parsed = parseDocumentJson(jsonText.value)
  if (!parsed.ok) {
    inlineError.value = parsed.error
    return
  }
  const document = parsed.doc
  if (props.mode === 'edit') {
    if (props.doc === null) return
    // _id 不可变更：JSON 内 _id 若存在必须与定位键一致（缺省由服务端以 id 补入）
    if ('_id' in document && JSON.stringify(document._id) !== JSON.stringify(originalId.value)) {
      inlineError.value = '_id 为定位键不可变更（请与原值保持一致，或删除 JSON 中的 _id 字段）'
      return
    }
  }
  saving.value = true
  inlineError.value = null
  try {
    if (props.mode === 'insert') {
      await mongoDocumentInsert({
        instanceId: props.instanceId,
        database: props.database,
        collection: props.collection,
        document
      })
      message.success('文档已插入')
    } else {
      await mongoDocumentUpdate({
        instanceId: props.instanceId,
        database: props.database,
        collection: props.collection,
        id: originalId.value,
        document
      })
      message.success('文档已更新')
    }
    emit('saved')
    show.value = false
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.is('DBM_027')) {
        inlineError.value = '文档已不存在'
        emit('stale')
        return
      }
      if (e.is('DBM_026')) {
        inlineError.value = `文档 JSON 非法${reasonOf(e.data) !== null ? `：${reasonOf(e.data)}` : ''}`
        return
      }
      if (e.is('DBM_008')) {
        inlineError.value = `_id 冲突${reasonOf(e.data) !== null ? `：${reasonOf(e.data)}` : ''}`
        return
      }
      const text = dbmErrorText(e.code, e.data)
      if (text !== null) message.error(text)
    }
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <n-modal
    v-model:show="show"
    preset="card"
    :title="mode === 'insert' ? `插入文档 · ${collection}` : `编辑文档 · ${collection}`"
    style="width: 640px"
    :mask-closable="false"
  >
    <n-alert v-if="inlineError !== null" type="error" class="inline-error" :title="inlineError" />
    <!-- 编辑态 _id 只读展示（定位键，JSON 内 _id 不可变更） -->
    <div v-if="mode === 'edit'" class="id-line">
      <n-text depth="3">_id（只读）：</n-text>
      <n-text code>{{ formatDocId(originalId) }}</n-text>
    </div>
    <n-input
      v-model:value="jsonText"
      type="textarea"
      :rows="14"
      class="json-editor"
      placeholder='JSON 对象，如 {"name": "示例"}'
    />
    <div class="hint">
      提交前经 JSON 校验（非法即时提示不提交）；ObjectId 以 {"$oid":"…"} 扩展 JSON 形态如实呈现。
    </div>
    <template #footer>
      <div class="modal-footer">
        <n-button :disabled="saving" @click="show = false">取消</n-button>
        <n-button type="primary" :loading="saving" @click="onSubmit">
          {{ mode === 'insert' ? '插入' : '保存' }}
        </n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.inline-error {
  margin-bottom: 12px;
}
.id-line {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 10px;
}
.json-editor {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}
.hint {
  font-size: 12px;
  opacity: 0.6;
  margin-top: 8px;
}
.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
