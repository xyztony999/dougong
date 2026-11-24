/**
 * 应用基址（utils/base）：面板自己的地址都要带上 <base href> 写的前缀。
 * node 环境没有 document，用一个只会回答 <base href> 的替身代替。
 */
import { afterEach, describe, expect, it, vi } from 'vitest'
import { appBasePath, joinBase, withBase } from '../base'

/** 模拟页面：baseHref 为 null 表示页面里没有 <base> */
function fakePage(baseHref: string | null): void {
  vi.stubGlobal('document', {
    querySelector: (selector: string) =>
      selector === 'base[href]' && baseHref !== null ? { getAttribute: () => baseHref } : null
  })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('appBasePath：只读页面里的 <base href>，只认以单个 / 开头的路径', () => {
  it('没有 document（非浏览器环境）：/ ', () => {
    expect(appBasePath()).toBe('/')
  })

  it('页面里没有 <base>（开发服务器）：/', () => {
    fakePage(null)
    expect(appBasePath()).toBe('/')
  })

  it('安全入口没开启：<base href="/"> → /', () => {
    fakePage('/')
    expect(appBasePath()).toBe('/')
  })

  it('安全入口开启：<base href="/abc12345/"> → /abc12345/', () => {
    fakePage('/abc12345/')
    expect(appBasePath()).toBe('/abc12345/')
  })

  it('少了结尾斜杠也补齐（/abc12345 → /abc12345/）', () => {
    fakePage('/abc12345')
    expect(appBasePath()).toBe('/abc12345/')
  })

  it.each([
    ['相对写法', 'abc/'],
    ['指向别的站点', '//evil.example/x/'],
    ['完整地址', 'https://evil.example/x/'],
    ['空值', '']
  ])('不认：%s（%s）', (_label, href) => {
    fakePage(href)
    expect(appBasePath()).toBe('/')
  })
})

describe('withBase：面板路径加上前缀，其它地址原样返回', () => {
  it('安全入口没开启时地址不变', () => {
    fakePage('/')
    expect(withBase('/auth/session/current')).toBe('/auth/session/current')
  })

  it('安全入口开启时带上入口路径', () => {
    fakePage('/abc12345/')
    expect(withBase('/auth/session/current')).toBe('/abc12345/auth/session/current')
  })

  it('带查询串的地址：前缀加在路径前面', () => {
    fakePage('/abc12345/')
    expect(withBase('/auth/audit-log/export?page=1&size=20#top')).toBe(
      '/abc12345/auth/audit-log/export?page=1&size=20#top'
    )
  })

  it.each([
    ['完整地址', 'https://other.example/path'],
    ['协议相对地址', '//other.example/path']
  ])('%s 不是面板路径，原样返回', (_label, path) => {
    fakePage('/abc12345/')
    expect(withBase(path)).toBe(path)
  })

  it('页面里没有 <base> 时按根地址', () => {
    fakePage(null)
    expect(withBase('/favicon.svg')).toBe('/favicon.svg')
  })
})

describe('joinBase：把路径拼到指定基址下', () => {
  it.each([
    ['/', '/settings', '/settings'],
    ['', '/settings', '/settings'],
    ['/abc/', '/settings', '/abc/settings'],
    ['/abc', '/settings', '/abc/settings'],
    ['/abc/', 'settings', '/abc/settings']
  ])('joinBase(%j, %j) → %j', (base, path, want) => {
    expect(joinBase(base, path)).toBe(want)
  })
})
