/**
 * db-tools API 类型 —— 与 api-db-tools.md v1.2 4.1.4/4.2.4 对齐（唯一事实源）。
 * - 工具条目为实时探测投影（D-02/I-11）：部署状态=档案存在性+已装版本（面板自持），
 *   运行态三态 true/false/null（探测失败不虚构）；依赖就绪度消费 env-mgmt 实况；
 * - tool_deploy_task 四态任务台账（D-04）：running/pending 态详情 5s 轮询终态即停，
 *   终态跃迁触发工具全景与任务清单重拉（env-mgmt 先例）；
 * - DTO 可空纪律（api 3.1）：JSON null ≠ TS undefined——可空字段一律显式 `|null`；
 * - 凭据零回显（I-02）：打开响应仅 url+tokenExpiresAt（令牌非凭据，url 片段下发 admin）；
 *   轮换响应仅 rotatedAt 无口令字段；
 * - dbtoolReopen（GET /dbtool/access/reopen）为浏览器 302 顶层导航落地（phpMyAdmin
 *   SignonURL 重认证），非前端 fetch Action——本文件无对应函数/类型（PageSpec 4.4 v1.1）。
 */
import type { DbEngine, DbToolTaskAction, DbToolTaskStatus, DbToolType, DictItem } from './enums'
import { ENV_TYPE, dictItem } from './enums'

/** 依赖缺失项（DBT_001 data.missing 同形状；kind=env 的 name 为 env_type 值透传，
 * kind=extension 的 name 为 PHP 扩展名开放集合原样呈现，PageSpec 第 3 节偏差声明） */
export interface DbToolDepMissing {
  kind: 'env' | 'extension'
  name: string
}

/** 工具全景条目（目录常量+实况投影，GET /dbtool/tool/list items[]） */
export interface DbToolItem {
  toolType: DbToolType
  displayName: string
  description: string
  targetEngines: DbEngine[]
  catalogVersion: string
  /** 部署状态=db_tool 档案存在性（D-02） */
  installed: boolean
  /** 未装=null（显式 null，api 3.1 DTO 可空纪律） */
  installedVersion: string | null
  /** mongo_express=部署绑定实例 id；phpmyadmin=null（打开时绑定，api v1.1） */
  boundInstanceId: number | null
  /** 运行态三态：true 运行中/false 已停止/null 探测失败不虚构（D-02） */
  running: boolean | null
  depsReady: boolean
  /** 空数组=就绪 */
  depsMissing: DbToolDepMissing[]
  /** 在途任务标记（pending/running 归组，I-06）；无=null */
  runningTaskId: number | null
}

/** GET /dbtool/tool/list 响应 data */
export interface DbToolListData {
  items: DbToolItem[]
  generatedAt: string
}

/** POST /dbtool/tool/{install,upgrade,uninstall} 响应（taskId 即刻返回，D-04 异步任务） */
export interface DbToolTaskCreated {
  taskId: number
  action: DbToolTaskAction
}

/** 任务清单项（GET /dbtool/task/list items[]；可空字段显式 null） */
export interface DbToolTaskItem {
  id: number
  toolType: DbToolType
  action: DbToolTaskAction
  status: DbToolTaskStatus
  /** mongo-express 安装溯源；其余=null */
  instanceId: number | null
  requestedBy: string
  createdAt: string
  startedAt: string | null
  finishedAt: string | null
  error: string | null
}

/** GET /dbtool/task/list 响应 data */
export interface DbToolTaskListData {
  items: DbToolTaskItem[]
  total: number
}

/** GET /dbtool/task/detail 响应 data（+输出尾部 ≤200 行 D-13） */
export interface DbToolTaskDetail extends DbToolTaskItem {
  outputTail: string
}

/** POST /dbtool/access/open 响应 data（凭据零回显 I-02：仅 url+tokenExpiresAt；
 * phpmyadmin tokenExpiresAt=iat+60s ISO-8601；mongo_express=null） */
export interface DbToolOpenResult {
  toolType: DbToolType
  url: string
  tokenExpiresAt: string | null
}

/** POST /dbtool/access/rotate 响应 data（仅 mongo_express；**无口令字段** I-02） */
export interface DbToolRotateResult {
  toolType: DbToolType
  rotatedAt: string
}

/** ---- 错误码文案（api 3.4 DBT_001~012；页面级分流，拦截器不含 DBT 码集） ---- */

export const DBT_ERROR_TEXT: Readonly<Record<string, string>> = {
  DBT_001: '依赖缺失（环境或 PHP 扩展未就绪）',
  DBT_002: '工具类型参数错误',
  DBT_003: '该工具有任务进行中',
  DBT_004: '工具未安装',
  DBT_005: '任务不存在，请刷新任务清单',
  DBT_006: '实例未就绪（未纳管/凭据未配置/引擎不匹配）',
  // 007/008/010 为 signon 内部端点错误码——前端不直接消费（浏览器经工具侧呈现拒绝，
  // 面板侧打开失败引导重新打开，PageSpec 第 1 节）；文案登记仅供锚点测试与兜底。
  DBT_007: 'signon 令牌校验失败',
  DBT_008: 'signon 令牌已过期',
  DBT_009: '轮换凭据仅 mongo-express 支持',
  DBT_010: 'signon 令牌已被使用',
  DBT_011: '当前已为目录版本，无需升级',
  DBT_012: '工具部署/轮换操作失败'
}

/** ApiError.data（unknown）→ data.reason 收窄（DBT_012 失败原因如实展示，api 3.4；
 * database-mgmt reasonOf 同构先例） */
export function dbtReasonOf(data: unknown): string | null {
  if (typeof data === 'object' && data !== null) {
    const o = data as Record<string, unknown>
    if (typeof o.reason === 'string' && o.reason !== '') return o.reason
  }
  return null
}

