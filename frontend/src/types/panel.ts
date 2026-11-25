/**
 * panel-core 业务类型 —— 与 api-panel-core.md v2.0 请求/响应字段逐字对齐（F02）。
 * detail 七分组结构 = api 4.1.4 GET /panel/config/detail data（v2.0 +bindDomain 组）；
 * update 为部分字段集语义（api 4.1.4：仅提交本 Card 字段）。
 */
import type {
  ConfigEffectMode,
  HttpsMode,
  ListenScope,
  PanelCertSource,
  PanelCertStatus
} from './enums'

/** 错误码标识（api 3.4；PANEL_001~PANEL_011，AUTH_* 复用 account-auth 码族；v2.0 +PANEL_007~011） */
export type PanelErrorCode =
  | 'PANEL_001'
  | 'PANEL_002'
  | 'PANEL_003'
  | 'PANEL_004'
  | 'PANEL_005'
  | 'PANEL_006'
  | 'PANEL_007'
  | 'PANEL_008'
  | 'PANEL_009'
  | 'PANEL_010'
  | 'PANEL_011'

/**
 * 拦截器统一提示文案（PageSpec 第 1 节错误处理行文案逐字）。
 * PANEL_009/010/011 不在此列——需 data.reason/notAfter/currentStatus 联动文案，
 * 由面板证书 Card 页面级处理（PageSpec 5.1 Card 5），避免拦截器无数据双提示。
 */
export const PANEL_ERROR_TEXT: Readonly<Record<string, string>> = {
  PANEL_001: '监听范围为自定义时必须填写合法的监听地址',
  PANEL_002: '证书解析失败或与私钥不配对，请检查 PEM 内容',
  PANEL_003: '请先导入证书与私钥',
  PANEL_004: '更新检查未开启',
  PANEL_005: '版本源不可达，可稍后手动重试',
  PANEL_006: '所选语言暂不支持',
  PANEL_007: '请先绑定域名再申请证书',
  PANEL_008: 'nginx 不可用或 80 端口未监听（HTTP-01 挑战通道不可用）'
}

/** ---- GET /panel/config/detail（api 4.1.4） ---- */

/** 证书摘要（I-P06：仅摘要，key_pem 永不下发） */
export interface PanelCertSummary {
  source: PanelCertSource
  issuer: string | null
  notBefore: string | null
  notAfter: string | null
}

/** 颁发者展示：空串/空白按未填写处理（导入证书 CN 为空时后端可能回空串） */
export function certIssuerText(issuer: string | null | undefined): string {
  const t = (issuer ?? '').trim()
  return t !== '' ? t : '—'
}

export interface PanelListenConfig {
  scope: ListenScope
  address: string | null
  port: number
  effectMode: ConfigEffectMode
}

export interface PanelEntryPathConfig {
  enabled: boolean
  path: string | null
  effectMode: ConfigEffectMode
}

/** 域名绑定（v2.0 新增，ADR-028/D-P12）：value=bind_domain 当前值（null=未绑定，readonly 可见——ADR-018 全可看）；即时生效（HostGuard 下一请求即用，D-P16） */
export interface PanelBindDomainConfig {
  value: string | null
  effectMode: ConfigEffectMode
}

export interface PanelHttpsConfig {
  mode: HttpsMode
  cert: PanelCertSummary | null
  effectMode: ConfigEffectMode
}

export interface PanelUpdateCheckConfig {
  enabled: boolean
  effectMode: ConfigEffectMode
}

export interface PanelLangConfig {
  value: string
  supported: string[]
  effectMode: ConfigEffectMode
}

/** 会话参数只读透传（D-P08：载体=配置文件，无修改接口）；两值为秒数整数 int64——
 * 契约 §4.1.4 已钉死（2026-11-16 WARN 修复轮 W1，与后端 SessionView 对齐），
 * 展示层格式化为人类可读时长（SessionCard → formatUptime） */
