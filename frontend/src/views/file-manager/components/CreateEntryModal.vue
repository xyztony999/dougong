<script setup lang="ts">
/**
 * 新建 Dialog（PageSpec 4.4.1，entryCreate）。
 * 字段：类型 NRadioGroup（file/directory 两值，api req type 取值域）+ 名称 NInput
 * （必填、不含路径分隔符——FILE_007 前端预校验）；FILE_004 → warning「{名称} 已存在」（D-13 无覆盖）。
 */
import { ref } from 'vue'
import { useMessage } from 'naive-ui'
import type { FormInst, FormRules } from 'naive-ui'
import { entryCreate } from '@/api/file'
import { ApiError } from '@/api/http'
import type { CreateEntryType } from '@/types/file'
import { fileActionError } from '../errors'

const show = defineModel<boolean>('show', { default: false })

const props = defineProps<{
  /** 创建目标目录（当前浏览目录） */
  dir: string
}>()

const emit = defineEmits<{
  /** 创建成功（页面按刷新纪律重拉列表） */
  created: []
}>()

const message = useMessage()
const formRef = ref<FormInst | null>(null)
const saving = ref(false)

const form = ref<{ type: CreateEntryType; name: string }>({ type: 'file', name: '' })

/** FILE_007 前端预校验：非空 + 不含路径分隔符（PageSpec 4.4.1） */
const rules: FormRules = {
  name: [
    { required: true, message: '请输入名称', trigger: ['blur', 'input'] },
    {
      validator: (_rule, value: string) => !value.includes('/') && !value.includes('\\'),
      message: '名称不能包含路径分隔符',
      trigger: ['blur', 'input']
    }
  ]
}

function resetForm(): void {
  form.value = { type: 'file', name: '' }
  formRef.value?.restoreValidation()
}

function onClose(v: boolean): void {
  if (!v) resetForm()
}

async function onSubmit(): Promise<void> {
  await formRef.value?.validate()
  const name = form.value.name.trim()
  saving.value = true
  try {
    await entryCreate(props.dir, name, form.value.type)
    message.success(`已创建${form.value.type === 'file' ? '文件' : '目录'} ${name}`)
    show.value = false
    resetForm()
    emit('created')
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.is('FILE_004')) {
        message.warning(`「${name}」已存在`)
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
    title="新建"
    style="width: 420px"
    :mask-closable="false"
    @update:show="onClose"
  >
    <n-form ref="formRef" :model="form" :rules="rules" label-placement="top">
      <n-form-item label="类型" path="type">
        <n-radio-group v-model:value="form.type">
          <n-radio value="file">文件</n-radio>
          <n-radio value="directory">目录</n-radio>
        </n-radio-group>
      </n-form-item>
      <n-form-item label="名称" path="name">
        <n-input v-model:value="form.name" placeholder="不含路径分隔符" @keydown.enter.prevent />
      </n-form-item>
    </n-form>
    <template #footer>
      <div class="footer">
        <n-button @click="show = false">取消</n-button>
        <n-button type="primary" :loading="saving" @click="onSubmit">创建</n-button>
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
