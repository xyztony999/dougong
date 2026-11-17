/**
 * site-mgmt API 类型 —— 与 api-site-mgmt.md v1.0 4.1.4/4.2.4 对齐（唯一事实源）。
 * - site 为配置文件实时投影（D-01/I-06）：列表/详情每请求即时扫描，响应带 generatedAt，
 *   无缓存层；nginx 未检出 list 返回空 items + nginxAvailable:false（SITE_012 语义空态非错误）；
 * - siteId=`{来源文件绝对路径}#{server 块序号}`（D-02）；
 * - certId/certStatus 为 D-11 实时推导值：certRef 非空但 certId 缺失=「未纳管引用」；
 * - listen 为监听声明列表（如 `80`、`443 ssl`）；create 请求 listen 为逗号分隔 string、
 *   update 请求/全部响应为 string[]（api 契约原样，不归一）。
 */
import type { AcmeChallenge, CertSource, CertStatus, DnsProvider, SiteEnabled, SiteLogType, SiteType } from './enums'

/** 站点主反代段（值对象；非反代站点 proxy=null；多反代 location 仅呈现首个，D-03）。
 *  hostHeader 为 string 且恒序列化（backend ProxyDTO.HostHeader string 无 omitempty，
 *  dto.go:41）——""=无 Host 覆写（透传原 Host）。Gate 2 W-1 对齐。 */
export interface SiteProxy {
  upstream: string
  hostHeader: string
  websocket: boolean
}

/**
 * GET /sites/site/list items 条目（v2.0 D-17 聚合条目）：同来源文件且 server_name
 * 全集相同的多个 server 块合并——listen 合并去重、enabled=任一块生效、证书状态=
 * 组内最优（443/ssl 块优先）、root=首个非空、proxy=组内首个反代段；寻址经
 * blocks[].siteId（全部写接口仍单块寻址，仅展示层聚合）。
 */
export interface SiteListItem {
  serverNames: string[]
  listen: string[]
  root: string
  enabled: SiteEnabled
  /** 组内最优证书引用值（无证书引用为 null） */
  certRef: string | null
  certId?: number
  certStatus?: CertStatus
  proxy: SiteProxy | null
  sourceFile: string
  /** 块级子清单（单块条目为 1 元素数组，结构统一；siteId 寻址键在块级） */
  blocks: SiteBlock[]
}

/** 聚合条目块级子项（D-17：siteId/listen/enabled/证书引用级明细） */
export interface SiteBlock {
  siteId: string
  listen: string[]
  enabled: SiteEnabled
  certRef: string | null
  certId?: number
  certStatus?: CertStatus
}

/** 同域名多文件重复定义（D-03 如实呈现，不静默丢弃） */
export interface SiteDuplicate {
  serverNames: string[]
  listen: string[]
  files: string[]
}

/** GET /sites/site/list 响应 data */
export interface SiteListData {
  nginxAvailable: boolean
  items: SiteListItem[]
  duplicates: SiteDuplicate[]
  generatedAt: string
}

/** location 解析投影（4.2.1；只读清单，结构化单 location 编辑不做） */
export interface SiteLocation {
  path: string
  proxyPass?: string
  fastcgiPass?: string
}

/** Raw 未识别指令块原文（R-07 保全呈现） */
export interface SiteRawBlock {
  line: number
  text: string
}

/** GET /sites/site/detail 响应 data（list 项全字段 + locations/rawBlocks） */
export interface SiteDetail extends SiteListItem {
  locations: SiteLocation[]
  rawBlocks: SiteRawBlock[]
}

/** POST /sites/site/create req（字段按 type 条件必填；listen 逗号分隔 string） */
export interface SiteCreateRequest {
  type: SiteType
  serverNames: string[]
  listen: string
  root?: string
  poolName?: string
  upstream?: string
  hostHeader?: string
  websocket?: boolean
}

export interface SiteCreateResult {
  siteId: string
  type: SiteType
  serverNames: string[]
  /** 原样回传请求的逗号分隔 string（backend CreateSiteData.Listen string，dto.go:84——Gate 2 W-1 对齐；区别于 update 响应与投影字段的 string[]） */
  listen: string
  sourceFile: string
}

/** POST /sites/site/update req（listen 数组，与 create 的 string 形态区分——api 契约原样） */
export interface SiteUpdateRequest {
  siteId: string
  serverNames: string[]
  listen: string[]
  root: string
}

export interface SiteUpdateResult {
  siteId: string
  serverNames: string[]
  listen: string[]
  root: string
}

