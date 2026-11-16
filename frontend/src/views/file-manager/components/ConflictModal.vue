<script setup lang="ts">
/**
 * 冲突决策 Modal（PageSpec 4.4.8，FILE_005 统一处理，D-07 前端落法）。
 * 触发：任一写 Action 返回 FILE_005（data.conflicts[]）；ask 分支服务端零落盘——
 * 前端无「部分完成」中间态。选项：覆盖全部→重发 onConflict=overwrite；跳过全部→重发 skip
 * （还原场景无此项，allowSkip=false，api D-07）；取消→关闭不重发（=放弃）。
 * files 页（上传/移动/解压）与 trash 页（还原）共用。
 */
import { computed } from 'vue'
import { conflictLabel } from '@/types/file'
import type { ConflictEntry, ConflictStrategy } from '@/types/file'

export interface ConflictPayload {
  conflicts: ConflictEntry[]
  /** 还原场景=false（无「跳过全部」，api D-07） */
  allowSkip: boolean
  /** 携带决策重发原 Action（成功后由闭包完成收尾：message/刷新/关闭原 Dialog） */
  retry: (strategy: Exclude<ConflictStrategy, 'ask'>) => Promise<void>
}

const props = defineProps<{
  payload: ConflictPayload | null
}>()

const emit = defineEmits<{
  /** 用户做出决策（modal 内已置 sending；调用方执行 retry） */
  decide: [strategy: Exclude<ConflictStrategy, 'ask'>]
  /** 取消=放弃（不重发，api D-07） */
  cancel: []
}>()

const show = computed(() => props.payload !== null)

function onOverwrite(): void {
  emit('decide', 'overwrite')
}

function onSkip(): void {
  emit('decide', 'skip')
}

function onCancel(): void {
  emit('cancel')
}
</script>

<template>
  <n-modal :show="show" preset="card" title="冲突决策" style="width: 480px" :mask-closable="false">
    <div class="body">
      <p class="hint">目标位置存在同名条目（服务端未做任何改动，选择处理方式后重试）：</p>
      <ul class="list">
        <li v-for="(c, i) in payload?.conflicts ?? []" :key="i" class="item">
          {{ conflictLabel(c) }}
        </li>
      </ul>
    </div>
    <template #footer>
      <div class="footer">
        <n-button @click="onCancel">取消（放弃）</n-button>
        <n-button v-if="payload?.allowSkip === true" @click="onSkip">跳过全部</n-button>
        <n-button type="primary" @click="onOverwrite">覆盖全部</n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.body {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.hint {
  margin: 0;
  font-size: 13px;
}
.list {
  margin: 0;
  padding: 8px 12px;
  max-height: 240px;
  overflow: auto;
  border-radius: 4px;
  background: rgba(128, 128, 128, 0.08);
}
.item {
  padding: 2px 0;
  font-size: 13px;
  word-break: break-all;
}
.footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
