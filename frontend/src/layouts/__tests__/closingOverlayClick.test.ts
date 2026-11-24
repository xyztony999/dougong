/**
 * 弹窗、抽屉正在关闭时的侧栏点击转发（PageSpec panel-core 第 7 节 v1.3）。
 *
 * 没有测试 DOM 库，用带 class 的假节点树复现真实结构（弹窗与抽屉的层级是在无头浏览器里
 * 导出后照着搭的，见下）；离场时相应节点带 *-leave-active 类。
 * 命中测试（elementsFromPoint）的顺序也照真实浏览器：最上层在前。
 */
import { describe, expect, it } from 'vitest'
import { findClosingOverlay, findMenuItemAt, resolveForwardedMenuItem } from '@/layouts/closingOverlayClick'
import type { ClickLike, HitElement } from '@/layouts/closingOverlayClick'

interface FakeNode {
  cls: string[]
  parent: FakeNode | null
  children: FakeNode[]
  clicks: number
  closest(selector: string): FakeNode | null
  matches(selector: string): boolean
  querySelector(selector: string): FakeNode | null
  contains(node: FakeNode | null): boolean
  click(): void
}

/** 选择器只用到「.类名」和逗号分组，与被测代码里的常量一致。 */
const names = (selector: string): string[] =>
  selector
    .split(',')
    .map((s) => s.trim().replace(/^\./, ''))
    .filter(Boolean)

function node(...cls: string[]): FakeNode {
  const n: FakeNode = {
    cls,
    parent: null,
    children: [],
    clicks: 0,
    closest(selector) {
      for (let cur: FakeNode | null = this; cur !== null; cur = cur.parent) {
        if (names(selector).some((c) => cur?.cls.includes(c))) return cur
      }
      return null
    },
    matches(selector) {
      return names(selector).some((c) => this.cls.includes(c))
    },
    querySelector(selector) {
      const walk = (cur: FakeNode): FakeNode | null => {
        for (const child of cur.children) {
          if (names(selector).some((c) => child.cls.includes(c))) return child
          const hit = walk(child)
          if (hit !== null) return hit
        }
        return null
      }
      return walk(this)
    },
    contains(node) {
      for (let cur = node; cur !== null; cur = cur.parent) if (cur === this) return true
      return false
    },
    click() {
      this.clicks += 1
    }
  }
  return n
}

function add(parent: FakeNode, ...children: FakeNode[]): void {
  for (const child of children) {
    child.parent = parent
    parent.children.push(child)
  }
}

/** 真实调用方给的是「菜单根元素包含它」；假节点上同样沿 parent 往上找。 */
function insideOf(root: FakeNode): (node: HitElement) => boolean {
  return (node) => {
    for (let cur = node as unknown as FakeNode | null; cur !== null; cur = cur.parent) if (cur === root) return true
    return false
  }
}

function clickOn(target: unknown): ClickLike & { stopped: number; prevented: number } {
  const event = {
    target,
    clientX: 102,
    clientY: 129,
    stopped: 0,
    prevented: 0,
    stopPropagation() {
      this.stopped += 1
    },
    preventDefault() {
      this.prevented += 1
    }
  }
  return event
}

/** 侧栏：根 → 菜单 → 菜单项内容 → 文字。 */
function sidebar(): { root: FakeNode; item: FakeNode; label: FakeNode; header: FakeNode } {
  const root = node('menu-root')
  const item = node('n-menu-item-content')
  const label = node('n-menu-item-content-header')
  const header = node('n-menu-item-content', 'n-menu-item-content--collapsed')
  add(root, item, header)
  add(item, label)
  return { root, item, label, header }
}

/**
 * 弹窗（Naive UI 2.45 实测结构，无头浏览器里导出）：
 *   n-modal-container → [ n-modal-mask（全屏遮罩）, n-modal-body-wrapper → n-scrollbar →
 *   n-scrollbar-container → n-scrollbar-content.n-modal-scroll-content（全屏滚动层）→ n-card.n-modal ]
 * 遮罩与外层容器是兄弟，外层容器排在后面、盖在遮罩上面——所以点哪里命中的都是那层全屏滚动层
 * （「点遮罩关闭」实际也是点在它上）。离场时带离场类的是遮罩和卡片，滚动层自己没有。
 */
function modal(leaving: boolean): { scroll: FakeNode; wrapper: FakeNode; mask: FakeNode; body: FakeNode; card: FakeNode } {
  const container = node('n-modal-container')
  const mask = node('n-modal-mask', ...(leaving ? ['fade-in-transition-leave-active', 'fade-in-transition-leave-to'] : []))
  const wrapper = node('n-modal-body-wrapper')
  const scrollbar = node('n-scrollbar')
  const scrollContainer = node('n-scrollbar-container')
  const scroll = node('n-scrollbar-content', 'n-modal-scroll-content')
  const card = node('n-card', 'n-modal', ...(leaving ? ['fade-in-scale-up-transition-leave-active'] : []))
  add(container, mask, wrapper)
  add(wrapper, scrollbar)
  add(scrollbar, scrollContainer)
  add(scrollContainer, scroll)
  add(scroll, card)
  return { scroll, wrapper, mask, body: card, card }
}

