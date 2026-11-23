/**
 * SitesHub 挂载测试（vitest，不依赖 DOM）——用真实 Vue 运行时 + 极简宿主挂载页面
 * 外层，验证切换分栏时面板是「卸载 + 重新挂载」，而不是被原地改属性。
 *
 * 为什么需要它：NTabs 在 display-directive="if" 下每次只往容器放「当前分栏」这一个
 * 子节点。分栏不带 key 时，从 Python 切到 Node 会被 Vue 当成同一个节点改了属性——
 * 面板的 setup 不会重跑：Node 分栏沿用 Python 的仓库与表单初始值，轮询定时器也
 * 不随卸载清理。分栏带上 key 才会走「卸载旧的 + 挂载新的」。这类问题源码断言
 * 看不见（v0.4.0 重组时在真实浏览器里撞到：直接打开 ?tab=node 一切正常，从监控页
 * 「项目」入口先进 Python 再切 Node 就永远显示「还没有Node项目」）。
 *
 * 替身：NTabs / NTabPane 按上述规则分发；面板只记录生命周期；路由是可写的响应式
 * 对象（replace 回写 query，与真实路由的行为一致）。Vue 的挂载、比对、卸载是真的。
 */
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createRenderer, nextTick } from 'vue'
import type { App, VNode } from 'vue'
import { useRoute } from 'vue-router'
import type { LocationQuery } from 'vue-router'
import SitesHub from '../SitesHub.vue'

/** 分栏节点（naive-ui 的 NTabPane 带 __TAB_PANE__ 标记） */
interface TabPaneVNode extends VNode {
  type: VNode['type'] & { __TAB_PANE__?: boolean }
  props: { name?: string | number }
}

const { paneRecords, reset, click, PaneStub, SiteListStub, TabPaneDouble, TabsDouble } = await vi.hoisted(
  async () => {
    const { defineComponent, h, onUnmounted } = await import('vue')

    interface PaneRecord {
      runtime: string
      id: number
      unmounted: boolean
    }
    const paneRecords: PaneRecord[] = []
    let seq = 0
    let clickTab: ((name: string) => void) | null = null

    /** 面板替身：记下 setup 执行时的 runtime（真实面板在这一步按 runtime 取仓库、
     * 定表单初始值），渲染结果也取这份值——被原地复用时它不会跟着属性变。 */
    const PaneStub = defineComponent({
      props: { runtime: { type: String, required: true } },
      setup(props) {
        const rec: PaneRecord = { runtime: props.runtime, id: ++seq, unmounted: false }
        paneRecords.push(rec)
        onUnmounted(() => {
          rec.unmounted = true
        })
        return () => h('pane', { runtime: rec.runtime, id: rec.id })
      }
    })
    const SiteListStub = defineComponent({ setup: () => () => h('site-list') })

    /** NTabPane 替身：与真实实现一致，总是渲染自己的默认插槽 */
    const TabPaneDouble = Object.assign(
      defineComponent({
        props: { name: [String, Number] },
        setup:
          (props: { name: string | number }, { slots }: { slots: { default?: () => VNode[] } }) =>
          () =>
            h('div', { pane: props.name }, slots.default?.())
      }),
      { __TAB_PANE__: true }
    )
    /** NTabs 替身：display-directive="if" 下只放当前分栏这一个子节点（同 naive-ui
     * Tabs 的 filterMapTabPanes）。
     *
     * 不照搬它「把分栏的 key 换成 name」那一行：节点随后会被重新克隆、key 按
     * props.key 重算，那一行在真实页面里没有留下效果——分栏只有自己写了 key 才
     * 带 key，否则到达比对环节时 key 是 null（修复前后在真实页面里都核对过）。 */
    const TabsDouble = defineComponent({
      props: { value: [String, Number] },
      emits: ['update:value'],
      setup(
        props: { value: string | number },
        { slots, emit }: { slots: { default?: () => VNode[] }; emit: (event: 'update:value', name: string) => void }
      ) {
        clickTab = (name: string) => emit('update:value', name)
        return () => {
          const children: TabPaneVNode[] = []
          for (const v of (slots.default?.() ?? []) as TabPaneVNode[]) {
            if (!v || typeof v.type !== 'object' || !v.type.__TAB_PANE__) continue
            if (props.value === v.props?.name) children.push(v)
          }
          return h('tabs', children)
        }
      }
    })

    return {
      paneRecords,
      reset: () => {
        paneRecords.length = 0
        seq = 0
      },
      click: (name: string) => clickTab?.(name),
      PaneStub,
      SiteListStub,
      TabPaneDouble,
      TabsDouble
    }
  }
)

