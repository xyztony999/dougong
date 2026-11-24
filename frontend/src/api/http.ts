/**
 * 统一请求出口（frontend-rules §3：唯一 fetch 出口；views/components 禁止直接 fetch）。
 * - Cookie 会话：credentials same-origin（HttpOnly dg_session，R-02——token 不入 localStorage）
 * - CSRF：POST 统一注入 X-CSRF-Token（值经 registerCsrfProvider 注入，内存态）
 * - 包络：{"code","message","data"}；code !== 0 → ApiError 抛出，拦截器按 PageSpec 第 1 节分流
 *   AUTH_001/002 → 清会话跳 /login（002 附过期提示）；AUTH_003 → 提示刷新；
 *   AUTH_008/009/010/014 → 业务提示；AUTH_004/005/006/007/011/012/013 由页面专属处理
 *   （silentAuth 请求不触发全局跳转/提示，调用方自行处理——sessionCurrent 首屏探测等）
 * - 地址：入参照旧写面板路径（'/auth/me'），发出前经 withBase() 加上应用基址——开着安全入口时
 *   请求要带入口路径前缀（api D-P03，utils/base）
 */
import type { Envelope } from '@/types/api'
import { AUTH_ERROR_TEXT, fieldErrorsToText } from '@/types/auth'
import { PANEL_ERROR_TEXT } from '@/types/panel'
import { withBase } from '@/utils/base'
import { normalizeCode } from '@/utils/errcode'
import { notifyError } from '@/utils/notify'

export class ApiError extends Error {
  readonly code: string
  readonly data: unknown

  constructor(code: string, message: string, data: unknown) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.data = data
  }

  is(code: string): boolean {
    return this.code === code
  }
}

export type QueryValue = string | number | boolean | undefined

export interface RequestOptions {
  method?: 'GET' | 'POST'
  /** 查询参数对象（TS interface 无索引签名，值类型在 buildQuery 内运行时收窄） */
  query?: object
  body?: unknown
  /** true = AUTH_001/002/003 不触发全局跳转与提示（首屏探测/公共认证流/登出容错） */
  silentAuth?: boolean
}

/** 拼接 query（跳过 undefined/空串/非标量值） */
export function buildQuery(query?: object): string {
  if (query === undefined) return ''
  const pairs: string[] = []
  for (const [k, v] of Object.entries(query)) {
    if (v === undefined || v === null || v === '') continue
    if (typeof v !== 'string' && typeof v !== 'number' && typeof v !== 'boolean') continue
    pairs.push(`${encodeURIComponent(k)}=${encodeURIComponent(String(v))}`)
  }
  return pairs.length > 0 ? `?${pairs.join('&')}` : ''
}

/** csrfToken 提供方（main.ts 注册，避免 http ↔ store 循环依赖） */
let csrfProvider: (() => string | null) | null = null

export function registerCsrfProvider(provider: () => string | null): void {
  csrfProvider = provider
}

/** 会话失效处理方（main.ts 注册：清 authStore + 跳 /login） */
let authFailureHandler: (() => void) | null = null

export function registerAuthFailureHandler(handler: () => void): void {
  authFailureHandler = handler
}

/** 拦截器统一提示的码集（页面专属码不在内，避免双提示） */
const INTERCEPTOR_TOAST_CODES: ReadonlySet<string> = new Set([
  'AUTH_003',
  'AUTH_008',
  'AUTH_009',
  'AUTH_010',
  'AUTH_014'
])

function handleGlobalError(err: ApiError, silentAuth: boolean): void {
  if (err.code === 'AUTH_001' || err.code === 'AUTH_002') {
    if (!silentAuth) {
      notifyError(err.code === 'AUTH_002' ? '登录已过期，请重新登录' : '请先登录')
      authFailureHandler?.()
    }
    return
  }
  if (INTERCEPTOR_TOAST_CODES.has(err.code)) {
    notifyError(AUTH_ERROR_TEXT[err.code] ?? err.message)
    return
  }
  // panel-core 业务码（PageSpec 第 1 节错误处理行：PANEL_001~006 统一文案，页面不重复提示）
  if (PANEL_ERROR_TEXT[err.code] !== undefined) {
    notifyError(PANEL_ERROR_TEXT[err.code])
    return
  }
  if (err.code === 'AUTH_012') {
    const fields = fieldErrorsToText(err.data)
    notifyError(`参数校验失败${fields !== null ? `：${fields}` : ''}`)
    return
  }
  if (err.code === 'NETWORK_ERROR' || err.code === 'BAD_RESPONSE' || err.code === 'NOT_FOUND') {
    notifyError(err.message)
  }
}

