/**
 * panel-core 业务类型 —— 与 api-panel-core.md v1.0 请求/响应字段逐字对齐（F02）。
 * detail 六分组结构 = api 4.1.4 GET /panel/config/detail data；
 * update 为部分字段集语义（api 4.1.4：仅提交本 Card 字段）。
 */
import type { ConfigEffectMode, HttpsMode, ListenScope, PanelCertSource } from './enums'

/** 错误码标识（api 3.4；PANEL_001~PANEL_006，AUTH_* 复用 account-auth 码族） */
export type PanelErrorCode =
  | 'PANEL_001'
  | 'PANEL_002'
  | 'PANEL_003'
  | 'PANEL_004'
  | 'PANEL_005'
  | 'PANEL_006'

/** 拦截器统一提示文案（PageSpec 第 1 节错误处理行 + 5.1 Card 3/6 文案逐字） */
export const PANEL_ERROR_TEXT: Readonly<Record<string, string>> = {
  PANEL_001: '监听范围为自定义时必须填写合法的监听地址',
  PANEL_002: '证书解析失败或与私钥不配对，请检查 PEM 内容',
  PANEL_003: '请先导入证书与私钥',
  PANEL_004: '更新检查未开启',
  PANEL_005: '版本源不可达，可稍后手动重试',
  PANEL_006: '所选语言暂不支持'
}

/** ---- GET /panel/config/detail（api 4.1.4） ---- */

/** 证书摘要（I-P06：仅摘要，key_pem 永不下发） */
export interface PanelCertSummary {
  source: PanelCertSource
  issuer: string | null
  notBefore: string | null
  notAfter: string | null
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

/** ---- GET /panel/menu/list（D-P07：构建期登记 + 服务端按角色过滤） ---- */

export interface PanelMenuItem {
  key: string
  title: string
  path: string
  icon: string
  order: number
  children?: PanelMenuItem[]
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