vi.mock('naive-ui', () => ({ NTabs: TabsDouble, NTabPane: TabPaneDouble }))
vi.mock('@/views/app-mgmt/AppProjectsPane.vue', () => ({ default: PaneStub }))
vi.mock('../index.vue', () => ({ default: SiteListStub }))
vi.mock('vue-router', async () => {
  const { reactive } = await import('vue')
  const route = reactive({ query: {} as Record<string, unknown> })
  return {
    useRoute: () => route,
    useRouter: () => ({
      replace: vi.fn(({ query }: { query: Record<string, unknown> }) => {
        route.query = query
      })
    })
  }
})

// ---- 极简宿主：只维护一棵节点树，够挂载与比对用 ----
interface HostNode {
  type: string
  text: string
  props: Record<string, unknown>
  children: HostNode[]
  parent: HostNode | null
}

const makeNode = (type: string, text = ''): HostNode => ({ type, text, props: {}, children: [], parent: null })

const detach = (el: HostNode) => {
  const p = el.parent
  if (!p) return
  const i = p.children.indexOf(el)
  if (i >= 0) p.children.splice(i, 1)
  el.parent = null
}

const insert = (el: HostNode, parent: HostNode, anchor: HostNode | null = null) => {
  detach(el)
  const i = anchor ? parent.children.indexOf(anchor) : -1
  if (i >= 0) parent.children.splice(i, 0, el)
  else parent.children.push(el)
  el.parent = parent
}

const renderer = createRenderer<HostNode, HostNode>({
  createElement: (type) => makeNode(type),
  createText: (text) => makeNode('#text', text),
  createComment: (text) => makeNode('#comment', text),
  setText: (el, text) => {
    el.text = text
  },
  setElementText: (el, text) => {
    el.text = text
    el.children = []
  },
  parentNode: (el) => el.parent,
  nextSibling: (el) => {
    const p = el.parent
    return p ? (p.children[p.children.indexOf(el) + 1] ?? null) : null
  },
  insert,
  remove: detach,
  patchProp: (el, key, _prev, next) => {
    el.props[key] = next
  },
  setScopeId: () => {},
  insertStaticContent: (content, parent, anchor) => {
    const el = makeNode('#static', content)
    insert(el, parent, anchor)
    return [el, el]
  }
})

const root = makeNode('root')
const route = useRoute()

function findType(type: string, n: HostNode = root): HostNode[] {
  if (n.type === type) return [n]
  return n.children.flatMap((c) => findType(type, c))
}
const renderedPanes = () =>
  findType('pane').map((p) => ({ runtime: p.props.runtime as string, id: p.props.id as number }))

describe('SitesHub：切换分栏 = 卸载 + 重新挂载（不原地复用面板实例）', () => {
  let app: App | null = null
  afterEach(() => {
    app?.unmount()
    app = null
  })

  function mountAt(query: LocationQuery) {
    reset()
    route.query = query
    app = renderer.createApp(SitesHub)
    app.mount(root)
  }

  it('从 Python 切到 Node：Node 面板是新实例，Python 面板已卸载', async () => {
    mountAt({ tab: 'python' })
    expect(renderedPanes()).toEqual([{ runtime: 'python', id: 1 }])

    click('node')
    await nextTick()

    expect(route.query).toEqual({ tab: 'node' })
    expect(renderedPanes()).toEqual([{ runtime: 'node', id: 2 }])
    expect(paneRecords.map((r) => [r.runtime, r.unmounted])).toEqual([
      ['python', true],
      ['node', false]
    ])
  })

  it('反过来也一样：直接进 Node 再切 Python，不会沿用 Node 的实例', async () => {
    mountAt({ tab: 'node' })
    expect(renderedPanes()).toEqual([{ runtime: 'node', id: 1 }])

    click('python')
    await nextTick()

    expect(renderedPanes()).toEqual([{ runtime: 'python', id: 2 }])
    expect(paneRecords.map((r) => [r.runtime, r.unmounted])).toEqual([
      ['node', true],
      ['python', false]
    ])
  })

  it('切到「网站」时项目面板卸载（面板内的轮询定时器随之清理）；地址栏去掉 tab 参数', async () => {
    mountAt({ tab: 'node' })

    click('site')
    await nextTick()

    expect(route.query).toEqual({})
    expect(renderedPanes()).toEqual([])
    expect(paneRecords).toEqual([{ runtime: 'node', id: 1, unmounted: true }])
    expect(findType('site-list')).toHaveLength(1)
  })
})
