/**
 * db-tools API 类型 —— 与 api-db-tools.md v1.3 4.1.4/4.2.4 对齐（唯一事实源）。
 * v1.3 增量（自动 nginx 落地轮，PageSpec admin/readonly v1.2）：tool/list +landingPort/
 * autoCreated（additive 可空，D-19/I-14）+访问形态呈现助手+安装成功放行提示+失败引导
 * （D-22 reason 可见/D-23 Open URL 端口口径——url 后端权威下发前端不拼接）。
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
  /** 落地监听端口（v1.3 additive 可空，api D-19/I-14 siteRef.port 投影）：
   * 未装（无档案）=null；已装=自动块 18090/既有 80/443 块=0（旧格式档案缺省 0，不虚构非零端口） */
  landingPort: number | null
  /** 落地块是否面板自动创建（v1.3 additive 可空，siteRef.autoCreated 投影）：
   * 未装=null；已装=true/false（旧格式档案缺省 false）——安装成功放行提示判定用 */
  autoCreated: boolean | null
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
  DBT_001: '缺少运行环境或 PHP 扩展',
  DBT_002: '工具类型参数错误',
  DBT_003: '该工具有任务正在进行',
  DBT_004: '工具未安装',
  DBT_005: '任务不存在，请刷新任务列表',
  DBT_006: '数据库实例不可用（未添加、未配置账号密码或类型不符）',
  // 007/008/010 为内部登录凭证错误码——前端不直接消费（浏览器经工具侧呈现拒绝，
  // 面板侧打开失败引导重新打开）；文案登记仅供锚点测试与兜底。
  DBT_007: '登录凭证校验失败',
  DBT_008: '登录凭证已过期',
  DBT_009: '只有 mongo-express 支持重置密码',
  DBT_010: '登录凭证已使用，请重新打开',
  DBT_011: '已是最新版本，无需升级',
  DBT_012: '安装或重置密码失败'
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
 * AUTH 族/网络错误已由拦截器统一分流，dbmErrorText 同构先例）。
 * v1.2 reason 细分引导（PageSpec 第 1 节，api D-19/D-22）：多候选附「到站点管理明确落点」
 * 引导；端口占用 reason 后端文案已含「请释放后重试」（api D-19③）全文呈现即引导，不重复叠加 */
export function dbtErrorText(code: string, data: unknown): string | null {
  const base = DBT_ERROR_TEXT[code]
  if (base === undefined) return null
  if (code === 'DBT_012') {
    const reason = dbtReasonOf(data)
    if (reason !== null) {
      if (dbtFailureGuide(reason) === 'site-landing') {
        return `${base}：${reason}——请到站点管理调整域名配置后重试`
      }
      return `${base}：${reason}`
    }
  }
  return base
}

/** 失败原因引导判定（v1.2，PageSpec 4.3/第 1 节，api D-22 reason 细分）：
 * 端口占用→释放端口后重试；多候选→到站点管理明确落点（/sites）；其余→null 不虚构引导。
 * 判据=reason 文本关键词（后端 reason 文案 api D-19/D-22 定稿：「工具端口 18090 被占用，
 * 请释放后重试」/「落点冲突：多候选 server 块…」），task.error 全文与 data.reason 同消费 */
export function dbtFailureGuide(errorText: string | null): 'release-port' | 'site-landing' | null {
  if (errorText === null || errorText === '') return null
  if (errorText.includes('被占用')) return 'release-port'
  if (errorText.includes('候选')) return 'site-landing'
  return null
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
  return `PHP 扩展 ${dep.name} 未安装`
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
  if (running === null) return { label: '运行状态未知', tag: 'warning' }
  return running ? { label: '运行中', tag: 'success' } : { label: '已停止', tag: 'default' }
}

/** 依赖就绪度 Tag（true=green 依赖就绪 / false=orange 依赖缺失） */
export function depsReadyTag(depsReady: boolean): DictItem {
  return depsReady ? { label: '环境就绪', tag: 'success' } : { label: '缺少依赖', tag: 'warning' }
}

/** 任务在途判定（pending/running=在途；succeeded/failed=终态，env-mgmt isInFlight 同构） */
export function isDbToolTaskInFlight(status: DbToolTaskStatus): boolean {
  return status === 'pending' || status === 'running'
}