/** POST /sites/proxy/update req（M3 验收主场景） */
export interface ProxyUpdateRequest {
  siteId: string
  upstream: string
  hostHeader?: string
  websocket?: boolean
}

export interface ProxyUpdateResult {
  siteId: string
  proxy: SiteProxy
}

/** POST enable/disable 响应 data：{siteId, enabled 新投影值} */
export interface SiteToggleResult {
  siteId: string
  enabled: SiteEnabled
}

/** GET /sites/site/source 响应 data（baseMtime 乐观锁基线，D-06） */
export interface SiteSourceData {
  siteId: string
  sourceFile: string
  content: string
  baseMtime: string
  size: number
}

export interface SiteSourceSaveResult {
  siteId: string
  baseMtime: string
}

/** 日志行：ts 有值显示时间，无值原样文本（D-07） */
export interface LogTailLine {
  ts?: string
  text: string
}

/** GET /sites/log/tail 响应 data（lines 默认 100 上限 1000，LOG_TAIL_MAX） */
export interface LogTailData {
  siteId: string
  type: SiteLogType
  logPath: string
  lines: LogTailLine[]
  truncated: boolean
}

/** POST /sites/nginx/reload 成功 data */
export interface NginxReloadResult {
  reloaded: boolean
}

/** GET /sites/php-pools/list 响应 data（实时读取无缓存，D-14；无 pool 空数组） */
export interface PhpPool {
  name: string
  socket: string
}

export interface PhpPoolsData {
  pools: PhpPool[]
}

/** ---- certificate（聚合根） ---- */

/** 证书被引用站点（D-11 实时推导） */
export interface CertReferencedSite {
  siteId: string
  serverNames: string[]
}

/** GET /sites/cert/list items 条目（storage_path/acme_account_ref 不下发） */
export interface CertItem {
  id: number
  primaryDomain: string
  domains: string[]
  issuer: string
  source: CertSource
  status: CertStatus
  /** imported 来源缺失（api challenge?） */
  challenge?: AcmeChallenge | null
  notBefore?: string | null
  notAfter?: string | null
  /** imported 来源专属（源站原路径，⚠️A16 不迁移） */
  originPath?: string | null
  /** renew_failed 失败原因（FR-24 排障呈现） */
  lastError?: string | null
  referencedSites: CertReferencedSite[]
  createdAt: string
}

export interface CertListData {
  items: CertItem[]
  generatedAt: string
}

/** POST cert/request 与 cert/retry-renewal 成功 data（同步终态，D-09） */
export interface CertRequestResult {
  certId: number
  status: CertStatus
  notAfter: string
}

/** POST cert/deploy / cert/undeploy 成功 data（undeploy 的 certRef=恢复后的原引用值，D-11） */
export interface CertDeployResult {
  certId: number
  siteId: string
  certRef: string
}

export interface CertDeleteResult {
  certId: number
}

/** ---- 错误 data 载荷（api 3.4；ApiError.data 为 unknown，入交互前收窄） ---- */

/** SITE_003 data：写回已自动回滚 + errors 错误行 + diff 差异（R-06/D-04） */
export interface SiteRollbackData {
  errors: string[]
  diff: string
}

/** SITE_010 data：基线过期（文件在打开后被外部修改，D-06） */
export interface SiteStaleData {
  currentMtime: string
}

export function isSiteRollbackData(d: unknown): d is SiteRollbackData {
  if (typeof d !== 'object' || d === null) return false
  const o = d as Record<string, unknown>
  return (
    Array.isArray(o.errors) &&
    o.errors.every((x) => typeof x === 'string') &&
    (o.diff === undefined || typeof o.diff === 'string')
  )
}

export function isSiteStaleData(d: unknown): d is SiteStaleData {
  if (typeof d !== 'object' || d === null) return false
  const o = d as Record<string, unknown>
  return typeof o.currentMtime === 'string'
}

/** CERT_005 data：引用站点清单（先解除部署，I-02） */
export function isCertSitesData(d: unknown): d is { sites: CertReferencedSite[] } {
  if (typeof d !== 'object' || d === null) return false
  const o = d as Record<string, unknown>
  return (
    Array.isArray(o.sites) &&
    o.sites.every(
      (x) =>
        typeof x === 'object' &&
        x !== null &&
        typeof (x as Record<string, unknown>).siteId === 'string' &&
        Array.isArray((x as Record<string, unknown>).serverNames)
    )
  )
}

