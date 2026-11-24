/**
 * 页面切换进行中状态的规则（PageSpec panel-core 第 7 节 v1.3）。
 *
 * 用真实的 vue-router（内存历史）加可控的「分包加载」验证，而不是对着 store 自说自话：
 * 这里要守的每一条都是真实交互里踩得到的情况——连点同一项、先点 A 再点 B、守卫改道、
 * 分包下载失败。路由跟踪的接线（navHooks）和真实页面用的是同一份代码。
 */
import { beforeEach, describe, expect, it } from 'vitest'
import { watch } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import type { RouteComponent, Router } from 'vue-router'
import { useNavigationStore } from '@/stores/navigation'
import { registerNavigationTracking } from '@/router/navHooks'
import type { AssetFailureHandlers } from '@/router/navFailure'

const Blank: RouteComponent = { render: () => null }
const CHROME_ERROR = 'Failed to fetch dynamically imported module: "https://x/assets/b-1.js"'

/** 可控的分包加载：每次调用拿一个新闸门，测试决定什么时候「下载完」。 */
function gateLoader() {
  const gates: Array<{ resolve: () => void; reject: (err: unknown) => void }> = []
  const load = (): Promise<RouteComponent> =>
    new Promise<RouteComponent>((resolve, reject) => {
      gates.push({ resolve: () => resolve(Blank), reject })
    })
  return { load, gates }
}

interface Harness {
  router: Router
  nav: ReturnType<typeof useNavigationStore>
  routerErrors: Array<{ err: unknown; to: string }>
}

function makeRouter(routes: Parameters<typeof createRouter>[0]['routes']): Harness {
  setActivePinia(createPinia())
  const router = createRouter({ history: createMemoryHistory(), routes })
  const routerErrors: Harness['routerErrors'] = []
  const handlers: AssetFailureHandlers = {
    onRouterError: (err, to) => {
      routerErrors.push({ err, to })
    },
    onPreloadError: () => {}
  }
  registerNavigationTracking(router, handlers)
  return { router, nav: useNavigationStore(), routerErrors }
}

/**
 * 让导航管线走到「开始下载分包」为止。守卫链每一环都是一个 promise 回调（微任务），走到
 * 懒加载被调用要经过十几跳；让出一个宏任务，微任务队列就一定排空了。
 */
const flush = async (): Promise<void> => {
  await new Promise<void>((resolve) => {
    setTimeout(resolve, 0)
  })
}

