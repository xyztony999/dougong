/**
 * 页面文件没加载下来时的判定、恢复与文案（PageSpec panel-core 第 7 节 v1.3）。
 *
 * 背景：页面组件按需下载，两种情况会让一次点击彻底没反应，而修复前没有任何提示：
 *  1. 网络抖动，分包请求被打断——再点一次就会重试，但用户不知道；
 *  2. 面板升级后浏览器还开着旧页面——旧页面引用的分包文件名（带内容哈希）已经不存在，
 *     点多少次都失败，只有刷新才恢复。
 * 这里把两种情况分开处理：第 1 种提示后再点；第 2 种自动重新加载一次（同一目标 30 秒内
 * 至多一次，防循环），不行就给出带刷新按钮的提示。
 *
 * 两个入口共用一套处理：路由跳转引起的失败走 router.onError；页面内的按需模块（如在线
 * 编辑器）走构建工具的 vite:preloadError 事件。同一次失败两处都会收到，只处理先到的那次。
 * 本文件不碰界面：结论写进 navigationStore，由 AppNavFeedback 转成提示。
 */
import type { NavFailure, NavFailureKind } from '@/stores/navigation'
import { useNavigationStore } from '@/stores/navigation'
import { withBase } from '@/utils/base'

/** 用户可见文案（栈文案规约：无 emoji、无感叹号，说清结果与下一步）。 */
export const NAV_TEXT = {
  loading: '正在加载页面…',
  network: '页面没加载出来。请检查网络后再点一次',
  reloaded: '面板已更新，页面已重新打开',
  staleTitle: '页面没打开',
  staleBlocked: '面板已更新，请刷新页面。',
  staleRefresh: '刷新页面',
  staleCancel: '取消',
  other: '页面打开出错。请刷新后重试'
} as const

/** 同一目标两次自动重新加载之间至少间隔这么久（防循环）。 */
export const STALE_RELOAD_WINDOW_MS = 30_000
/** 探测入口脚本的最长等待；超时按网络原因处理。 */
export const PROBE_TIMEOUT_MS = 8_000
const RELOADED_FLAG_KEY = 'dg.nav.reloaded'
/** 「已重新打开」提示只在重新加载后这么久内有效，避免旧标记误报。 */
const RELOADED_FLAG_MAX_AGE_MS = 60_000
const RELOAD_TICKET_PREFIX = 'dg.nav.reload:'
/** 同一条失败信息在这段时间内只处理一次。 */
const HANDLED_WINDOW_MS = 2_000

/** 各浏览器对「页面文件没加载下来」的说法不同，逐一列出。 */
const ASSET_LOAD_ERROR =
  /dynamically imported module|Importing a module script failed|Unable to preload CSS|Loading (?:CSS )?chunk .+ failed|error loading module script/i

export function errorMessage(err: unknown): string {
  if (err instanceof Error) return err.message
  if (typeof err === 'object' && err !== null && 'message' in err) {
    return String((err as { message: unknown }).message)
  }
  return String(err)
}

/** 是不是「页面文件没加载下来」：是才走下面的恢复，其它错误原样提示。 */
export function isAssetLoadError(err: unknown): boolean {
  return ASSET_LOAD_ERROR.test(errorMessage(err))
}

export interface StorageLike {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
  removeItem(key: string): void
}

export interface BuildProbeResult {
  ok: boolean
  /** 响应是不是 JavaScript（面板已升级时入口脚本的地址会变成 404 或回退的 HTML） */
  isScript: boolean
  /** HTTP 状态码。探测请求拿到了响应就一定有；拿不到响应时 probe 自己抛错，走不到判定 */
  status?: number
}

export type AssetFailureCause = 'network' | 'stale'

/**
 * 判定没加载下来的原因：只有服务器明确回答「入口脚本已经不在了」才算面板已升级——
 * 标准 404，或 2xx 但不是 JavaScript（没升到的旧服务器走 SPA 兜底回的 HTML）。
 * 入口脚本还在 = 网络原因。其余一律按网络原因：探测请求自己失败（网络不通、超时），
 * 或者拿到的是别的状态码（反向代理的 5xx、不支持 HEAD 的 405、鉴权代理的 403）——
 * 这些说明链路或代理出了问题，宁可让用户再点一次，也不在网络不通时重新加载页面。
 */
export async function detectAssetFailureCause(probe: () => Promise<BuildProbeResult>): Promise<AssetFailureCause> {
  try {
    const result = await probe()
    if (result.ok) return result.isScript ? 'network' : 'stale'
    return result.status === 404 ? 'stale' : 'network'
  } catch {
    return 'network'
  }
}

