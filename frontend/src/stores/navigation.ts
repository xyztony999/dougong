/**
 * navigationStore —— 页面切换的进行中状态与失败记录（框架级，两角色一致；
 * PageSpec panel-core 第 7 节 v1.3「页面切换的加载反馈」「页面文件没下载下来」）。
 *
 * 为什么需要它：页面组件按需下载，慢链路上第一次打开某个页面要等分包下载完，路由才会切换。
 * 这段时间界面原本没有任何变化，用户以为没点上而连点（v0.4.0 实测：50KB/s 的链路上
 * 「历史趋势」首次打开空等 10.9 秒）。这里记下「正在切去哪」：
 *  - 侧栏据此让被点击的菜单项立即高亮；
 *  - 顶栏据此在等待超过 200ms 时显示「正在加载页面…」；
 *  - 失败由 AppNavFeedback 按种类提示（判定与文案见 router/navFailure.ts）。
 *
 * 状态只由路由钩子写（router/navHooks.ts），组件只读。
 */
import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

/** 页面切换失败的种类（决定给用户看哪句话）。 */
export type NavFailureKind = 'network' | 'stale-reload-blocked' | 'other'

export interface NavFailure {
  /** 每次失败自增，监听方据此触发（同一种失败连着发生两次也要各提示一次） */
  seq: number
  kind: NavFailureKind
  /** 失败时想去的地址（完整地址，含查询串）；页面内按需模块失败时为 null */
  target: string | null
}

export const useNavigationStore = defineStore('navigation', () => {
  /** 进行中的导航目标（完整地址）；null = 没有进行中的切换 */
  const pendingTo = ref<string | null>(null)
  /** 进行中目标的路径（不含查询串），供侧栏菜单提前高亮 */
  const pendingPath = ref<string | null>(null)
  const failureSeq = ref(0)
  const failure = ref<NavFailure | null>(null)

  const pending = computed(() => pendingTo.value !== null)

  function begin(to: { fullPath: string; path: string }): void {
    pendingTo.value = to.fullPath
    pendingPath.value = to.path
  }

  /**
   * 一次导航到达终点（切换完成、被守卫中止，或出错）。只有终点就是当前进行中的目标才清：
   * 被更新的导航顶替而取消的那一次不清——进行中的状态属于顶替它的那一次，否则同一目标
   * 连点两下时，第一下的取消会把第二下的「切换中」提前清掉。
   */
  function end(to: { fullPath: string } | null): void {
    if (to === null || pendingTo.value === to.fullPath) {
      pendingTo.value = null
      pendingPath.value = null
    }
  }

  function report(kind: NavFailureKind, target: string | null): void {
    failureSeq.value += 1
    failure.value = { seq: failureSeq.value, kind, target }
  }

  function reset(): void {
    pendingTo.value = null
    pendingPath.value = null
    failure.value = null
  }

  return { pendingTo, pendingPath, failureSeq, failure, pending, begin, end, report, reset }
})
