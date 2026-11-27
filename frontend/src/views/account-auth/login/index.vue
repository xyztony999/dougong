<script setup lang="ts">
import { computed, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useMessage, useThemeVars } from 'naive-ui'
import type { FormInst, FormRules } from 'naive-ui'
import { login, passkeyVerify, recoveryLogin, totpVerify, webauthnLoginBegin, webauthnLoginFinish } from '@/api/auth'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import { extractLoginTicket, extractRemainingSeconds } from '@/types/auth'
import { withBase } from '@/utils/base'
import { formatCountdown } from '@/utils/format'
import { passkeySupported, prepareGetOptions, serializeAssertion } from '@/utils/webauthn'

const router = useRouter()
const message = useMessage()
const authStore = useAuthStore()

/** 面板图标：public/ 下的文件不参与打包，地址要自己带上应用基址（utils/base） */
const logoSrc = withBase('/favicon.svg')

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
        const needPasskey = (e.data as { requiresPasskey?: boolean } | null)?.requiresPasskey === true
        if (ticket !== null) {
          if (needPasskey) {
            passkeyTicket.value = ticket
            message.info('请用 passkey 完成验证')
          } else {
            loginTicket.value = ticket
            message.info('请输入 TOTP 动态码')
          }
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

// ---- passkey / 恢复码（v0.7.0 登录安全增量）----
const supportsPasskey = passkeySupported()
/** 密码后 passkey 二因素的 ticket（requiresPasskey） */
const passkeyTicket = ref<string | null>(null)
const recoveryStage = ref(false)
const recoveryCode = ref('')

/** 取断言凭据（login-begin → navigator.credentials.get）。 */
async function getAssertion(user: string): Promise<{ token: string; credential: unknown }> {
  const begin = await webauthnLoginBegin({ username: user })
  const cred = (await navigator.credentials.get({
    publicKey: prepareGetOptions(begin.options)
  })) as PublicKeyCredential | null
  if (cred === null) throw new ApiError('AUTH_015', '未获取到 passkey', null)
  return { token: begin.token, credential: serializeAssertion(cred) }
}

/** 无密码登录：用户名 + passkey。 */
async function onPasskeyLogin(): Promise<void> {
  if (submitting.value || username.value === '') {
    if (username.value === '') message.info('请先输入用户名')
    return
  }
  submitting.value = true
  try {
    const { token, credential } = await getAssertion(username.value)
    const data = await webauthnLoginFinish({ username: username.value, token, credential })
    authStore.applySession(data)
    message.success(`欢迎，${data.user.username}`)
    void router.replace('/')
  } catch (e) {
    if (e instanceof ApiError && e.is('AUTH_015')) message.error('passkey 验证失败，请重试或用密码登录')
  } finally {
    submitting.value = false
  }
}

/** 密码后 passkey 二因素验证。 */
async function onPasskeyVerify(): Promise<void> {
  if (submitting.value || passkeyTicket.value === null) return
  submitting.value = true
  try {
    const { token, credential } = await getAssertion(username.value)
    const data = await passkeyVerify({ loginTicket: passkeyTicket.value, token, credential })
    authStore.applySession(data)
    message.success(`欢迎，${data.user.username}`)
    void router.replace('/')
  } catch (e) {
    if (e instanceof ApiError && e.is('AUTH_015')) message.error('passkey 验证失败，请重试')
  } finally {
    submitting.value = false
  }
}

/** 恢复码登录。 */
async function onRecoveryLogin(): Promise<void> {
  if (submitting.value || recoveryCode.value === '' || username.value === '') {
    message.info('请输入用户名和恢复码')
    return
  }
  submitting.value = true
  try {
    const data = await recoveryLogin({ username: username.value, code: recoveryCode.value })
    authStore.applySession(data)
    message.success(`欢迎，${data.user.username}`)
    void router.replace('/')
  } catch (e) {
    if (e instanceof ApiError && e.is('AUTH_016')) message.error('恢复码无效或已使用')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="page">
    <img class="login-logo" :src="logoSrc" alt="斗拱 Dougong" />
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

        <!-- passkey 二因素（密码后 requiresPasskey） -->
        <n-button
          v-if="passkeyTicket"
          type="primary"
          block
          style="margin-top: 8px"
          :loading="submitting"
          @click="onPasskeyVerify"
        >
          用 passkey 验证
        </n-button>

        <!-- 恢复码登录（passkey/TOTP 丢失时的兜底） -->
        <template v-if="recoveryStage">
          <n-form-item label="恢复码" path="recovery" style="margin-top: 8px">
            <n-input v-model:value="recoveryCode" placeholder="如 1A2B-3C4D-5E6F-7G8H" />
          </n-form-item>
          <n-button block :loading="submitting" @click="onRecoveryLogin">用恢复码登录</n-button>
        </template>

        <div class="alt-actions">
          <n-button
            v-if="supportsPasskey && !inTotpStage && !passkeyTicket && !recoveryStage"
            text
            type="primary"
            @click="onPasskeyLogin"
          >
            使用 passkey 登录
          </n-button>
          <n-button text @click="recoveryStage = !recoveryStage">
            {{ recoveryStage ? '返回密码登录' : '用恢复码登录' }}
          </n-button>
        </div>
      </n-form>
      <p class="hint">本面板不提供注册与找回密码；重置请使用命令行或联系其他管理员。</p>
    </n-card>
  </div>
</template>

<style scoped>
.page {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 18px;
}
.login-logo {
  width: 64px;
  height: 64px;
}
.card {
  width: 380px;
}
.hint {
  margin: 12px 0 0;
  font-size: 12px;
  color: v-bind(hintColor);
}
.alt-actions {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  margin-top: 10px;
}
</style>