/**
 * 自动重新加载的「车票」：同一目标在 windowMs 内只发一张。存储不可用（隐私模式等）时
 * 一律不发——没有记录就挡不住循环，这时改为让用户手动刷新。
 */
export function takeReloadTicket(
  storage: StorageLike,
  key: string,
  now: number,
  windowMs: number = STALE_RELOAD_WINDOW_MS
): boolean {
  try {
    const previous = Number(storage.getItem(key))
    if (Number.isFinite(previous) && previous > 0 && now - previous < windowMs) return false
    storage.setItem(key, String(now))
    return true
  } catch {
    return false
  }
}

/** 取走「刚自动重新加载过」的标记；标记太旧（超过 60 秒）视为没有。取走即清除。 */
export function consumeReloadedFlag(storage: StorageLike, now: number): boolean {
  try {
    const raw = storage.getItem(RELOADED_FLAG_KEY)
    if (raw === null) return false
    storage.removeItem(RELOADED_FLAG_KEY)
    const at = Number(raw)
    return Number.isFinite(at) && now - at <= RELOADED_FLAG_MAX_AGE_MS
  } catch {
    return false
  }
}

export interface NavRecoveryDeps {
  /** 探测入口脚本是否还在 */
  probe: () => Promise<BuildProbeResult>
  storage: StorageLike
  /** 重新加载：target 为 null 时重新加载当前页面 */
  reload: (target: string | null) => void
  report: (kind: NavFailureKind, target: string | null) => void
  now: () => number
}

/**
 * 处理一次「页面文件没加载下来」：返回 'reloading' 表示正在自动重新加载（之后不必再提示），
 * 否则已通过 deps.report 交出该给用户看的提示种类。
 */
export async function recoverFromAssetLoadError(
  err: unknown,
  target: string | null,
  deps: NavRecoveryDeps
): Promise<'reloading' | NavFailureKind> {
  if (!isAssetLoadError(err)) {
    deps.report('other', target)
    return 'other'
  }
  const cause = await detectAssetFailureCause(deps.probe)
  if (cause === 'network') {
    deps.report('network', target)
    return 'network'
  }
  if (!takeReloadTicket(deps.storage, RELOAD_TICKET_PREFIX + (target ?? 'current'), deps.now())) {
    deps.report('stale-reload-blocked', target)
    return 'stale-reload-blocked'
  }
  try {
    deps.storage.setItem(RELOADED_FLAG_KEY, String(deps.now()))
  } catch {
    // 存不下标记只是少一句「页面已重新打开」，不影响重新加载本身
  }
  deps.reload(target)
  return 'reloading'
}

export interface AssetFailureHandlers {
  /** 路由跳转引起的失败（router.onError） */
  onRouterError(err: unknown, target: string): void
  /** 页面内按需模块的失败（vite:preloadError）；路由引起的失败也会先到这里 */
  onPreloadError(err: unknown): void
}

/**
 * 两个入口共用的处理器。路由引起的失败两处都会收到：preload 事件先到（同步），router.onError
 * 紧随其后（微任务）。preload 这边推迟一个宏任务再看——那一次失败若已由路由这边处理，
 * 就不再重复；没被处理的才是真正的页面内按需模块失败。
 */
export function createAssetFailureHandlers(deps: NavRecoveryDeps): AssetFailureHandlers {
  const handled = new Map<string, number>()
  const markHandled = (message: string): void => {
    const now = Date.now()
    for (const [key, at] of handled) {
      if (now - at > HANDLED_WINDOW_MS) handled.delete(key)
    }
    handled.set(message, now)
  }
  const handledRecently = (message: string): boolean => {
    const at = handled.get(message)
    return at !== undefined && Date.now() - at <= HANDLED_WINDOW_MS
  }
  return {
    onRouterError(err, target) {
      // 同一条报错 2 秒内只处理一次：用户连点两下、两下都失败时，不需要两条一样的提示，
      // 更不能触发两次重新加载
      if (isAssetLoadError(err)) {
        const message = errorMessage(err)
        if (handledRecently(message)) return
        markHandled(message)
      }
      void recoverFromAssetLoadError(err, target, deps)
    },
    onPreloadError(err) {
      if (!isAssetLoadError(err)) return
      const message = errorMessage(err)
      setTimeout(() => {
        if (handledRecently(message)) return
        markHandled(message)
        void recoverFromAssetLoadError(err, null, deps)
      }, 0)
    }
  }
}

