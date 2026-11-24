/**
 * 弹窗、抽屉正在关闭时，把落在侧栏上的点击转发给菜单项（PageSpec panel-core 第 7 节 v1.3）。
 *
 * 背景：弹窗/抽屉关闭后的离场动画约 0.2~0.25 秒，这期间全屏层仍然盖在整个页面上、
 * 照样接收点击。用户关掉弹窗立刻去点侧栏，那一下会落在正在消失的全屏层上被吃掉
 * （v0.4.0 实测：关闭后 264ms 内点侧栏无效；主线程忙时会拖到 1 秒）。
 *
 * 做法：在 document 的捕获阶段看一眼每个点击。若它落在「正在关闭」的全屏层上，而该位置
 * 底下正是侧栏的某个菜单项，就不让它继续传到全屏层，改为直接触发那个菜单项。
 * 只转发侧栏：落在页面其它位置的点击照旧被全屏层吃掉——用户连点两下「点遮罩关闭」时，
 * 第二下不会穿到弹窗下面的按钮上。
 *
 * 依赖组件库 Naive UI 的内部类名（弹窗外层 n-modal-body-wrapper、抽屉遮罩 n-drawer-mask、
 * 离场过渡类 *-leave-active）。组件库升级后若类名变了，这里识别不到就什么都不做，
 * 行为退回「关闭后约 0.25 秒内点侧栏会被吃掉」，不会出错。
 */
import type { Ref } from 'vue'
import { onBeforeUnmount, onMounted } from 'vue'

/**
 * 全屏层的识别类名（Naive UI 2.45 实测结构）：
 *  - 弹窗：n-modal-container → [ n-modal-mask, n-modal-body-wrapper → … → 全屏滚动层 → 弹窗卡片 ]。
 *    遮罩与外层容器是兄弟，外层容器盖在遮罩上面，点哪里命中的都是外层容器里的全屏滚动层；
 *    离场类加在遮罩和卡片上。所以既要认外层容器（往里找离场元素），也要认遮罩自己。
 *  - 抽屉：n-drawer-container → [ n-drawer-mask, 抽屉体 ]。离场类加在遮罩上，点中的就是遮罩。
 */
export const OVERLAY_SELECTOR = '.n-modal-body-wrapper, .n-modal-mask, .n-drawer-mask'
export const LEAVING_SELECTOR = '.fade-in-transition-leave-active, .fade-in-scale-up-transition-leave-active'
export const MENU_ITEM_SELECTOR = '.n-menu-item-content'

/** 命中测试给出的元素里，这里只用到「往上找祖先」这一样。 */
export interface HitElement {
  closest(selector: string): HitElement | null
}

/** 点击事件里本转发器用到的部分（真实 MouseEvent 满足它；测试传替身）。 */
export interface ClickLike {
  target: unknown
  clientX: number
  clientY: number
  stopPropagation(): void
  preventDefault(): void
}

/**
 * 被点到的元素是否在「正在关闭」的弹窗/抽屉全屏层里；是则返回那层，否则 null。
 * 抽屉遮罩自己就是离场中的元素；弹窗的离场元素（遮罩或弹窗体）在外层容器里面。
 * 还开着的弹窗不算——它的遮罩点击（点遮罩关闭）必须照常工作。
 */
export function findClosingOverlay(target: unknown): Element | null {
  if (target === null || typeof target !== 'object') return null
  const node = target as Element
  if (typeof node.closest !== 'function') return null
  const overlay = node.closest(OVERLAY_SELECTOR)
  if (overlay === null) return null
  if (overlay.matches(LEAVING_SELECTOR)) return overlay
  return overlay.querySelector(LEAVING_SELECTOR) !== null ? overlay : null
}

/**
 * 一叠命中的元素（最上层在前）里，位于本侧栏菜单里的第一个菜单项内容节点；没有则 null。
 * isInMenu 由调用方给出（真实 DOM 里就是「菜单根元素包含它」）。
 */
export function findMenuItemAt(hits: readonly HitElement[], isInMenu: (node: HitElement) => boolean): HitElement | null {
  for (const hit of hits) {
    const item = hit.closest(MENU_ITEM_SELECTOR)
    if (item !== null && isInMenu(item)) return item
  }
  return null
}

/**
 * 这次点击要不要转发给侧栏？要的话返回该触发的菜单项。
 * hits = document.elementsFromPoint(点击位置)，最上层在前。
 */
export function resolveForwardedMenuItem(
  event: ClickLike,
  hits: readonly HitElement[],
  isInMenu: ((node: HitElement) => boolean) | null
): HitElement | null {
  if (isInMenu === null || findClosingOverlay(event.target) === null) return null
  return findMenuItemAt(hits, isInMenu)
}

/** 在组件里登记转发：挂载时在 document 捕获阶段监听，卸载时移除。menuRoot 是包着侧栏菜单的元素。 */
export function useClosingOverlayForwarder(doc: Document, menuRoot: Ref<Element | null>): void {
  const onClick = (event: MouseEvent): void => {
    const root = menuRoot.value
    if (root === null || findClosingOverlay(event.target) === null) return
    // 命中测试给的本来就是真实 Element，这里的结构类型只是为了能测试；转回 Node 只为调用 contains
    const item = resolveForwardedMenuItem(event, doc.elementsFromPoint(event.clientX, event.clientY), (node) =>
      root.contains(node as unknown as Node)
    )
    // 菜单项内容节点是 div；不是可点击元素就别拦这一下（拦了却没东西响应，等于把点击丢了）
    if (!(item instanceof HTMLElement)) return
    // 这一下点击是给侧栏的：不再让它传到正在关闭的全屏层（避免又触发一次「点遮罩关闭」）
    event.stopPropagation()
    event.preventDefault()
    item.click()
  }
  onMounted(() => doc.addEventListener('click', onClick, true))
  onBeforeUnmount(() => doc.removeEventListener('click', onClick, true))
}
