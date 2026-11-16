/**
 * trashStore —— PageSpec 第 1 节声明：trashItems/total/inFlightSet（还原逐行在途）。
 * 数据源 trashList（GET /files/trash/list，两角色）；deleted_at 倒序服务端固定（无排序控件，D-03）；
 * 无 generatedAt（api 4.1.4——页面不渲染投影时刻，与 files 页差异）；
 * 刷新纪律：进入页面/刷新按钮/还原/清空成功后重拉；无轮询。
 */
import { ref } from 'vue'
import { defineStore } from 'pinia'
import { trashList } from '@/api/file'
import type { TrashItem } from '@/types/file'

export const useTrashStore = defineStore('trash', () => {
  const items = ref<TrashItem[]>([])
  const total = ref(0)
  const listLoading = ref(false)
  /** 孤儿对账剔除事实（后端 data.warning，A-F11/PRD N2——页面 NAlert 呈现；W1 修复） */
  const listWarning = ref<string | null>(null)
  /** 行级在途互斥（还原按钮 loading/禁用） */
  const inFlightSet = ref<ReadonlySet<number>>(new Set<number>())

  async function loadList(): Promise<void> {
    if (listLoading.value) return // 刷新按钮 loading 防抖
    listLoading.value = true
    try {
      const data = await trashList()
      items.value = data.items
      total.value = data.total
      listWarning.value = data.warning ?? null
    } catch {
      // 错误（网络/AUTH 等）已由 http 拦截器统一提示；保留既有数据（trash 无页面级横幅语义）
    } finally {
      listLoading.value = false
    }
  }

  function setInFlight(id: number): void {
    const next = new Set(inFlightSet.value)
    next.add(id)
    inFlightSet.value = next
  }

  function releaseInFlight(id: number): void {
    const next = new Set(inFlightSet.value)
    next.delete(id)
    inFlightSet.value = next
  }

  function isInFlight(id: number): boolean {
    return inFlightSet.value.has(id)
  }

  /** 页面卸载：清理在途态（刷新纪律末行） */
  function resetView(): void {
    inFlightSet.value = new Set<number>()
  }

  return {
    items,
    total,
    listLoading,
    listWarning,
    inFlightSet,
    loadList,
    setInFlight,
    releaseInFlight,
    isInFlight,
    resetView
  }
})
