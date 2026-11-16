<script setup lang="ts">
/**
 * 建库弹窗（PageSpec 5.2；databaseCreate）。字符集候选=实例实况（databaseCharsets，
 * D-11 开放集合动态 options），默认 utf8mb4；库名本地预校验（R-14 标识符白名单，
 * 失焦触发），重复以 DBM_008 服务端为准（如实提示）。
 */
import { computed, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import type { FormInst, FormRules, SelectOption } from 'naive-ui'
import { databaseCharsets, databaseCreate } from '@/api/database'
import { ApiError } from '@/api/http'
import type { DatabaseCreateResult } from '@/types/database'
import { dbmErrorText } from '@/types/database'

const show = defineModel<boolean>('show', { default: false })

const props = defineProps<{
  instanceId: number
}>()

const emit = defineEmits<{
  /** 建库成功（页面重拉库清单） */
  created: [result: DatabaseCreateResult]
}>()

const message = useMessage()
const formRef = ref<FormInst | null>(null)
const saving = ref(false)

interface CreateDbForm {
  name: string
  charset: string | null
}

/** 库名标识符白名单形态（R-14：字母/数字/_/$，不以纯数字组成，≤64——服务端终审） */
const DB_NAME_RE = /^(?![0-9]+$)[A-Za-z0-9_$]{1,64}$/

const form = ref<CreateDbForm>({ name: '', charset: 'utf8mb4' })

/** 字符集实况候选（D-11：每次打开弹窗实时读取，无缓存） */
const charsets = ref<string[]>([])
const charsetsLoading = ref(false)

watch(show, async (v) => {
  if (!v) return
  form.value = { name: '', charset: 'utf8mb4' }
  formRef.value?.restoreValidation()
  charsetsLoading.value = true
  try {
    const data = await databaseCharsets(props.instanceId)
    charsets.value = data.items
    // 候选含默认值则归位（实况以服务端为准）
    if (!data.items.includes('utf8mb4') && data.items.length > 0) {
      form.value.charset = data.items[0] ?? null
    }
  } catch {
    // 错误已由拦截器/DBM 分流提示；候选保持空（提交时服务端 AUTH_012/DBM 终审）
  } finally {
    charsetsLoading.value = false
  }
})

const charsetOptions = computed<SelectOption[]>(() =>
  charsets.value.map((c) => ({ label: c, value: c }))
)

const rules = computed<FormRules>(() => ({
  name: [
    { required: true, message: '请输入库名', trigger: ['blur'] },
    {
      validator: (_rule, value: string) => DB_NAME_RE.test(value),
      message: '库名须为标识符（字母/数字/_/$，非纯数字，≤64 字符，R-14）',
      trigger: ['blur']
    }
  ],
  charset: [{ required: true, message: '请选择字符集', trigger: ['blur', 'change'] }]
}))

async function onSubmit(): Promise<void> {
  const valid = await formRef.value?.validate().then(() => true).catch(() => false)
  if (valid !== true || form.value.charset === null) return
  saving.value = true
  try {
    const result = await databaseCreate({
      instanceId: props.instanceId,
      name: form.value.name,
      charset: form.value.charset
    })
    message.success(`数据库 ${result.name} 已创建（字符集 ${result.charset}）`)
    emit('created', result)
    show.value = false
  } catch (e) {
    if (e instanceof ApiError) {
      const text = dbmErrorText(e.code, e.data) // DBM_008 同名/003 不可达/004 未配置 等
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
    title="新建数据库"
    style="width: 440px"
    :mask-closable="false"
  >
    <n-form ref="formRef" :model="form" :rules="rules" label-placement="top">
      <n-form-item label="库名" path="name">
        <n-input v-model:value="form.name" placeholder="如 app_prod" />
      </n-form-item>
      <n-form-item label="字符集" path="charset">
        <n-select
          v-model:value="form.charset"
          :options="charsetOptions"
          :loading="charsetsLoading"
          placeholder="实例实况字符集"
        />
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
.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
