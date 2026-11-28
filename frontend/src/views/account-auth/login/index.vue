<script setup lang="ts">
import { computed, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useMessage, useThemeVars } from 'naive-ui'
import type { FormInst, FormRules, InputInst } from 'naive-ui'
import { FingerPrintOutline } from '@vicons/ionicons5'
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
const usernameInput = ref<InputInst | null>(null)
const username = ref('')
const password = ref('')
const code = ref('')
const submitting = ref(false)

/** 二步式（D-02）：AUTH_007 后持有 loginTicket（仅内存，R-02），展开动态码输入 */
const loginTicket = ref<string | null>(null)

/** AUTH_004 锁定倒计时（remainingSeconds；期满自动恢复提交） */
const remainingLabel = ref<string | null>(null)
let lockTimer: ReturnType<typeof setInterval> | null = null

/** 登录方式（分段切换；用户名为三种方式共用，放最上面） */
type LoginMode = 'password' | 'passkey' | 'recovery'
const mode = ref<LoginMode>('password')

const inTotpStage = computed(() => loginTicket.value !== null)
const inPasskeyStage = computed(() => passkeyTicket.value !== null)
const locked = computed(() => remainingLabel.value !== null)
const submitText = computed(() => {
  if (locked.value) return `账户已锁定（${remainingLabel.value}）`
  return inTotpStage.value ? '验证动态码' : '登录'
})

const rules = computed<FormRules>(() => ({
  username: { required: true, message: '请输入用户名', trigger: 'blur' },
  password:
    !inTotpStage.value && mode.value === 'password'
      ? { required: true, message: '请输入密码', trigger: 'blur' }
      : {},
  code: inTotpStage.value
    ? {
        required: true,
        message: '请输入 6 位动态码',
        trigger: ['blur', 'input'],
        pattern: /^\d{6}$/
      }
    : {},
  recovery:
    mode.value === 'recovery'
      ? { required: true, message: '请输入恢复码', trigger: 'blur' }
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

function switchMode(next: string): void {
  mode.value = next as LoginMode
  // 清掉上一个方式残留的校验提示，避免切回来时旧报错还挂着
  formRef.value?.restoreValidation()
}

/** 回车提交：按当前所处步骤/方式分派 */
function onKeyEnter(): void {
  if (passkeyTicket.value !== null) {
    void onPasskeyVerify()
    return
  }
  if (inTotpStage.value) {
    void onSubmit()
    return
  }
  if (mode.value === 'passkey') {
    void onPasskeyLogin()
    return
  }
  if (mode.value === 'recovery') {
    void onRecoveryLogin()
    return
  }
  void onSubmit()
}

/** 二因素中途放弃：回到密码方式重新登录（ticket 仅内存，丢弃即失效） */
function resetSecondStage(): void {
  loginTicket.value = null
  passkeyTicket.value = null
  code.value = ''
}

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

// ---- passkey / 恢复码（登录安全增量）----
const supportsPasskey = passkeySupported()
/** 密码后 passkey 二因素的 ticket（requiresPasskey） */
const passkeyTicket = ref<string | null>(null)
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

/** 无密码登录：用户名 + passkey。用户名是三种方式共用的第一格，没填就先提示并聚焦。 */
async function onPasskeyLogin(): Promise<void> {
  if (submitting.value) return
  if (username.value === '') {
    message.info('请先输入用户名')
    usernameInput.value?.focus()
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
    else if (e instanceof ApiError && e.is('AUTH_017')) message.error('该账户还没有注册 passkey，可改用密码登录')
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
  if (submitting.value) return
  if (username.value === '') {
    message.info('请先输入用户名')
    usernameInput.value?.focus()
    return
  }
  if (recoveryCode.value === '') {
    message.info('请输入恢复码')
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
        :model="{ username, password, code, recoveryCode }"
        :rules="rules"
        label-placement="top"
        @keyup.enter="onKeyEnter"
      >
        <n-form-item label="用户名" path="username">
          <n-input
            ref="usernameInput"
            v-model:value="username"
            :disabled="inTotpStage || inPasskeyStage"
            placeholder="用户名"
          />
        </n-form-item>

        <!-- 二因素步骤（选定方式后的第二步）：只保留当前步骤的动作与「返回重新登录」 -->
        <template v-if="inTotpStage">
          <n-form-item label="TOTP 动态码" path="code">
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
          <div class="alt-actions">
            <n-button text @click="resetSecondStage">返回重新登录</n-button>
          </div>
        </template>
        <template v-else-if="inPasskeyStage">
          <n-button
            type="primary"
            block
            :loading="submitting"
            @click="onPasskeyVerify"
          >
            <n-icon :component="FingerPrintOutline" class="btn-icon" />
            用 passkey 验证
          </n-button>
          <p class="method-hint">验证本机的指纹、面部或安全密钥</p>
          <div class="alt-actions">
            <n-button text @click="resetSecondStage">返回重新登录</n-button>
          </div>
        </template>

        <!-- 登录方式分段切换（密码 / passkey / 恢复码），各方式一份完整表单 -->
        <n-tabs v-else :value="mode" type="segment" size="small" class="mode-tabs" @update:value="switchMode">
          <n-tab-pane name="password" tab="密码">
            <n-form-item label="密码" path="password" class="pane-first">
              <n-input
                v-model:value="password"
                type="password"
                show-password-on="click"
                placeholder="密码"
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
          </n-tab-pane>
          <n-tab-pane v-if="supportsPasskey" name="passkey" tab="passkey">
            <n-button
              type="primary"
              block
              :loading="submitting"
              :disabled="locked"
              class="pane-first"
              @click="onPasskeyLogin"
            >
              <n-icon :component="FingerPrintOutline" class="btn-icon" />
              使用 passkey 登录
            </n-button>
            <p class="method-hint">验证本机的指纹、面部或安全密钥</p>
          </n-tab-pane>
          <n-tab-pane name="recovery" tab="恢复码">
            <n-form-item label="恢复码" path="recovery" class="pane-first">
              <n-input v-model:value="recoveryCode" placeholder="如 1A2B-3C4D-5E6F-7G8H" />
            </n-form-item>
            <n-button
              type="primary"
              block
              :loading="submitting"
              :disabled="locked"
              @click="onRecoveryLogin"
            >
              登录
            </n-button>
          </n-tab-pane>
        </n-tabs>
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
.method-hint {
  margin: 8px 0 0;
  font-size: 12px;
  color: v-bind(hintColor);
}
/* 分段条与首格之间留呼吸，窗格自带的上内边距收掉 */
.mode-tabs {
  margin-top: 4px;
}
.mode-tabs :deep(.n-tab-pane) {
  padding-top: 12px;
}
.pane-first {
  margin-bottom: 12px;
}
.btn-icon {
  margin-right: 6px;
  font-size: 18px;
}
.alt-actions {
  margin-top: 10px;
}
</style>