describe('页面切换进行中状态（路由跟踪）', () => {
  let a!: ReturnType<typeof gateLoader>
  let b!: ReturnType<typeof gateLoader>
  let h!: Harness

  beforeEach(async () => {
    a = gateLoader()
    b = gateLoader()
    h = makeRouter([
      { path: '/a', component: Blank },
      { path: '/b', component: a.load },
      { path: '/c', component: b.load },
      { path: '/old', component: Blank },
      { path: '/new', component: Blank },
      { path: '/blocked', component: Blank }
    ])
    await h.router.push('/a')
    await flush()
  })

  it('切换进行中记下目标（完整地址 + 路径），分包下载完成后清掉', async () => {
    const done = h.router.push('/b')
    await flush()
    expect(h.nav.pending).toBe(true)
    expect(h.nav.pendingTo).toBe('/b')
    expect(h.nav.pendingPath).toBe('/b')
    expect(h.router.currentRoute.value.path).toBe('/a') // 路由还没切：分包没下载完
    a.gates[0].resolve()
    await done
    expect(h.nav.pending).toBe(false)
    expect(h.nav.pendingPath).toBeNull()
    expect(h.router.currentRoute.value.path).toBe('/b')
  })

  it('切换到不需要下载的页面：完成后不在进行中', async () => {
    await h.router.push('/a')
    await flush()
    expect(h.nav.pending).toBe(false)
    expect(h.nav.pendingPath).toBeNull()
  })

  it('同一项连点两下：第一下被取消时不能把第二下的「切换中」清掉', async () => {
    // 逐次记下「进行中」的变化：从点击到分包下载完，中途不该掉回 false
    const seen: boolean[] = []
    const stopWatch = watch(
      () => h.nav.pending,
      (on) => seen.push(on),
      { flush: 'sync' }
    )
    const first = h.router.push('/b')
    const second = h.router.push('/b')
    await flush()
    expect(h.nav.pending).toBe(true)
    // 路由库在第一下开始下载之前就把它取消了（不会重复下载），所以只有一次加载
    expect(a.gates).toHaveLength(1)
    stopWatch()
    a.gates[0].resolve()
    await Promise.all([first, second])
    expect(h.nav.pending).toBe(false)
    expect(h.router.currentRoute.value.path).toBe('/b')
    // 第一下以「已取消」收场，那一瞬间进行中没有被清掉
    expect(seen).toEqual([true])
  })

  it('先点 B 再点 C：进行中跟随最后一次点击，B 的取消不清它', async () => {
    const seen: Array<string | null> = []
    const stopWatch = watch(
      () => h.nav.pendingPath,
      (path) => seen.push(path),
      { flush: 'sync' }
    )
    const toB = h.router.push('/b')
    await flush()
    expect(h.nav.pendingPath).toBe('/b')
    const toC = h.router.push('/c')
    await flush()
    expect(h.nav.pendingPath).toBe('/c')
    a.gates[0].resolve()
    await toB
    expect(h.nav.pending).toBe(true)
    expect(h.nav.pendingPath).toBe('/c')
    stopWatch()
    b.gates[0].resolve()
    await toC
    expect(h.nav.pending).toBe(false)
    // B 被顶替而取消时没有把进行中清掉：路径从 /b 直接跟到 /c，中途没有 null
    expect(seen).toEqual(['/b', '/c'])
  })

  it('分包下载失败：进行中清掉，错误连同目标地址交给失败处理', async () => {
    const failure = new TypeError('Failed to fetch dynamically imported module: "https://x/assets/b-1.js"')
    const done = h.router.push('/b').catch((err: unknown) => err)
    await flush()
    expect(h.nav.pending).toBe(true)
    a.gates[0].reject(failure)
    await done
    expect(h.nav.pending).toBe(false)
    expect(h.router.currentRoute.value.path).toBe('/a') // 留在原页面
    expect(h.routerErrors).toHaveLength(1)
    expect(h.routerErrors[0].err).toBe(failure)
    expect(h.routerErrors[0].to).toBe('/b')
  })

  it('交给失败处理的目标是完整地址（带查询串）：自动重新加载要落回同一个页面', async () => {
    const done = h.router.push('/b?tab=python').catch((err: unknown) => err)
    await flush()
    a.gates[0].reject(new TypeError(CHROME_ERROR))
    await done
    expect(h.routerErrors[0].to).toBe('/b?tab=python')
  })

  it('失败后再点一次：重新进入切换中（下一次点击即重试）', async () => {
    const failure = new TypeError('Failed to fetch dynamically imported module: "https://x/assets/b-1.js"')
    const first = h.router.push('/b').catch(() => undefined)
    await flush()
    a.gates[0].reject(failure)
    await first
    expect(h.nav.pending).toBe(false)
    const retry = h.router.push('/b')
    await flush()
    expect(h.nav.pending).toBe(true)
    a.gates[1].resolve()
    await retry
    expect(h.router.currentRoute.value.path).toBe('/b')
  })

  it('守卫改道（如会话失效跳登录）：进行中先记原目标、随即跟到改道后的目标，改道完成后清掉', async () => {
    const seen: Array<string | null> = []
    const stopWatch = watch(
      () => h.nav.pendingPath,
      (path) => seen.push(path),
      { flush: 'sync' }
    )
    const stop = h.router.beforeEach((to) => (to.path === '/old' ? { path: '/new', replace: true } : true))
    await h.router.push('/old')
    stop()
    stopWatch()
    expect(h.nav.pending).toBe(false)
    expect(h.router.currentRoute.value.path).toBe('/new')
    expect(seen).toEqual(['/old', '/new', null])
  })

  it('守卫中止（返回 false）：进行中清掉', async () => {
    const stop = h.router.beforeEach((to) => to.path !== '/blocked')
    await h.router.push('/blocked')
    expect(h.nav.pending).toBe(false)
    expect(h.router.currentRoute.value.path).toBe('/a')
    stop()
  })

  it('点当前所在页面：不会有进行中状态', async () => {
    await h.router.push('/a')
    expect(h.nav.pending).toBe(false)
    expect(h.nav.pendingPath).toBeNull()
  })

  it('完整地址带查询串：pendingTo 记完整地址，pendingPath 只记路径（菜单高亮用）', async () => {
    const done = h.router.push('/b?tab=python')
    await flush()
    expect(h.nav.pendingTo).toBe('/b?tab=python')
    expect(h.nav.pendingPath).toBe('/b')
    a.gates[0].resolve()
    await done
    expect(h.nav.pending).toBe(false)
  })

  it('浏览器后退走同一套跟踪：后退完成后不在进行中', async () => {
    const toC = h.router.push('/c')
    await flush()
    b.gates[0].resolve()
    await toC
    await new Promise<void>((resolve) => {
      const stop = h.router.afterEach(() => {
        stop()
        resolve()
      })
      void h.router.back()
    })
    await flush()
    expect(h.router.currentRoute.value.path).toBe('/a')
    expect(h.nav.pending).toBe(false)
  })
})

describe('navigationStore 本身', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('end 只清「就是当前进行中目标」的那次导航', () => {
    const nav = useNavigationStore()
    nav.begin({ fullPath: '/b?x=1', path: '/b' })
    nav.end({ fullPath: '/a' }) // 另一次导航的终点，不该影响 /b 的进行中
    expect(nav.pending).toBe(true)
    nav.end({ fullPath: '/b?x=1' })
    expect(nav.pending).toBe(false)
  })

  it('失败记录每次自增序号并带上种类与目标', () => {
    const nav = useNavigationStore()
    expect(nav.failure).toBeNull()
    nav.report('network', '/b')
    nav.report('network', '/b')
    nav.report('stale-reload-blocked', null)
    expect(nav.failureSeq).toBe(3)
    expect(nav.failure).toEqual({ seq: 3, kind: 'stale-reload-blocked', target: null })
  })

  it('reset 清空进行中与失败记录', () => {
    const nav = useNavigationStore()
    nav.begin({ fullPath: '/b', path: '/b' })
    nav.report('other', '/b')
    nav.reset()
    expect(nav.pending).toBe(false)
    expect(nav.pendingPath).toBeNull()
    expect(nav.failure).toBeNull()
  })
})
