/**
 * firewallStore —— PageSpec 第 1 节显式声明的状态面：
 * backend（status 结论）/ ruleItems / portItems / generatedAt / listParams / inFlight / addDialog 态。
 *
 * 刷新纪律（api D-01 按需拉取 + PageSpec 第 2 节纪律表）：
 * - 进入页面 backendStatus 先行 → backend≠none 时激活 Tab 懒加载对应清单（首次切 Tab 亦然）；
 * - create/delete 成功 → 重拉 ruleList + portList 标记过期（Tab 切回端口对照时重拉，D-04 派生口径）；
 *   backendStatus 不重拉（规则操作不改变后端识别结论）；FW_006（等价已存在）未发生变更 → 不重拉；
 * - 无自动轮询（store 不持定时器，document.hidden 无需处理）；页面卸载 resetView() 清理
 *   Dialog/inFlight/Tab 过期标记。
 * FW_009（正常流程不可达，status 先行）→ 兜底降级引导态；FW_010 → listUnavailable=true 且清空数据
 * （I-01 不显示过期数据，数据区不渲染）。
 */
import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { ApiError } from '@/api/http'
import { backendStatus as fetchBackendStatus, portList, ruleList } from '@/api/firewall'
import type {
  FirewallBackendStatusData,
  FirewallPortItem,
  FirewallRuleItem
} from '@/types/firewall'
import type { FirewallRuleAction, FirewallRuleProtocol } from '@/types/enums'

/** Tab 1 查询条件（PageSpec 4.1：参数变更即触发 ruleList） */
export interface FirewallListParams {
  /** firewall_rule_action 多选，空=全部（逗号拼接 query action） */
  action: FirewallRuleAction[]
  /** 单选，null=全部 */
  protocol: FirewallRuleProtocol | null
}

