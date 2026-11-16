/**
 * authStore —— 跨页面共享的认证态（PageSpec 第 1 节：Pinia authStore(currentUser/csrfToken)）。
 * R-02 纪律：csrfToken/loginTicket 只存内存（Pinia state），不落 localStorage。
 * 服务端数据不进 Pinia（列表数据跟随请求生命周期，留在页面内）。
 */
import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { initCheck, sessionCurrent } from '@/api/auth'
import { ApiError } from '@/api/http'
import type { LoginResultData, SessionUser } from '@/types/auth'

/** sessionCurrent 探测结果：ok=已登录；unauthenticated=AUTH_001；expired=AUTH_002；error=其他失败 */
export type ProbeResult = 'ok' | 'unauthenticated' | 'expired' | 'error'

export const useAuthStore = defineStore('auth', () => {
  /** 当前登录用户（null=未登录/未探测） */
  const currentUser = ref<SessionUser | null>(null)
  /** 会话绑定 CSRF token（内存态；POST 经 http.ts 注入 X-CSRF-Token） */
  const csrfToken = ref<string | null>(null)
  /** 初始化状态缓存（null=未探测；守卫首跳 initCheck 后缓存，PageSpec 第 7 节） */
  const initialized = ref<boolean | null>(null)

  const isAdmin = computed(() => currentUser.value?.role === 'admin')

  /** login / totpVerify / initWizard 成功后写入登录态 */
  function applySession(data: LoginResultData): void {
    currentUser.value = data.user
    csrfToken.value = data.csrfToken
  }

  /** 登出/会话失效：仅清内存态（Cookie 由服务端过期） */
  function reset(): void {
    currentUser.value = null
    csrfToken.value = null
  }

  /** 路由守卫首跳：initCheck（api I-01/InitGuard），结果缓存 */
  async function refreshInitState(): Promise<boolean> {
    const data = await initCheck()
    initialized.value = data.initialized
    return data.initialized
  }

  /** sessionCurrent 探测（api 4.2.4；AUTH_001 非错误流） */
  async function probeCurrent(): Promise<ProbeResult> {
    try {
      const data = await sessionCurrent()
      currentUser.value = data.user
      csrfToken.value = data.csrfToken
      return 'ok'
    } catch (e) {
      if (e instanceof ApiError) {
        if (e.code === 'AUTH_001') return 'unauthenticated'
        if (e.code === 'AUTH_002') return 'expired'
      }
      return 'error'
    }
  }

  return {
    currentUser,
    csrfToken,
    initialized,
    isAdmin,
    applySession,
    reset,
    refreshInitState,
    probeCurrent
  }
})
