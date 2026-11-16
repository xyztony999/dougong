/**
 * serviceStore —— PageSpec 第 1 节声明的跨页面共享状态：
 * units/total/generatedAt/listParams/unitDetail/inFlightSet（含属性 Drawer 态）。
 *
 * 刷新纪律（api D-03 + PageSpec 第 2 节）：手动 + 操作驱动，无自动轮询（本 store 不持定时器，
 * document.hidden 无需处理）；动作成功以响应内操作后快照直更该行 + generatedAt 更新为响应时刻，
 * 不整表重拉；页面卸载 resetView() 清理 Drawer/inFlight 状态。
 * SVC_008 → listUnavailable=true 且清空数据（I-01 不显示过期数据，表格区域不渲染）。
 */
import { ref } from 'vue'
import { defineStore } from 'pinia'
import { ApiError } from '@/api/http'
import { unitDetail as fetchUnitDetail, unitList } from '@/api/service'
import { isServiceActiveState } from '@/types/enums'
import type { ServiceActiveState } from '@/types/enums'
import type { ServiceUnitDetail, ServiceUnitItem } from '@/types/service'
import { SVC_ERROR_TEXT } from '@/types/service'

/** 列表查询条件（PageSpec 4.1：参数变更即触发 unitList） */
export interface ServiceListParams {
  state: ServiceActiveState[]
  search: string
}

/** Drawer 错误态：SVC_003 服务不存在 / SVC_008 通道不可用（PageSpec 第 6 节） */
export type DetailError = 'notfound' | 'unavailable' | null

/** 动作成功响应的行级直更补丁（generatedAt 同步更新为响应时刻） */
export interface UnitRowPatch {
  name: string
  activeState?: ServiceActiveState
  subState?: string
  enabledState?: string
}

export const useServiceStore = defineStore('service', () => {
  /** ---- 列表（进入页面/刷新按钮/参数变更拉取） ---- */
  const units = ref<ServiceUnitItem[]>([])
  const total = ref(0)
  const generatedAt = ref<string | null>(null)
  const listLoading = ref(false)
  /** SVC_008 页面级横幅开关（true=表格区域不渲染） */
  const listUnavailable = ref(false)
  /** SVC_002 兜底文案（正常 UI 不可达；store 无 Message 上下文，页面 watch 提示——monitoring 先例） */
  const listBizError = ref<string | null>(null)
  const listParams = ref<ServiceListParams>({ state: [], search: '' })

  /** ---- 属性 Drawer（打开时刻快照，不自动刷新——PageSpec 第 6 节） ---- */
  const drawerOpen = ref(false)
  const drawerName = ref<string | null>(null)
  const unitDetail = ref<ServiceUnitDetail | null>(null)
  const detailLoading = ref(false)
  const detailError = ref<DetailError>(null)

  /** ---- 行级在途互斥（PageSpec 4.3：该行全部操作按钮与自启开关置 loading/禁用） ---- */
  const inFlightSet = ref<ReadonlySet<string>>(new Set<string>())

  async function loadList(): Promise<void> {
    if (listLoading.value) return // 刷新按钮 loading 防抖
    listLoading.value = true
    listUnavailable.value = false
    listBizError.value = null
    try {
      const data = await unitList({
        state:
          listParams.value.state.length > 0 ? listParams.value.state.join(',') : undefined,
        search: listParams.value.search
      })
      units.value = data.items
      total.value = data.total
      generatedAt.value = data.generatedAt
    } catch (e) {
      if (e instanceof ApiError && e.is('SVC_008')) {
        listUnavailable.value = true
        units.value = []
        total.value = 0
        generatedAt.value = null
      } else if (e instanceof ApiError && e.is('SVC_002')) {
        listBizError.value = SVC_ERROR_TEXT['SVC_002'] ?? '参数非法'
      }
      // 其余（网络等）已由 http 拦截器统一提示
    } finally {
      listLoading.value = false
    }
  }

  function setStateFilter(states: ServiceActiveState[]): void {
    listParams.value.state = [...states]
    void loadList()
  }

  function setSearch(term: string): void {
    listParams.value.search = term
    void loadList()
  }

  /** SVC_008（动作路径归一为页面级横幅 + 清空过期数据，I-01） */
  function markListUnavailable(): void {
    listUnavailable.value = true
    units.value = []
    total.value = 0
    generatedAt.value = null
  }

  function setInFlight(name: string): void {
    const next = new Set(inFlightSet.value)
    next.add(name)
    inFlightSet.value = next
  }

  function releaseInFlight(name: string): void {
    const next = new Set(inFlightSet.value)
    next.delete(name)
    inFlightSet.value = next
  }

  function isInFlight(name: string): boolean {
    return inFlightSet.value.has(name)
  }

  /** 动作成功：响应快照直更该行（不整表重拉，D-03）+ generatedAt=响应时刻 */
  function applySnapshot(patch: UnitRowPatch): void {
    const row = units.value.find((u) => u.name === patch.name)
    if (row !== undefined) {
      if (patch.activeState !== undefined) row.activeState = patch.activeState
      if (patch.subState !== undefined) row.subState = patch.subState
      if (patch.enabledState !== undefined) row.enabledState = patch.enabledState
    }
    generatedAt.value = new Date().toISOString()
  }

  /** SVC_007：行数据以响应实际终态直更（透传 string，非三态不入行——防御收窄） */
  function applyActiveStateRaw(name: string, raw: string): void {
    if (!isServiceActiveState(raw)) return
    applySnapshot({ name, activeState: raw })
  }

  /** ---- Drawer ---- */

  async function loadDetail(name: string): Promise<void> {
    detailLoading.value = true
    detailError.value = null
    unitDetail.value = null
    try {
      const data = await fetchUnitDetail(name)
      if (!drawerOpen.value) return // 关闭后到达的迟到响应不写入
      unitDetail.value = data
    } catch (e) {
      if (e instanceof ApiError) {
        if (e.is('SVC_003')) detailError.value = 'notfound'
        else if (e.is('SVC_008')) detailError.value = 'unavailable'
      }
      // SVC_001（名称形态）正常 UI 不可达（name 恒来自列表投影）；其余由拦截器提示
    } finally {
      detailLoading.value = false
    }
  }

  function openDrawer(name: string): void {
    drawerOpen.value = true
    drawerName.value = name
    void loadDetail(name)
  }

  /** Drawer 头部「刷新」图标按钮：重拉 unitDetail（快照不自动刷新） */
  function refreshDetail(): void {
    if (drawerName.value !== null) void loadDetail(drawerName.value)
  }

  function closeDrawer(): void {
    drawerOpen.value = false
    drawerName.value = null
    unitDetail.value = null
    detailError.value = null
    detailLoading.value = false
  }

  /** 页面卸载：清理 Drawer/inFlight 状态（PageSpec 第 2 节刷新纪律末行） */
  function resetView(): void {
    closeDrawer()
    inFlightSet.value = new Set<string>()
  }

  return {
    units,
    total,
    generatedAt,
    listLoading,
    listUnavailable,
    listBizError,
    listParams,
    drawerOpen,
    drawerName,
    unitDetail,
    detailLoading,
    detailError,
    inFlightSet,
    loadList,
    setStateFilter,
    setSearch,
    markListUnavailable,
    setInFlight,
    releaseInFlight,
    isInFlight,
    applySnapshot,
    applyActiveStateRaw,
    openDrawer,
    refreshDetail,
    closeDrawer,
    resetView
  }
})
