<script setup lang="ts">
/**
 * 新建集合弹窗（PageSpec v1.4 5.13；mongoCollectionCreate）。
 * 顶部说明文案：MongoDB 库随首集合写入产生——新建集合即完成建库（隐式建库语义，api D-25）。
 * 集合名形态白名单本地预校验（validateCollectionName 纯函数，R-14：非空/禁 $ 前缀/禁空字符）；
 * 重复以 DBM_008 服务端为准（data.reason 内联提示）。
 */
import { ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import type { FormInst, FormRules } from 'naive-ui'
import { mongoCollectionCreate } from '@/api/database'
import { ApiError } from '@/api/http'
import type { MongoCollectionMutationResult } from '@/types/database'
import { dbmErrorText, reasonOf, validateCollectionName } from '@/types/database'

const show = defineModel<boolean>('show', { default: false })

const props = defineProps<{
  instanceId: number
  database: string
}>()

const emit = defineEmits<{
  /** 建集合成功（页面重拉集合清单） */
  created: [result: MongoCollectionMutationResult]
}>()

const message = useMessage()
const formRef = ref<FormInst | null>(null)
const saving = ref(false)
const inlineError = ref<string | null>(null)

const form = ref({ name: '' })

watch(show, (v) => {
  if (!v) return
  form.value = { name: '' }
  inlineError.value = null
  formRef.value?.restoreValidation()
})

const rules: FormRules = {
  name: [
    { required: true, message: '请输入集合名', trigger: ['blur'] },
    {
      validator: (_rule, value: string) => validateCollectionName(value) === null,
      message: '集合名须非空、不以 $ 开头、不含空白或空字符（R-14）',
      trigger: ['blur']
    }
  ]
}

async function onSubmit(): Promise<void> {
  const valid = await formRef.value?.validate().then(() => true).catch(() => false)
  if (valid !== true) return
  saving.value = true
  inlineError.value = null
  try {
    const result = await mongoCollectionCreate({
      instanceId: props.instanceId,
      database: props.database,
      name: form.value.name
    })
    message.success(`集合 ${result.name} 已创建`)
    emit('created', result)
    show.value = false
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.is('DBM_008')) {
        // 同名集合实况校验（语义复用，data.reason 引擎原文内联）
        inlineError.value = `同名集合已存在${reasonOf(e.data) !== null ? `：${reasonOf(e.data)}` : ''}`
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
    title="新建集合"
    style="width: 440px"
    :mask-closable="false"
  >
    <n-alert type="info" class="hint">
      MongoDB 库随首集合写入产生——新建集合即完成建库（隐式建库语义）。
    </n-alert>
    <n-alert v-if="inlineError !== null" type="error" class="hint" :title="inlineError" />
    <n-form ref="formRef" :model="form" :rules="rules" label-placement="top">
      <n-form-item label="集合名" path="name">
        <n-input v-model:value="form.name" placeholder="如 orders" />
      </n-form-item>
    </n-form>
    <template #footer>
      <div class="modal-footer">
        <n-button :disabled="saving" @click="show = false">取消</n-button>
        <n-button type="primary" :loading="saving" @click="onSubmit">创建</n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.hint {
  margin-bottom: 12px;
}
.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
