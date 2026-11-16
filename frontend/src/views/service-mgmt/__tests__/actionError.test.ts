/**
 * 动作错误分流回归测试（Gate 2 W1 修复轮 2026-11-16——handleActionError 八分支
 * 从 index.vue 抽取为 routeActionError 纯函数后的针对性用例；行为与抽取前一致）：
 * 非 ApiError / SVC_007 两形态（fail-shaped 与形态不符）/ SVC_006 / SVC_003 /
 * SVC_008 / SVC_001·004·005 参数非法兜底 / 未知码静默。
 */
import { describe, expect, it, vi } from 'vitest'

// http.ts 模块级 createDiscreteApi 依赖 DOM——以最小 ApiError 假体替换（仅用 instanceof + is()，file.test.ts 先例）
vi.mock('@/api/http', () => {
  class ApiError extends Error {
    readonly code: string
    readonly data: unknown
    constructor(code: string, message: string, data: unknown) {
      super(message)
      this.name = 'ApiError'
      this.code = code
      this.data = data
    }
    is(code: string): boolean {
      return this.code === code
    }
  }
  return { ApiError }
})

import { ApiError } from '@/api/http'
import { isFailResult, routeActionError } from '@/views/service-mgmt/actionError'
import type { ServiceUnitFailResult } from '@/types/service'

describe('routeActionError · 八分支分流', () => {
  it('非 ApiError（网络外的普通异常/undefined）→ silent', () => {
    expect(routeActionError(new Error('boom'))).toEqual({ kind: 'silent' })
    expect(routeActionError(undefined)).toEqual({ kind: 'silent' })
  })

  it('SVC_007 + fail 形态 data → fail-result（透传原对象）', () => {
    const data: ServiceUnitFailResult = { name: 'nginx.service', reason: 'Job failed', activeState: 'failed' }
    const route = routeActionError(new ApiError('SVC_007', '操作失败', data))
    expect(route).toEqual({ kind: 'fail-result', data })
  })

  it('SVC_007 + activeState=null（调用期拒绝无终态，I1）→ 仍 fail-result（Modal 展示「—」）', () => {
    const data: ServiceUnitFailResult = { name: 'nginx.service', reason: 'rejected', activeState: null }
    expect(routeActionError(new ApiError('SVC_007', '操作失败', data))).toEqual({
      kind: 'fail-result',
      data
    })
  })

  it('SVC_007 + data 形态不符（null/缺字段）→ silent 不弹 Modal', () => {
    expect(routeActionError(new ApiError('SVC_007', '操作失败', null))).toEqual({ kind: 'silent' })
    expect(
      routeActionError(new ApiError('SVC_007', '操作失败', { name: 'a.service', activeState: 'failed' }))
    ).toEqual({ kind: 'silent' })
  })

  it('SVC_006（90s 超时上限）→ warn + 终态确认文案', () => {
    expect(routeActionError(new ApiError('SVC_006', 'timeout', { name: 'a.service', note: 'x' }))).toEqual({
      kind: 'warn',
      text: '操作仍在执行，请稍后刷新列表确认终态'
    })
  })

  it('SVC_003（unit 已消失）→ warn-reload + 提示后列表重拉（api I-03）', () => {
    expect(routeActionError(new ApiError('SVC_003', 'gone', null))).toEqual({
      kind: 'warn-reload',
      text: '该服务已不存在（可能已被卸载）'
    })
  })

  it('SVC_008（dbus 通道不可用）→ unavailable 页面横幅（I-01）', () => {
    expect(routeActionError(new ApiError('SVC_008', 'unavailable', null))).toEqual({
      kind: 'unavailable'
    })
  })

  it('SVC_001/002/004/005 参数非法兜底 → error +「参数非法」（正常 UI 不可达）', () => {
    for (const code of ['SVC_001', 'SVC_002', 'SVC_004', 'SVC_005']) {
      expect(routeActionError(new ApiError(code, 'bad param', null))).toEqual({
        kind: 'error',
        text: '参数非法'
      })
    }
  })

  it('未知码（如 AUTH_008，拦截器已提示）→ silent 不重复提示', () => {
    expect(routeActionError(new ApiError('AUTH_008', 'forbidden', null))).toEqual({ kind: 'silent' })
  })
})

describe('isFailResult · SVC_007 data 收窄（ApiError.data 为 unknown）', () => {
  it('完整形态（含 activeState=null）→ true', () => {
    expect(isFailResult({ name: 'a.service', reason: 'r', activeState: 'inactive' })).toBe(true)
    expect(isFailResult({ name: 'a.service', reason: 'r', activeState: null })).toBe(true)
  })

  it('null / 缺字段 / 字段类型不符 → false', () => {
    expect(isFailResult(null)).toBe(false)
    expect(isFailResult({ name: 1, reason: 'r', activeState: 'failed' })).toBe(false)
    expect(isFailResult({ name: 'a.service', reason: 'r', activeState: 3 })).toBe(false)
    expect(isFailResult({ name: 'a.service', activeState: 'failed' })).toBe(false)
  })
})