/** data.reason 收窄（SITE_004 output / SITE_012/013、CERT_004 reason 同形键挑选） */
export function fieldOfString(d: unknown, key: string): string | null {
  if (typeof d !== 'object' || d === null) return null
  const v = (d as Record<string, unknown>)[key]
  return typeof v === 'string' ? v : null
}

/**
 * SITE_001~014 错误文案（PageSpec 第 1 节错误处理，逐条对应 api 3.4）：
 * 003/004/010/012 为专项交互（回滚 diff 弹窗/output 弹窗/基线冲突弹窗/空态引导），
 * 由调用方先截获后兜底到本表。
 */
export const SITE_ERROR_TEXT: Readonly<Record<string, string>> = {
  SITE_001: '参数非法',
  SITE_002: '站点不存在（可能已被面板外改动），请刷新列表',
  SITE_003: '保存失败：nginx -t 校验未通过，配置已自动回滚',
  SITE_004: '配置已保存，但 nginx 重载失败',
  SITE_005: '同域名+端口的站点已存在',
  SITE_006: '监听端口非法（须为 1~65535）',
  SITE_007: '域名形态非法（不允许空值、空格或通配符）',
  SITE_008: '上游地址非法（须为 http(s)://host[:port] 形态）',
  SITE_009: '指定的 php-fpm pool 不存在',
  SITE_010: '配置已被外部修改，请重新加载后编辑',
  SITE_011: '日志文件不存在或不可读',
  SITE_012: '未检出 nginx 或 nginx -t 通道不可用',
  SITE_013: '写前快照失败，本次写入未执行（配置未动）',
  SITE_014: '内容非法：超过 1MiB 上限或非 UTF-8'
}

/** CERT_001~007 错误文案（PageSpec 第 1 节；003/005 专项交互由调用方截获） */
export const CERT_ERROR_TEXT: Readonly<Record<string, string>> = {
  CERT_001: '证书不存在',
  CERT_002: '证书状态不允许该操作',
  CERT_003: '该域名已有生效中的 ACME 证书或进行中的申请',
  CERT_004: 'ACME 签发/续期失败',
  CERT_005: '证书仍被站点引用，请先解除部署',
  CERT_006: '该站点当前未部署此证书',
  CERT_007: '参数非法',
  CERT_008: '证书 PEM 非法（解析失败或超 1MiB）',
  CERT_009: 'DNS 凭证不存在',
  CERT_010: '域名输入非法（siteId 与 domains 二选一；泛域名须为 *.domain 形态）'
}


/** ---- v1.2/v2.0 增量（D-18/D-20） ---- */

/** POST /sites/cert/renew 结果（与 CertRequestResult 同形） */
export type CertRenewResult = CertRequestResult

/** POST /sites/cert/replace req（imported 文件替换，D-18） */
export interface CertReplaceRequest {
  certId: number
  certPem: string
}

/** POST /sites/cert/replace 结果（读时解析元数据；域名差异如实呈现不阻断） */
export interface CertReplaceResult {
  certId: number
  notBefore?: string
  notAfter?: string
  issuer: string
  domains: string[]
}

/** DNS 凭证清单项（凭据材料零回显 I-D01；referencedCertCount 为 I-C02 实时推导） */
export interface DnsCredentialItem {
  id: number
  name: string
  provider: DnsProvider
  createdAt: string
  referencedCertCount: number
}

/** GET /sites/dns-credential/list 响应 data */
export interface DnsCredentialListData {
  items: DnsCredentialItem[]
}

/** DNS 凭据材料（按 provider 形态；明文仅本请求到达后端即加密，任何响应不回显） */
export interface DnsCredentialPayload {
  apiToken?: string
  accessKeyId?: string
  accessKeySecret?: string
  secretId?: string
  secretKey?: string
}

/** POST /sites/dns-credential/create req */
export interface DnsCredentialCreateRequest {
  name: string
  provider: DnsProvider
  credential: DnsCredentialPayload
}

/** POST /sites/dns-credential/create 结果（零凭据材料） */
export interface DnsCredentialCreateResult {
  id: number
  name: string
  provider: DnsProvider
}

/** 凭证删除时的引用证书呈现项（I-C01 警示不硬阻断） */
export interface DnsCertRefItem {
  certId: number
  primaryDomain: string
}

/** POST /sites/dns-credential/delete 结果 */
export interface DnsCredentialDeleteResult {
  id: number
  referencedCerts: DnsCertRefItem[]
}