/** 抽屉：容器 → 遮罩 + 抽屉体。 */
function drawer(leaving: boolean): { mask: FakeNode; container: FakeNode } {
  const container = node('n-drawer-container')
  const mask = node('n-drawer-mask', ...(leaving ? ['fade-in-transition-leave-active'] : []))
  add(container, mask, node('n-drawer', ...(leaving ? ['slide-in-from-right-transition-leave-active'] : [])))
  return { mask, container }
}

describe('findClosingOverlay：只认「正在关闭」的弹窗/抽屉全屏层', () => {
  it('弹窗离场中：点到全屏滚动层上的任何位置都算', () => {
    const m = modal(true)
    expect(findClosingOverlay(m.scroll)).toBe(m.wrapper) // 全屏滚动层 → 往里找到离场中的卡片
    expect(findClosingOverlay(m.card)).toBe(m.wrapper) // 点在正在消失的卡片上
    expect(findClosingOverlay(m.mask)).toBe(m.mask) // 遮罩是外层容器的兄弟，自己带离场类
  })

  it('弹窗还开着（没有离场类）→ 不算：它的「点遮罩关闭」必须照常工作', () => {
    const m = modal(false)
    expect(findClosingOverlay(m.scroll)).toBeNull()
    expect(findClosingOverlay(m.mask)).toBeNull()
  })

  it('抽屉遮罩离场中：遮罩自己就是离场元素', () => {
    const closing = drawer(true)
    const open = drawer(false)
    expect(findClosingOverlay(closing.mask)).toBe(closing.mask)
    expect(findClosingOverlay(open.mask)).toBeNull()
  })

  it('点到页面上别的东西 → null', () => {
    const sb = sidebar()
    expect(findClosingOverlay(sb.label)).toBeNull()
    expect(findClosingOverlay(node('n-card'))).toBeNull()
  })

  it('目标不是元素（文本节点、null、数字）→ null，不抛错', () => {
    expect(findClosingOverlay(null)).toBeNull()
    expect(findClosingOverlay(42)).toBeNull()
    expect(findClosingOverlay('text')).toBeNull()
    expect(findClosingOverlay({ nodeType: 3 })).toBeNull()
  })
})

describe('findMenuItemAt：一叠命中里找本侧栏的菜单项', () => {
  it('跳过盖在上面的层，取第一个属于侧栏的菜单项内容节点', () => {
    const sb = sidebar()
    const overlay = node('n-modal-scroll-content')
    expect(findMenuItemAt([overlay, sb.label, sb.item, sb.root], insideOf(sb.root))).toBe(sb.item)
  })

  it('分组标题也是菜单项（点它应该展开/收起分组）', () => {
    const sb = sidebar()
    const groupLabel = node('n-menu-item-content-header')
    add(sb.header, groupLabel)
    expect(findMenuItemAt([groupLabel], insideOf(sb.root))).toBe(sb.header)
  })

  it('命中里没有菜单项 / 菜单项不属于本侧栏 / 没有侧栏 → null', () => {
    const sb = sidebar()
    const outsider = node('n-menu-item-content')
    const inside = insideOf(sb.root)
    expect(findMenuItemAt([node('n-card'), node('button')], inside)).toBeNull()
    expect(findMenuItemAt([outsider], inside)).toBeNull()
    expect(findMenuItemAt([sb.label], () => false)).toBeNull()
    expect(findMenuItemAt([], inside)).toBeNull()
  })
})

describe('resolveForwardedMenuItem：这次点击要不要转发给侧栏', () => {
  it('弹窗正在关闭、点在侧栏上 → 转发给那个菜单项', () => {
    const sb = sidebar()
    const m = modal(true)
    const hit = resolveForwardedMenuItem(clickOn(m.scroll), [m.scroll, sb.label, sb.item], insideOf(sb.root))
    expect(hit).toBe(sb.item)
  })

  it('抽屉正在关闭、点在侧栏上 → 转发', () => {
    const sb = sidebar()
    const d = drawer(true)
    const hit = resolveForwardedMenuItem(clickOn(d.mask), [d.mask, sb.label], insideOf(sb.root))
    expect(hit).toBe(sb.item)
  })

  it('弹窗还开着时点侧栏位置 → 不转发（那一下是「点遮罩关闭」）', () => {
    const sb = sidebar()
    const m = modal(false)
    expect(resolveForwardedMenuItem(clickOn(m.mask), [m.mask, sb.label], insideOf(sb.root))).toBeNull()
  })

  it('弹窗正在关闭，但点在页面内容上（底下不是菜单项）→ 不转发：连点不会穿到弹窗下面的按钮', () => {
    const m = modal(true)
    const button = node('n-button')
    expect(resolveForwardedMenuItem(clickOn(m.scroll), [m.scroll, button], () => true)).toBeNull()
  })

  it('侧栏还没渲染出来（没有根）→ 不转发', () => {
    const sb = sidebar()
    const m = modal(true)
    expect(resolveForwardedMenuItem(clickOn(m.scroll), [m.scroll, sb.label], null)).toBeNull()
  })

  it('点击根本没落在全屏层上（正常点侧栏）→ 不转发，走浏览器自己的点击', () => {
    const sb = sidebar()
    expect(resolveForwardedMenuItem(clickOn(sb.label), [sb.label, sb.item], insideOf(sb.root))).toBeNull()
  })
})