/** ApiError.data（unknown）→ data.missing 收窄（DBT_001 依赖缺失内联清单数据源） */
export function dbtMissingOf(data: unknown): DbToolDepMissing[] {
  if (typeof data !== 'object' || data === null) return []
  const raw = (data as Record<string, unknown>).missing
  if (!Array.isArray(raw)) return []
  const out: DbToolDepMissing[] = []
  for (const it of raw) {
    if (typeof it !== 'object' || it === null) continue
    const o = it as Record<string, unknown>
    if ((o.kind === 'env' || o.kind === 'extension') && typeof o.name === 'string') {
      out.push({ kind: o.kind, name: o.name })
    }
  }
  return out
}

/** ApiError.data（unknown）→ data.taskId 收窄（DBT_003 在途任务查看引导） */
export function dbtTaskIdOf(data: unknown): number | null {
  if (typeof data === 'object' && data !== null) {
    const v = (data as Record<string, unknown>).taskId
    if (typeof v === 'number') return v
  }
  return null
}

/** DBT 错误 → toast 文案（DBT_012 追加服务端 reason；未知码返回 null 不提示——
 * AUTH 族/网络错误已由拦截器统一分流，dbmErrorText 同构先例） */
export function dbtErrorText(code: string, data: unknown): string | null {
  const base = DBT_ERROR_TEXT[code]
  if (base === undefined) return null
  if (code === 'DBT_012') {
    const reason = dbtReasonOf(data)
    if (reason !== null) return `${base}：${reason}`
  }
  return base
}

/** ---- 纯函数呈现助手（PageSpec 第 3 节颜色方案 + 4.1/4.4 文案；vitest 共用） ---- */

/** 引擎 → 工具映射（database-mgmt PageSpec 6.8：mysql/mariadb→phpmyadmin、
 * mongodb→mongo_express、redis 无入口——Redis 集成工具 OUT，D0 范围边界） */
export function toolForEngine(engine: DbEngine): DbToolType | null {
  if (engine === 'mysql' || engine === 'mariadb') return 'phpmyadmin'
  if (engine === 'mongodb') return 'mongo_express'
  return null
}

/** 依赖缺失项文案（kind=env → env_type displayName 映射透传〔env-mgmt 字典，
 * 未知值原样呈现不硬编码七/八值，D-18〕；kind=extension → PHP 扩展名原样） */
export function depMissingText(dep: DbToolDepMissing): string {
  if (dep.kind === 'env') return `${dictItem(ENV_TYPE, dep.name).label} 环境未安装`
  return `PHP 扩展 ${dep.name} 缺失`
}

/** 依赖缺失清单汇总（安装按钮禁用 tooltip / 未装引导弹窗共用） */
export function depsMissingSummary(deps: DbToolDepMissing[]): string {
  return deps.map(depMissingText).join('、')
}

/** 部署状态 Tag（installed=true → green「已安装 · 版本」；false → gray「未安装」） */
export function installedStateTag(installed: boolean, version: string | null): DictItem {
  if (!installed) return { label: '未安装', tag: 'default' }
  return { label: `已安装 · ${version ?? ''}`, tag: 'success' }
}

/** 运行态三态 Tag（true=green 运行中 / false=gray 已停止 / null=orange 运行态未知——
 * 探测失败不虚构，D-02/I-11） */
export function runningStateTag(running: boolean | null): DictItem {
  if (running === null) return { label: '运行态未知', tag: 'warning' }
  return running ? { label: '运行中', tag: 'success' } : { label: '已停止', tag: 'default' }
}

/** 依赖就绪度 Tag（true=green 依赖就绪 / false=orange 依赖缺失） */
export function depsReadyTag(depsReady: boolean): DictItem {
  return depsReady ? { label: '依赖就绪', tag: 'success' } : { label: '依赖缺失', tag: 'warning' }
}

/** 任务在途判定（pending/running=在途；succeeded/failed=终态，env-mgmt isInFlight 同构） */
export function isDbToolTaskInFlight(status: DbToolTaskStatus): boolean {
  return status === 'pending' || status === 'running'
}

/** 打开成功 toast（PageSpec 4.4：phpmyadmin 令牌 60s 时效呈现；mongo_express 无令牌） */
export function openToastText(result: DbToolOpenResult): string {
  if (result.toolType === 'phpmyadmin' && result.tokenExpiresAt !== null) {
    return '已打开（令牌 60 秒内有效）'
  }
  return '已打开'
}

/** 实例标识文本（安装/打开实例选择器选项呈现——database-mgmt render.ts identityText
 * 同口径；跨模块禁止 import 其他模块 views 内部文件〔frontend-rules §2〕，故本域
 * 自持纯函数，形状仅取连接字段不依赖对方模块类型） */
export function instanceIdentityText(item: {
  connMode: string
  socketPath?: string
  host?: string
  port?: number
}): string {
  if (item.connMode === 'socket') return item.socketPath ?? '—'
  return `${item.host ?? ''}:${item.port ?? '—'}`
}

/** 工具安全访问 tooltip 文案（database-mgmt PageSpec 6.8 安全口径呈现，⚠️A6~A10 用户面） */
export const DBTOOL_ACCESS_TOOLTIP =
  '经面板会话安全访问（仅管理员）；工具仅监听本机，凭据加密存储不回显'

/** 页头说明条文案（PageSpec 4.1：gating 安全口径 + 轻量入口并存 Q3） */
export const DBTOOL_PAGE_INTRO =
  '集成工具经面板会话安全访问（仅管理员）；工具仅监听本机，不暴露公网端口。面板内表/文档浏览（轻量入口）保留并存。'
