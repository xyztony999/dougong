/**
 * 页面文件没加载下来时的判定与恢复（PageSpec panel-core 第 7 节 v1.3）。
 *
 * 这里守的是修复前完全缺失的行为：分包下载失败不该静默；「面板已升级」要能和「网络不好」
 * 分开；自动重新加载不能成循环；两个入口收到同一次失败时只处理一次。
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  NAV_TEXT,
  consumeReloadedFlag,
  createAssetFailureHandlers,
  createBrowserRecoveryDeps,
  detectAssetFailureCause,
  errorMessage,
  installPreloadErrorGuard,
  installUnhandledRejectionGuard,
  isAssetLoadError,
  recoverFromAssetLoadError,
  resolveNavFeedback,
  STALE_RELOAD_WINDOW_MS,
  takeReloadTicket
} from '@/router/navFailure'
import type { AssetFailureHandlers, NavRecoveryDeps, PreloadEventTarget, StorageLike } from '@/router/navFailure'

const CHROME_CHUNK_ERROR = 'Failed to fetch dynamically imported module: https://panel/assets/history-abc123.js'
const VITE_CSS_ERROR = 'Unable to preload CSS for /assets/firewall-abc123.css'

function memoryStorage(initial: Record<string, string> = {}): StorageLike & { data: Map<string, string> } {
  const data = new Map(Object.entries(initial))
  return {
    data,
    getItem: (key) => data.get(key) ?? null,
    setItem: (key, value) => {
      data.set(key, value)
    },
    removeItem: (key) => {
      data.delete(key)
    }
  }
}

function fakeDeps(overrides: Partial<NavRecoveryDeps> = {}): {
  deps: NavRecoveryDeps
  calls: { reloads: Array<string | null>; reports: Array<{ kind: string; target: string | null }> }
} {
  const calls = { reloads: [] as Array<string | null>, reports: [] as Array<{ kind: string; target: string | null }> }
  const deps: NavRecoveryDeps = {
    probe: async () => ({ ok: true, isScript: true }),
    storage: memoryStorage(),
    reload: (target) => {
      calls.reloads.push(target)
    },
    report: (kind, target) => {
      calls.reports.push({ kind, target })
    },
    now: () => 1_000_000,
    ...overrides
  }
  return { deps, calls }
}

describe('isAssetLoadError：认出各浏览器的「页面文件没加载下来」', () => {
  it.each([
    ['Chrome / Edge 分包', new TypeError(CHROME_CHUNK_ERROR)],
    ['Firefox 分包', new TypeError('error loading dynamically imported module: https://panel/assets/a.js')],
    ['Safari 分包', new TypeError('Importing a module script failed.')],
    ['构建工具的样式预加载', new Error(VITE_CSS_ERROR)],
    ['另一种分包说法', new Error('Loading chunk 5 failed.')],
    ['样式分包', new Error('Loading CSS chunk 5 failed.')],
    ['带 message 的非 Error 对象', { message: CHROME_CHUNK_ERROR }],
    ['纯字符串', CHROME_CHUNK_ERROR]
  ])('%s → 是', (_name, err) => {
    expect(isAssetLoadError(err)).toBe(true)
  })

  it.each([
    ['守卫里抛的普通错误', new Error("Cannot read properties of undefined (reading 'id')")],
    ['undefined', undefined],
    ['null', null],
    ['没有 message 的对象', {}],
    ['数字', 42],
    ['无关字符串', 'Request failed with status code 500']
  ])('%s → 不是（不该被当成加载失败去重新加载）', (_name, err) => {
    expect(isAssetLoadError(err)).toBe(false)
  })

  it('errorMessage 对各种输入都给出字符串', () => {
    expect(errorMessage(new Error('x'))).toBe('x')
    expect(errorMessage({ message: 'y' })).toBe('y')
    expect(errorMessage('z')).toBe('z')
    expect(errorMessage(undefined)).toBe('undefined')
  })
})

describe('detectAssetFailureCause：网络原因还是面板已升级', () => {
  it('入口脚本还在（200 + JavaScript）→ 网络原因', async () => {
    expect(await detectAssetFailureCause(async () => ({ ok: true, isScript: true }))).toBe('network')
  })

  it('入口脚本地址成了标准 404 → 面板已升级', async () => {
    expect(await detectAssetFailureCause(async () => ({ ok: false, isScript: false, status: 404 }))).toBe('stale')
  })

  it('入口脚本地址回了 HTML（没升到的旧服务器走 SPA 兜底）→ 面板已升级', async () => {
    expect(await detectAssetFailureCause(async () => ({ ok: true, isScript: false, status: 200 }))).toBe('stale')
  })

  it.each([502, 503, 504])('反向代理回 %i（网关/链路出问题）→ 按网络原因，不算面板已升级', async (status) => {
    expect(await detectAssetFailureCause(async () => ({ ok: false, isScript: false, status }))).toBe('network')
  })

  it.each([403, 405, 429, 500])('探测拿到 %i → 按网络原因（只有 404 才说明文件已不存在）', async (status) => {
    expect(await detectAssetFailureCause(async () => ({ ok: false, isScript: false, status }))).toBe('network')
  })

  it('拿不到状态码时不猜「面板已升级」→ 按网络原因', async () => {
    expect(await detectAssetFailureCause(async () => ({ ok: false, isScript: false }))).toBe('network')
  })

  it('探测请求自己失败 / 超时 → 按网络原因，绝不因此重新加载', async () => {
    const broken = async (): Promise<{ ok: boolean; isScript: boolean }> => {
      throw new TypeError('Failed to fetch')
    }
    expect(await detectAssetFailureCause(broken)).toBe('network')
  })
})

describe('takeReloadTicket：同一目标 30 秒内只自动重新加载一次', () => {
  it('第一次发车票，窗口内第二次不发，过了窗口再发', () => {
    const storage = memoryStorage()
    const now = 50_000
    expect(take(storage, now)).toBe(true)
    expect(take(storage, now + STALE_RELOAD_WINDOW_MS - 1)).toBe(false)
    expect(take(storage, now + STALE_RELOAD_WINDOW_MS)).toBe(true)
  })

  it('不同目标互不影响', () => {
    const storage = memoryStorage()
    expect(takeReloadTicket(storage, 'dg.nav.reload:/a', 1)).toBe(true)
    expect(takeReloadTicket(storage, 'dg.nav.reload:/b', 1)).toBe(true)
  })

  it('存储不可用（读写抛错）→ 不发车票：没有记录就挡不住循环', () => {
    const broken: StorageLike = {
      getItem: () => {
        throw new Error('SecurityError')
      },
      setItem: () => {
        throw new Error('SecurityError')
      },
      removeItem: () => {
        throw new Error('SecurityError')
      }
    }
    expect(takeReloadTicket(broken, 'k', 1)).toBe(false)
    const writeOnlyBroken = memoryStorage()
    writeOnlyBroken.setItem = () => {
      throw new Error('QuotaExceededError')
    }
    expect(takeReloadTicket(writeOnlyBroken, 'k', 1)).toBe(false)
  })

  it('存过的值不是数字 → 当作没有记录', () => {
    const storage = memoryStorage({ k: 'garbage' })
    expect(takeReloadTicket(storage, 'k', 10)).toBe(true)
  })

  function take(storage: StorageLike, now: number): boolean {
    return takeReloadTicket(storage, 'dg.nav.reload:/a', now)
  }
})

describe('consumeReloadedFlag：「已重新打开」提示的标记', () => {
  it('新鲜标记 → 读到并清除，只认一次', () => {
    const storage = memoryStorage({ 'dg.nav.reloaded': '1000' })
    expect(consumeReloadedFlag(storage, 1000 + 30_000)).toBe(true)
    expect(consumeReloadedFlag(storage, 1000 + 30_001)).toBe(false)
  })

  it('超过 60 秒的旧标记 → 不认，但仍清掉', () => {
    const storage = memoryStorage({ 'dg.nav.reloaded': '1000' })
    expect(consumeReloadedFlag(storage, 1000 + 60_001)).toBe(false)
    expect(storage.data.has('dg.nav.reloaded')).toBe(false)
  })

  it('没有标记 / 存储不可用 → false', () => {
    expect(consumeReloadedFlag(memoryStorage(), 1)).toBe(false)
    const broken: StorageLike = {
      getItem: () => {
        throw new Error('SecurityError')
      },
      setItem: () => {},
      removeItem: () => {}
    }
    expect(consumeReloadedFlag(broken, 1)).toBe(false)
  })
})

describe('recoverFromAssetLoadError', () => {
  it('不是加载失败的错误 → 只提示「页面打开出错」，不探测、不重新加载', async () => {
    const probe = vi.fn(async () => ({ ok: true, isScript: true }))
    const { deps, calls } = fakeDeps({ probe })
    const result = await recoverFromAssetLoadError(new Error('boom'), '/x', deps)
    expect(result).toBe('other')
    expect(calls.reports).toEqual([{ kind: 'other', target: '/x' }])
    expect(probe).not.toHaveBeenCalled()
    expect(calls.reloads).toEqual([])
  })

  it('网络原因 → 提示后再点一次，不重新加载', async () => {
    const { deps, calls } = fakeDeps({ probe: async () => ({ ok: true, isScript: true }) })
    const result = await recoverFromAssetLoadError(new TypeError(CHROME_CHUNK_ERROR), '/x', deps)
    expect(result).toBe('network')
    expect(calls.reports).toEqual([{ kind: 'network', target: '/x' }])
    expect(calls.reloads).toEqual([])
    expect(deps.storage.getItem('dg.nav.reload:/x')).toBeNull()
  })

  it('探测被反向代理挡回（502）→ 同样按网络原因，不重新加载', async () => {
    const { deps, calls } = fakeDeps({ probe: async () => ({ ok: false, isScript: false, status: 502 }) })
    const result = await recoverFromAssetLoadError(new TypeError(CHROME_CHUNK_ERROR), '/x', deps)
    expect(result).toBe('network')
    expect(calls.reports).toEqual([{ kind: 'network', target: '/x' }])
    expect(calls.reloads).toEqual([])
    expect(deps.storage.getItem('dg.nav.reload:/x')).toBeNull()
  })

  it('面板已升级 → 重新加载到目标地址，并留下「已重新打开」标记', async () => {
    const { deps, calls } = fakeDeps({ probe: async () => ({ ok: false, isScript: false, status: 404 }) })
    const result = await recoverFromAssetLoadError(new TypeError(CHROME_CHUNK_ERROR), '/cron', deps)
    expect(result).toBe('reloading')
    expect(calls.reloads).toEqual(['/cron'])
    expect(calls.reports).toEqual([]) // 正在重新加载，不必再提示
    expect(deps.storage.getItem('dg.nav.reloaded')).toBe('1000000')
    expect(deps.storage.getItem('dg.nav.reload:/cron')).toBe('1000000')
  })

  it('面板已升级、同一目标 30 秒内已自动重新加载过 → 不再重新加载，给出带刷新按钮的提示', async () => {
    const now = { value: 50_000 }
    const { deps, calls } = fakeDeps({ probe: async () => ({ ok: false, isScript: false, status: 404 }), now: () => now.value })
    const err = new TypeError(CHROME_CHUNK_ERROR)
    expect(await recoverFromAssetLoadError(err, '/cron', deps)).toBe('reloading')
    now.value += 5_000
    expect(await recoverFromAssetLoadError(err, '/cron', deps)).toBe('stale-reload-blocked')
    expect(calls.reloads).toEqual(['/cron']) // 只重新加载了一次，没有循环
    expect(calls.reports).toEqual([{ kind: 'stale-reload-blocked', target: '/cron' }])
  })

  it('过了 30 秒窗口后同一目标可以再自动重新加载一次', async () => {
    const now = { value: 50_000 }
    const { deps, calls } = fakeDeps({ probe: async () => ({ ok: false, isScript: false, status: 404 }), now: () => now.value })
    const err = new TypeError(CHROME_CHUNK_ERROR)
    await recoverFromAssetLoadError(err, '/cron', deps)
    now.value += STALE_RELOAD_WINDOW_MS
    expect(await recoverFromAssetLoadError(err, '/cron', deps)).toBe('reloading')
    expect(calls.reloads).toEqual(['/cron', '/cron'])
  })

  it('页面内按需模块失败（没有目标地址）→ 重新加载的是当前页面，车票按「当前页」记', async () => {
    const { deps, calls } = fakeDeps({ probe: async () => ({ ok: false, isScript: false, status: 404 }) })
    const result = await recoverFromAssetLoadError(new Error(VITE_CSS_ERROR), null, deps)
    expect(result).toBe('reloading')
    expect(calls.reloads).toEqual([null])
    expect(deps.storage.getItem('dg.nav.reload:current')).toBe('1000000')
  })

  it('面板已升级但存储不可用 → 不自动重新加载（挡不住循环），改为提示', async () => {
    const broken: StorageLike = {
      getItem: () => {
        throw new Error('SecurityError')
      },
      setItem: () => {
        throw new Error('SecurityError')
      },
      removeItem: () => {}
    }
    const { deps, calls } = fakeDeps({ probe: async () => ({ ok: false, isScript: false, status: 404 }), storage: broken })
    const result = await recoverFromAssetLoadError(new TypeError(CHROME_CHUNK_ERROR), '/x', deps)
    expect(result).toBe('stale-reload-blocked')
    expect(calls.reloads).toEqual([])
    expect(calls.reports).toEqual([{ kind: 'stale-reload-blocked', target: '/x' }])
  })
})

describe('两个入口共用一套处理：同一次失败只处理一次', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  function harness(overrides: Partial<NavRecoveryDeps> = {}): {
    handlers: AssetFailureHandlers
    calls: { reloads: Array<string | null>; reports: Array<{ kind: string; target: string | null }> }
  } {
    const { deps, calls } = fakeDeps(overrides)
    return { handlers: createAssetFailureHandlers(deps), calls }
  }

  it('路由跳转引起的失败：router.onError 处理，目标 = 想去的页面', async () => {
    const h = harness({ probe: async () => ({ ok: false, isScript: false, status: 404 }) })
    h.handlers.onRouterError(new TypeError(CHROME_CHUNK_ERROR), '/cron')
    await vi.runOnlyPendingTimersAsync()
    expect(h.calls.reloads).toEqual(['/cron'])
  })

  it('路由引起的失败两处都收到：preload 先到、路由紧随其后 → 只重新加载一次，用的是路由那边的目标', async () => {
    const h = harness({ probe: async () => ({ ok: false, isScript: false, status: 404 }) })
    const err = new TypeError(CHROME_CHUNK_ERROR)
    h.handlers.onPreloadError(err) // 同步：事件刚派发
    h.handlers.onRouterError(err, '/cron') // 微任务：router.onError
    await vi.runOnlyPendingTimersAsync()
    expect(h.calls.reloads).toEqual(['/cron'])
    expect(h.calls.reports).toEqual([])
  })

  it('页面内按需模块的失败（没有路由参与）：preload 这边自己处理，重新加载当前页', async () => {
    const h = harness({ probe: async () => ({ ok: false, isScript: false, status: 404 }) })
    h.handlers.onPreloadError(new Error(VITE_CSS_ERROR))
    expect(h.calls.reloads).toEqual([]) // 先等一等，看路由那边会不会接手
    await vi.runOnlyPendingTimersAsync()
    expect(h.calls.reloads).toEqual([null])
  })

  it('同一条报错 2 秒内连着来（如连点两下都失败）→ 只处理一次', async () => {
    const h = harness()
    const err = new TypeError(CHROME_CHUNK_ERROR)
    h.handlers.onRouterError(err, '/a')
    h.handlers.onRouterError(err, '/a')
    await vi.runOnlyPendingTimersAsync()
    expect(h.calls.reports).toEqual([{ kind: 'network', target: '/a' }])
  })

  it('不同报错各自处理', async () => {
    const h = harness()
    h.handlers.onRouterError(new TypeError(CHROME_CHUNK_ERROR), '/a')
    h.handlers.onRouterError(new Error(VITE_CSS_ERROR), '/b')
    await vi.runOnlyPendingTimersAsync()
    expect(h.calls.reports).toEqual([
      { kind: 'network', target: '/a' },
      { kind: 'network', target: '/b' }
    ])
  })

  it('不是加载失败的报错：路由这边提示「页面打开出错」，preload 这边不闻不问', async () => {
    const h = harness()
    h.handlers.onPreloadError(new Error('some other module error'))
    h.handlers.onRouterError(new Error('boom'), '/a')
    await vi.runOnlyPendingTimersAsync()
    expect(h.calls.reports).toEqual([{ kind: 'other', target: '/a' }])
    expect(h.calls.reloads).toEqual([])
  })
})

describe('resolveNavFeedback 与文案', () => {
  it('网络原因 → 错误消息条，说清下一步是再点一次', () => {
    expect(resolveNavFeedback({ seq: 1, kind: 'network', target: '/a' })).toEqual({
      kind: 'toast',
      level: 'error',
      text: '页面没加载出来。请检查网络后再点一次'
    })
  })

  it('面板升级后自动重新加载没成功 → 带刷新按钮的提示框', () => {
    const feedback = resolveNavFeedback({ seq: 2, kind: 'stale-reload-blocked', target: null })
    expect(feedback).toEqual({
      kind: 'dialog',
      title: '页面没打开',
      content: '面板已更新，请刷新页面。',
      positiveText: '刷新页面',
      negativeText: '取消'
    })
  })

  it('其它导航错误 → 错误消息条', () => {
    expect(resolveNavFeedback({ seq: 3, kind: 'other', target: '/a' })).toEqual({
      kind: 'toast',
      level: 'error',
      text: '页面打开出错。请刷新后重试'
    })
  })

  it('用户可见文案：没有感叹号 / emoji / 禁用词，每句都说清了结果或下一步', () => {
    const banned = /[！!🎉✨]/
    const words = /纳管|凭据|治理|落点|回显|幂等/
    for (const text of Object.values(NAV_TEXT)) {
      expect(banned.test(text), text).toBe(false)
      expect(words.test(text), text).toBe(false)
    }
    expect(NAV_TEXT.loading).toBe('正在加载页面…')
    expect(NAV_TEXT.reloaded).toBe('面板已更新，页面已重新打开')
  })
})

describe('installPreloadErrorGuard', () => {
  it('登记后把事件的 payload 交给处理器；取消登记后不再收到', () => {
    const listeners = new Map<string, EventListenerOrEventListenerObject[]>()
    const target: PreloadEventTarget = {
      addEventListener: (type, listener) => {
        listeners.set(type, [...(listeners.get(type) ?? []), listener])
      },
      removeEventListener: (type, listener) => {
        listeners.set(type, (listeners.get(type) ?? []).filter((l) => l !== listener))
      }
    }
    const received: unknown[] = []
    const handlers: AssetFailureHandlers = {
      onRouterError: () => {},
      onPreloadError: (err) => {
        received.push(err)
      }
    }
    const dispose = installPreloadErrorGuard(target, handlers)
    const emit = (payload: unknown): void => {
      for (const listener of listeners.get('vite:preloadError') ?? []) {
        (listener as (event: Event) => void)({ payload } as unknown as Event)
      }
    }
    emit(new Error(VITE_CSS_ERROR))
    expect(received).toEqual([new Error(VITE_CSS_ERROR)])
    dispose()
    emit(new Error('again'))
    expect(received).toHaveLength(1)
    expect(listeners.get('vite:preloadError')).toEqual([])
  })
})

describe('installUnhandledRejectionGuard：只吞已经提示过的加载失败', () => {
  function harness(): { emit: (reason: unknown) => boolean; dispose: () => void } {
    const listeners: EventListener[] = []
    const target = {
      addEventListener: (_type: string, listener: EventListenerOrEventListenerObject) => {
        listeners.push(listener as EventListener)
      },
      removeEventListener: (_type: string, listener: EventListenerOrEventListenerObject) => {
        const at = listeners.indexOf(listener as EventListener)
        if (at >= 0) listeners.splice(at, 1)
      }
    }
    const dispose = installUnhandledRejectionGuard(target)
    return {
      emit: (reason) => {
        let prevented = false
        const event = { reason, preventDefault: () => (prevented = true) } as unknown as Event
        for (const listener of [...listeners]) listener(event)
        return prevented
      },
      dispose
    }
  }

  it('分包没加载下来的拒绝 → 不再冒成未捕获错误', () => {
    const h = harness()
    expect(h.emit(new TypeError(CHROME_CHUNK_ERROR))).toBe(true)
    expect(h.emit(new Error(VITE_CSS_ERROR))).toBe(true)
    h.dispose()
  })

  it('别的未处理拒绝 → 照常暴露（不吞）', () => {
    const h = harness()
    expect(h.emit(new Error('some real bug'))).toBe(false)
    expect(h.emit(undefined)).toBe(false)
    expect(h.emit('Failed to fetch dynamically imported module')).toBe(true) // 纯字符串也能认出
    h.dispose()
  })

  it('取消登记后不再拦', () => {
    const h = harness()
    h.dispose()
    expect(h.emit(new TypeError(CHROME_CHUNK_ERROR))).toBe(false)
  })
})

describe('浏览器环境下的依赖（createBrowserRecoveryDeps）：整页跳转带应用基址', () => {
  /** 模拟浏览器：地址栏、会话存储、页面里的 <base>（baseHref 为 null 表示没有） */
  function fakeBrowser(baseHref: string | null): { assign: ReturnType<typeof vi.fn>; reload: ReturnType<typeof vi.fn> } {
    const assign = vi.fn()
    const reload = vi.fn()
    vi.stubGlobal('window', { location: { assign, reload }, sessionStorage: memoryStorage() })
    vi.stubGlobal('document', {
      querySelector: (selector: string) =>
        selector === 'base[href]' && baseHref !== null ? { getAttribute: () => baseHref } : null
    })
    return { assign, reload }
  }

  it('开着安全入口：自动重新加载到的地址带入口路径（api D-P03）', () => {
    const { assign } = fakeBrowser('/abc12345/')
    createBrowserRecoveryDeps().reload('/sites/12/source')
    expect(assign).toHaveBeenCalledWith('/abc12345/sites/12/source')
  })

  it('安全入口没开启：地址不变', () => {
    const { assign } = fakeBrowser('/')
    createBrowserRecoveryDeps().reload('/sites/12/source')
    expect(assign).toHaveBeenCalledWith('/sites/12/source')
  })

  it('没有目标页时重新加载当前页', () => {
    const { assign, reload } = fakeBrowser('/abc12345/')
    createBrowserRecoveryDeps().reload(null)
    expect(reload).toHaveBeenCalledTimes(1)
    expect(assign).not.toHaveBeenCalled()
  })
})