/** 用户看得到的提示：消息条，或带「刷新页面」按钮的提示框。 */
export type NavFeedback =
  | { kind: 'toast'; level: 'error' | 'info'; text: string }
  | { kind: 'dialog'; title: string; content: string; positiveText: string; negativeText: string }

export function resolveNavFeedback(failure: NavFailure): NavFeedback {
  switch (failure.kind) {
    case 'network':
      return { kind: 'toast', level: 'error', text: NAV_TEXT.network }
    case 'stale-reload-blocked':
      return {
        kind: 'dialog',
        title: NAV_TEXT.staleTitle,
        content: NAV_TEXT.staleBlocked,
        positiveText: NAV_TEXT.staleRefresh,
        negativeText: NAV_TEXT.staleCancel
      }
    default:
      return { kind: 'toast', level: 'error', text: NAV_TEXT.other }
  }
}

/** sessionStorage 的安全包装：被浏览器禁用时读写都退化为什么都没发生。 */
export function createBrowserStorage(): StorageLike {
  const safe = <T>(run: () => T, fallback: T): T => {
    try {
      return run()
    } catch {
      return fallback
    }
  }
  return {
    getItem: (key) => safe(() => window.sessionStorage.getItem(key), null),
    setItem: (key, value) => safe(() => void window.sessionStorage.setItem(key, value), undefined),
    removeItem: (key) => safe(() => void window.sessionStorage.removeItem(key), undefined)
  }
}

/** 浏览器环境下的恢复依赖（生产用；测试注入替身）。 */
export function createBrowserRecoveryDeps(): NavRecoveryDeps {
  return {
    probe: async () => {
      // 当前页面入口脚本的地址（文件名带内容哈希）：面板升级后这个名字就不存在了
      const entry = document.querySelector('script[type="module"][src]')
      const src = entry?.getAttribute('src')
      if (src === null || src === undefined || src === '') throw new Error('entry script not found')
      const controller = new AbortController()
      const timer = setTimeout(() => controller.abort(), PROBE_TIMEOUT_MS)
      try {
        const res = await fetch(src, { method: 'HEAD', cache: 'no-store', signal: controller.signal })
        const contentType = res.headers.get('content-type') ?? ''
        return { ok: res.ok, isScript: contentType.includes('javascript'), status: res.status }
      } finally {
        clearTimeout(timer)
      }
    },
    storage: createBrowserStorage(),
    reload: (target) => {
      if (target === null) window.location.reload()
      // 目标是面板路径（如 /sites）：整页跳转要带上应用基址，否则开着安全入口时跳过去是 404
      else window.location.assign(withBase(target))
    },
    report: (kind, target) => useNavigationStore().report(kind, target),
    now: () => Date.now()
  }
}

export interface PreloadEventTarget {
  addEventListener(type: string, listener: EventListenerOrEventListenerObject): void
  removeEventListener(type: string, listener: EventListenerOrEventListenerObject): void
}

/** 监听构建工具的分包预加载失败事件；返回取消监听的函数。 */
export function installPreloadErrorGuard(target: PreloadEventTarget, handlers: AssetFailureHandlers): () => void {
  // 不阻止事件继续抛出：路由引起的失败要让它走到 router.onError，由那边处理。
  const onPreloadError = (event: Event): void => {
    handlers.onPreloadError((event as Event & { payload?: unknown }).payload)
  }
  target.addEventListener('vite:preloadError', onPreloadError)
  return () => target.removeEventListener('vite:preloadError', onPreloadError)
}

/**
 * 页面文件加载失败的 Promise 拒绝不再冒成「未捕获错误」。
 *
 * 页面里跳转都写成 `void router.push(...)`，加载失败时这个 Promise 会拒绝；失败本身已经由
 * router.onError / vite:preloadError 给了提示和出路，再在控制台刷一条 Uncaught (in promise)
 * 只会吓到看控制台的人、污染错误上报。只吞这一类（文案能认出是加载失败的），其它未处理的
 * 拒绝照常暴露。返回取消监听的函数。
 */
export function installUnhandledRejectionGuard(
  target: PreloadEventTarget,
  isLoadError: (err: unknown) => boolean = isAssetLoadError
): () => void {
  const onUnhandled = (event: Event): void => {
    if (isLoadError((event as Event & { reason?: unknown }).reason)) event.preventDefault()
  }
  target.addEventListener('unhandledrejection', onUnhandled)
  return () => target.removeEventListener('unhandledrejection', onUnhandled)
}
