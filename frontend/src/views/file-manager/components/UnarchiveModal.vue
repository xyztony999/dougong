<script setup lang="ts">
/**
 * 解压 Dialog（PageSpec 4.4.6，entryUnarchive，仅归档行）。
 * 目标目录=目录选择器（起始=归档所在目录）+ 冲突策略 NRadioGroup（ask[默认]/overwrite/skip）；
 * FILE_009 → error 安全拦截（zip slip 整体取消零落盘，D-07）；FILE_005（ask 分支）→ 冲突决策。
 */
import { ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { entryUnarchive } from '@/api/file'
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
}>()

const emit = defineEmits<{
  /** 解压成功（页面重拉） */
  done: []
  conflict: [payload: ConflictPayload]
}>()

const message = useMessage()
const store = useFileStore()
const saving = ref(false)
const strategy = ref<ConflictStrategy>('ask')

/** 归档所在目录（起始浏览目录 = path 去除末段名） */
function parentDir(path: string): string {
  const segs = path.split('/').filter(Boolean)
  segs.pop()
  return segs.length === 0 ? '' : `/${segs.join('/')}`
}

watch(show, (v) => {
  if (v && props.entry !== null) {
    strategy.value = 'ask'
    store.openPicker('unarchive', parentDir(props.entry.path))
  } else if (store.pickerMode === 'unarchive') {
    store.closePicker()
  }
})

async function run(archivePath: string, targetDir: string, onConflict: ConflictStrategy): Promise<void> {
  saving.value = true
  try {
    const data = await entryUnarchive(archivePath, targetDir, onConflict)
    message.success(`已解压 ${data.extractedCount} 项至 ${data.targetDir}`)
    show.value = false
    emit('done')
  } catch (e) {
    if (e instanceof ApiError && e.is('FILE_005') && isConflictList(e.data)) {
      emit('conflict', {
        conflicts: e.data,
        allowSkip: true,
        retry: async (s) => {
          await run(archivePath, targetDir, s)
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
        if (info.refreshAfter) emit('done')
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
  void run(props.entry.path, store.pickerPath, strategy.value)
}
</script>

<template>
  <n-modal
    :show="show"
    preset="card"
    title="解压"
    style="width: 520px"
    :mask-closable="false"
    @update:show="(v: boolean) => (show = v)"
  >
    <div v-if="entry !== null" class="source">归档文件：{{ entry.name }}</div>
    <dir-browser />
    <div class="target">目标目录：<code>{{ store.pickerPath === '' ? '受管根' : store.pickerPath }}</code></div>
    <div class="strategy">
      <span class="label">冲突策略</span>
      <n-radio-group v-model:value="strategy">
        <n-radio value="ask">逐项询问（默认）</n-radio>
        <n-radio value="overwrite">覆盖</n-radio>
        <n-radio value="skip">跳过</n-radio>
      </n-radio-group>
    </div>
    <template #footer>
      <div class="footer">
        <n-button @click="show = false">取消</n-button>
        <n-button
          type="primary"
          :loading="saving"
          :disabled="store.pickerLoading || store.pickerPath === ''"
          @click="onSubmit"
        >
          解压
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
.strategy {
  margin-top: 12px;
  display: flex;
  align-items: center;
  gap: 12px;
}
.label {
  font-size: 13px;
}
.footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
