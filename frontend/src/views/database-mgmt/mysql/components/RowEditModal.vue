<script setup lang="ts">
/**
 * 行编辑弹窗（PageSpec v1.4 5.12；rowUpdate）。前置：仅单列主键表开放（structure.singlePk，
 * I-10/D-22——入口禁用态在页面侧，服务端 DBM_023 为最终守卫）。
 * 主键值只读展示（定位键不可编辑）；各列逐列动态渲染 NInput + NULL NCheckbox
 * （勾选 NULL → 提交 null=SET col=NULL；未勾选按文本提交，MySQL 隐式类型转换）。
 * values 仅含变更列（buildChangedRowValues 纯函数，未改列不提交减少隐式转换面）。
 * DBM_024 内联「行已不存在或已被修改」+ 页面刷新；本轮无插入行入口（OUT-12）。
 */
import { computed, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import type { FormInst } from 'naive-ui'
import { rowUpdate } from '@/api/database'
import { ApiError } from '@/api/http'
import type { TableRow, TableStructureData } from '@/types/database'
import { buildChangedRowValues, dbmErrorText } from '@/types/database'

const show = defineModel<boolean>('show', { default: false })

const props = defineProps<{
  instanceId: number
  database: string
  table: string
  structure: TableStructureData | null
  row: TableRow | null
}>()

const emit = defineEmits<{
  /** 行更新成功（页面重拉当前页） */
  updated: []
  /** DBM_024 行定位失败（页面刷新数据页，弹窗内联提示保持打开） */
  stale: []
}>()

const message = useMessage()
const formRef = ref<FormInst | null>(null)
const saving = ref(false)
const inlineError = ref<string | null>(null)

/** 可编辑列集 = 结构列序排除主键列（定位键不可编辑） */
const editableColumns = computed(() => {
  const s = props.structure
  if (s === null) return []
  return s.columns.filter((c) => c.column !== s.pkColumn)
})

/** 主键定位值（单列主键恒非空；null 形态兜底禁用提交不虚构） */
const pkValue = computed<string | null>(() => {
  const s = props.structure
  if (s === null || s.pkColumn === null || props.row === null) return null
  const v = props.row[s.pkColumn]
  return v === null || v === undefined ? null : String(v)
})

interface ColumnEditState {
  text: string
  isNull: boolean
}

const edited = ref<Record<string, ColumnEditState>>({})

/** 打开时按行现值初始化（null 值列默认勾选 NULL——JSON null≠TS undefined） */
watch(show, (v) => {
  if (!v) return
  inlineError.value = null
  const next: Record<string, ColumnEditState> = {}
  for (const c of editableColumns.value) {
    const raw = props.row?.[c.column]
    if (raw === null || raw === undefined) {
      next[c.column] = { text: '', isNull: true }
    } else {
      next[c.column] = { text: String(raw), isNull: false }
    }
  }
  edited.value = next
  formRef.value?.restoreValidation()
})

async function onSubmit(): Promise<void> {
  const valid = await formRef.value?.validate().then(() => true).catch(() => false)
  if (valid !== true || props.row === null || pkValue.value === null) return
  const values = buildChangedRowValues(props.row, edited.value, props.structure?.pkColumn ?? null)
  if (Object.keys(values).length === 0) {
    message.info('未修改任何列值')
    return
  }
  saving.value = true
  inlineError.value = null
  try {
    await rowUpdate({
      instanceId: props.instanceId,
      database: props.database,
      table: props.table,
      pkValue: pkValue.value,
      values
    })
    message.success('行已更新')
    emit('updated')
    show.value = false
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.is('DBM_024')) {
        inlineError.value = '行已不存在或已被修改（主键旧值定位失败）'
        emit('stale')
        return
      }
      if (e.is('DBM_023')) {
        inlineError.value = '该表无单列主键，不支持行级编辑/删除'
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
    :title="`编辑行 · ${table}`"
    style="width: 560px"
    :mask-closable="false"
  >
    <n-alert v-if="inlineError !== null" type="error" class="inline-error" :title="inlineError" />
    <n-form ref="formRef" label-placement="top">
      <!-- 主键值只读展示（定位键不可编辑，5.12） -->
      <n-form-item v-if="structure !== null && structure.pkColumn !== null" label="主键值（只读）">
        <n-text>{{ structure.pkColumn }} = {{ pkValue ?? '—' }}</n-text>
      </n-form-item>
      <n-form-item
        v-for="col in editableColumns"
        :key="col.column"
        :label="`${col.column}（${col.type}）`"
      >
        <div class="col-edit">
          <n-input
            v-model:value="edited[col.column].text"
            :disabled="edited[col.column].isNull"
            placeholder="文本值（MySQL 隐式类型转换）"
          />
          <n-checkbox v-model:checked="edited[col.column].isNull">NULL</n-checkbox>
        </div>
      </n-form-item>
    </n-form>
    <template #footer>
      <div class="modal-footer">
        <n-button :disabled="saving" @click="show = false">取消</n-button>
        <n-button
          type="primary"
          :loading="saving"
          :disabled="pkValue === null"
          @click="onSubmit"
        >
          保存
        </n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.inline-error {
  margin-bottom: 12px;
}
.col-edit {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
}
.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
