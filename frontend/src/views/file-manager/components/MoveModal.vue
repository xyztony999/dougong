<script setup lang="ts">
/**
 * 移动 Dialog（PageSpec 4.4.3，entryMove + 内嵌 mini 目录浏览器）。
 * 起始=当前浏览目录；「选择此目录」→ entryMove {sourcePath, targetDir, onConflict:"ask"}；
 * 移动到自身/自身子目录的服务端拒绝如实呈现（FILE_013 通道，不做前端预判虚设规则）；
 * FILE_005 → 上抛冲突决策（4.4.8：覆盖全部/跳过全部/取消，重发闭包含 targetDir）。
 */
import { ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { entryMove } from '@/api/file'
import { ApiError } from '@/api/http'
import { useFileStore } from '@/stores/file'
import type { ConflictStrategy, FileEntry } from '@/types/file'
import { isConflictList } from '@/types/file'
import { fileActionError } from '../errors'
import type { ConflictPayload } from './ConflictModal.vue'
import DirBrowser from './DirBrowser.vue'

const show = defineModel<boolean>('show', { default: false })

const props = defineProps<{
  entry: FileEntry | null
  /** 起始浏览目录（当前浏览目录） */
  startDir: string
}>()

const emit = defineEmits<{
  /** 移动成功（页面重拉——改变目录内容类） */
  moved: []
  /** FILE_005 冲突决策上抛（页面统一 ConflictModal） */
  conflict: [payload: ConflictPayload]
}>()

const message = useMessage()
const store = useFileStore()
const saving = ref(false)

watch(show, (v) => {
  if (v) store.openPicker('move', props.startDir)
  else if (store.pickerMode === 'move') store.closePicker()
})

async function run(sourcePath: string, targetDir: string, onConflict: ConflictStrategy): Promise<void> {
  saving.value = true
  try {
    const data = await entryMove(sourcePath, targetDir, onConflict)
    message.success(
      data.skippedCount > 0
        ? `已移动（跳过 ${data.skippedCount} 项同名）`
        : '已移动'
    )
    show.value = false
    emit('moved')
  } catch (e) {
    if (e instanceof ApiError && e.is('FILE_005') && isConflictList(e.data)) {
      // ask 零落盘（D-07）：携带决策重发
      emit('conflict', {
        conflicts: e.data,
        allowSkip: true,
        retry: async (strategy) => {
          await run(sourcePath, targetDir, strategy)
        }
      })
      show.value = false
      return
    }
    if (e instanceof ApiError) {
      const info = fileActionError(e)
      if (info !== null) {
        if (info.level === 'warning') message.warning(info.text)
        else message.error(info.text)
        if (info.refreshAfter) emit('moved')
      }
    }
  } finally {
    saving.value = false
  }
}

function onSubmit(): void {
  if (props.entry === null) return
  // targetDir 必为具体绝对路径（picker 规范化，E1 修复；空串时按钮禁用——防御兜底）
  if (store.pickerPath === '') return
  void run(props.entry.path, store.pickerPath, 'ask')
}
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    title="移动到…"
    style="width: 520px"
    :mask-closable="false"
    @update:show="(v: boolean) => (show = v)"
  >
    <div v-if="entry !== null" class="source">移动对象：{{ entry.name }}</div>
    <dir-browser />
    <div class="target">目标目录：<code>{{ store.pickerPath === '' ? '受管根' : store.pickerPath }}</code></div>
    <template #footer>
      <div class="footer">
        <n-button @click="show = false">取消</n-button>
        <n-button
          type="primary"
          :loading="saving"
          :disabled="store.pickerLoading || store.pickerPath === ''"
          @click="onSubmit"
        >
          选择此目录
        </n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.source {
  font-size: 13px;
  margin-bottom: 8px;
  word-break: break-all;
  opacity: 0.8;
}
.target {
  margin-top: 10px;
  font-size: 13px;
  word-break: break-all;
}
.footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
