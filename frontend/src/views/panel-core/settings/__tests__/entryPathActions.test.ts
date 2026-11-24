/**
 * 安全入口卡片的三个动作（entryPathActions）：成功后整页跳到新地址，不再在当前页面上发任何请求。
 *
 * 当前页面的 <base> 是旧值——成功后再发的请求带旧前缀，开着安全入口时会被拦掉。修复前「开启」
 * 成功后会重新读取一次配置，正是这次请求让页面坏掉。
 */
import { afterEach, describe, expect, it, vi } from 'vitest'
import * as panelApi from '@/api/panel'
import type { PanelConfigDetail } from '@/types/panel'
import {
  CHANGE_TTL_MS,
  createBrowserEntryPathDeps,
  createEntryPathActions,
  entrySettingsUrl,
  rememberEntryPathChange,
  takeEntryPathChange
} from '../components/entryPathActions'
import type { EntryPathActionsDeps } from '../components/entryPathActions'

/** 接口返回的配置视图（卡片只用到入口路径这一项） */
function configView(path: string | null): PanelConfigDetail {
  return { entryPath: { enabled: true, path, effectMode: 'immediate' } } as PanelConfigDetail
}

function fakeDeps(overrides: Partial<EntryPathActionsDeps> = {}): {
  deps: EntryPathActionsDeps
  /** 依赖被调用的先后顺序 */
  calls: string[]
} {
  const calls: string[] = []
  const deps: EntryPathActionsDeps = {
    configUpdate: vi.fn(async () => {
      calls.push('configUpdate')
      return configView('abc12345')
    }),
    regenerate: vi.fn(async () => {
      calls.push('regenerate')
      return { entryPath: 'newpath99' }
    }),
    remember: vi.fn((change) => {
      calls.push(`remember ${change}`)
    }),
    navigate: vi.fn((url) => {
      calls.push(`navigate ${url}`)
    }),
    ...overrides
  }
  return { deps, calls }
}

/** mock 每次被调用时收到的第一个参数 */
function firstArgsOf(mock: EntryPathActionsDeps['configUpdate']): unknown[] {
  return vi.mocked(mock).mock.calls.map((call) => call[0])
}

describe('开启', () => {
  it('成功：记一笔，整页跳到入口路径下的设置页；入口路径取接口返回的值', async () => {
    const { deps, calls } = fakeDeps()
    await expect(createEntryPathActions(deps).enable()).resolves.toBe(true)
    expect(firstArgsOf(deps.configUpdate)).toEqual([{ entryPathEnabled: true }])
    expect(calls).toEqual(['configUpdate', 'remember enabled', 'navigate /abc12345/settings'])
  })

  it('没有既有路径时服务端会自动生成：用的也是返回里的路径', async () => {
    const { deps, calls } = fakeDeps({
      configUpdate: vi.fn(async () => configView('generated1'))
    })
    await createEntryPathActions(deps).enable()
    expect(calls).toEqual(['remember enabled', 'navigate /generated1/settings'])
  })

  it('只发了这一次请求——没有再重新读取配置（那次请求带旧前缀，会被拦掉）', async () => {
    const { deps } = fakeDeps()
    await createEntryPathActions(deps).enable()
    expect(vi.mocked(deps.configUpdate)).toHaveBeenCalledTimes(1)
    expect(vi.mocked(deps.regenerate)).not.toHaveBeenCalled()
  })

  it('失败：什么都没变——不记、不跳，返回 false', async () => {
    const { deps, calls } = fakeDeps({
      configUpdate: vi.fn(async () => {
        throw new Error('PANEL_001')
      })
    })
    await expect(createEntryPathActions(deps).enable()).resolves.toBe(false)
    expect(calls).toEqual([])
  })
})

describe('重新生成', () => {
  it('成功：整页跳到新路径下的设置页', async () => {
    const { deps, calls } = fakeDeps()
    await expect(createEntryPathActions(deps).regenerate()).resolves.toBe(true)
    expect(calls).toEqual(['regenerate', 'remember regenerated', 'navigate /newpath99/settings'])
  })

  it('只发了这一次请求', async () => {
    const { deps } = fakeDeps()
    await createEntryPathActions(deps).regenerate()
    expect(vi.mocked(deps.regenerate)).toHaveBeenCalledTimes(1)
    expect(vi.mocked(deps.configUpdate)).not.toHaveBeenCalled()
  })

  it('失败（原路径不变）：不记、不跳，返回 false', async () => {
    const { deps, calls } = fakeDeps({
      regenerate: vi.fn(async () => {
        throw new Error('PANEL_002')
      })
    })
    await expect(createEntryPathActions(deps).regenerate()).resolves.toBe(false)
    expect(calls).toEqual([])
  })
})

