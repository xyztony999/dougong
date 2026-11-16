/**
 * site-mgmt 跨页面共享状态（PageSpec 第 1 节声明 Pinia：sitesStore + certsStore；
 * 枚举常量 store 按第 3 节偏差声明落 types/enums.ts 常量源，不建 dict store）。
 *
 * 刷新纪律（D-01 实时扫描投影）：手动 + 动作驱动，无自动轮询（本 store 不持定时器）；
 * 全部写动作成功后由页面调用 loadList() 重取实况投影（不信任乐观更新）。
 * sitesStore 同时作为 cert-list 页申请/部署弹窗的站点数据源（跨页面共享根因）。
 */
import { ref } from 'vue'
import { defineStore } from 'pinia'
import { certList as fetchCertList, siteList } from '@/api/site'
import type { CertSource, CertStatus, SiteEnabled } from '@/types/enums'
import type { CertItem, SiteDuplicate, SiteListItem } from '@/types/site'

export const useSitesStore = defineStore('sites', () => {
  /** ---- 站点列表投影（进入页面/筛选变更/写动作成功后拉取） ---- */
  const items = ref<SiteListItem[]>([])
  const duplicates = ref<SiteDuplicate[]>([])
  const generatedAt = ref<string | null>(null)
  /** nginx 未检出信号（D-15：页面渲染空态引导卡，隐藏工具栏与筛选区） */
  const nginxAvailable = ref(true)
  /** 首次加载完成标志（区分「加载中」与「nginx 不可用空态」） */
  const loaded = ref(false)
  const listLoading = ref(false)
  const enabledFilter = ref<SiteEnabled | null>(null)

  async function loadList(): Promise<void> {
    if (listLoading.value) return // 刷新按钮 loading 防抖（service-mgmt 先例）
    listLoading.value = true
    try {
      const data = await siteList(enabledFilter.value ?? undefined)
      items.value = data.items
      duplicates.value = data.duplicates
      generatedAt.value = data.generatedAt
      nginxAvailable.value = data.nginxAvailable
      loaded.value = true
      // 其余错误（网络等）已由 http 拦截器统一提示；列表保持上一投影（I-06 实况语义由下次成功拉取对齐）
    } finally {
      listLoading.value = false
    }
  }

  function setEnabledFilter(v: SiteEnabled | null): void {
    enabledFilter.value = v
    void loadList()
  }

  /** 按 siteId 取投影行（source 页顶栏站点标识呈现；未命中返回 null 不虚构） */
  function findBySiteId(siteId: string): SiteListItem | null {
    return items.value.find((s) => s.siteId === siteId) ?? null
  }

  return {
    items,
    duplicates,
    generatedAt,
    nginxAvailable,
    loaded,
    listLoading,
    enabledFilter,
    loadList,
    setEnabledFilter,
    findBySiteId
  }
})

export const useCertsStore = defineStore('certs', () => {
  /** ---- 证书清单（进入页面/筛选变更/写动作成功后拉取） ---- */
  const items = ref<CertItem[]>([])
  const generatedAt = ref<string | null>(null)
  const listLoading = ref(false)
  const sourceFilter = ref<CertSource | null>(null)
  const statusFilter = ref<CertStatus | null>(null)

  async function loadList(): Promise<void> {
    if (listLoading.value) return
    listLoading.value = true
    try {
      const data = await fetchCertList({
        source: sourceFilter.value ?? undefined,
        status: statusFilter.value ?? undefined
      })
      items.value = data.items
      generatedAt.value = data.generatedAt
    } finally {
      listLoading.value = false
    }
  }

  function setSourceFilter(v: CertSource | null): void {
    sourceFilter.value = v
    void loadList()
  }

  function setStatusFilter(v: CertStatus | null): void {
    statusFilter.value = v
    void loadList()
  }

  return {
    items,
    generatedAt,
    listLoading,
    sourceFilter,
    statusFilter,
    loadList,
    setSourceFilter,
    setStatusFilter
  }
})
