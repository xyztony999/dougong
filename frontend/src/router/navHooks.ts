/**
 * 路由 → 页面切换进行中状态的接线（PageSpec panel-core 第 7 节 v1.3）。
 *
 * 单独成文件是为了能用内存路由做测试：router/index.ts 一加载就要读浏览器地址，测试环境里
 * 导入不了它，而这里的规则恰恰需要逐条验证（连点、被顶替、守卫改道、分包失败……）。
 */
import { isNavigationFailure, NavigationFailureType } from 'vue-router'
import type { Router } from 'vue-router'
import { useNavigationStore } from '@/stores/navigation'
import type { AssetFailureHandlers } from './navFailure'

/**
 * 登记「切换中」跟踪与失败处理。要先于其它前置守卫登记：这样认证探测等异步守卫的耗时
 * 也算在「切换中」里，顶栏的加载提示才不会晚出现。
 */
export function registerNavigationTracking(router: Router, handlers: AssetFailureHandlers): void {
  router.beforeEach((to) => {
    useNavigationStore().begin(to)
  })
  router.afterEach((to, _from, failure) => {
    // 被更新的导航顶替而取消的那一次不清：进行中的状态属于顶替它的那一次。
    // 否则「先点 A 再点 B」或「同一项连点两下」时，A 的取消会把 B 的切换中提前清掉。
    if (isNavigationFailure(failure, NavigationFailureType.cancelled)) return
    useNavigationStore().end(to)
  })
  router.onError((error, to) => {
    useNavigationStore().end(to)
    handlers.onRouterError(error, to.fullPath)
  })
}