export const useFirewallStore = defineStore('firewall', () => {
  /** ---- 页头后端状态（4.0，恒为页面第一数据源） ---- */
  const backend = ref<FirewallBackendStatusData | null>(null)
  const statusLoading = ref(false)
  /** 首拉网络级失败（拦截器已提示；页头呈重试入口） */
  const statusFailed = ref(false)
  /** backend=none 引导态判定（api FW_009/I-01：无后端≠无规则，不呈现空规则列表） */
  const noBackend = computed(() => backend.value?.backend === 'none')

  /** ---- Tab 1 端口规则 ---- */
  const ruleItems = ref<FirewallRuleItem[]>([])
  const ruleTotal = ref(0)
  const ruleGeneratedAt = ref<string | null>(null)
  const ruleLoading = ref(false)
  /** Tab 懒加载标记（首次进入 Tab 拉取） */
  const ruleLoaded = ref(false)
  const listParams = ref<FirewallListParams>({ action: [], protocol: null })

  /** ---- Tab 2 监听端口对照 ---- */
  const portItems = ref<FirewallPortItem[]>([])
  const portTotal = ref(0)
  const portGeneratedAt = ref<string | null>(null)
  const portLoading = ref(false)
  const portLoaded = ref(false)
  /** 写操作成功后的过期标记（D-04：openState 为规则×监听读取时派生；切回 Tab 时重拉） */
  const portStale = ref(false)

  /** FW_010 页面级横幅开关（后端已识别但不可用；true=数据区不渲染，I-01） */
  const listUnavailable = ref(false)

  /** ---- 行级在途互斥（PageSpec 4.3：该行删除按钮 loading/禁用 + 添加按钮禁用） ---- */
  const inFlightKeys = ref<ReadonlySet<string>>(new Set<string>())
  const hasInFlight = computed(() => inFlightKeys.value.size > 0)

  /** ---- 添加规则 Dialog（PageSpec 第 1 节 addDialog 态；表单字段在组件内） ---- */
  const addDialogVisible = ref(false)

  /** FW_009 兜底：合成 none 结论降级引导态（正常流程不可达——status 先行判定；generatedAt 为本地近似时刻） */
  function markNoBackend(): void {
    backend.value = {
      backend: 'none',
      active: false,
      version: null,
      supportedActions: [],
      generatedAt: new Date().toISOString()
    }
    ruleItems.value = []
    ruleTotal.value = 0
    ruleGeneratedAt.value = null
    portItems.value = []
    portTotal.value = 0
    portGeneratedAt.value = null
    listUnavailable.value = false
  }

  /** FW_010：清空过期数据 + 页面级横幅（I-01 不显示过期数据） */
  function markUnavailable(): void {
    listUnavailable.value = true
    ruleItems.value = []
    ruleTotal.value = 0
    ruleGeneratedAt.value = null
    portItems.value = []
    portTotal.value = 0
    portGeneratedAt.value = null
  }

  /** rule/list 随行后端摘要同步页头（api D-02：backend 摘要随行携带；version/supportedActions 仍以 status 为准） */
  function syncBackendSummary(summary: { backend: 'ufw' | 'firewalld' | 'none'; active: boolean }): void {
    if (backend.value === null) return
    backend.value.backend = summary.backend
    backend.value.active = summary.active
  }

  async function loadStatus(): Promise<void> {
    if (statusLoading.value) return // 刷新按钮 loading 防抖
    statusLoading.value = true
    statusFailed.value = false
    try {
      backend.value = await fetchBackendStatus()
    } catch {
      // 网络/会话级错误已由 http 拦截器统一提示；首拉失败时页头呈重试入口
      if (backend.value === null) statusFailed.value = true
    } finally {
      statusLoading.value = false
    }
  }

  async function loadRules(): Promise<void> {
    if (backend.value === null || noBackend.value) return // status 先行（PageSpec 第 2 节纪律）
    if (ruleLoading.value) return
    ruleLoading.value = true
    listUnavailable.value = false
    try {
      const data = await ruleList({
        action:
          listParams.value.action.length > 0 ? listParams.value.action.join(',') : undefined,
        protocol: listParams.value.protocol ?? undefined
      })
      ruleItems.value = data.items
      ruleTotal.value = data.total
      ruleGeneratedAt.value = data.generatedAt
      ruleLoaded.value = true
      syncBackendSummary(data.backend)
    } catch (e) {
      if (e instanceof ApiError && e.is('FW_009')) markNoBackend()
      else if (e instanceof ApiError && e.is('FW_010')) markUnavailable()
      // FW_001（筛选值非法）正常 UI 下拉不可达（PageSpec 第 1 节）；其余由拦截器提示
    } finally {
      ruleLoading.value = false
    }
  }

  async function loadPorts(): Promise<void> {
    if (backend.value === null || noBackend.value) return
    if (portLoading.value) return
    portLoading.value = true
    listUnavailable.value = false
    try {
      const data = await portList()
      portItems.value = data.items
      portTotal.value = data.total
      portGeneratedAt.value = data.generatedAt
      portLoaded.value = true
      portStale.value = false
    } catch (e) {
      if (e instanceof ApiError && e.is('FW_009')) markNoBackend()
      else if (e instanceof ApiError && e.is('FW_010')) markUnavailable()
    } finally {
      portLoading.value = false
    }
  }

  function setActionFilter(actions: FirewallRuleAction[]): void {
    listParams.value.action = [...actions]
    void loadRules()
  }

  function setProtocolFilter(protocol: FirewallRuleProtocol | null): void {
    listParams.value.protocol = protocol
    void loadRules()
  }

  /** create/delete 成功后：portList 标记过期（规则集合已变，D-04 派生口径） */
  function markPortsStale(): void {
    portStale.value = true
  }

  function setInFlight(ruleKey: string): void {
    const next = new Set(inFlightKeys.value)
    next.add(ruleKey)
    inFlightKeys.value = next
  }

  function releaseInFlight(ruleKey: string): void {
    const next = new Set(inFlightKeys.value)
    next.delete(ruleKey)
    inFlightKeys.value = next
  }

  function isInFlight(ruleKey: string): boolean {
    return inFlightKeys.value.has(ruleKey)
  }

  function openAddDialog(): void {
    addDialogVisible.value = true
  }

  function closeAddDialog(): void {
    addDialogVisible.value = false
  }

  /** 页面卸载：清理 Dialog/inFlight/Tab 过期标记（PageSpec 第 2 节纪律末行；投影数据不驻留缓存层） */
  function resetView(): void {
    addDialogVisible.value = false
    inFlightKeys.value = new Set<string>()
    portStale.value = false
  }

  return {
    backend,
    statusLoading,
    statusFailed,
    noBackend,
    ruleItems,
    ruleTotal,
    ruleGeneratedAt,
    ruleLoading,
    ruleLoaded,
    listParams,
    portItems,
    portTotal,
    portGeneratedAt,
    portLoading,
    portLoaded,
    portStale,
    listUnavailable,
    hasInFlight,
    addDialogVisible,
    loadStatus,
    loadRules,
    loadPorts,
    setActionFilter,
    setProtocolFilter,
    markPortsStale,
    markNoBackend,
    markUnavailable,
    setInFlight,
    releaseInFlight,
    isInFlight,
    openAddDialog,
    closeAddDialog,
    resetView
  }
})
