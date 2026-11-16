/**
 * dbStore —— database-mgmt 跨页面共享状态（PageSpec 第 1 节声明 Pinia：authStore + dbStore
 * 「当前实例档案缓存」；枚举常量按第 3 节偏差声明落 types/enums.ts 常量源，不建 dict store）。
 *
 * 共享根因：db-list 与两个治理页同源消费实例档案；engineGuard 路由守卫以本 store 档案校验
 * （PageSpec 第 7 节守卫顺序）。刷新纪律（D-04/I-04）：手动 + 动作驱动，无自动轮询；
 * 凭据/移除/重扫等写动作后由页面调用 loadList() 重取（不信任乐观更新）。
 * 治理页过程数据（库/用户/状态/慢日志/key）跟随请求生命周期留在页面内，不进 Pinia。
 */
import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { instanceList } from '@/api/database'
import type { DbEngine } from '@/types/enums'
import type { CredentialsResult, DbInstanceItem } from '@/types/database'

export const useDbStore = defineStore('db', () => {
  /** ---- 实例档案清单（进入页面/筛选变更/写动作成功后拉取） ---- */
  const items = ref<DbInstanceItem[]>([])
  const generatedAt = ref<string | null>(null)
  /** 首次加载完成标志（engineGuard 直达治理页时判定是否需要先拉清单） */
  const loaded = ref(false)
  const listLoading = ref(false)
  const engineFilter = ref<DbEngine | null>(null)

  async function loadList(): Promise<void> {
    if (listLoading.value) return // 刷新按钮 loading 防抖（service-mgmt 先例）
    listLoading.value = true
    try {
      const data = await instanceList({ engine: engineFilter.value ?? undefined })
      items.value = data.items
      generatedAt.value = data.generatedAt
      loaded.value = true
      // 其余错误（网络等）已由 http 拦截器统一提示；清单保持上一投影（I-04 实况语义由下次成功拉取对齐）
    } finally {
      listLoading.value = false
    }
  }

  /** engineGuard 守卫入口：直达治理页时确保档案已加载（D-07 进程内首探由 list 触发） */
  async function ensureLoaded(): Promise<void> {
    if (!loaded.value) await loadList()
  }

  function setEngineFilter(v: DbEngine | null): void {
    engineFilter.value = v
    void loadList()
  }

  /** 按 instanceId 取档案行（治理页摘要条/守卫校验；未命中返回 null 不虚构） */
  function findById(id: number): DbInstanceItem | null {
    return items.value.find((it) => it.id === id) ?? null
  }

  /** 凭据配置成功行级直更（D-03：响应恒 reachable + version 实况刷新；credentialsSet 派生置位） */
  function applyCredentials(result: CredentialsResult): void {
    const row = findById(result.instanceId)
    if (row === null) return
    row.connState = result.connState
    row.credentialsSet = true
    if (result.version !== undefined && result.version !== '') row.version = result.version
  }

  /** 治理动作前置：可达且凭据已配置（I-05 前置链前端呈现——不可达/未配置以页面横幅前置提示） */
  const governableIds = computed<ReadonlySet<number>>(
    () =>
      new Set(
        items.value
          .filter((it) => it.connState === 'reachable' && it.credentialsSet)
          .map((it) => it.id)
      )
  )

  return {
    items,
    generatedAt,
    loaded,
    listLoading,
    engineFilter,
    loadList,
    ensureLoaded,
    setEngineFilter,
    findById,
    applyCredentials,
    governableIds
  }
})
