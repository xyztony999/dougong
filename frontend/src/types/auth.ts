/**
 * account-auth 业务类型 —— 与 api-account-auth.md v1.0/v1.1 请求/响应字段逐字对齐（F02）。
 * 字段名以 API 文档接口详情为准（camelCase，如 session list 的 current 布尔）。
 */
import type {
  AuditCategory,
  AuditResult,
  AuditSource,
  PanelRole,
  PanelUserStatus
} from './enums'

/** 错误码标识（api 3.4；AUTH_001~AUTH_014） */
export type AuthErrorCode =
  | 'AUTH_001'
  | 'AUTH_002'
  | 'AUTH_003'
  | 'AUTH_004'
  | 'AUTH_005'
  | 'AUTH_006'
  | 'AUTH_007'
  | 'AUTH_008'
  | 'AUTH_009'
  | 'AUTH_010'
  | 'AUTH_011'
  | 'AUTH_012'
  | 'AUTH_013'
  | 'AUTH_014'

/** 传输层失败标识（非后端错误码：网络不可达/响应异常/标准 404——D-P03 EntryPathGuard 语义） */
export type TransportErrorCode = 'NETWORK_ERROR' | 'BAD_RESPONSE' | 'NOT_FOUND'

/** 拦截器统一提示文案（PageSpec 第 1 节错误处理分流；页面专属码由页面处理，不在此列） */
export const AUTH_ERROR_TEXT: Readonly<Record<string, string>> = {
  AUTH_003: '安全校验失败（CSRF），请刷新页面后重试',
  AUTH_008: '权限不足：只读角色禁止变更操作',
  AUTH_009: '面板已初始化，无需重复初始化',
  AUTH_010: '系统需保留至少一个管理员账户',
  AUTH_014: '不能删除当前登录账户'
}

/** ---- 通用数据结构 ---- */

export interface SessionUser {
  id: number
  username: string
  role: PanelRole
}

/** login / totp-verify / init-wizard 成功响应 data（等同登录成功响应，api 4.4.4） */
export interface LoginResultData {
  csrfToken: string
  user: SessionUser
}

/** GET /auth/init/check（api 4.4.4） */
export interface InitCheckData {
  initialized: boolean
  allowWizard: boolean
}

/** GET /auth/session/current（api 4.2.4；未登录返回 AUTH_001 非错误流） */
export interface SessionCurrentData {
  authenticated: boolean
  user: SessionUser
  csrfToken: string
}

/** ---- panel_user（api 4.1） ---- */

export interface UserListItem {
  id: number
  username: string
  role: PanelRole
  status: PanelUserStatus
  totpEnabled: boolean
  lockedUntil: string | null
  createdAt: string
}

export interface UserCreateReq {
  username: string
  password: string
  role: PanelRole
}

export interface UserCreatedData {
  id: number
  username: string
  role: PanelRole
  createdAt: string
}

export interface UserDeleteReq {
  userId: number
}

export interface UserResetPasswordReq {
  userId: number
  newPassword: string
}

export interface PasswordChangeReq {
  oldPassword: string
  newPassword: string
}

export interface TotpEnableReq {
  code: string
}

export interface TotpDisableReq {
  password: string
}

export interface TotpSetupData {
  otpauthUri: string
  secret: string
}

/** POST /auth/init/wizard（api 4.4.4）：TOTP 可选一步完成，totpSecret+totpCode 成对（Gate 1 W4） */
export interface InitWizardReq {
  username: string
  password: string
  totpSecret?: string
  totpCode?: string
}

/** ---- session（api 4.2） ---- */

export interface LoginReq {
  username: string
  password: string
}

export interface TotpVerifyReq {
  loginTicket: string
  code: string
}

/** 在线会话行：id=token_hash 前 8 位摘要；current=是否当前会话（api 4.2.4） */
export interface SessionListItem {
  id: string
  username: string
  createdAt: string
  lastActiveAt: string
  sourceIp: string | null
  current: boolean
}

export interface SessionRevokeReq {
  sessionId: string
}

/** ---- audit_log（api 4.3） ---- */

export interface AuditLogItem {
  id: number
  actorName: string
  category: AuditCategory
  action: string
  objectType: string
  objectId: string | null
  detail: string | null
  result: AuditResult
  source: AuditSource
  createdAt: string
}

/** query 与导出共用筛选参数（api 4.3.4；objectId 等值匹配 Gate 1 W3） */
export interface AuditFilter {
  from?: string
  to?: string
  actorName?: string
  category?: AuditCategory
  action?: string
  objectType?: string
  objectId?: string
}

export type AuditListQuery = AuditFilter & {
  page: number
  pageSize: number
}

/** ---- 错误 data 收窄（无 any：unknown + 类型守卫） ---- */

/** AUTH_012 data.fields[]（R-14 逐字段错误） */
export interface FieldError {
  field: string
  message: string
}

export function isFieldErrorList(v: unknown): v is FieldError[] {
  if (!Array.isArray(v)) return false
  return v.every(
    (it) =>
      typeof it === 'object' &&
      it !== null &&
      'field' in it &&
      'message' in it &&
      typeof (it as Record<string, unknown>).field === 'string' &&
      typeof (it as Record<string, unknown>).message === 'string'
  )
}

/** AUTH_004 data.remainingSeconds（登录锁定倒计时） */
export function extractRemainingSeconds(v: unknown): number | null {
  if (typeof v === 'object' && v !== null && 'remainingSeconds' in v) {
    const s = (v as Record<string, unknown>).remainingSeconds
    if (typeof s === 'number' && Number.isFinite(s)) return s
  }
  return null
}

/** AUTH_007 data.loginTicket（二步式一次性票据，5 分钟） */
export function extractLoginTicket(v: unknown): string | null {
  if (typeof v === 'object' && v !== null && 'loginTicket' in v) {
    const t = (v as Record<string, unknown>).loginTicket
    if (typeof t === 'string' && t.length > 0) return t
  }
  return null
}

/** AUTH_012 fields 拼接为可读文案 */
export function fieldErrorsToText(v: unknown): string | null {
  if (!isFieldErrorList(v) || v.length === 0) return null
  return v.map((f) => `${f.field}: ${f.message}`).join('；')
}
