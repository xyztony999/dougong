/**
 * 数据库域顶栏面包屑（database-mgmt PageSpec v1.4 7.2，F5；panel-core S-09-01 通用能力消费口径）。
 * crumb 链：数据库 → {引擎} 治理·{identity} → {database} → {table|collection}；
 * 末级=当前页不可点（path=null），其余各级可点 router.push 对应层级。
 * 动态段组装：identity=dbStore 档案缓存（identityKey 呈现 host:port/socket 路径，D-01）；
 * 档案未加载先 ensureLoaded（I-05 不虚构——找不到档案显示 instanceId 原值兜底）；
 * database/table/collection=route.params 原值（vue-router 已 URL decode）。
 * 其余模块保持单标题兜底不强制改（panel-core 通用能力渐进接入）。
 * buildDbCrumbs 为纯函数（MainLayout 与单测共用，CI 无路由/store 依赖）。
 */
import { computed, watch } from 'vue'
import type { ComputedRef } from 'vue'
import { useRoute } from 'vue-router'
import { useDbStore } from '@/stores/database'
import { DB_ENGINE } from '@/types/enums'
import type { DbInstanceItem } from '@/types/database'

export interface DbCrumb {
  label: string
  /** 点击目标路径；null=末级当前页不可点 */
  path: string | null
}

export interface DbCrumbInput {
  routeName: string
  /** 路由 params.instanceId 原值（档案缺失兜底呈现） */
  instanceId: string
  database?: string
  table?: string
  collection?: string
  crumb?: 'browse-database' | 'browse-table' | 'browse-collection'
  instance: Pick<DbInstanceItem, 'engine' | 'connMode' | 'socketPath' | 'host' | 'port'> | null
}

/** 数据库域路由名集合（治理页+浏览页；db-list 单 crumb） */
const DB_MANAGE_ROUTES: Readonly<Record<string, string>> = {
  'db-mysql-manage': 'mysql',
  'db-redis-manage': 'redis',
  'db-mongo-manage': 'mongo'
}
const DB_BROWSE_ROUTES: Readonly<Record<string, string>> = {
  'db-mysql-tables': 'mysql',
  'db-mysql-table-detail': 'mysql',
  'db-mongo-collections': 'mongo',
  'db-mongo-documents': 'mongo'
}

/** 引擎显示名兜底（档案未命中时按路由段映射，不虚构档案） */
const SEGMENT_ENGINE_LABEL: Readonly<Record<string, string>> = {
  mysql: 'MySQL',
  redis: 'Redis',
  mongo: 'MongoDB'
}

/** 实例标识文本（D-01 组装规则呈现形态，与 views/database-mgmt/render.ts identityText 同口径） */
function identityOf(
  instance: DbCrumbInput['instance'],
  instanceId: string
): string {
  if (instance === null) return instanceId
  if (instance.connMode === 'socket') return instance.socketPath ?? '—'
  return `${instance.host ?? ''}:${instance.port ?? '—'}`
}

/** crumb 链纯组装（7.2 规格；非数据库域路由返回空数组——MainLayout 单标题兜底） */
export function buildDbCrumbs(input: DbCrumbInput): DbCrumb[] {
  const { routeName, instanceId, instance } = input
  if (routeName === 'db-list') {
    return [{ label: '数据库', path: null }]
  }
  const segment = DB_MANAGE_ROUTES[routeName] ?? DB_BROWSE_ROUTES[routeName]
  if (segment === undefined) return []

  const root: DbCrumb = { label: '数据库', path: '/databases' }
  const engineLabel =
    instance !== null ? DB_ENGINE[instance.engine].label : SEGMENT_ENGINE_LABEL[segment]
  const managePath = `/databases/${segment}/${instanceId}`
  const manageCrumb: DbCrumb = {
    label: `${engineLabel} 管理·${identityOf(instance, instanceId)}`,
    path: null
  }

  // 治理页：数据库 / {引擎} 治理·{identity}（末级当前页）
  if (DB_MANAGE_ROUTES[routeName] !== undefined) {
    return [root, manageCrumb]
  }

  // 浏览页：治理级可点回治理页
  manageCrumb.path = managePath
  const database = input.database ?? ''
  const browsePath = `${managePath}/browse/${encodeURIComponent(database)}`
  const dbCrumb: DbCrumb = { label: database, path: null }

  if (input.crumb === 'browse-database') {
    return [root, manageCrumb, dbCrumb]
  }
  // 表/集合级：库级可点回表清单/集合清单页
  dbCrumb.path = browsePath
  const leaf = input.crumb === 'browse-table' ? (input.table ?? '') : (input.collection ?? '')
  return [root, manageCrumb, dbCrumb, { label: leaf, path: null }]
}

/** MainLayout 消费入口：route.name+params+dbStore 响应式组装 */
export function useDbCrumbs(): { crumbs: ComputedRef<DbCrumb[]> } {
  const route = useRoute()
  const dbStore = useDbStore()

  const crumbs = computed<DbCrumb[]>(() => {
    const rawId = route.params.instanceId
    const instanceId = typeof rawId === 'string' ? rawId : ''
    const idNum = /^\d+$/.test(instanceId) ? Number(instanceId) : NaN
    return buildDbCrumbs({
      routeName: typeof route.name === 'string' ? route.name : '',
      instanceId,
      database: typeof route.params.database === 'string' ? route.params.database : undefined,
      table: typeof route.params.table === 'string' ? route.params.table : undefined,
      collection:
        typeof route.params.collection === 'string' ? route.params.collection : undefined,
      crumb: route.meta.crumb,
      instance: Number.isNaN(idNum) ? null : dbStore.findById(idNum)
    })
  })

  // 档案未加载先 ensureLoaded（engineGuard 已确保同链；此处为 db-list 直达等路径兜底，I-05）
  watch(
    () => route.name,
    (name) => {
      const n = typeof name === 'string' ? name : ''
      if ((n === 'db-list' || DB_MANAGE_ROUTES[n] !== undefined || DB_BROWSE_ROUTES[n] !== undefined) && !dbStore.loaded) {
        void dbStore.ensureLoaded()
      }
    },
    { immediate: true }
  )

  return { crumbs }
}
