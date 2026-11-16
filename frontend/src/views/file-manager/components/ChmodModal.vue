<script setup lang="ts">
/**
 * 权限 Dialog（PageSpec 4.4.4，entryChmod）。
 * 3×3 NCheckbox（所有者/组/其他 × 读/写/执行；预勾选自 mode 八进制解析）；
 * 八进制合成值随勾选联动只读展示（合成 ∈ 0~0777，FILE_015 兜底）；
 * 不提供递归选项（D-06 仅作用目标自身）；成功 → 行 mode 直更（响应刷新值，不整表重拉）。
 */
import { computed, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { entryChmod } from '@/api/file'
import { ApiError } from '@/api/http'
import type { FileEntry } from '@/types/file'
import { fileActionError } from '../errors'

const show = defineModel<boolean>('show', { default: false })

const props = defineProps<{
  entry: FileEntry | null
}>()

const emit = defineEmits<{
  /** 成功（页面以响应 mode 直更该行） */
  done: [path: string, mode: string]
}>()

const message = useMessage()
const saving = ref(false)

/** 勾选矩阵 [owner, group, other] × [read, write, exec] */
type TriState = [boolean, boolean, boolean]
type Matrix = [TriState, TriState, TriState]
const matrix = ref<Matrix>([
  [false, false, false],
  [false, false, false],
  [false, false, false]
])

const ROW_LABELS = ['所有者', '组', '其他'] as const
const COL_LABELS = ['读', '写', '执行'] as const

/** mode 八进制（如 `0644`）→ 勾选矩阵（取末三位数字逐位拆 rwx） */
function parseMode(mode: string): Matrix {
  const digits = mode.slice(-3).padStart(3, '0')
  const toTri = (ch: string): TriState => {
    const d = Number.parseInt(ch, 8)
    return [(d & 4) !== 0, (d & 2) !== 0, (d & 1) !== 0]
  }
  return [toTri(digits[0] ?? '0'), toTri(digits[1] ?? '0'), toTri(digits[2] ?? '0')]
}

/** 勾选矩阵 → 八进制字符串（`0` + 三位） */
const octalText = computed<string>(() => {
  const digits = matrix.value.map((tri) => String((tri[0] ? 4 : 0) + (tri[1] ? 2 : 0) + (tri[2] ? 1 : 0)))
  return `0${digits.join('')}`
})

watch(show, (v) => {
  if (v && props.entry !== null) {
    matrix.value = parseMode(props.entry.mode)
  }
})

async function onSubmit(): Promise<void> {
  if (props.entry === null) return
  saving.value = true
  try {
    const data = await entryChmod(props.entry.path, octalText.value)
    message.success('权限已修改')
    show.value = false
    emit('done', data.path, data.mode)
  } catch (e) {
    if (e instanceof ApiError) {
      const info = fileActionError(e)
      if (info !== null) {
        if (info.level === 'warning') message.warning(info.text)
        else message.error(info.text)
      }
    }
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    title="权限"
    style="width: 420px"
    :mask-closable="false"
    @update:show="(v: boolean) => (show = v)"
  >
    <div class="grid-head">
      <span />
      <span v-for="col in COL_LABELS" :key="col" class="col-label">{{ col }}</span>
    </div>
    <div v-for="(row, ri) in matrix" :key="ri" class="grid-row">
      <span class="row-label">{{ ROW_LABELS[ri] }}</span>
      <n-checkbox v-for="ci in 3" :key="ci" v-model:checked="row[ci - 1]" />
    </div>
    <div class="octal">
      八进制值：<code>{{ octalText }}</code>（仅作用目标自身，不递归）
    </div>
    <template #footer>
      <div class="footer">
        <n-button @click="show = false">取消</n-button>
        <n-button type="primary" :loading="saving" @click="onSubmit">保存</n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.grid-head,
.grid-row {
  display: grid;
  grid-template-columns: 72px repeat(3, 1fr);
  align-items: center;
  padding: 6px 0;
}
.col-label {
  font-size: 12px;
  opacity: 0.65;
  text-align: center;
}
.row-label {
  font-size: 13px;
}
.octal {
  margin-top: 12px;
  font-size: 13px;
}
.footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
