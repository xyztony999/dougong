<script setup lang="ts">
import { ref } from 'vue'
import { useMessage } from 'naive-ui'
import type { FormInst, FormRules } from 'naive-ui'
import { userResetPassword } from '@/api/auth'
import type { UserListItem } from '@/types/auth'

const props = defineProps<{
  user: UserListItem | null
}>()

const emit = defineEmits<{
  close: []
  reset: []
}>()

const message = useMessage()
const formRef = ref<FormInst | null>(null)
const newPassword = ref('')
const saving = ref(false)

const rules: FormRules = {
  newPassword: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { min: 8, message: '密码长度至少 8 位（A10）', trigger: 'blur' }
  ]
}

function onUpdateShow(v: boolean): void {
  if (!v) {
    newPassword.value = ''
    formRef.value?.restoreValidation()
    emit('close')
  }
}

async function onSubmit(): Promise<void> {
  if (props.user === null) return
  await formRef.value?.validate()
  saving.value = true
  try {
    await userResetPassword({ userId: props.user.id, newPassword: newPassword.value })
    // api FR：重置成功吊销该用户全部会话（cascade）
    message.success(`已重置 ${props.user.username} 的密码，该用户全部会话已下线`)
    newPassword.value = ''
    emit('close')
    emit('reset')
  } catch {
    // 错误已由拦截器统一提示
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <n-modal
    :show="user !== null"
    preset="card"
    :title="`重置密码 · ${user?.username ?? ''}`"
    style="width: 420px"
    :mask-closable="false"
    @update:show="onUpdateShow"
  >
    <n-form ref="formRef" :model="{ newPassword }" :rules="rules" label-placement="top">
      <n-form-item label="新密码" path="newPassword">
        <n-input
          v-model:value="newPassword"
          type="password"
          show-password-on="click"
          placeholder="至少 8 位"
        />
      </n-form-item>
    </n-form>
    <template #footer>
      <div class="footer">
        <n-button @click="onUpdateShow(false)">取消</n-button>
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
