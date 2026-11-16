/**
 * panelStore 回归测试（Gate 2 W4——panelStore 此前 0 用例）：
 * 覆盖 loadConfig/loadMenu 成败流、pendingRestart 派生（D-P02 横幅源）与
 * loadMenu 重挂载先清残留语义（D-P07：服务端按角色过滤、前端不残留上一角色菜单）；
 * fixture session 为 number 秒——Gate 2 W1 契约 §4.1.4 钉死 int64 秒的类型锁定
 * （若 types/panel.ts 残留 string，本文件 typecheck 即 FAIL）。
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { configDetail, menuList } from '@/api/panel'
import { usePanelStore } from '@/stores/panel'
import type { PanelConfigDetail, PanelMenuItem } from '@/types/panel'

vi.mock('@/api/panel', () => ({
  configDetail: vi.fn(),
  menuList: vi.fn()
}))

const mockConfigDetail = vi.mocked(configDetail)
const mockMenuList = vi.mocked(menuList)

/** 默认 12h/2h（api §3.5 配置矩阵），int64 秒（契约 §4.1.4，W1 钉死） */
const DETAIL: PanelConfigDetail = {
  listen: { scope: 'all', address: null, port: 44300, effectMode: 'restart_required' },
  entryPath: { enabled: false, path: null, effectMode: 'immediate' },
  https: { mode: 'off', cert: null, effectMode: 'immediate' },
  updateCheck: { enabled: false, effectMode: 'immediate' },
  lang: { value: 'zh-CN', supported: ['zh-CN'], effectMode: 'immediate' },
  session: { absoluteTimeout: 43200, idleTimeout: 7200 },
  pendingRestart: false
}

const MENU: PanelMenuItem[] = [
  { key: 'about', title: '关于', path: '/about', icon: 'information-circle', order: 1 },
  { key: 'settings', title: '面板设置', path: '/settings', icon: 'settings-outline', order: 2 }
]

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
})

describe('panelStore · loadConfig 加载流', () => {
  it('成功：detail 落位、loading 复位、无错误态', async () => {
    mockConfigDetail.mockResolvedValueOnce(DETAIL)
    const store = usePanelStore()
    await store.loadConfig()

    expect(store.configDetail).toEqual(DETAIL)
    expect(store.configLoading).toBe(false)
    expect(store.configError).toBe(false)
  })

  it('失败：configError 置位且既有 detail 保留（错误已由拦截器统一提示，页面错误态+重试）', async () => {
    mockConfigDetail.mockResolvedValueOnce(DETAIL)
    mockConfigDetail.mockRejectedValueOnce(new Error('PANEL_005'))
    const store = usePanelStore()
    await store.loadConfig()
    expect(store.configDetail).toEqual(DETAIL)

    await store.loadConfig()
    expect(store.configError).toBe(true)
    expect(store.configLoading).toBe(false)
    expect(store.configDetail).toEqual(DETAIL)
  })

  it('pendingRestart 派生：未加载 → false；detail.pendingRestart=true → true（D-P02 横幅源）', async () => {
    const store = usePanelStore()
    expect(store.pendingRestart).toBe(false)

    mockConfigDetail.mockResolvedValueOnce({ ...DETAIL, pendingRestart: true })
    await store.loadConfig()
    expect(store.pendingRestart).toBe(true)
  })
})

describe('panelStore · loadMenu 菜单流（D-P07 服务端按角色过滤，前端不二次过滤）', () => {
  it('成功：items=服务端下发集直存（readonly 子集原样，不做前端过滤）', async () => {
    mockMenuList.mockResolvedValueOnce({ items: MENU })
    const store = usePanelStore()
    await store.loadMenu()

    expect(store.menuItems).toEqual(MENU)
    expect(store.menuLoading).toBe(false)
    expect(store.menuError).toBe(false)
  })

  it('失败：menuError 置位且 items 清空（重挂载先清残留——登出换角色登录不残留上一角色菜单）', async () => {
    mockMenuList.mockResolvedValueOnce({ items: MENU })
    mockMenuList.mockRejectedValueOnce(new Error('AUTH_001'))
    const store = usePanelStore()
    await store.loadMenu()
    expect(store.menuItems).toEqual(MENU)

    await store.loadMenu()
    expect(store.menuError).toBe(true)
    expect(store.menuLoading).toBe(false)
    expect(store.menuItems).toEqual([])
  })
})