export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const method = options.method ?? 'GET'
  const headers: Record<string, string> = { Accept: 'application/json' }
  let body: string | undefined
  if (options.body !== undefined) {
    headers['Content-Type'] = 'application/json'
    body = JSON.stringify(options.body)
  }
  if (method === 'POST') {
    const token = csrfProvider?.() ?? null
    if (token !== null) headers['X-CSRF-Token'] = token
  }

  let res: Response
  try {
    res = await fetch(withBase(path + buildQuery(options.query)), {
      method,
      headers,
      body,
      credentials: 'same-origin'
    })
  } catch {
    const err = new ApiError('NETWORK_ERROR', '网络请求失败，请检查面板服务是否可达', null)
    handleGlobalError(err, options.silentAuth === true)
    throw err
  }

  return unwrapEnvelope<T>(res, options.silentAuth === true)
}

/** 响应 → 包络解包（code=0 取 data；否则 ApiError + 全局分流） */
async function unwrapEnvelope<T>(res: Response, silentAuth: boolean): Promise<T> {
  // EntryPathGuard 语义（panel-core D-P03）：未携带/错携带安全入口 → 标准 404 无业务码包裹
  if (res.status === 404) {
    const err = new ApiError('NOT_FOUND', '请求的资源不存在（404）', null)
    handleGlobalError(err, silentAuth)
    throw err
  }

  let env: Envelope<unknown>
  try {
    env = (await res.json()) as Envelope<unknown>
  } catch {
    const err = new ApiError('BAD_RESPONSE', `响应解析失败（HTTP ${res.status}）`, null)
    handleGlobalError(err, silentAuth)
    throw err
  }

  if (env.code === 0) {
    return env.data as T
  }

  // 包络边界归一：现行后端 symbol 符号串直上（response.go Fail→ec.Symbol()，Gate 2 E1 双侧修复）；
  // 数值 401NN 为 E1 期间后端历史通道，normalizeCode 保留归一作兼容兜底（见 utils/errcode.ts）——W-3 订正
  const err = new ApiError(normalizeCode(env.code), env.message, env.data)
  handleGlobalError(err, silentAuth)
  throw err
}

/**
 * multipart/form-data POST（file-manager D-04：entry/upload 直传与 upload/chunk 逐块）。
 * 不设 Content-Type（浏览器自动携带 boundary）；CSRF 注入与包络解包同 request()。
 */
export async function postForm<T>(
  path: string,
  form: FormData,
  options: { silentAuth?: boolean } = {}
): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  const token = csrfProvider?.() ?? null
  if (token !== null) headers['X-CSRF-Token'] = token

  let res: Response
  try {
    res = await fetch(withBase(path), {
      method: 'POST',
      headers,
      body: form,
      credentials: 'same-origin'
    })
  } catch {
    const err = new ApiError('NETWORK_ERROR', '网络请求失败，请检查面板服务是否可达', null)
    handleGlobalError(err, options.silentAuth === true)
    throw err
  }

  return unwrapEnvelope<T>(res, options.silentAuth === true)
}

/**
 * GET 流式下载（file-manager D-08/D-16：校验全过才开流——错误体为 JSON 包络、成功为文件流）。
 * 成功返回 Blob（调用方触发浏览器保存）；失败走包络错误通道（与 request 同分流）。
 */
export async function downloadFile(
  path: string,
  query?: object
): Promise<{ blob: Blob; filename: string | null }> {
  const headers: Record<string, string> = { Accept: 'application/octet-stream' }
  let res: Response
  try {
    res = await fetch(withBase(path + buildQuery(query)), {
      method: 'GET',
      headers,
      credentials: 'same-origin'
    })
  } catch {
    const err = new ApiError('NETWORK_ERROR', '网络请求失败，请检查面板服务是否可达', null)
    handleGlobalError(err, false)
    throw err
  }

  return unwrapStream(res, false)
}

/**
 * POST 流式下载（database-mgmt D-06：backup/export 为数据取出动作 POST+CSRF，
 * 双形态响应同上——JSON 错误体走包络通道、成功为 SQL 文件流）。
 * 携带 JSON 请求体（{instanceId, database}）；Content-Disposition 文件名同 downloadFile 解析。
 */
