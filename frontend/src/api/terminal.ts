/**
 * terminal api 层 —— 2 Action 与 api-terminal.md 端点一一对应（S7，无虚构接口）：
 * userOptions（GET /terminal/user/options）+ sessionConnect（WS /terminal/session/connect）。
 * WS 帧协议（D-01 ready/input/resize/output/closed）为 sessionConnect 的消息层，不单列 Action。
 * 握手 = Cookie 会话鉴权 + Origin 同源校验（D-09）：浏览器 WS 自动携带同源 dg_session，帧层无 CSRF 面。
 * WS 封装于 api 层（frontend-rules §3：组件不裸连）；无自动重连（api 4.1.2 辨析③：重连恒为用户显式新建会话）。
 */
import type { TerminalIdentitySource } from '@/types/enums'
import type {
  TerminalClientFrame,
  TerminalServerFrame,
  TerminalUserOptionsData
} from '@/types/terminal'
import { parseServerFrame } from '@/types/terminal'
import { withBase } from '@/utils/base'
import { buildQuery, request } from './http'

/** 指定身份候选清单（登录 shell 过滤，D-02 实况直读；每次打开对话框均拉取不缓存） */
export function userOptions(): Promise<TerminalUserOptionsData> {
  return request<TerminalUserOptionsData>('/terminal/user/options')
}

/** sessionConnect 握手参数（api 4.1.4：query {identitySource, username?}） */
export interface TerminalConnectParams {
  identitySource: TerminalIdentitySource
  /** 仅 identitySource=specified_system_user 时携带 */
  username?: string
}

export interface TerminalSocketCloseInfo {
  /** ready 帧是否已收到（区分握手拒绝与会话中断两条路径） */
  readyReceived: boolean
  /** WS 关闭码（握手 HTTP 拒绝时浏览器固定给 1006） */
  code: number
  /** 服务端关闭理由（尽力而为通道：握手拒绝的 HTTP 错误体浏览器不透出，通常为空串） */
  reason: string
}

export interface TerminalSocketEvents {
  /** 合法服务端帧（ready/output/closed 已逐字段收窄） */
  onFrame(frame: TerminalServerFrame): void
  /** 非法帧（未知类型/残缺字段/坏 JSON/解码异常）——I-06 协议违规前端镜像 */
  onInvalidFrame(): void
  onClose(info: TerminalSocketCloseInfo): void
}

export interface TerminalSocket {
  /** 发送客户端帧（input/resize）；非 OPEN 态静默丢弃 */
  send(frame: TerminalClientFrame): void
  /** 客户端主动关闭（服务端走 self_close 统一清理，api 4.1.2） */
  close(): void
}

/** 同源 WS 地址（https→wss；开发态经 vite proxy /terminal ws 透传，禁止硬编码后端地址；
 *  路径同样要带应用基址——开着安全入口时不带入口前缀的连接会被拦掉） */
function terminalWsUrl(params: TerminalConnectParams): string {
  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
  const query = buildQuery({ identitySource: params.identitySource, username: params.username })
  return `${proto}://${window.location.host}${withBase('/terminal/session/connect')}${query}`
}

/**
 * sessionConnect Action：建立终端 WS 通道。传输层无业务语义——会话状态机归 stores/terminal；
 * 输出帧 data 为 Base64，解码归消费方（utils/terminal）。
 */
export function connectTerminalSession(
  params: TerminalConnectParams,
  events: TerminalSocketEvents
): TerminalSocket {
  const ws = new WebSocket(terminalWsUrl(params))
  let readyReceived = false
  let closeSent = false

  ws.onmessage = (ev: MessageEvent): void => {
    try {
      if (typeof ev.data !== 'string') {
        events.onInvalidFrame() // 协议为 JSON 文本帧（D-01），二进制帧=违规
        return
      }
      const frame = parseServerFrame(ev.data)
      if (frame === null) {
        events.onInvalidFrame()
        return
      }
      if (frame.type === 'ready') readyReceived = true
      events.onFrame(frame)
    } catch {
      events.onInvalidFrame() // Base64 解码等异常按协议违规处理
    }
  }

  ws.onclose = (ev: CloseEvent): void => {
    events.onClose({ readyReceived, code: ev.code, reason: ev.reason })
  }

  return {
    send(frame: TerminalClientFrame): void {
      if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(frame))
    },
    close(): void {
      if (!closeSent && ws.readyState === WebSocket.OPEN) {
        closeSent = true
        ws.close(1000, '')
      }
    }
  }
}
