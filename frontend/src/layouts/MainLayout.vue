<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useMessage } from 'naive-ui'
import { NIcon } from 'naive-ui'
import type { MenuOption } from 'naive-ui'
import type { Component } from 'vue'
import {
  AppsOutline,
  CloudDownloadOutline,
  CloudUploadOutline,
  ConstructOutline,
  CubeOutline,
  DiscOutline,
  DocumentTextOutline,
  FlameOutline,
  FolderOutline,
  GlobeOutline,
  InformationCircleOutline,
  LockClosedOutline,
  PeopleOutline,
  PersonCircleOutline,
  PersonOutline,
  PulseOutline,
  ServerOutline,
  SettingsOutline,
  ShieldCheckmarkOutline,
  TerminalOutline,
  TimeOutline,
  TimerOutline,
  TrashOutline,
  TrendingUpOutline
} from '@vicons/ionicons5'
import { logout } from '@/api/auth'
import { ApiError } from '@/api/http'
import { useDbCrumbs } from '@/composables/useDbCrumbs'
import type { DbCrumb } from '@/composables/useDbCrumbs'
import { useAuthStore } from '@/stores/auth'
import { usePanelStore } from '@/stores/panel'
import { PANEL_ROLE, dictItem } from '@/types/enums'
import type { PanelMenuItem } from '@/types/panel'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const authStore = useAuthStore()
const panelStore = usePanelStore()

function renderIcon(icon: Component): () => ReturnType<typeof h> {
  return () => h(NIcon, null, { default: () => h(icon) })
}

/**
 * 侧边菜单 = menuList 服务端下发（panel-core PageSpec 第 7 节 / api D-P07）：
 * 服务端按角色过滤（admin 全量 / readonly 查看类），前端不硬编码菜单、不做二次过滤。
 * 图标名为 ModuleMenuRegistry 登记字符串，前端维护映射（未知名兜底，tasks 偏差 2）。
 */
const MENU_ICONS: Readonly<Record<string, Component>> = {
  people: PeopleOutline,
  time: TimeOutline,
  document: DocumentTextOutline,
  shield: ShieldCheckmarkOutline,
  settings: SettingsOutline,
  info: InformationCircleOutline,
  // 「用户与安全」分组子项（main.go 登记：users=person / sessions=time /
  // audit-logs=document / profile=person-circle）——分组用 people，与子项区分
  person: PersonOutline,
  'person-circle': PersonCircleOutline,
  // monitoring 菜单项（backend main.go 菜单登记：monitoring=pulse / monitoring-history=trend）
  pulse: PulseOutline,
  trend: TrendingUpOutline,
  // service-mgmt 菜单项（main.go ModuleMenuRegistry 登记名 services；server 为
  // 预置键保留兼容，未命中兜底 AppsOutline）
  services: ServerOutline,
  server: ServerOutline,
  // file-manager 菜单项（main.go 登记名 files；folder 预置键保留兼容，同上兜底口径）
  files: FolderOutline,
  folder: FolderOutline,
  trash: TrashOutline,
  // site-mgmt 菜单项（后端模块 todo：「网站管理」globe /「SSL 证书」lock 预置映射，同上兜底口径）
  globe: GlobeOutline,
  lock: LockClosedOutline,
  // database-mgmt 菜单项（后端模块 todo：「数据库」database 预置映射，同上兜底口径）
  database: DiscOutline,
  // terminal 菜单项（后端模块 todo：ModuleMenuRegistry 登记名暂按 terminal 预置映射；
  // 菜单项仅对 admin 下发——ADR-018 禁 Web 终端，readonly 零可达，服务端不下发即无入口）
  terminal: TerminalOutline,
  // firewall 菜单项（后端模块 todo：「防火墙」flame 预置映射，同上兜底口径；
  // 菜单项对 admin/readonly 均下发——ADR-018 全可看，readonly 可进入查看）
  flame: FlameOutline,
  // docker-mgmt 菜单项（后端已登记 icon 键 container（main.go ModuleMenuRegistry，A-D19），
  // 此处补映射闭环；docker 预置键保留兼容，未命中兜底 AppsOutline；
  // 菜单项对 admin/readonly 均下发——ADR-018 全可看，readonly 可进入查看五类清单与日志）
  docker: CubeOutline,
  container: CubeOutline,
  // cron-backup 菜单项（后端模块 todo：「计划任务」cron /「备份」backup 预置映射，同上兜底口径；
  // 菜单项对 admin/readonly 均下发——ADR-018 全可看，readonly 可进入查看，操作面为其不可见）
  cron: TimerOutline,
  backup: CloudUploadOutline,
  // env-mgmt 菜单项（main.go 登记 icon=env；两角色下发——ADR-018 全可看）
  env: CloudDownloadOutline,
  // db-tools 菜单项（main.go 登记 icon=dbtool；两角色下发——ADR-018 目录/任务可看，
  // 工具访问整体 admin 专属 ⚠️A10：readonly 操作面不渲染+工具路径 gating 不可达）
  dbtool: ConstructOutline
}

function resolveIcon(name: string): Component {
  return MENU_ICONS[name] ?? AppsOutline
}

function toMenuOptions(items: PanelMenuItem[]): MenuOption[] {
  return [...items]
    .sort((a, b) => a.order - b.order)
    .map((it) => {
      const option: MenuOption = {
        label: it.title,
        key: it.path !== '' ? it.path : it.key,
        icon: renderIcon(resolveIcon(it.icon))
      }
      // 契约：叶子节点后端序列化为 children:null（非 undefined），双防护。
      const kids = it.children
      if (kids != null && kids.length > 0) {
        option.children = toMenuOptions(kids)
      }
      return option
    })
}

