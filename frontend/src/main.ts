import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { router } from './router'
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

app.use(router)
app.mount('#app')
