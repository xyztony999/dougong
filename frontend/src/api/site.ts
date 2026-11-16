/**
 * site-mgmt api 层 —— 18 Action 与 api-site-mgmt.md 4.1.4/4.2.4 一一对应（S7，无虚构接口）。
 * 读 4 GET 两角色（siteList/siteDetail/logTail/certList，D0 口径 + ADR-018 全可看）；
 * admin 专属 GET 2（siteSource 编辑前置读取、phpPoolsList 建站候选）；
 * 写 12 POST（admin + CSRF——http.ts 统一注入 X-CSRF-Token）。
 * 函数名 = PageSpec 第 2 节 Action 名（service-mgmt/monitoring 先例）。
 */
import type {
  AcmeChallenge,
  CertSource,
  CertStatus,
  SiteEnabled,
  SiteLogType
} from '@/types/enums'
import type {
  CertDeleteResult,
  CertDeployResult,
  CertItem,
  CertListData,
  CertRequestResult,
  LogTailData,
  NginxReloadResult,
  PhpPoolsData,
  ProxyUpdateRequest,
  ProxyUpdateResult,
  SiteCreateRequest,
  SiteCreateResult,
  SiteDetail,
  SiteListData,
  SiteSourceData,
  SiteSourceSaveResult,
  SiteToggleResult,
  SiteUpdateRequest,
  SiteUpdateResult
} from '@/types/site'
import { request } from './http'

/** siteList：站点列表投影（唯一 query 参数 enabled；实时扫描 D-01 无缓存） */
export function siteList(enabled?: SiteEnabled): Promise<SiteListData> {
  return request<SiteListData>('/sites/site/list', { query: { enabled } })
}

/** siteDetail：站点详情（locations/Raw 块/来源文件） */
export function siteDetail(siteId: string): Promise<SiteDetail> {
  return request<SiteDetail>('/sites/site/detail', { query: { siteId } })
}

/** siteCreate：三模板建站（D-15 防重 SITE_005；落点优先级 D-03） */
export function siteCreate(req: SiteCreateRequest): Promise<SiteCreateResult> {
  return request<SiteCreateResult>('/sites/site/create', { method: 'POST', body: req })
}

/** siteUpdate：基本信息结构化编辑（serverNames/listen/root；I-09 Raw 保全） */
export function siteUpdate(req: SiteUpdateRequest): Promise<SiteUpdateResult> {
  return request<SiteUpdateResult>('/sites/site/update', { method: 'POST', body: req })
}

/** proxyUpdate：反代参数写回（M3 验收主场景） */
export function proxyUpdate(req: ProxyUpdateRequest): Promise<ProxyUpdateResult> {
  return request<ProxyUpdateResult>('/sites/proxy/update', { method: 'POST', body: req })
}

/** siteSource：读取配置原文（admin 编辑前置读取；baseMtime 基线 D-06） */
export function siteSource(siteId: string): Promise<SiteSourceData> {
  return request<SiteSourceData>('/sites/site/source', { query: { siteId } })
}

/** siteSourceSave：源码保存（baseMtime 乐观锁 SITE_010 + 保存前 nginx -t 前置） */
export function siteSourceSave(
  siteId: string,
  content: string,
  baseMtime: string
): Promise<SiteSourceSaveResult> {
  return request<SiteSourceSaveResult>('/sites/site/source-save', {
    method: 'POST',
    body: { siteId, content, baseMtime }
  })
}

/** siteEnable / siteDisable：启停投影（D-16 注释/还原；对已处目标态幂等成功） */
export function siteEnable(siteId: string): Promise<SiteToggleResult> {
  return request<SiteToggleResult>('/sites/site/enable', { method: 'POST', body: { siteId } })
}

export function siteDisable(siteId: string): Promise<SiteToggleResult> {
  return request<SiteToggleResult>('/sites/site/disable', { method: 'POST', body: { siteId } })
}

/** logTail：访问/错误日志尾部（lines 默认 100 上限 1000；⚠️A15 回退链穷尽 → SITE_011） */
export function logTail(
  siteId: string,
  type: SiteLogType,
  lines = 100
): Promise<LogTailData> {
  return request<LogTailData>('/sites/log/tail', { query: { siteId, type, lines } })
}

/** nginxReload：用户主动平滑 reload（失败 SITE_004 配置保留，D-05） */
export function nginxReload(): Promise<NginxReloadResult> {
  return request<NginxReloadResult>('/sites/nginx/reload', { method: 'POST', body: {} })
}

/** phpPoolsList：php-fpm pool 候选清单（实时读取 D-14；admin 专属） */
export function phpPoolsList(): Promise<PhpPoolsData> {
  return request<PhpPoolsData>('/sites/php-pools/list')
}

/** certList：证书清单（source/status 筛选；状态读时推算 D-08） */
export function certList(
  q: { source?: CertSource; status?: CertStatus } = {}
): Promise<CertListData> {
  return request<CertListData>('/sites/cert/list', { query: { source: q.source, status: q.status } })
}

/** certRequest：ACME 申请（同步终态 ≤120s，D-09；challenge 覆盖两挑战菜单） */
export function certRequest(
  siteId: string,
  challenge: AcmeChallenge
): Promise<CertRequestResult> {
  return request<CertRequestResult>('/sites/cert/request', {
    method: 'POST',
    body: { siteId, challenge }
  })
}

/** certDeploy / certUndeploy：部署（写 ssl_cert 引用）/ 解除（恢复部署前原值 D-11） */
export function certDeploy(certId: number, siteId: string): Promise<CertDeployResult> {
  return request<CertDeployResult>('/sites/cert/deploy', {
    method: 'POST',
    body: { certId, siteId }
  })
}

export function certUndeploy(certId: number, siteId: string): Promise<CertDeployResult> {
  return request<CertDeployResult>('/sites/cert/undeploy', {
    method: 'POST',
    body: { certId, siteId }
  })
}

/** certRetryRenewal：renew_failed 手动重试（同步终态，进入自动续期同一通道） */
export function certRetryRenewal(certId: number): Promise<CertRequestResult> {
  return request<CertRequestResult>('/sites/cert/retry-renewal', {
    method: 'POST',
    body: { certId }
  })
}

/** certDelete：删除（引用守卫 CERT_005；acme 连带托管文件、imported 仅删行，I-04） */
export function certDelete(certId: number): Promise<CertDeleteResult> {
  return request<CertDeleteResult>('/sites/cert/delete', { method: 'POST', body: { certId } })
}

/** 便捷别名类型（CertItem 供页面行类型引用） */
export type { CertItem }
