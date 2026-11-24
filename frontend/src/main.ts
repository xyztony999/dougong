import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { router, assetFailureHandlers } from './router'
import { installPreloadErrorGuard, installUnhandledRejectionGuard } from './router/navFailure'
import { registerAuthFailureHandler, registerCsrfProvider } from './api/http'
import { useAuthStore } from './stores/auth'

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)

// http 拦截器依赖注入（避免 http ↔ store/router 循环引用）：
// - POST 统一携带 X-CSRF-Token（R-02）
// - AUTH_001/002 全局失效：清 authStore + 跳 /login
const authStore = useAuthStore(pinia)
registerCsrfProvider(() => authStore.csrfToken)
registerAuthFailureHandler(() => {
  authStore.reset()
  void router.replace({ path: '/login' })
})

// 页面内的按需模块（如在线编辑器）没加载下来时也要有提示和出路：
// 与路由跳转引起的失败共用同一套处理（PageSpec panel-core 第 7 节 v1.3）
installPreloadErrorGuard(window, assetFailureHandlers)
// 这类失败抛出的 Promise 拒绝已经有提示，不再在控制台报成未捕获错误
installUnhandledRejectionGuard(window)

app.use(router)
app.mount('#app')
