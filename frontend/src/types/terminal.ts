/**
 * terminal 业务类型 —— 与 api-terminal.md v1.1 端点/帧协议逐字对齐（F02）。
 * 零持久化（I-04 无表无 DDL）：前端全部为运行时态；
 * WS 帧协议（D-01）= JSON 文本帧 + data Base64；ready 帧 limits 为前端权威值（R-14：前端不自行推断）。
 */
import { isTerminalEndReason, isTerminalIdentitySource } from './enums'
import type { TerminalEndReason, TerminalIdentitySource } from './enums'

/** ---- GET /terminal/user/options（api 4.1.4：登录 shell 过滤候选，name 字典序升序，全量不分页） ---- */

export interface TerminalSystemUser {
  name: string
  uid: number
  shell: string
}

export interface TerminalUserOptionsData {
  items: TerminalSystemUser[]
  generatedAt: string
}

/** ---- ready 帧 data（连接后首帧，api 4.1.4） ---- */

export interface TerminalLimits {
  maxConcurrent: number
  idleTimeoutSeconds: number
  hardLimitSeconds: number
}

export interface TerminalReadyData {
  sessionId: string
  identity: { source: TerminalIdentitySource; runUser: string }
  limits: TerminalLimits
}

/** ---- closed 帧 data（断开通知：reason/closedAt/durationSeconds，api 4.1.2 六原因） ---- */

export interface TerminalClosedData {
  reason: TerminalEndReason
  closedAt: string
  durationSeconds: number
}

/** ---- WS 帧（D-01；单帧 data 解码后 ≤16KB，I-06） ---- */

/** 客户端 → 服务端：input（data=Base64 键入字节）/ resize（cols/rows） */
export type TerminalClientFrame =
  | { type: 'input'; data: string }
  | { type: 'resize'; cols: number; rows: number }

/** 服务端 → 客户端：ready（首帧会话元信息）/ output（truncated=true=背压丢弃标记 D-04）/ closed */
export type TerminalServerFrame =
  | { type: 'ready'; data: TerminalReadyData }
  | { type: 'output'; data: string; truncated?: boolean }
  | { type: 'closed'; data: TerminalClosedData }

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null
}

/**
 * 服务端帧解析守卫（I-06 前端镜像）：非法 JSON / 未知帧类型 / 字段残缺 → null（协议违规，
 * 上层按 protocol_error 断开处理）。字段逐项收窄，不信任服务端形状。
 */
export function parseServerFrame(raw: string): TerminalServerFrame | null {
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return null
  }
  if (!isRecord(parsed)) return null
  switch (parsed.type) {
    case 'ready': {
      if (!isRecord(parsed.data)) return null
      const d = parsed.data
      if (
        typeof d.sessionId !== 'string' ||
        !isRecord(d.identity) ||
        !isRecord(d.limits)
      ) {
        return null
      }
      const { identity, limits } = d
      if (
        typeof identity.runUser !== 'string' ||
        !isTerminalIdentitySource(identity.source) ||
        typeof limits.maxConcurrent !== 'number' ||
        typeof limits.idleTimeoutSeconds !== 'number' ||
        typeof limits.hardLimitSeconds !== 'number'
      ) {
        return null
      }
      return {
        type: 'ready',
        data: {
          sessionId: d.sessionId,
          identity: { source: identity.source, runUser: identity.runUser },
          limits: {
            maxConcurrent: limits.maxConcurrent,
            idleTimeoutSeconds: limits.idleTimeoutSeconds,
            hardLimitSeconds: limits.hardLimitSeconds
          }
        }
      }
    }
    case 'output': {
      if (typeof parsed.data !== 'string') return null
      return parsed.truncated === true
        ? { type: 'output', data: parsed.data, truncated: true }
        : { type: 'output', data: parsed.data }
    }
    case 'closed': {
      if (!isRecord(parsed.data)) return null
      const d = parsed.data
      if (
        !isTerminalEndReason(d.reason) ||
        typeof d.closedAt !== 'string' ||
        typeof d.durationSeconds !== 'number'
      ) {
        return null
      }
      return { type: 'closed', data: { reason: d.reason, closedAt: d.closedAt, durationSeconds: d.durationSeconds } }
    }
    default:
      return null
  }
}

/** ---- 握手失败详情（尽力而为通道） ---- */

/**
 * 握手拒绝为升级前 HTTP 4xx/5xx + 错误码 JSON（api 3.1）——浏览器 WS API 不透出 HTTP 错误体，
 * 仅 CloseEvent.reason 为服务端可选通道。容忍解析：纯符号串 / JSON 包络 {code,data} 两种形态。
 */
export interface TerminalHandshakeFailure {
  code: string
  data: unknown
}

export function parseHandshakeFailure(reason: string): TerminalHandshakeFailure | null {
  if (reason === '') return null
  const trimmed = reason.trim()
  if (/^(TERM|AUTH)_\d{3}$/.test(trimmed)) return { code: trimmed, data: null }
  try {
    const parsed: unknown = JSON.parse(trimmed)
    if (
      isRecord(parsed) &&
      typeof parsed.code === 'string' &&
      /^(TERM|AUTH)_\d{3}$/.test(parsed.code)
    ) {
      return { code: parsed.code, data: parsed.data ?? null }
    }
  } catch {
    // 非 JSON 形态：无法识别
  }
  return null
}

/** 握手失败 toast 文案（PageSpec 第 1/5 节逐字；TERM_007 走对话框 NAlert 非 toast） */
export function terminalErrorText(code: string, data: unknown): string {
  if (code === 'TERM_001') {
    if (isRecord(data) && typeof data.limit === 'number' && typeof data.current === 'number') {
      return `已达并发上限（${data.current}/${data.limit}），请先关闭其他终端会话`
    }
    return '已达并发上限，请先关闭其他终端会话'
  }
  switch (code) {
    case 'TERM_002':
      return '指定的系统用户不存在'
    case 'TERM_003':
      return '指定的系统用户无可登录 shell'
    case 'TERM_004':
      return '终端进程创建失败，请稍后重试'
    case 'TERM_005':
      return '参数校验失败：请检查身份来源与系统用户选择'
    case 'TERM_006':
      return '审计服务暂不可用，会话未建立'
    case 'AUTH_003':
      return '终端连接被拒绝：非同源请求'
    case 'AUTH_008':
      return '权限不足：只读角色禁止使用 Web 终端'
    default:
      return '终端连接失败：连接被拒绝或网络不可达'
  }
}

/** TERM_007 候选清单失败文案（对话框 NAlert + 重试；不显示虚构数据，api 3.4） */
export const TERM_007_TEXT = '系统用户实况读取失败，无法提供候选清单（不显示虚构数据）'