describe('关闭', () => {
  it('成功：整页跳到根地址下的设置页', async () => {
    const { deps, calls } = fakeDeps()
    await expect(createEntryPathActions(deps).disable()).resolves.toBe(true)
    expect(firstArgsOf(deps.configUpdate)).toEqual([{ entryPathEnabled: false }])
    expect(calls).toEqual(['configUpdate', 'remember disabled', 'navigate /settings'])
  })

  it('失败：不记、不跳，返回 false', async () => {
    const { deps, calls } = fakeDeps({
      configUpdate: vi.fn(async () => {
        throw new Error('PANEL_001')
      })
    })
    await expect(createEntryPathActions(deps).disable()).resolves.toBe(false)
    expect(calls).toEqual([])
  })
})

describe('entrySettingsUrl', () => {
  it.each([
    ['安全入口没开启（路径为空）', null, '/settings'],
    ['路径是空串', '', '/settings'],
    ['正常路径', 'abc12345', '/abc12345/settings'],
    ['路径里的特殊字符会被转义', 'a/b', '/a%2Fb/settings']
  ])('%s', (_label, path, want) => {
    expect(entrySettingsUrl(path)).toBe(want)
  })
})

/** 会话存储替身 */
function fakeSessionStorage(initial: Record<string, string> = {}): {
  data: Map<string, string>
  assign: ReturnType<typeof vi.fn>
} {
  const data = new Map(Object.entries(initial))
  const assign = vi.fn()
  vi.stubGlobal('window', {
    location: { assign },
    sessionStorage: {
      getItem: (key: string) => data.get(key) ?? null,
      setItem: (key: string, value: string) => void data.set(key, value),
      removeItem: (key: string) => void data.delete(key)
    }
  })
  return { data, assign }
}

describe('跨整页跳转的记录：提示和路径要在新页面上补上', () => {
  it('记下什么就取出什么，取过即清除', () => {
    const { data } = fakeSessionStorage()
    rememberEntryPathChange('regenerated', 1_000)
    expect(takeEntryPathChange(1_001)).toBe('regenerated')
    expect(data.size).toBe(0)
    expect(takeEntryPathChange(1_002)).toBeNull()
  })

  it('没有记录时返回 null', () => {
    fakeSessionStorage()
    expect(takeEntryPathChange(1_000)).toBeNull()
  })

  it('刚好在有效期内还有效，过了就不再当作刚发生', () => {
    const { data } = fakeSessionStorage()
    rememberEntryPathChange('enabled', 1_000)
    expect(takeEntryPathChange(1_000 + CHANGE_TTL_MS)).toBe('enabled')

    rememberEntryPathChange('enabled', 1_000)
    expect(takeEntryPathChange(1_000 + CHANGE_TTL_MS + 1)).toBeNull()
    expect(data.size).toBe(0)
  })

  it.each([
    ['不认识的内容', 'nonsense'],
    ['不认识的变更名', 'bogus@1000'],
    ['时间不是数字', 'enabled@abc'],
    ['没有时间', 'enabled']
  ])('记录坏了（%s）返回 null，并清除', (_label, raw) => {
    const { data } = fakeSessionStorage({ 'panel.entryPath.change': raw })
    expect(takeEntryPathChange(1_000)).toBeNull()
    expect(data.size).toBe(0)
  })

  it('存储不可用：记的时候不报错，取的时候当作没有', () => {
    const deny = (): never => {
      throw new Error('SecurityError')
    }
    vi.stubGlobal('window', {
      sessionStorage: { getItem: deny, setItem: deny, removeItem: deny }
    })
    expect(() => rememberEntryPathChange('enabled')).not.toThrow()
    expect(takeEntryPathChange()).toBeNull()
  })

  it('没有 window（非浏览器环境）：同上', () => {
    vi.stubGlobal('window', undefined)
    expect(() => rememberEntryPathChange('enabled')).not.toThrow()
    expect(takeEntryPathChange()).toBeNull()
  })
})

describe('浏览器环境下的依赖', () => {
  it('整页跳转用地址栏跳转；记录写进会话存储', () => {
    const { data, assign } = fakeSessionStorage()
    const deps = createBrowserEntryPathDeps()
    deps.navigate('/abc12345/settings')
    deps.remember('disabled')
    expect(assign).toHaveBeenCalledWith('/abc12345/settings')
    expect(data.size).toBe(1)
    expect(takeEntryPathChange()).toBe('disabled')
  })

  it('接口依赖就是面板接口模块里的那两个函数', () => {
    fakeSessionStorage()
    const deps = createBrowserEntryPathDeps()
    expect(deps.configUpdate).toBe(panelApi.configUpdate)
    expect(deps.regenerate).toBe(panelApi.entryPathRegenerate)
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
})
