<script setup lang="ts">
import { ref } from 'vue'
import { useMessage } from 'naive-ui'
import type { FormInst, FormRules } from 'naive-ui'
import { userCreate } from '@/api/auth'
import { ApiError } from '@/api/http'
import { PANEL_ROLE } from '@/types/enums'
import type { PanelRole } from '@/types/enums'
import type { UserCreateReq } from '@/types/auth'
import { fieldErrorsToText } from '@/types/auth'

const show = defineModel<boolean>('show', { default: false })

const emit = defineEmits<{
  created: []
}>()

const message = useMessage()
const formRef = ref<FormInst | null>(null)
const saving = ref(false)
/** AUTH_011 回填字段级错误（API 无查重端点——提交时唯一性校验，tasks.md 偏差 1） */
const usernameFeedback = ref<string | null>(null)

interface CreateForm {
  username: string
  password: string
  role: PanelRole
}

const form = ref<CreateForm>({ username: '', password: '', role: 'readonly' })

const USERNAME_PATTERN = /^[a-zA-Z0-9_.-]{1,32}$/

const rules: FormRules = {
  username: [
    { required: true, message: '请输入用户名', trigger: ['blur', 'input'] },
    { pattern: USERNAME_PATTERN, message: '仅限字母/数字/下划线/点/连字符，长度 1-32', trigger: ['blur', 'input'] }
  ],
  password: [
    { required: true, message: '请输入初始密码', trigger: 'blur' },
    { min: 8, message: '密码长度至少 8 位（A10）', trigger: 'blur' }
  ],
  role: { required: true, message: '请选择角色', trigger: 'blur' }
}

const roleOptions = (Object.keys(PANEL_ROLE) as PanelRole[]).map((value) => ({
  label: PANEL_ROLE[value].label,
  value
}))

function resetForm(): void {
  form.value = { username: '', password: '', role: 'readonly' }
  usernameFeedback.value = null
  formRef.value?.restoreValidation()
}

function onClose(v: boolean): void {
  if (!v) resetForm()
}

async function onSubmit(): Promise<void> {
  await formRef.value?.validate()
  saving.value = true
  try {
    const req: UserCreateReq = { ...form.value }
    const data = await userCreate(req)
    message.success(`用户 ${data.username} 已创建`)
    show.value = false
    resetForm()
    emit('created')
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.is('AUTH_011')) {
        usernameFeedback.value = '用户名已存在（AUTH_011）'
      } else if (e.is('AUTH_012')) {
        const fields = fieldErrorsToText(e.data)
        message.error(`参数校验失败${fields !== null ? `：${fields}` : ''}`)
      }
      // 其余码已由拦截器提示
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
    title="新增用户"
    style="width: 460px"
    :mask-closable="false"
    @update:show="onClose"
  >
    <n-form ref="formRef" :model="form" :rules="rules" label-placement="top">
      <n-form-item label="用户名" path="username" :feedback="usernameFeedback ?? undefined">
        <n-input v-model:value="form.username" placeholder="登录用户名" />
      </n-form-item>
      <n-form-item label="初始密码" path="password">
        <n-input
          v-model:value="form.password"
          type="password"
          show-password-on="click"
          placeholder="至少 8 位"
        />
      </n-form-item>
      <n-form-item label="角色" path="role">
        <n-select v-model:value="form.role" :options="roleOptions" />
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