export async function postDownload(
  path: string,
  body: unknown,
  options: { silentAuth?: boolean } = {}
): Promise<{ blob: Blob; filename: string | null }> {
  const headers: Record<string, string> = {
    Accept: 'application/octet-stream',
    'Content-Type': 'application/json'
  }
  const token = csrfProvider?.() ?? null
  if (token !== null) headers['X-CSRF-Token'] = token

  let res: Response
  try {
    res = await fetch(withBase(path), {
      method: 'POST',
      headers,
      body: JSON.stringify(body),
      credentials: 'same-origin'
    })
  } catch {
    const err = new ApiError('NETWORK_ERROR', '网络请求失败，请检查面板服务是否可达', null)
    handleGlobalError(err, options.silentAuth === true)
    throw err
  }

  return unwrapStream(res, options.silentAuth === true)
}

/**
 * GET 纯文本响应（cron-backup D-04：/cron/log/tail 为 200+text/plain 日志行流，
 * 非 JSON 包络）。错误/异常形态仍为 JSON 包络（unwrapEnvelope 通道统一分流）；
 * 成功返回 res.text()。
 */
export async function requestText(path: string, query?: object): Promise<string> {
  const headers: Record<string, string> = { Accept: 'text/plain' }
  let res: Response
  try {
    res = await fetch(withBase(path + buildQuery(query)), {
      method: 'GET',
      headers,
      credentials: 'same-origin'
    })
  } catch {
    const err = new ApiError('NETWORK_ERROR', '网络请求失败，请检查面板服务是否可达', null)
    handleGlobalError(err, false)
    throw err
  }

  if (res.status === 404) {
    const err = new ApiError('NOT_FOUND', '请求的资源不存在（404）', null)
    handleGlobalError(err, false)
    throw err
  }
  const contentType = res.headers.get('Content-Type') ?? ''
  if (contentType.includes('application/json')) {
    // JSON 形态 = 包络错误体通道（非 0 code → ApiError + 全局分流）
    await unwrapEnvelope<string>(res, false)
    // code=0 却返回 JSON：tail 契约为纯文本，视为坏响应
    const err = new ApiError('BAD_RESPONSE', '日志响应异常（期待纯文本）', null)
    handleGlobalError(err, false)
    throw err
  }
  if (!res.ok) {
    const err = new ApiError('BAD_RESPONSE', `日志获取失败（HTTP ${res.status}）`, null)
    handleGlobalError(err, false)
    throw err
  }
  return res.text()
}

/** 双形态响应分流（成功=文件流 Blob；失败=JSON 包络错误体——D-06/D-16 共用） */
async function unwrapStream(
  res: Response,
  silentAuth: boolean
): Promise<{ blob: Blob; filename: string | null }> {
  const contentType = res.headers.get('Content-Type') ?? ''
  if (contentType.includes('application/json')) {
    // 开流前校验/执行失败：JSON 错误体（错误必发生在开流之前）
    await unwrapEnvelope<never>(res, silentAuth)
    // unwrapEnvelope 对非 0 code 恒 throw；走到此处说明 code=0 却返回 JSON，视为坏响应
    const err = new ApiError('BAD_RESPONSE', '下载响应异常（期待文件流）', null)
    handleGlobalError(err, silentAuth)
    throw err
  }
  if (!res.ok) {
    const err = new ApiError('BAD_RESPONSE', `下载失败（HTTP ${res.status}）`, null)
    handleGlobalError(err, silentAuth)
    throw err
  }

  return { blob: await res.blob(), filename: contentDispositionFilename(res.headers) }
}

/** Content-Disposition → 文件名（attachment; filename=…；支持 filename*=UTF-8''… 编码形态） */
function contentDispositionFilename(headers: Headers): string | null {
  const disposition = headers.get('Content-Disposition')
  if (disposition === null) return null
  const parts = disposition.split(';')
  for (const part of parts) {
    const trimmed = part.trim()
    // RFC 5987：filename*=UTF-8''%E5%90%8D%E7%A7%B0（优先，按 URI 编码解出）
    if (trimmed.startsWith("filename*=")) {
      const raw = trimmed.slice('filename*='.length)
      const quoted = raw.startsWith("'") ? raw.indexOf("'", 1) : -1
      const value = quoted >= 0 ? raw.slice(quoted + 1) : raw
      try {
        return decodeURIComponent(value.replace(/"/g, ''))
      } catch {
        return value
      }
    }
  }
  for (const part of parts) {
    const trimmed = part.trim()
    if (trimmed.startsWith('filename=')) {
      return trimmed.slice('filename='.length).replace(/"/g, '')
    }
  }
  return null
}
