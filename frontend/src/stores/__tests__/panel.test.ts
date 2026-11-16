/**
 * panelStore 回归测试（Gate 2 W4——panelStore 此前 0 用例）：
 * 覆盖 loadConfig/loadMenu/loadCertStatus 成败流、pendingRestart 派生（D-P02 横幅源）与
 * loadMenu 重挂载先清残留语义（D-P07：服务端按角色过滤、前端不残留上一角色菜单）；
 * fixture session 为 number 秒——Gate 2 W1 契约 §4.1.4 钉死 int64 秒的类型锁定
 * （若 types/panel.ts 残留 string，本文件 typecheck 即 FAIL）。
 * v2.0 增量（2026-11-16）：+bindDomain 分组 fixture（D-P12）与 loadCertStatus 流
 * （面板证书 Card 数据源，certStatus 两角色可见）。
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { certStatus, configDetail, menuList } from '@/api/panel'
import { usePanelStore } from '@/stores/panel'
import type { PanelCertStatusData, PanelConfigDetail, PanelMenuItem } from '@/types/panel'

vi.mock('@/api/panel', () => ({
  certStatus: vi.fn(),
  configDetail: vi.fn(),
  menuList: vi.fn()
}))

const mockCertStatus = vi.mocked(certStatus)
const mockConfigDetail = vi.mocked(configDetail)
const mockMenuList = vi.mocked(menuList)

/** 默认 12h/2h（api §3.5 配置矩阵），int64 秒（契约 §4.1.4，W1 钉死） */
const DETAIL: PanelConfigDetail = {
  listen: { scope: 'all', address: null, port: 44300, effectMode: 'restart_required' },
  entryPath: { enabled: false, path: null, effectMode: 'immediate' },
  bindDomain: { value: null, effectMode: 'immediate' },
  https: { mode: 'off', cert: null, effectMode: 'immediate' },
  updateCheck: { enabled: false, effectMode: 'immediate' },
  lang: { value: 'zh-CN', supported: ['zh-CN'], effectMode: 'immediate' },
  session: { absoluteTimeout: 43200, idleTimeout: 7200 },
  pendingRestart: false
}

/** certStatus fixture（api 4.1.4 v2.0：未绑定域名 + 未配置证书 + ACME 概要） */
const CERT_STATUS_EMPTY: PanelCertStatusData = {
  cert: null,
  acme: { domain: null, challenge: 'http_01', autoRenew: true }
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

describe('panelStore · loadCertStatus 面板证书流（v2.0，PageSpec 5.1 Card 5 数据源）', () => {
  const CERT_STATUS_ACME: PanelCertStatusData = {
    cert: {
      source: 'acme',
      status: 'valid',
      issuer: "Let's Encrypt R11",
      notBefore: '2026-11-01T00:00:00Z',
      notAfter: '2027-01-30T00:00:00Z',
      lastError: null
    },
    acme: { domain: 'panel.example.com', challenge: 'http_01', autoRenew: true }
  }

  it('成功：certStatusData 落位、loading 复位、无错误态', async () => {
    mockCertStatus.mockResolvedValueOnce(CERT_STATUS_ACME)
    const store = usePanelStore()
    await store.loadCertStatus()

    expect(store.certStatusData).toEqual(CERT_STATUS_ACME)
    expect(store.certStatusLoading).toBe(false)
    expect(store.certStatusError).toBe(false)
  })

  it('失败：certStatusError 置位且既有数据保留（同 loadConfig 语义；错误已由拦截器统一提示）', async () => {
    mockCertStatus.mockResolvedValueOnce(CERT_STATUS_EMPTY)
    mockCertStatus.mockRejectedValueOnce(new Error('PANEL_008'))
    const store = usePanelStore()
    await store.loadCertStatus()
    expect(store.certStatusData).toEqual(CERT_STATUS_EMPTY)

    await store.loadCertStatus()
    expect(store.certStatusError).toBe(true)
    expect(store.certStatusLoading).toBe(false)
    // 失败不清空既有数据（面板证书 Card 摘要仍可见）
    expect(store.certStatusData).toEqual(CERT_STATUS_EMPTY)
  })

  it('cert=null 未配置证书 + acme.domain=bind_domain 当前值透传（fixture 锁定 v2.0 契约形态）', async () => {
    mockCertStatus.mockResolvedValueOnce(CERT_STATUS_EMPTY)
    const store = usePanelStore()
    await store.loadCertStatus()

    expect(store.certStatusData?.cert).toBeNull()
    expect(store.certStatusData?.acme.challenge).toBe('http_01')
    expect(store.certStatusData?.acme.autoRenew).toBe(true)
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