const menuOptions = computed<MenuOption[]>(() => toMenuOptions(panelStore.menuItems))

/** 侧栏折叠态（品牌区折叠时仅留 logo）。 */
const siderCollapsed = ref(false)

/** 顶栏标题（D-P18）：路由 meta.title 优先；兜底按当前 path 在菜单树（含分组
 * children 遍历）中取 title；再兜底「斗拱面板」。 */
const headerTitle = computed<string>(() => {
  const t = route.meta.title
  if (typeof t === 'string' && t !== '') return t
  const path = route.path
  const find = (items: PanelMenuItem[]): string | null => {
    for (const it of items) {
      if (it.path !== '' && it.path === path) return it.title
      if (it.children != null && it.children.length > 0) {
        const hit = find(it.children)
        if (hit !== null) return hit
      }
    }
    return null
  }
  return find(panelStore.menuItems) ?? '斗拱面板'
})

/**
 * 顶栏面包屑（database-mgmt PageSpec v1.4 7.2，F5——panel-core S-09-01 能力消费）：
 * 数据库域多级路由渲染可点击 crumb 链（末级当前页不可点）；其余模块保持单标题兜底。
 */
const { crumbs } = useDbCrumbs()

function onCrumbClick(crumb: DbCrumb): void {
  if (crumb.path !== null) void router.push(crumb.path)
}

/** 布局每次挂载重新拉取（登出换角色登录后不残留上一角色菜单） */
onMounted(() => {
  void panelStore.loadMenu()
})

function retryMenu(): void {
  void panelStore.loadMenu()
}

const activeKey = computed(() => route.path)

function onMenuSelect(key: string): void {
  void router.push(key)
}

const roleDict = computed(() =>
  dictItem(PANEL_ROLE, authStore.currentUser?.role ?? 'readonly')
)

const logoutLoading = ref(false)

async function onLogout(): Promise<void> {
  logoutLoading.value = true
  let success = false
  try {
    await logout()
    success = true
  } catch (e) {
    // AUTH_001/002：会话已失效，同样按登出跳转（PageSpec 第 7 节顶栏容错）
    if (!(e instanceof ApiError && (e.is('AUTH_001') || e.is('AUTH_002')))) {
      // 其余错误已由拦截器提示
    }
  } finally {
    logoutLoading.value = false
    authStore.reset()
    if (success) message.success('已退出登录')
    void router.replace('/login')
  }
}
</script>

<template>
  <n-layout class="shell">
    <n-layout has-sider class="shell">
      <n-layout-sider
        bordered
        collapse-mode="width"
        :collapsed-width="64"
        :width="220"
        show-trigger
        v-model:collapsed="siderCollapsed"
      >
        <div class="brand" :class="{ 'brand-collapsed': siderCollapsed }">
          <img class="brand-logo" src="/favicon.svg" alt="斗拱 Dougong" />
          <span v-show="!siderCollapsed">斗拱面板</span>
        </div>
        <n-spin v-if="panelStore.menuLoading" size="small" class="menu-state" />
        <div v-else-if="panelStore.menuError" class="menu-state menu-error">
          <span>菜单加载失败</span>
          <n-button size="tiny" @click="retryMenu">重试</n-button>
        </div>
        <n-menu
          v-else
          :options="menuOptions"
          :value="activeKey"
          :collapsed-width="64"
          :collapsed-icon-size="20"
          @update:value="onMenuSelect"
        />
      </n-layout-sider>
      <n-layout>
        <n-layout-header bordered class="header">
          <!-- v0.1.2 反馈第 5 项（D-P18）：顶栏标题=当前页面标题（路由 meta.title 优先、
           菜单 title 兜底）；v1.4 F5：数据库域升级为 n-breadcrumb 返回导航（7.2 crumb 链，
           末级当前页不可点），其余模块保持单标题兜底（panel-core 通用能力渐进接入） -->
          <n-breadcrumb v-if="crumbs.length > 0" class="header-title">
            <n-breadcrumb-item
              v-for="(crumb, idx) in crumbs"
              :key="idx"
              :class="{ 'crumb-current': crumb.path === null }"
              @click="onCrumbClick(crumb)"
            >
              {{ crumb.label }}
            </n-breadcrumb-item>
          </n-breadcrumb>
          <span v-else class="header-title">{{ headerTitle }}</span>
          <div class="header-right">
            <span class="username">{{ authStore.currentUser?.username ?? '' }}</span>
            <n-tag :type="roleDict.tag" size="small">{{ roleDict.label }}</n-tag>
            <n-button size="small" :loading="logoutLoading" @click="onLogout">登出</n-button>
          </div>
        </n-layout-header>
        <n-layout-content class="content">
          <router-view />
        </n-layout-content>
      </n-layout>
    </n-layout>
  </n-layout>
</template>

<style scoped>
.shell {
  height: 100vh;
}
.brand {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  font-size: 16px;
  padding: 14px 24px;
}
.brand-collapsed {
  justify-content: center;
  padding: 14px 0;
}
.brand-logo {
  width: 26px;
  height: 26px;
  flex: none;
}
.menu-state {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px 0;
}
.menu-error {
  flex-direction: column;
  gap: 8px;
  font-size: 12px;
  opacity: 0.75;
}
.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
  height: 56px;
}
.header-title {
  font-weight: 500;
}
.crumb-current {
  cursor: default;
}
.header-right {
  display: flex;
  align-items: center;
  gap: 12px;
}
.username {
  font-size: 14px;
}
.content {
  padding: 16px 24px;
}
</style>
