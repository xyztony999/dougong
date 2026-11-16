<script setup lang="ts">
/**
 * 压缩 Dialog（PageSpec 4.4.9，entryArchive）。
 * paths=selection 只读摘要「已选 N 项」（不逐项罗列不可编辑）；format=NSelect archive_format
 * 三选一（默认 zip 预选——请求恒带 format）；targetName 选填（留空=服务端缺省 archive.{ext}）；
 * 产物落当前浏览目录；FILE_004 同名直接错误（D-13 无覆盖，不触发冲突决策）。
 */
import { ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import type { FormInst, FormRules, SelectOption } from 'naive-ui'
import { entryArchive } from '@/api/file'
import { ApiError } from '@/api/http'
import { ARCHIVE_FORMAT } from '@/types/enums'
import type { ArchiveFormat } from '@/types/enums'
import type { FileEntry } from '@/types/file'
import { fileActionError } from '../errors'
import { archiveExtension } from '../icons'

const show = defineModel<boolean>('show', { default: false })

const props = defineProps<{
  /** 打包对象（4.1 工具栏 selection≥1 保证） */
  selected: FileEntry[]
}>()

const emit = defineEmits<{
  /** 成功（页面重拉当前列表——改变目录内容类） */
  done: []
}>()

const message = useMessage()
const formRef = ref<FormInst | null>(null)
const saving = ref(false)
const format = ref<ArchiveFormat>('zip')
const targetName = ref('')

const formatOptions: SelectOption[] = (Object.keys(ARCHIVE_FORMAT) as ArchiveFormat[]).map((v) => ({
  label: ARCHIVE_FORMAT[v].label,
  value: v
}))

const rules: FormRules = {
  targetName: [
    {
      validator: (_rule, value: string) => value === '' || (!value.includes('/') && !value.includes('\\')),
      message: '产物名不能包含路径分隔符',
      trigger: ['blur', 'input']
    }
  ]
}

watch(show, (v) => {
  if (v) {
    format.value = 'zip'
    targetName.value = ''
    formRef.value?.restoreValidation()
  }
})

async function onSubmit(): Promise<void> {
  await formRef.value?.validate()
  saving.value = true
  try {
    const data = await entryArchive(
      props.selected.map((e) => e.path),
      format.value,
      targetName.value.trim() === '' ? undefined : targetName.value.trim()
    )
    message.success(`已压缩 ${props.selected.length} 项 → ${data.path}`)
    show.value = false
    emit('done')
  } catch (e) {
    if (e instanceof ApiError) {
      // D-13：产物同名直接错误，无覆盖决策（非 D-07 冲突流）
      if (e.is('FILE_004')) {
        message.warning(`「${targetName.value.trim() === '' ? `archive.${archiveExtension(format.value)}` : targetName.value.trim()}」已存在`)
        return
      }
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
    title="压缩"
    style="width: 440px"
    :mask-closable="false"
    @update:show="(v: boolean) => (show = v)"
  >
    <n-form ref="formRef" :model="{ targetName }" :rules="rules" label-placement="top">
      <n-form-item label="打包对象">
        <span class="summary">已选 {{ selected.length }} 项（产物将落当前浏览目录，含目录递归打包）</span>
      </n-form-item>
      <n-form-item label="格式" path="format">
        <n-select v-model:value="format" :options="formatOptions" />
      </n-form-item>
      <n-form-item label="产物名" path="targetName">
        <n-input v-model:value="targetName" :placeholder="`留空 = archive.${archiveExtension(format)}`" @keydown.enter.prevent />
      </n-form-item>
    </n-form>
    <template #footer>
      <div class="footer">
        <n-button @click="show = false">取消</n-button>
        <n-button type="primary" :loading="saving" @click="onSubmit">压缩</n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.summary {
  font-size: 13px;
  opacity: 0.8;
}
.footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
