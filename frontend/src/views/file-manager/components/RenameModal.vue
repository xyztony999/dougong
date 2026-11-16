<script setup lang="ts">
/**
 * 重命名 Dialog（PageSpec 4.4.2，entryRename）。
 * 预填当前名；同名 FILE_004 → warning「目标名称已存在」（D-13 无覆盖决策）；
 * 成功 → 页面重拉（改变目录内容类，刷新纪律）。
 */
import { ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import type { FormInst, FormRules } from 'naive-ui'
import { entryRename } from '@/api/file'
import { ApiError } from '@/api/http'
import type { FileEntry } from '@/types/file'
import { fileActionError } from '../errors'

const show = defineModel<boolean>('show', { default: false })

const props = defineProps<{
  entry: FileEntry | null
}>()

const emit = defineEmits<{
  renamed: []
}>()

const message = useMessage()
const formRef = ref<FormInst | null>(null)
const saving = ref(false)
const newName = ref('')

const rules: FormRules = {
  newName: [
    { required: true, message: '请输入新名称', trigger: ['blur', 'input'] },
    {
      validator: (_rule, value: string) => !value.includes('/') && !value.includes('\\'),
      message: '名称不能包含路径分隔符',
      trigger: ['blur', 'input']
    }
  ]
}

/** 打开时预填当前名（PageSpec 4.4.2） */
watch(show, (v) => {
  if (v && props.entry !== null) {
    newName.value = props.entry.name
    formRef.value?.restoreValidation()
  }
})

async function onSubmit(): Promise<void> {
  if (props.entry === null) return
  await formRef.value?.validate()
  const name = newName.value.trim()
  saving.value = true
  try {
    await entryRename(props.entry.path, name)
    message.success('已重命名')
    show.value = false
    emit('renamed')
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.is('FILE_004')) {
        message.warning('目标名称已存在')
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
    title="重命名"
    style="width: 420px"
    :mask-closable="false"
    @update:show="(v: boolean) => (show = v)"
  >
    <n-form ref="formRef" :model="{ newName }" :rules="rules" label-placement="top">
      <n-form-item label="新名称" path="newName">
        <n-input v-model:value="newName" placeholder="不含路径分隔符" @keydown.enter.prevent />
      </n-form-item>
    </n-form>
    <template #footer>
      <div class="footer">
        <n-button @click="show = false">取消</n-button>
        <n-button type="primary" :loading="saving" @click="onSubmit">确定</n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
