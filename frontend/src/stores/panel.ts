/**
 * panelStore —— PageSpec 第 1 节声明的跨页面共享状态：
 * configDetail/pendingRestart（settings 页 + 页面级横幅）、menuTree（框架侧边菜单，全局）。
 * D-P07：menuList 服务端按角色过滤下发，前端不硬编码菜单、不做二次过滤；
 * 布局每次挂载重新拉取（登出换角色登录后不残留上一角色菜单）。
 */
import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import {
  certStatus as fetchCertStatus,
  configDetail as fetchConfigDetail,
  menuList as fetchMenuList
} from '@/api/panel'
import type { PanelCertStatusData, PanelConfigDetail, PanelMenuItem } from '@/types/panel'

export const usePanelStore = defineStore('panel', () => {
  /** 面板配置单例（null=未加载；settings 各 Card 数据源） */
  const configDetail = ref<PanelConfigDetail | null>(null)
  const configLoading = ref(false)
  /** loadConfig 失败标记（settings 页错误态 + 重试） */
  const configError = ref(false)

  /** 面板证书状态视图（v2.0 增量：null=未加载；面板证书 Card 数据源，两角色可见） */
  const certStatusData = ref<PanelCertStatusData | null>(null)
  const certStatusLoading = ref(false)
  /** loadCertStatus 失败标记（失败保留既有值，同 loadConfig 语义） */
  const certStatusError = ref(false)

  /** 框架侧边菜单（服务端按角色过滤后的可见集） */
  const menuItems = ref<PanelMenuItem[]>([])
  const menuLoading = ref(false)
  const menuError = ref(false)

  /** 监听配置待重启（D-P02 派生标识；settings 页警示横幅，重启后随 false 自动消失） */
  const pendingRestart = computed(() => configDetail.value?.pendingRestart === true)

  /** 拉取配置单例（进入 settings + 每次变更成功后统一 re-fetch，PageSpec 5.1 提交粒度决策） */
  async function loadConfig(): Promise<void> {
    configLoading.value = true
    configError.value = false
    try {
      configDetail.value = await fetchConfigDetail()
    } catch {
      configError.value = true
      // 错误已由拦截器统一提示
    } finally {
      configLoading.value = false
    }
  }

  /** 拉取面板证书状态（进入 settings 与 loadConfig 并行；申请/重试成功后 re-fetch，PageSpec 5.1 Card 5） */
  async function loadCertStatus(): Promise<void> {
    certStatusLoading.value = true
    certStatusError.value = false
    try {
      certStatusData.value = await fetchCertStatus()
    } catch {
      certStatusError.value = true
      // 错误已由拦截器统一提示
    } finally {
      certStatusLoading.value = false
    }
  }

  /** 框架侧边菜单（进入框架后调用一次，D-P07） */
  async function loadMenu(): Promise<void> {
    menuLoading.value = true
    menuError.value = false
    menuItems.value = []
    try {
      const data = await fetchMenuList()
      menuItems.value = data.items
    } catch {
      menuError.value = true
      // 错误已由拦截器统一提示
    } finally {
      menuLoading.value = false
    }
  }

  return {
    configDetail,
    configLoading,
    configError,
    certStatusData,
    certStatusLoading,
    certStatusError,
    menuItems,
    menuLoading,
    menuError,
    pendingRestart,
    loadConfig,
    loadCertStatus,
    loadMenu
  }
})
