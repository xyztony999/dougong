<script setup lang="ts">
/**
 * 建用户弹窗（PageSpec 5.3；dbuserCreate）。host 支持 % 通配惯例（api 4.3.4，失焦提示）；
 * 密码 type=password 不回显（I-02 同源纪律）；同名同 host 以 DBM_010 服务端为准。
 */
import { ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import type { FormInst, FormRules } from 'naive-ui'
import { dbuserCreate } from '@/api/database'
import { ApiError } from '@/api/http'
import type { DbUserMutationResult } from '@/types/database'
import { dbmErrorText } from '@/types/database'

const show = defineModel<boolean>('show', { default: false })

const props = defineProps<{
  instanceId: number
}>()

const emit = defineEmits<{
  /** 建用户成功（页面重拉用户清单） */
  created: [result: DbUserMutationResult]
}>()

const message = useMessage()
const formRef = ref<FormInst | null>(null)
const saving = ref(false)

interface CreateUserForm {
  username: string
  host: string
  password: string
}

/** 用户名标识符形态（R-14）；host 允许 hostname/IP/% 通配与 IPv6 冒号（api 4.3.4 惯例） */
const USERNAME_RE = /^(?![0-9]+$)[A-Za-z0-9_$]{1,64}$/
const HOST_RE = /^[A-Za-z0-9._%:-]{1,255}$/

const form = ref<CreateUserForm>({ username: '', host: 'localhost', password: '' })

watch(show, (v) => {
  if (!v) return
  form.value = { username: '', host: 'localhost', password: '' }
  formRef.value?.restoreValidation()
})

const rules: FormRules = {
  username: [
    { required: true, message: '请输入用户名', trigger: ['blur'] },
    {
      validator: (_rule, value: string) => USERNAME_RE.test(value),
      message: '用户名须为标识符（字母/数字/_/$，非纯数字，≤64 字符，R-14）',
      trigger: ['blur']
    }
  ],
  host: [
    { required: true, message: '请输入 host', trigger: ['blur'] },
    {
      validator: (_rule, value: string) => HOST_RE.test(value),
      message: 'host 须为 hostname / IP / % 通配（不含空格与引号）',
      trigger: ['blur']
    }
  ],
  password: [{ required: true, message: '请输入密码', trigger: ['blur', 'input'] }]
}

async function onSubmit(): Promise<void> {
  const valid = await formRef.value?.validate().then(() => true).catch(() => false)
  if (valid !== true) return
  saving.value = true
  try {
    const result = await dbuserCreate({
      instanceId: props.instanceId,
      username: form.value.username,
      host: form.value.host,
      password: form.value.password
    })
    message.success(`用户 ${result.username}@${result.host} 已创建`)
    emit('created', result)
    show.value = false
  } catch (e) {
    if (e instanceof ApiError) {
      const text = dbmErrorText(e.code, e.data) // DBM_010 同名同 host 等
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
    title="新建数据库用户"
    style="width: 440px"
    :mask-closable="false"
  >
    <n-form ref="formRef" :model="form" :rules="rules" label-placement="top">
      <n-form-item label="用户名" path="username">
        <n-input v-model:value="form.username" placeholder="如 app_user" />
      </n-form-item>
      <n-form-item label="host" path="host">
        <n-input v-model:value="form.host" placeholder="localhost（支持 % 通配，如 192.168.%.%）" />
      </n-form-item>
      <n-form-item label="密码" path="password">
        <n-input
          v-model:value="form.password"
          type="password"
          show-password-on="click"
          placeholder="密码不回显"
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
