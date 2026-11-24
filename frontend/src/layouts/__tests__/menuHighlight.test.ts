/**
 * 侧栏菜单该高亮哪一项（PageSpec panel-core 第 7 节 v1.3「页面切换的加载反馈」）。
 */
import { describe, expect, it } from 'vitest'
import { collectMenuPaths, resolveMenuValue } from '@/layouts/menuHighlight'
import type { PanelMenuItem } from '@/types/panel'

function item(path: string, children: PanelMenuItem[] | null = null): PanelMenuItem {
  return { key: path === '' ? 'group' : path, title: path, path, icon: 'x', order: 1, visibility: 'admin', children } as PanelMenuItem
}

describe('collectMenuPaths', () => {
  it('收集所有叶子项的路径，分组节点（path 为空）不算，分组内的子项要算进去', () => {
    const items = [
      item('/monitoring'),
      item('', [item('/users'), item('', [item('/sessions'), item('/audit-logs')])]),
      item('/sites'),
      item('/about', null)
    ]
    expect(collectMenuPaths(items)).toEqual(new Set(['/monitoring', '/users', '/sessions', '/audit-logs', '/sites', '/about']))
  })

  it('菜单还没加载（空数组）→ 空集合', () => {
    expect(collectMenuPaths([]).size).toBe(0)
  })
})

describe('resolveMenuValue', () => {
  const menuPaths = new Set(['/monitoring', '/sites', '/about'])

  it('没有切换在进行 → 跟随当前路由', () => {
    expect(resolveMenuValue(null, menuPaths, '/monitoring')).toBe('/monitoring')
  })

  it('切换进行中且目标是菜单页 → 提前高亮目标（不等分包下载完）', () => {
    expect(resolveMenuValue('/sites', menuPaths, '/monitoring')).toBe('/sites')
  })

  it('切换进行中但目标不是菜单页（在线编辑、表详情）→ 保持当前路由的高亮', () => {
    expect(resolveMenuValue('/sites/12/source', menuPaths, '/sites')).toBe('/sites')
  })

  it('菜单还没加载出来时来了切换 → 不乱高亮', () => {
    expect(resolveMenuValue('/sites', new Set<string>(), '/monitoring')).toBe('/monitoring')
  })
})
