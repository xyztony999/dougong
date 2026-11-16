import { describe, expect, it } from 'vitest'
import { normalizeCode } from '../errcode'

describe('normalizeCode（E1 错误码通道归一）', () => {
  it('契约符号串原样透传', () => {
    expect(normalizeCode('AUTH_001')).toBe('AUTH_001')
    expect(normalizeCode('AUTH_004')).toBe('AUTH_004')
    expect(normalizeCode('AUTH_007')).toBe('AUTH_007')
    expect(normalizeCode('AUTH_014')).toBe('AUTH_014')
  })

  it('后端数值 401NN 归一为契约符号 AUTH_0NN', () => {
    expect(normalizeCode(40101)).toBe('AUTH_001')
    expect(normalizeCode(40102)).toBe('AUTH_002')
    expect(normalizeCode(40104)).toBe('AUTH_004')
    expect(normalizeCode(40107)).toBe('AUTH_007')
    expect(normalizeCode(40111)).toBe('AUTH_011')
    expect(normalizeCode(40112)).toBe('AUTH_012')
    expect(normalizeCode(40113)).toBe('AUTH_013')
    expect(normalizeCode(40114)).toBe('AUTH_014')
  })

  it('跨模块公共码（backend errcode）映射', () => {
    expect(normalizeCode(40400)).toBe('NOT_FOUND')
    expect(normalizeCode(50000)).toBe('INTERNAL')
  })

  it('未知/非整数数值不误映射，按字符串透传', () => {
    expect(normalizeCode(49999)).toBe('49999')
    expect(normalizeCode(40100)).toBe('40100')
    expect(normalizeCode(40115)).toBe('40115')
    expect(normalizeCode(40101.5)).toBe('40101.5')
  })
})
