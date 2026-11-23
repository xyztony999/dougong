/**
 * dockerAppsStore —— 容器管理「应用」页签的清单状态（ADR-033 S-62-01）。
 *
 * 独立于 dockerStore：面板应用清单来自落盘扫描 + 容器实况（零持久化），只在
 * 「应用」页签加载；成败流沿 dockerStore 各清单的口径（失败保留上一投影 + 错误态）。
 */
import { ref } from 'vue'
import { defineStore } from 'pinia'
import { appList } from '@/api/docker'
import type { DockerAppItem } from '@/types/docker'

export const useDockerAppsStore = defineStore('docker-apps', () => {
  const items = ref<DockerAppItem[]>([])
  const total = ref(0)
  const generatedAt = ref<string | null>(null)
  const loading = ref(false)
  const loaded = ref(false)
  const loadError = ref<string | null>(null)

  /** load 清单（成功覆盖投影；失败保留上一投影并置错误态——拦截器已提示） */
  async function load() {
    loading.value = true
    try {
      const data = await appList()
      items.value = data.items
      total.value = data.total
      generatedAt.value = data.generatedAt
      loadError.value = null
    } catch (e) {
      loadError.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
      loaded.value = true
    }
  }

  /** resetView 离开页面时清空（与 dockerStore.resetView 同口径，避免残留上一会话投影） */
  function resetView() {
    items.value = []
    total.value = 0
    generatedAt.value = null
    loadError.value = null
    loaded.value = false
  }

  return { items, total, generatedAt, loading, loaded, loadError, load, resetView }
})