/** 打开成功 toast（PageSpec 4.4：phpmyadmin 令牌 60s 时效呈现；mongo_express 无令牌） */
export function openToastText(result: DbToolOpenResult): string {
  if (result.toolType === 'phpmyadmin' && result.tokenExpiresAt !== null) {
    return '已打开（登录凭证 60 秒内有效）'
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
  '仅管理员可用，需先登录面板；工具只在本机运行，密码加密保存、不会显示'

/** ---- v1.2 自动 nginx 落地呈现助手（PageSpec admin/readonly 4.1/4.3，api D-19/D-22/D-23） ---- */

/** 工具落地端口（api D-13 编译期常量 TOOL_LANDING_PORT=18090 同值；前端仅用于静态引导文案，
 * 卡片/详情实际端口取 tool/list landingPort 投影——不虚构非零端口） */
export const DBTOOL_LANDING_PORT = 18090

/** 页头说明条文案（PageSpec 4.1 v1.2：gating 安全口径 + 自动落地端口 18090 每请求会话校验
 * + 绑域名仅域名访问语义〔api D-19/D-20〕+ 轻量入口并存 Q3） */
export const DBTOOL_PAGE_INTRO =
  `在这里安装 phpMyAdmin、mongo-express 等工具来管理数据库（仅管理员可打开）。工具只在本机运行，登录面板后才能访问，不会直接暴露到公网。用 IP 直连面板时会自动开放工具端口 ${DBTOOL_LANDING_PORT}（需在防火墙/云安全组放行）；绑定域名后则只能用域名访问。数据库页里的表/文档浏览仍然可用。`

/** 工具路径前缀（api D-13 TOOL_PATH_PREFIX_* 同值；访问地址展示引导用——
 * 实际打开 URL 以 dbtoolOpen 响应权威下发〔含端口/域名/signon 令牌片段〕，前端不拼接不修改，D-23） */
export const DBTOOL_PATH_PREFIX: Readonly<Record<DbToolType, string>> = {
  phpmyadmin: '/dbtool/phpmyadmin/',
  mongo_express: '/dbtool/mongo-express/'
}

/** 访问形态 Tag（PageSpec 4.1 v1.2，landingPort/autoCreated 投影）：
 * 已装+landingPort≠0 → info「独立访问端口 {port}」（自动落地块形态）；
 * 已装+landingPort=0 → default「面板域路径访问」（既有面板域块，无独立端口）；
 * 未装/landingPort=null → null（整块不渲染，不虚构端口；JSON null≠undefined 显式判空） */
export function landingTag(item: DbToolItem): DictItem | null {
  if (!item.installed || item.landingPort === null) return null
  if (item.landingPort === 0) return { label: '经面板域名访问', tag: 'default' }
  return { label: `独立访问端口 ${item.landingPort}`, tag: 'info' }
}

/** 访问地址展示引导（hostname=当前面板访问主机 window.location.hostname——IP 直连呈现 IP，
 * 绑域名后经域名访问面板呈现域名，与 api D-23 BindDomain 优先口径一致）；
 * 仅自动落地块形态（landingPort≠0）渲染，port=0/null → null 不渲染地址行 */
export function landingAddress(item: DbToolItem, hostname: string): string | null {
  if (!item.installed || item.landingPort === null || item.landingPort === 0) return null
  return `http://${hostname}:${item.landingPort}${DBTOOL_PATH_PREFIX[item.toolType]}`
}

/** 放行提示行（自动落地块形态；readonly 同等呈现——纯观测信息无放行操作能力，
 * PageSpec readonly 4.1 v1.2）；port=0/null → null */
export function landingHint(item: DbToolItem): string | null {
  if (!item.installed || item.landingPort === null || item.landingPort === 0) return null
  return `请在防火墙/云安全组开放 ${item.landingPort} 端口；绑定域名后只能用域名访问`
}

/** 安装成功放行提示（PageSpec 4.1/4.3 v1.2）：install 任务终态 succeeded 且工具档案
 * autoCreated=true 且 landingPort≠0（自动落地块形态，经终态跃迁重拉 dbtoolList 判定）→
 * NAlert success 文案；既有面板域块形态（landingPort=0）/未装/非 install/非终态 → null 不渲染。
 * hostname 口径同卡片访问形态区块；绑域名后地址为 http://域名:{port}/...（api D-23） */
export function installReadyAlertText(
  task: Pick<DbToolTaskItem, 'action' | 'status' | 'toolType'>,
  tool: DbToolItem | null,
  hostname: string
): string | null {
  if (task.action !== 'install' || task.status !== 'succeeded') return null
  if (tool === null || tool.autoCreated !== true) return null
  if (tool.landingPort === null || tool.landingPort === 0) return null
  const port = tool.landingPort
  return `工具已安装好：请在防火墙/云安全组开放端口 ${port}，然后访问 http://${hostname}:${port}${DBTOOL_PATH_PREFIX[task.toolType]}（绑定域名后用 http://域名:${port}/...）`
}
