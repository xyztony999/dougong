/**
 * 侧栏菜单该高亮哪一项（PageSpec panel-core 第 7 节 v1.3「页面切换的加载反馈」）。
 *
 * 页面组件按需下载，切换完成前路由还停在旧页面。若高亮只跟随当前路由，用户点了「历史趋势」
 * 之后要等分包下载完（慢链路上数秒）那一项才会亮——看起来就像没点上。所以切换进行中且
 * 目标是菜单页时，提前高亮目标；目标不是菜单页（如在线编辑、表详情）则保持原样。
 */
import type { PanelMenuItem } from '@/types/panel'

/** 菜单树里所有叶子项的路径（分组节点 path 为空，不算；递归分组内的子项）。 */
export function collectMenuPaths(items: readonly PanelMenuItem[]): Set<string> {
  const paths = new Set<string>()
  const walk = (nodes: readonly PanelMenuItem[]): void => {
    for (const node of nodes) {
      if (node.path !== '') paths.add(node.path)
      if (node.children != null) walk(node.children)
    }
  }
  walk(items)
  return paths
}

/** 切换进行中且目标是菜单页 → 目标路径；否则 → 当前路由路径。 */
export function resolveMenuValue(
  pendingPath: string | null,
  menuPaths: ReadonlySet<string>,
  routePath: string
): string {
  return pendingPath !== null && menuPaths.has(pendingPath) ? pendingPath : routePath
}
