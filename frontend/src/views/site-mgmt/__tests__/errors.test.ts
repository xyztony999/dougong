/**
 * site-mgmt 错误分流与类型护栏回归测试（Gate 2 第 1 轮 W-2 前端侧补测）：
 * - siteActionError：SITE_004 output 弹窗通道（warning + output 字段）、SITE_012 reason
 *   追加、SITE_/CERT_ 文案表覆盖、未知码/非 ApiError 返回 null（不重复提示）；
 * - SITE_003/010/CERT_005 data 载荷收窄护栏：isSiteRollbackData/isSiteStaleData/
 *   isCertSitesData/fieldOfString（交互层消费 unknown 前的类型守卫）。
 * 类型基准：types/site.ts（api-site-mgmt.md §3.4/4.1.4/4.2.4——W-1 对齐后
 * SiteCreateResult.listen=string、SiteProxy.hostHeader=string 一并断言）。
 */
import { describe, expect, it } from 'vitest'
import { ApiError } from '@/api/http'
import type { CertItem, SiteCreateResult, SiteProxy } from '@/types/site'
import { fieldOfString, isCertSitesData, isSiteRollbackData, isSiteStaleData } from '@/types/site'
import { siteActionError } from '../errors'

describe('siteActionError（写动作错误兜底分流）', () => {
  it('SITE_004：warning 级 + data.output 通道（弹窗呈现、配置已保留语义 D-05）', () => {
    const info = siteActionError(new ApiError('SITE_004', 'reload failed', { output: 'bind() failed' }))
    expect(info).not.toBeNull()
    expect(info?.level).toBe('warning')
    expect(info?.output).toBe('bind() failed')
    expect(info?.text).toContain('重载失败')
    expect(info?.text).toContain('bind() failed')
  })

  it('SITE_012：data.reason 追加进文案，无 output', () => {
    const info = siteActionError(new ApiError('SITE_012', 'unavailable', { reason: 'nginx binary not found' }))
    expect(info?.level).toBe('error')
    expect(info?.output).toBeUndefined()
    expect(info?.text).toContain('nginx binary not found')
  })

  it('SITE_/CERT_ 文案表全覆盖（SITE_001~014 + CERT_001~007 无缺行）', () => {
    for (const code of Array.from({ length: 14 }, (_, i) => `SITE_${String(i + 1).padStart(3, '0')}`)) {
      expect(siteActionError(new ApiError(code, 'x', null))?.text).toBeTruthy()
    }
    for (const code of Array.from({ length: 7 }, (_, i) => `CERT_${String(i + 1).padStart(3, '0')}`)) {
      expect(siteActionError(new ApiError(code, 'x', null))?.text).toBeTruthy()
    }
  })

  it('无 data 载荷时仅文案；未知码与非 ApiError 返回 null', () => {
    expect(siteActionError(new ApiError('SITE_005', 'x', { existingSiteId: 'a#0' }))?.text).toContain('已存在')
    expect(siteActionError(new ApiError('PANEL_999', 'x', null))).toBeNull()
    expect(siteActionError(new Error('network'))).toBeNull()
  })
})

describe('错误 data 载荷收窄护栏（unknown → 具体载荷）', () => {
  it('isSiteRollbackData：SITE_003 errors[]+diff', () => {
    expect(isSiteRollbackData({ errors: ['line 3: unexpected "}"'], diff: '- a\n+ b' })).toBe(true)
    expect(isSiteRollbackData({ errors: [] })).toBe(true)
    expect(isSiteRollbackData({ errors: 'line 3' })).toBe(false)
    expect(isSiteRollbackData(null)).toBe(false)
  })

  it('isSiteStaleData：SITE_010 currentMtime', () => {
    expect(isSiteStaleData({ currentMtime: '2026-11-15T00:00:00Z' })).toBe(true)
    expect(isSiteStaleData({ currentMtime: 123 })).toBe(false)
    expect(isSiteStaleData({})).toBe(false)
  })

  it('isCertSitesData：CERT_005 sites[] 引用清单', () => {
    expect(
      isCertSitesData({ sites: [{ siteId: '/etc/nginx/a.conf#0', serverNames: ['a.com'] }] })
    ).toBe(true)
    expect(isCertSitesData({ sites: [{ serverNames: ['a.com'] }] })).toBe(false)
    expect(isCertSitesData({ sites: 'x' })).toBe(false)
  })

  it('fieldOfString：字符串键提取 / 缺失与异型返回 null', () => {
    expect(fieldOfString({ output: 'x' }, 'output')).toBe('x')
    expect(fieldOfString({ output: 1 }, 'output')).toBeNull()
    expect(fieldOfString(null, 'output')).toBeNull()
  })
})

describe('W-1 类型对齐静态断言（编译期契约，防类型漂移回归）', () => {
  it('SiteCreateResult.listen=string（CreateSiteData.Listen string 原样回传）', () => {
    const r: SiteCreateResult = { siteId: 'a#0', type: 'static', serverNames: ['a.com'], listen: '80', sourceFile: '/f' }
    expect(r.listen).toBe('80')
  })

  it('SiteProxy.hostHeader=string（""=无覆写）；CertItem 字段形态', () => {
    const p: SiteProxy = { upstream: 'http://127.0.0.1:8080', hostHeader: '', websocket: false }
    expect(p.hostHeader).toBe('')
    const c: CertItem = {
      id: 1,
      primaryDomain: 'a.com',
      domains: ['a.com'],
      issuer: '',
      source: 'acme',
      status: 'valid',
      referencedSites: [],
      createdAt: '2026-11-15T00:00:00Z'
    }
    expect(c.status).toBe('valid')
  })
})
