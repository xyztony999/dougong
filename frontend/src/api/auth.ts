/**
 * account-auth api 层 —— 18 Action 与 api-account-auth.md 接口一一对应（S7，无虚构接口）。
 * 命名遵循 Action 定义（PageSpec 第 2 节）。
 */
import type { ListResult } from '@/types/api'
import type {
  AuditFilter,
  AuditListQuery,
  AuditLogItem,
  InitCheckData,
  InitWizardReq,
  LoginReq,
  LoginResultData,
  PasswordChangeReq,
  SessionCurrentData,
  SessionListItem,
  SessionRevokeReq,
  TotpDisableReq,
  TotpEnableReq,
  TotpSetupData,
  TotpVerifyReq,
  UserCreateReq,
  UserCreatedData,
  UserDeleteReq,
  UserListItem,
  UserResetPasswordReq
} from '@/types/auth'
import { ApiError, buildQuery, request } from './http'
import { normalizeCode } from '@/utils/errcode'
import { normalizeAuditListResult } from '@/utils/audit'

/** ---- init（api 4.4） ---- */

export function initCheck(): Promise<InitCheckData> {
  return request<InitCheckData>('/auth/init/check')
}

export function initWizard(req: InitWizardReq): Promise<LoginResultData> {
  // 公开流（未初始化）：AUTH_001/002 语义不适用，页面自持错误 UX
  return request<LoginResultData>('/auth/init/wizard', {
    method: 'POST',
    body: req,
    silentAuth: true
  })
}

/** ---- session（api 4.2） ---- */

export function login(req: LoginReq): Promise<LoginResultData> {
  return request<LoginResultData>('/auth/session/login', {
    method: 'POST',
    body: req,
    silentAuth: true
  })
}

export function totpVerify(req: TotpVerifyReq): Promise<LoginResultData> {
  return request<LoginResultData>('/auth/session/totp-verify', {
    method: 'POST',
    body: req,
    silentAuth: true
  })
}

/** 首屏探测：未登录返回 AUTH_001 属非错误流（api 4.2.4），调用方静默处理 */
export function sessionCurrent(): Promise<SessionCurrentData> {
  return request<SessionCurrentData>('/auth/session/current', { silentAuth: true })
}

/** 登出：AUTH_002 容错（会话已失效同样按登出处理，PageSpec 第 7 节顶栏） */
export function logout(): Promise<void> {
  return request<void>('/auth/session/logout', { method: 'POST', silentAuth: true })
}

export function sessionList(): Promise<ListResult<SessionListItem>> {
  return request<ListResult<SessionListItem>>('/auth/session/list')
}

export function sessionRevoke(req: SessionRevokeReq): Promise<void> {
  return request<void>('/auth/session/revoke', { method: 'POST', body: req })
}

/** ---- user（api 4.1） ---- */

export function userList(): Promise<ListResult<UserListItem>> {
  return request<ListResult<UserListItem>>('/auth/user/list')
}

export function userCreate(req: UserCreateReq): Promise<UserCreatedData> {
  return request<UserCreatedData>('/auth/user/create', { method: 'POST', body: req })
}

export function userDelete(req: UserDeleteReq): Promise<void> {
  return request<void>('/auth/user/delete', { method: 'POST', body: req })
}

export function userResetPassword(req: UserResetPasswordReq): Promise<void> {
  return request<void>('/auth/user/reset-password', { method: 'POST', body: req })
}

export function passwordChange(req: PasswordChangeReq): Promise<void> {
  return request<void>('/auth/user/password-change', { method: 'POST', body: req })
}

export function totpSetup(): Promise<TotpSetupData> {
  return request<TotpSetupData>('/auth/user/totp-setup', { method: 'POST' })
}

export function totpEnable(req: TotpEnableReq): Promise<void> {
  return request<void>('/auth/user/totp-enable', { method: 'POST', body: req })
}

export function totpDisable(req: TotpDisableReq): Promise<void> {
  return request<void>('/auth/user/totp-disable', { method: 'POST', body: req })
}

/** ---- audit-log（api 4.3） ---- */

/**
 * E2 修复（双侧 2026-11-14）：后端 AuditRecord 已带全量 camelCase json tag（store/auth/models.go），条目键即
 * 契约 camelCase（api §4.3.4 ↔ types/auth.ts AuditLogItem）直通；normalizeAuditListResult 的 PascalCase 回退仅作 E2 前历史形态兼容垫片。
 */
export async function auditList(query: AuditListQuery): Promise<ListResult<AuditLogItem>> {
  const raw = await request<unknown>('/auth/audit-log/list', { query })
  return normalizeAuditListResult(raw)
}

/** 客户端导出文件名（本地日期，如 audit-log-2026-11-14.csv） */
function exportFilename(): string {
  const now = new Date()
  const pad = (n: number): string => String(n).padStart(2, '0')
  const date = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`
  return `audit-log-${date}.csv`
}

/**
 * 导出 CSV（D-07：200 + CSV 流，非包络；行数上限 100,000，超限返回 AUTH_012 包络）。
 * 按当前筛选条件触发下载，成功提示由调用方展示（导出动作已记审计）。
 */
export async function exportAuditLog(filter: AuditFilter): Promise<void> {
  const res = await fetch(`/auth/audit-log/export${buildQuery(filter)}`, {
    method: 'GET',
    credentials: 'same-origin',
    headers: { Accept: 'text/csv, application/json' }
  })
  if (res.status === 404) {
    throw new ApiError('NOT_FOUND', '请求的资源不存在（404）', null)
  }
  const contentType = res.headers.get('content-type') ?? ''
  if (contentType.includes('application/json')) {
    const env = (await res.json()) as { code: number | string; message: string; data: unknown }
    if (env.code !== 0) {
      // E1 修复：与 request() 同口径——现行后端 symbol 符号串直上，数值 401NN 仅历史兜底归一（utils/errcode.ts）
      throw new ApiError(normalizeCode(env.code), env.message, env.data)
    }
    throw new ApiError('BAD_RESPONSE', '导出响应异常：业务码为 0 但内容为 JSON', null)
  }
  if (!res.ok) {
    throw new ApiError('BAD_RESPONSE', `导出失败（HTTP ${res.status}）`, null)
  }
  const blob = await res.blob()
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = exportFilename()
  anchor.click()
  URL.revokeObjectURL(url)
}
