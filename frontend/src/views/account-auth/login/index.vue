<script setup lang="ts">
import { computed, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useMessage, useThemeVars } from 'naive-ui'
import type { FormInst, FormRules } from 'naive-ui'
import { login, totpVerify } from '@/api/auth'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { extractLoginTicket, extractRemainingSeconds } from '@/types/auth'
import { formatCountdown } from '@/utils/format'

const router = useRouter()
const message = useMessage()
const authStore = useAuthStore()

/** 说明文字颜色随主题（dark 适配修复轮）：textColor2 在暗色下为 0.82 白，保证小字号可读 */
const themeVars = useThemeVars()
const hintColor = computed(() => themeVars.value.textColor2)

const formRef = ref<FormInst | null>(null)
const username = ref('')
const password = ref('')
const code = ref('')
const submitting = ref(false)

/** 二步式（D-02）：AUTH_007 后持有 loginTicket（仅内存，R-02），展开动态码输入 */
const loginTicket = ref<string | null>(null)

/** AUTH_004 锁定倒计时（remainingSeconds；期满自动恢复提交） */
const remainingLabel = ref<string | null>(null)
let lockTimer: ReturnType<typeof setInterval> | null = null

const inTotpStage = computed(() => loginTicket.value !== null)
const locked = computed(() => remainingLabel.value !== null)
const submitText = computed(() => {
  if (locked.value) return `账户已锁定（${remainingLabel.value}）`
  return inTotpStage.value ? '验证动态码' : '登录'
})

const rules = computed<FormRules>(() => ({
  username: { required: true, message: '请输入用户名', trigger: 'blur' },
  password:
    inTotpStage.value
      ? {}
      : { required: true, message: '请输入密码', trigger: 'blur' },
  code: inTotpStage.value
    ? {
        required: true,
        message: '请输入 6 位动态码',
        trigger: ['blur', 'input'],
        pattern: /^\d{6}$/
      }
    : {}
}))

function updateRemaining(deadline: number): boolean {
  const left = Math.ceil((deadline - Date.now()) / 1000)
  if (left <= 0) {
    remainingLabel.value = null
    return false
  }
  remainingLabel.value = formatCountdown(left)
  return true
}

function startLockCountdown(seconds: number): void {
  stopLockCountdown()
  const deadline = Date.now() + seconds * 1000
  updateRemaining(deadline)
  lockTimer = setInterval(() => {
    if (!updateRemaining(deadline)) stopLockCountdown()
  }, 1000)
}

function stopLockCountdown(): void {
  if (lockTimer !== null) {
    clearInterval(lockTimer)
    lockTimer = null
  }
}

onUnmounted(() => {
  stopLockCountdown()
})

async function onSubmit(): Promise<void> {
  if (locked.value || submitting.value) return
  await formRef.value?.validate()
  submitting.value = true
  try {
    if (loginTicket.value === null) {
      const data = await login({ username: username.value, password: password.value })
      authStore.applySession(data)
      message.success(`欢迎，${data.user.username}`)
      void router.replace('/')
    } else {
      const data = await totpVerify({ loginTicket: loginTicket.value, code: code.value })
      authStore.applySession(data)
      message.success(`欢迎，${data.user.username}`)
      void router.replace('/')
    }
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.is('AUTH_007')) {
        const ticket = extractLoginTicket(e.data)
        if (ticket !== null) {
          loginTicket.value = ticket
          message.info('请输入 TOTP 动态码')
        }
      } else if (e.is('AUTH_004')) {
        const seconds = extractRemainingSeconds(e.data)
        if (seconds !== null && seconds > 0) startLockCountdown(seconds)
        message.error('账户已锁定，禁止登录')
      } else if (e.is('AUTH_005')) {
        message.error('用户名或密码错误')
      } else if (e.is('AUTH_006')) {
        message.error('动态码错误，请重输')
      } else if (e.is('AUTH_013')) {
        loginTicket.value = null
        code.value = ''
        message.error('验证已失效，请重新登录')
      }
      // 其余错误码已由拦截器统一提示
    }
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="page">
    <n-card class="card" title="登录斗拱面板">
      <n-form
        ref="formRef"
        :model="{ username, password, code }"
        :rules="rules"
        label-placement="top"
        @keyup.enter="onSubmit"
      >
        <n-form-item label="用户名" path="username">
          <n-input
            v-model:value="username"
            :disabled="inTotpStage"
            placeholder="用户名"
          />
        </n-form-item>
        <n-form-item label="密码" path="password">
          <n-input
            v-model:value="password"
            type="password"
            show-password-on="click"
            :disabled="inTotpStage"
            placeholder="密码"
          />
        </n-form-item>
        <n-form-item v-if="inTotpStage" label="TOTP 动态码" path="code">
          <n-input
            v-model:value="code"
            placeholder="6 位数字动态码"
            maxlength="6"
            inputmode="numeric"
          />
        </n-form-item>
        <n-button
          type="primary"
          block
          attr-type="submit"
          :loading="submitting"
          :disabled="locked"
          @click="onSubmit"
        >
          {{ submitText }}
        </n-button>
      </n-form>
      <p class="hint">本面板不提供注册与找回密码；重置请使用命令行或联系其他管理员。</p>
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
  width: 380px;
}
.hint {
  margin: 12px 0 0;
  font-size: 12px;
  color: v-bind(hintColor);
}
</style>
