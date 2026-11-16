import { describe, expect, it } from 'vitest'
import {
  parseHandshakeFailure,
  parseServerFrame,
  terminalErrorText
} from '@/types/terminal'
import { base64ToBytes, bytesToBase64 } from '@/utils/terminal'

describe('terminal base64 编解码（D-01 帧 data 通道）', () => {
  it('控制字符/ANSI 转义/UTF-8 多字节/emoji 字节往返一致', () => {
    const src = new Uint8Array([
      0x1b, 0x5b, 0x41, 0x0d, 0x0a, 0x7f, 0xe6, 0x96, 0x87, 0xf0, 0x9f, 0x8c, 0x80
    ])
    expect(base64ToBytes(bytesToBase64(src))).toEqual(src)
  })

  it('UTF-8 文本 TextEncoder→Base64→解码 roundtrip', () => {
    const text = 'ls -la /var/log 中文 ✓'
    const encoded = bytesToBase64(new TextEncoder().encode(text))
    expect(new TextDecoder().decode(base64ToBytes(encoded))).toBe(text)
  })

  it('空字节流往返为空串', () => {
    expect(bytesToBase64(new Uint8Array(0))).toBe('')
    expect(base64ToBytes('').length).toBe(0)
  })
})

describe('parseServerFrame（I-06 前端镜像：逐字段收窄）', () => {
  const readyFrame = JSON.stringify({
    type: 'ready',
    data: {
      sessionId: 'a1b2c3d4e5f6a7b8',
      identity: { source: 'specified_system_user', runUser: 'www-data' },
      limits: { maxConcurrent: 5, idleTimeoutSeconds: 1800, hardLimitSeconds: 28800 }
    }
  })

  it('ready 帧合法解出（limits 权威值字段一致）', () => {
    const frame = parseServerFrame(readyFrame)
    expect(frame).not.toBeNull()
    if (frame?.type === 'ready') {
      expect(frame.data.sessionId).toBe('a1b2c3d4e5f6a7b8')
      expect(frame.data.identity.runUser).toBe('www-data')
      expect(frame.data.limits.maxConcurrent).toBe(5)
    } else {
      throw new Error('frame type mismatch')
    }
  })

  it('ready 帧 limits 缺字段 → null（协议违规）', () => {
    const broken = JSON.stringify({
      type: 'ready',
      data: { sessionId: 'x', identity: { source: 'panel_run_user', runUser: 'root' }, limits: { maxConcurrent: 5 } }
    })
    expect(parseServerFrame(broken)).toBeNull()
  })

  it('output 帧 truncated 标记保留/缺省不携带', () => {
    const truncated = parseServerFrame(JSON.stringify({ type: 'output', data: 'aGk=', truncated: true }))
    const plain = parseServerFrame(JSON.stringify({ type: 'output', data: 'aGk=' }))
    expect(truncated).toEqual({ type: 'output', data: 'aGk=', truncated: true })
    if (plain?.type === 'output') {
      expect(plain.truncated).toBeUndefined()
    } else {
      throw new Error('frame type mismatch')
    }
  })

  it('closed 帧 6 原因合法值解出、未知 reason → null', () => {
    const closed = parseServerFrame(
      JSON.stringify({ type: 'closed', data: { reason: 'idle_timeout', closedAt: '2026-11-15T08:00:00Z', durationSeconds: 1812 } })
    )
    expect(closed).toEqual({
      type: 'closed',
      data: { reason: 'idle_timeout', closedAt: '2026-11-15T08:00:00Z', durationSeconds: 1812 }
    })
    expect(
      parseServerFrame(
        JSON.stringify({ type: 'closed', data: { reason: 'mystery', closedAt: 'x', durationSeconds: 1 } })
      )
    ).toBeNull()
  })

  it('未知帧类型/坏 JSON → null', () => {
    expect(parseServerFrame(JSON.stringify({ type: 'ping' }))).toBeNull()
    expect(parseServerFrame('not json')).toBeNull()
  })
})

describe('parseHandshakeFailure（握手拒绝详情容忍解析）', () => {
  it('空 reason（浏览器不透出 HTTP 错误体的常态）→ null', () => {
    expect(parseHandshakeFailure('')).toBeNull()
  })

  it('纯符号串形态解出', () => {
    expect(parseHandshakeFailure('TERM_001')).toEqual({ code: 'TERM_001', data: null })
    expect(parseHandshakeFailure(' AUTH_008 ')).toEqual({ code: 'AUTH_008', data: null })
  })

  it('JSON 包络形态解出（含 data）', () => {
    const parsed = parseHandshakeFailure('{"code":"TERM_001","message":"limit","data":{"limit":5,"current":5}}')
    expect(parsed?.code).toBe('TERM_001')
    expect(parsed?.data).toEqual({ limit: 5, current: 5 })
  })

  it('不可识别文本 → null', () => {
    expect(parseHandshakeFailure('upstream error')).toBeNull()
  })
})

describe('terminalErrorText（PageSpec 第 1/5 节文案）', () => {
  it('TERM_001 带 data 格式化 current/limit', () => {
    expect(terminalErrorText('TERM_001', { limit: 5, current: 5 })).toBe(
      '已达并发上限（5/5），请先关闭其他终端会话'
    )
  })

  it('TERM_001 无 data 降级通用文案', () => {
    expect(terminalErrorText('TERM_001', null)).toBe('已达并发上限，请先关闭其他终端会话')
  })

  it('各码文案与 AUTH_003/008 逐字', () => {
    expect(terminalErrorText('TERM_004', null)).toBe('终端进程创建失败，请稍后重试')
    expect(terminalErrorText('TERM_006', null)).toBe('审计服务暂不可用，会话未建立')
    expect(terminalErrorText('AUTH_003', null)).toBe('终端连接被拒绝：非同源请求')
    expect(terminalErrorText('AUTH_008', null)).toBe('权限不足：只读角色禁止使用 Web 终端')
  })

  it('未知码兜底', () => {
    expect(terminalErrorText('UNKNOWN', null)).toBe('终端连接失败：连接被拒绝或网络不可达')
  })
})
