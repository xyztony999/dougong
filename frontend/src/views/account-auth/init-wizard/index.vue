<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useMessage, useThemeVars } from 'naive-ui'
import type { FormInst, FormItemRule, FormRules } from 'naive-ui'
import { initWizard } from '@/api/auth'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { fieldErrorsToText } from '@/types/auth'
import { buildOtpauthUri, generateTotpSecret } from '@/utils/totp'

const router = useRouter()
const message = useMessage()
const authStore = useAuthStore()

/** 说明文字颜色随主题（dark 适配修复轮）：textColor2 在暗色下为 0.82 白，保证小字号可读 */
const themeVars = useThemeVars()
const hintColor = computed(() => themeVars.value.textColor2)

interface WizardForm {
  username: string
  password: string
  confirmPassword: string
  totpEnabled: boolean
  totpCode: string
}

const formRef = ref<FormInst | null>(null)
const form = ref<WizardForm>({
  username: '',
  password: '',
  confirmPassword: '',
  totpEnabled: false,
  totpCode: ''
})
const submitting = ref(false)

/** TOTP secret 由前端生成（PageSpec 5.1 设计决策），随向导一并提交 totpSecret+totpCode */
const totpSecret = ref<string | null>(null)

function onTotpSwitch(value: boolean): void {
  form.value.totpEnabled = value
  totpSecret.value = value ? generateTotpSecret() : null
  form.value.totpCode = ''
}

const otpauthUri = computed<string>(() =>
  totpSecret.value === null
    ? ''
    : buildOtpauthUri(totpSecret.value, form.value.username || 'admin')
)

/** 密码强度弱提示（从宽不阻断，PageSpec 5.1） */
const weakPassword = computed(() => {
  const p = form.value.password
  return p.length > 0 && p.length < 12

})

const USERNAME_PATTERN = /^[a-zA-Z0-9_.-]{1,32}$/

const rules = computed<FormRules>(() => ({
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' },
    {
      pattern: USERNAME_PATTERN,
      message: '仅限字母/数字/下划线/点/连字符，长度 1-32',
      trigger: 'blur'
    }
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { min: 8, message: '密码长度至少 8 位（A10）', trigger: 'blur' }
  ],
  confirmPassword: [
    { required: true, message: '请再次输入密码', trigger: 'blur' },
    {
      trigger: 'blur',
      validator: (_rule: FormItemRule, value: string): true | Error =>
        value === form.value.password ? true : new Error('两次输入的密码不一致')
    }
  ],
  totpCode: form.value.totpEnabled
    ? [
        { required: true, message: '请输入 6 位动态码', trigger: 'blur' },
        { pattern: /^\d{6}$/, message: '动态码为 6 位数字', trigger: 'blur' }
      ]
    : []
}))

async function onSubmit(): Promise<void> {
  await formRef.value?.validate()
  submitting.value = true
  try {
    const data = await initWizard({
      username: form.value.username,
      password: form.value.password,
      ...(form.value.totpEnabled && totpSecret.value !== null
        ? { totpSecret: totpSecret.value, totpCode: form.value.totpCode }
        : {})
    })
    authStore.applySession(data)
    message.success(`欢迎使用斗拱面板，${data.user.username}`)
    void router.replace('/')
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.is('AUTH_009')) {
        // 拦截器已提示「面板已初始化」；向导不再可达
        void router.replace('/login')
      } else if (e.is('AUTH_006')) {
        message.error('动态码错误，请重输')
      } else if (e.is('AUTH_012')) {
        const fields = fieldErrorsToText(e.data)
        message.error(`参数校验失败${fields !== null ? `：${fields}` : ''}`)
      }
      // AUTH_011 等其余码已由拦截器提示
    }
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="page">
    <n-card class="card" title="初始化向导 · 创建首个管理员">
      <n-form
        ref="formRef"
        :model="form"
        :rules="rules"
        label-placement="top"
        @keyup.enter="onSubmit"
      >
        <n-form-item label="用户名" path="username">
          <n-input v-model:value="form.username" placeholder="首个管理员用户名" />
        </n-form-item>
        <n-form-item label="密码" path="password">
          <n-input
            v-model:value="form.password"
            type="password"
            show-password-on="click"
            placeholder="至少 8 位"
          />
        </n-form-item>
        <n-form-item v-if="weakPassword" label=" " :show-feedback="false">
          <span class="hint">密码强度偏弱，建议使用 12 位以上并混合多种字符</span>
        </n-form-item>
        <n-form-item label="确认密码" path="confirmPassword">
          <n-input
            v-model:value="form.confirmPassword"
            type="password"
            show-password-on="click"
            placeholder="再次输入密码"
          />
        </n-form-item>
        <n-form-item label="启用 TOTP 两步验证（可选）">
          <n-switch :value="form.totpEnabled" @update:value="onTotpSwitch" />
        </n-form-item>
        <template v-if="form.totpEnabled && totpSecret !== null">
          <n-form-item label="认证器绑定">
            <div class="totp-area">
              <n-qr-code :value="otpauthUri" :size="160" error-correction-level="M" />
              <div class="totp-meta">
                <p class="hint">使用认证器 App 扫码，或手动输入密钥：</p>
                <n-code :code="totpSecret" word-wrap />
              </div>
            </div>
          </n-form-item>
          <n-form-item label="动态码" path="totpCode">
            <n-input
              v-model:value="form.totpCode"
              placeholder="认证器显示的 6 位数字"
              maxlength="6"
              inputmode="numeric"
            />
          </n-form-item>
        </template>
        <n-button
          type="primary"
          block
          attr-type="submit"
          :loading="submitting"
          @click="onSubmit"
        >
          创建管理员
        </n-button>
      </n-form>
      <p class="hint">面板仅可初始化一次，也可通过服务器命令行完成初始化。</p>
    </n-card>
  </div>
</template>

<style scoped>
.page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
}
.card {
  width: 460px;
}
.hint {
  font-size: 12px;
  color: v-bind(hintColor);
}
.totp-area {
  display: flex;
  gap: 16px;
  align-items: flex-start;
}
.totp-meta {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
</style>
