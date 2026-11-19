/**
 * 路由与守卫（PageSpec 第 7 节）。
 * 守卫顺序 = api 3.1 中间件链前端镜像：
 * initCheck（首跳缓存）→ sessionCurrent 探测（AUTH_001→/login；AUTH_002→/login+过期提示）
 * → roles 校验（越界→/403 边界原因页，ADR-018）。
 */
import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useDbStore } from '@/stores/database'
import type { PanelRole } from '@/types/enums'
import { notifyError } from '@/utils/notify'

declare module 'vue-router' {
  interface RouteMeta {
    /** 页面标题（meta.title 供布局/面包屑） */
    title?: string
    /** 仅未登录可访问（/login、/init-wizard） */
    guestOnly?: boolean
    /** 需要有效会话 */
    requiresAuth?: boolean
    /** 允许的角色（缺省=登录即可，admin+readonly） */
    roles?: PanelRole[]
    /**
     * 数据库治理页引擎守卫（database-mgmt PageSpec 第 7 节）：以 dbStore 档案校验——
     * 'mysql'=engine∈mysql/mariadb（mariadb 同 mysql 路由）；'redis'=engine=redis；
     * 'mongodb'=engine=mongodb。不匹配 → DBM_018 语义提示 + 返回 /databases；
     * 档案不存在 → DBM_002 语义 + 返回。v1.4 browse 路由继承同链守卫。
     */
    engineGuard?: 'mysql' | 'redis' | 'mongodb'
    /**
     * 数据库域面包屑静态段声明（database-mgmt PageSpec v1.4 7.2，F5）：
     * MainLayout 按 route.name+params+dbStore 组装 crumb 链（动态段不虚构）。
     */
    crumb?: 'browse-database' | 'browse-table' | 'browse-collection'
  }
}

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/init-wizard',
      name: 'init-wizard',
      component: () => import('@/views/account-auth/init-wizard/index.vue'),
      meta: { title: '初始化向导', guestOnly: true }
    },
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/account-auth/login/index.vue'),
      meta: { title: '登录', guestOnly: true }
    },
    {
      path: '/',
      component: () => import('@/layouts/MainLayout.vue'),
      meta: { requiresAuth: true },
      children: [
        // 首页重定向由面板全局路由决定（PageSpec 第 7 节：非本模块范围）；本模块先落 /users
        { path: '', redirect: '/users' },
        {
          path: 'users',
          name: 'user-list',
          component: () => import('@/views/account-auth/user-list/index.vue'),
          meta: { title: '用户管理' }
        },
        {
          path: 'sessions',
          name: 'session-list',
          component: () => import('@/views/account-auth/session-list/index.vue'),
          meta: { title: '在线会话' }
        },
        {
          path: 'audit-logs',
          name: 'audit-log-list',
          component: () => import('@/views/account-auth/audit-log-list/index.vue'),
          meta: { title: '审计日志' }
        },
        {
          path: 'profile',
          name: 'profile-self',
          component: () => import('@/views/account-auth/profile/index.vue'),
          meta: { title: '个人安全设置' }
        },
        // ---- panel-core（PageSpec 第 7 节：requiresAuth 继承布局路由，admin+readonly 无 roles 限制）----
        {
          path: 'settings',
          name: 'panel-settings',
          component: () => import('@/views/panel-core/settings/index.vue'),
          meta: { title: '面板设置' }
        },
        {
          path: 'about',
          name: 'panel-about',
          component: () => import('@/views/panel-core/about/index.vue'),
          meta: { title: '关于' }
        },
        // ---- monitoring（PageSpec 第 7 节：两路由同守卫继承；菜单项由后端 ModuleMenuRegistry 下发）----
        {
          path: 'monitoring',
          name: 'monitoring',
          component: () => import('@/views/monitoring/index.vue'),
          meta: { title: '系统监控' }
        },
        {
          path: 'monitoring/history',
          name: 'monitoring-history',
          component: () => import('@/views/monitoring/history/index.vue'),
          meta: { title: '历史趋势' }
        },
        // ---- service-mgmt（PageSpec 第 7 节：requiresAuth 继承布局路由；admin+readonly 同路由，
        // 组件内按角色渲染子集——readonly 操作列整列不渲染、自启恒 NTag，ADR-018）----
        {
          path: 'services',
          name: 'services',
          component: () => import('@/views/service-mgmt/index.vue'),
          meta: { title: '服务管理' }
        },
        // ---- file-manager（PageSpec 第 7 节：requiresAuth 继承布局路由；admin+readonly 同路由，
        // 组件内按角色渲染子集——readonly 仅导航/排序/视图/刷新（20 接口负空间）；edit 组件内角色兜底跳回 /files）----
        {
          path: 'files',
          name: 'file-manager',
          component: () => import('@/views/file-manager/index.vue'),
          meta: { title: '文件管理' }
        },
        {
          path: 'files/trash',
          name: 'file-trash',
          component: () => import('@/views/file-manager/trash/index.vue'),
          meta: { title: '回收站' }
        },
        {
          path: 'files/edit',
          name: 'file-edit',
          component: () => import('@/views/file-manager/edit/index.vue'),
          meta: { title: '在线编辑' }
        },
        // ---- site-mgmt（PageSpec 第 7 节：/sites 与 /certificates 两角色同路由组件内子集渲染；
        // site-source 路由级 roles:['admin'] 守卫——readonly 直达 → 403 边界页，ADR-018/⚠️A17）----
        {
          path: 'sites',
          name: 'site-list',
          component: () => import('@/views/site-mgmt/index.vue'),
          meta: { title: '网站管理' }
        },
        {
          path: 'sites/:siteId/source',
          name: 'site-source',
          component: () => import('@/views/site-mgmt/source/index.vue'),
          meta: { title: '源码编辑', roles: ['admin'] }
        },
        {
          path: 'certificates',
          name: 'cert-list',
          component: () => import('@/views/site-mgmt/certificates/index.vue'),
          meta: { title: 'SSL 证书' }
        },
        // ---- database-mgmt（PageSpec 第 7 节：/databases 两角色同路由组件内子集渲染；
        // 治理页二级路由不占菜单位，engineGuard 以 dbStore 档案校验——mariadb 走 mysql 路由）----
        {
          path: 'databases',
          name: 'db-list',
          component: () => import('@/views/database-mgmt/index.vue'),
          meta: { title: '数据库' }
        },
        {
          path: 'databases/mysql/:instanceId',
          name: 'db-mysql-manage',
          component: () => import('@/views/database-mgmt/mysql/index.vue'),
          meta: { title: 'MySQL 治理', engineGuard: 'mysql' }
        },
        {
          path: 'databases/redis/:instanceId',
          name: 'db-redis-manage',
          component: () => import('@/views/database-mgmt/redis/index.vue'),
          meta: { title: 'Redis 治理', engineGuard: 'redis' }
        },
        {
          path: 'databases/mongo/:instanceId',
          name: 'db-mongo-manage',
          component: () => import('@/views/database-mgmt/mongo/index.vue'),
          meta: { title: 'MongoDB 治理', engineGuard: 'mongodb' }
        },
        // v1.4 浏览页三/四级路由（不占菜单位，返回导航经面包屑 7.2；engineGuard 同链）
        {
          path: 'databases/mysql/:instanceId/browse/:database',
          name: 'db-mysql-tables',
          component: () => import('@/views/database-mgmt/mysql/tables.vue'),
          meta: { title: '表清单', engineGuard: 'mysql', crumb: 'browse-database' }
        },
        {
          path: 'databases/mysql/:instanceId/browse/:database/:table',
          name: 'db-mysql-table-detail',
          component: () => import('@/views/database-mgmt/mysql/table-detail.vue'),
          meta: { title: '表详情', engineGuard: 'mysql', crumb: 'browse-table' }
        },
        {
          path: 'databases/mongo/:instanceId/browse/:database',
          name: 'db-mongo-collections',
          component: () => import('@/views/database-mgmt/mongo/collections.vue'),
          meta: { title: '集合清单', engineGuard: 'mongodb', crumb: 'browse-database' }
        },
        {
          path: 'databases/mongo/:instanceId/browse/:database/:collection',
          name: 'db-mongo-documents',
          component: () => import('@/views/database-mgmt/mongo/documents.vue'),
          meta: { title: '文档浏览', engineGuard: 'mongodb', crumb: 'browse-collection' }
        },
        // ---- db-tools（PageSpec db-tools v1.1 第 7 节：requiresAuth 继承布局路由；
        // admin+readonly 同路由组件内子集渲染——readonly 操作区整区不渲染（6 负空间：
        // install/upgrade/uninstall/rotate/open/reopen，ADR-018 + ⚠️A10 工具访问整体
        // admin 专属）；工具本体路径（/dbtool/phpmyadmin/、/dbtool/mongo-express/）与
        // /dbtool/access/reopen 非面板路由——nginx gating 网络层闸门/后端 GET 重定向落地，
        // 前端无路由守卫承载；菜单项「数据库工具」由后端 ModuleMenuRegistry 按角色下发
        // （两角色同项，Order 位于数据库管理之后）----
        {
          path: 'db-tools',
          name: 'db-tools',
          component: () => import('@/views/db-tools/index.vue'),
          meta: { title: '数据库工具' }
        },
        // ---- firewall（PageSpec 第 7 节：requiresAuth 继承布局路由；admin+readonly 同路由，
        // 组件内按角色渲染子集——readonly 无添加按钮+操作列整列不渲染（2 负空间，ADR-018）；
        // 菜单项由后端 ModuleMenuRegistry 按角色下发（防火墙菜单对两角色均可见）----
        {
          path: 'firewall',
          name: 'firewall',
          component: () => import('@/views/firewall/index.vue'),
          meta: { title: '防火墙' }
        },
        // ---- terminal（PageSpec 第 7 节：admin 专属——meta 声明 requiresAuth+requiresAdmin，
        // 本路由表以 roles:['admin'] 承载（守卫第 4 步 → 403 边界页）；readonly 零可达整域禁用
        // （ADR-018：菜单不下发 + 路由拦截 + 服务端 AUTH_008 三层，readonly PageSpec 第 2 节负空间）----
        {
          path: 'terminal',
          name: 'terminal',
          component: () => import('@/views/terminal/index.vue'),
          meta: { title: 'Web 终端', roles: ['admin'] }
        },
        // ---- docker-mgmt（PageSpec 第 7 节：requiresAuth 继承布局路由；admin+readonly 同路由，
        // 组件内按角色渲染子集——readonly 五 Tab 操作列整列不渲染+无拉取按钮（13 写负空间，ADR-018）；
        // 菜单项由后端 ModuleMenuRegistry 按角色下发（容器管理菜单对两角色均可见——readonly 可进入查看，
        // 操作面为其不可见）；观测（stats/日志/WS 跟随）为查看类两角色全保留 ----
        {
          path: 'docker',
          name: 'docker',
          component: () => import('@/views/docker/index.vue'),
          meta: { title: '容器管理' }
        },
        // ---- cron-backup（PageSpec 第 7 节：requiresAuth 继承布局路由；admin+readonly 同路由，
        // 组件内按角色渲染子集——readonly 10 负空间（两表操作列整列不渲染+新建按钮/任务 Dialog/
        // 任务 Drawer/触发 Modal/download 按钮不渲染，ADR-018）；菜单项由后端 ModuleMenuRegistry
        // 按角色下发（计划任务/备份两菜单对两角色均可见——readonly 可进入查看）----
        {
          path: 'cron',
          name: 'cron-jobs',
          component: () => import('@/views/cron-backup/index.vue'),
          meta: { title: '计划任务' }
        },
        {
          path: 'cron/backup',
          name: 'cron-backup',
          component: () => import('@/views/cron-backup/backup/index.vue'),
          meta: { title: '备份' }
        },
        // ---- env-mgmt（PageSpec env-mgmt v1.0 第 7 节：requiresAuth 继承布局路由；
        // admin+readonly 同路由，组件内按角色渲染子集——readonly 卡片操作区整区不渲染
        //（安装/升级/卸载/服务操作/自启开关，7 负空间），任务 Drawer 只读可看，ADR-018；
        // 菜单项「环境管理」由后端 ModuleMenuRegistry 按角色下发（两角色可见）----
        {
          path: 'env',
          name: 'env-mgmt',
          component: () => import('@/views/env-mgmt/index.vue'),
          meta: { title: '环境管理' }
        }
      ]
    },
    {
      path: '/403',
      name: 'forbidden',
      component: () => import('@/views/common/403.vue'),
      meta: { title: '无权限', requiresAuth: true }
    },
    { path: '/:pathMatch(.*)*', redirect: '/' }
  ]
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()

  // 1. 初始化判定（api I-01/InitGuard 前端镜像；首跳缓存）
  if (auth.initialized === null) {
    try {
      await auth.refreshInitState()
    } catch {
      // initCheck 不可达（网络层）：放行，由页面级请求暴露错误（不做臆测跳转）
      return true
    }
  }
  // 未初始化：除 /init-wizard 外全部重定向（PageSpec 5.1 进入条件）
  if (auth.initialized === false) {
    return to.path === '/init-wizard' ? true : { path: '/init-wizard', replace: true }
  }
  // 已初始化访问向导 → /login（PageSpec 5.1）
  if (to.path === '/init-wizard') {
    return { path: '/login', replace: true }
  }

  const requiresAuth = to.matched.some((r) => r.meta.requiresAuth === true)

  // 2. 已登录访问 guestOnly 页 → 框架首页（PageSpec 5.2）
  if (to.meta.guestOnly === true) {
    if (auth.currentUser === null) {
      const probe = await auth.probeCurrent()
      if (probe === 'ok') return { path: '/', replace: true }
    } else {
      return { path: '/', replace: true }
    }
    return true
  }

  // 3. 需会话路由：首访探测（AUTH_001→/login；AUTH_002→/login+提示过期）
  if (requiresAuth && auth.currentUser === null) {
    const probe = await auth.probeCurrent()
    if (probe !== 'ok') {
      if (probe === 'expired') notifyError('登录已过期，请重新登录')
      return { path: '/login', replace: true }
    }
  }

  // 4. roles 校验（越界 → 403 边界原因页，ADR-018；本模块页面均 admin+readonly，无受限路由）
  if (to.meta.roles !== undefined && auth.currentUser !== null) {
    if (!to.meta.roles.includes(auth.currentUser.role)) {
      return { path: '/403', replace: true }
    }
  }

  // 5. engineGuard（database-mgmt PageSpec 第 7 节）：dbStore 档案校验——
  //    档案不存在（DBM_002 语义）或引擎不匹配（DBM_018 语义）→ toast + 返回 /databases
  if (to.meta.engineGuard !== undefined) {
    const db = useDbStore()
    await db.ensureLoaded()
    const rawId = to.params.instanceId
    const instanceId =
      typeof rawId === 'string' && /^\d+$/.test(rawId) ? Number(rawId) : null
    const instance = instanceId === null ? null : db.findById(instanceId)
    if (instance === null) {
      notifyError('实例档案不存在（可能已被移除），已返回实例清单')
      return { path: '/databases', replace: true }
    }
    const guard = to.meta.engineGuard
    const engineOk =
      guard === 'redis'
        ? instance.engine === 'redis'
        : guard === 'mongodb'
          ? instance.engine === 'mongodb'
          : instance.engine === 'mysql' || instance.engine === 'mariadb'
    if (!engineOk) {
      notifyError('实例引擎与页面不符（DBM_018），已返回实例清单')
      return { path: '/databases', replace: true }
    }
  }

  return true
})

export { router }
