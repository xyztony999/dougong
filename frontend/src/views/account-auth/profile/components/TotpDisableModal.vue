<script setup lang="ts">
import { ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import type { FormInst, FormRules } from 'naive-ui'
import { totpDisable } from '@/api/auth'
import { ApiError } from '@/api/http'

const show = defineModel<boolean>('show', { default: false })

const emit = defineEmits<{
  disabled: []
}>()

const message = useMessage()
const formRef = ref<FormInst | null>(null)
const password = ref('')
const saving = ref(false)

watch(show, (v) => {
  if (!v) {
    password.value = ''
    formRef.value?.restoreValidation()
  }
})

const rules: FormRules = {
  password: { required: true, message: '请输入登录密码确认', trigger: 'blur' }
}

async function onSubmit(): Promise<void> {
  await formRef.value?.validate()
  saving.value = true
  try {
    await totpDisable({ password: password.value })
    message.success('TOTP 两步验证已解绑')
    emit('disabled')
  } catch (e) {
    // 旧密码比对失败等错误码由拦截器统一提示（AUTH_005 语义：密码错误）
    if (e instanceof ApiError && e.is('AUTH_005')) {
      message.error('密码错误')
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
    title="解绑 TOTP 两步验证"
    style="width: 420px"
    :mask-closable="false"
  >
    <p class="hint">解绑后登录将不再要求动态码（api FR-13：需登录密码确认）。</p>
    <n-form ref="formRef" :model="{ password }" :rules="rules" label-placement="top">
      <n-form-item label="登录密码" path="password">
        <n-input
          v-model:value="password"
          type="password"
          show-password-on="click"
          @keyup.enter="onSubmit"
        />
      </n-form-item>
    </n-form>
    <template #footer>
      <div class="footer">
        <n-button @click="show = false">取消</n-button>
        <n-button type="primary" :loading="saving" @click="onSubmit">确认解绑</n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped>
.hint {
  font-size: 12px;
  opacity: 0.65;
  margin: 0 0 12px;
}
.footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>