export interface PanelSessionTransit {
  absoluteTimeout: number
  idleTimeout: number
}

export interface PanelConfigDetail {
  listen: PanelListenConfig
  entryPath: PanelEntryPathConfig
  /** v2.0 新增分组（api 4.1.4 detail 结构逐字对齐） */
  bindDomain: PanelBindDomainConfig
  https: PanelHttpsConfig
  updateCheck: PanelUpdateCheckConfig
  lang: PanelLangConfig
  session: PanelSessionTransit
  /** 落库监听值 ≠ 进程启动绑定值（D-P02 派生，readonly 可见；重启后自动消失） */
  pendingRestart: boolean
}

/** ---- POST /panel/config/update（部分字段集；响应 data = 更新后配置视图，结构同 detail） ---- */

export interface PanelConfigUpdateReq {
  listenScope?: ListenScope
  /** scope≠custom 时显式传 null 清残留自定义地址（I-P02） */
  listenAddress?: string | null
  listenPort?: number
  entryPathEnabled?: boolean
  /** 域名绑定/解绑唯一写入口（D-P12）：携带 null/空串=解绑、非空合法域名=绑定、未携带=不变 */
  bindDomain?: string | null
  httpsMode?: HttpsMode
  updateCheck?: boolean
  lang?: string
}

/** ---- POST /panel/config/regenerate-entry-path ---- */

export interface EntryPathRegeneratedData {
  entryPath: string
}

/** ---- POST /panel/config/upload-certificate（D-P05：PEM 文本 JSON 字段） ---- */

export interface CertUploadReq {
  certPem: string
  keyPem: string
}

export interface CertUploadData {
  cert: PanelCertSummary
}

/** ---- GET /panel/cert/status（v2.0 新增，两角色——面板证书状态视图，D-P13） ---- */

/** 证书行（cert=null 即未配置证书）；status 读时惰性推算——source≠acme 时为 null（自签/导入无续期语义）；lastError=renew_failed 落库态（首次申请失败无行不落库，D-P13） */
export interface PanelCertStatusItem {
  source: PanelCertSource
  status: PanelCertStatus | null
  issuer: string | null
  notBefore: string | null
  notAfter: string | null
  lastError: string | null
}

/** ACME 概要：domain=bind_domain 当前值（未绑定为 null）；挑战固定 http_01；autoRenew 固定 true（每日后台调度内置，4.1.3） */
export interface PanelCertAcmeSummary {
  domain: string | null
  challenge: 'http_01'
  autoRenew: boolean
}

export interface PanelCertStatusData {
  cert: PanelCertStatusItem | null
  acme: PanelCertAcmeSummary
}

/** ---- POST /panel/cert/request · /panel/cert/retry-renewal（v2.0 新增，admin；响应 data 同形态，api 4.1.4） ---- */

/** 成功后 https_mode 联动切换 acme（D-P14 唯一自动切换路径） */
export interface PanelCertActionResultData {
  cert: {
    source: 'acme'
    status: PanelCertStatus
    issuer: string | null
    notBefore: string | null
    notAfter: string | null
  }
  httpsMode: 'acme'
}

/** ---- GET /panel/menu/list（D-P07：构建期登记 + 服务端按角色过滤） ---- */

export interface PanelMenuItem {
  key: string
  title: string
  path: string
  icon: string
  order: number
  /** 叶子节点后端序列化为 null（api-panel-core 契约），消费端须双防护。 */
  children?: PanelMenuItem[] | null
}

export interface MenuListData {
  items: PanelMenuItem[]
}

/** ---- GET /panel/system/version（FR-03：构建 ldflags 注入） ---- */

export interface PanelVersionData {
  version: string
  gitCommit: string
  buildTime: string
}

/** ---- POST /panel/system/update-check（过程数据不落库，D-P09） ---- */

export interface UpdateCheckResultData {
  currentVersion: string
  latestVersion: string | null
  updateAvailable: boolean
  checkedAt: string
}
