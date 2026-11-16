/**
 * terminalStore —— PageSpec 第 1 节：tabs[]（sessionId/runUser/limits/status）+ userOptions +
 * connecting 标志。会话数据为 WS 帧流（D-01）：无轮询、无 HTTP 列表；output 帧经 writer 注册表
 * 直写对应标签的 xterm 实例。
 * WS 连接对象与 writer 注册表为非响应式运行时资源（模块级 Map），不入 Pinia state。
 * 刷新纪律（PageSpec 第 2 节）：userOptions 开框即拉不缓存（D-02）；closed 帧到达即转 closed、
 * 不自动重连（api 4.1.2 辨析③）；页面卸载 disposeAll 尽力 close（D-03，前端不承担一致性责任）。
 */
import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { ApiError } from '@/api/http'
import { connectTerminalSession, userOptions } from '@/api/terminal'
import type { TerminalSocket } from '@/api/terminal'
import type { TerminalIdentitySource } from '@/types/enums'
import type {
  TerminalClosedData,
  TerminalLimits,
  TerminalUserOptionsData
} from '@/types/terminal'
import { parseHandshakeFailure } from '@/types/terminal'
import { base64ToBytes, bytesToBase64 } from '@/utils/terminal'

/** 并发上限前端预置值（ADR-023 标准档 5）；ready 帧 limits.maxConcurrent 到达后以权威值覆盖 */
const PRESET_MAX_CONCURRENT = 5

const encoder = new TextEncoder()

export type TerminalTabStatus = 'connecting' | 'connected' | 'closed'

export interface TerminalTab {
  /** 标签本地标识（与 sessionId 区分：closed 后 sessionId 仅为展示） */
  id: string
  status: TerminalTabStatus
  /** 建立参数（身份来源/指定用户；重连=重走新建流程不继承，仅留存展示） */
  identitySource: TerminalIdentitySource
  username: string | null
  sessionId: string | null
  runUser: string | null
  limits: TerminalLimits | null
  closeInfo: TerminalClosedData | null
  /** 本地建立时刻（ready 前断开/无 closed 帧时 durationSeconds 本地推算用） */
  createdAt: number
}

/** 握手失败事实（PageSpec 第 5 节流程表；页面 watch 后按码分流提示——store 无 Message 上下文） */
export interface TerminalHandshakeFailureState {
  code: string
  data: unknown
  at: number
}

/** output 帧 → 对应标签 xterm 的直写通道（truncated=背压丢弃标记 D-04，由呈现层写提示行） */
export type TerminalOutputWriter = (bytes: Uint8Array, truncated: boolean) => void

let seq = 0
/** 运行时资源（非响应式）：tabId → WS 通道 / 输出 writer */
const sockets = new Map<string, TerminalSocket>()
const writers = new Map<string, TerminalOutputWriter>()

function localClosed(reason: TerminalClosedData['reason'], createdAt: number): TerminalClosedData {
  return {
    reason,
    closedAt: new Date().toISOString(),
    durationSeconds: Math.max(0, Math.floor((Date.now() - createdAt) / 1000))
  }
}

export const useTerminalStore = defineStore('terminal', () => {
  const tabs = ref<TerminalTab[]>([])
  const activeTabId = ref<string | null>(null)
  /** 新建会话对话框（PageSpec 第 5 节；TERM_002/003 失败重开与「重新连接」共用入口） */
  const dialogVisible = ref(false)
  const userOptionsData = ref<TerminalUserOptionsData | null>(null)
  const userOptionsLoading = ref(false)
  /** 候选清单失败码（'TERM_007' | 其他 ApiError code | 'NETWORK'）——对话框 NAlert + 重试 */
  const userOptionsError = ref<string | null>(null)
  const handshakeFailure = ref<TerminalHandshakeFailureState | null>(null)
  const concurrentLimit = ref(PRESET_MAX_CONCURRENT)

  /** 会话建立中标志（任一标签处于 connecting） */
  const connecting = computed(() => tabs.value.some((t) => t.status === 'connecting'))
  /** 活跃标签数（closed 不占；「+」达上限判定，ADR-023/D1 落定：多标签上限=并发阈值） */
  const activeCount = computed(() => tabs.value.filter((t) => t.status !== 'closed').length)

  function findTab(id: string): TerminalTab | undefined {
    return tabs.value.find((t) => t.id === id)
  }

  /** userOptions 拉取（D-02 实况直读；TERM_007 不虚构数据——data 保持 null） */
  async function loadUserOptions(): Promise<void> {
    userOptionsLoading.value = true
    userOptionsError.value = null
    try {
      userOptionsData.value = await userOptions()
    } catch (e) {
      userOptionsError.value = e instanceof ApiError ? e.code : 'NETWORK'
    } finally {
      userOptionsLoading.value = false
    }
  }

  /** 打开新建会话对话框（每次打开均重拉候选，不缓存——PageSpec 第 2 节刷新纪律） */
  function openConnectDialog(): void {
    dialogVisible.value = true
    void loadUserOptions()
  }

  function closeConnectDialog(): void {
    dialogVisible.value = false
  }

  function markClosed(id: string, info: TerminalClosedData): void {
    const tab = findTab(id)
    if (tab === undefined || tab.status === 'closed') return
    tab.status = 'closed'
    tab.closeInfo = info
  }

  /** WS 关闭分流：ready 前=握手拒绝；ready 后且无 closed 帧=网络层异常（network_error 语义收口） */
  function handleSocketClose(id: string, readyReceived: boolean, reason: string, createdAt: number): void {
    const tab = findTab(id)
    if (tab === undefined) return
    if (!readyReceived) {
      // 握手拒绝（HTTP 4xx/5xx，api 3.1）——移除「连接中…」标签（TERM_004/006 不占并发配额，I-02 对称回收）；
      // 浏览器 WS API 不透出错误体，reason 为服务端可选通道（容忍解析，未识别走通用文案）
      removeTab(id)
      const parsed = parseHandshakeFailure(reason)
      handshakeFailure.value = { code: parsed?.code ?? 'UNKNOWN', data: parsed?.data ?? null, at: Date.now() }
      return
    }
    if (tab.status !== 'closed') {
      markClosed(id, localClosed('network_error', createdAt))
    }
  }

  /**
   * 新建会话（PageSpec 第 5 节连接流程）：关对话框 → 新建「连接中…」标签 → 发起 WS；
   * ready 帧到达定稿标题/状态条（api 4.1.4 权威值）。
   */
  function createSession(identitySource: TerminalIdentitySource, username: string): void {
    seq += 1
    const id = `term-${seq}-${Date.now().toString(36)}`
    const createdAt = Date.now()
    tabs.value.push({
      id,
      status: 'connecting',
      identitySource,
      username: username === '' ? null : username,
      sessionId: null,
      runUser: null,
      limits: null,
      closeInfo: null,
      createdAt
    })
    activeTabId.value = id
    dialogVisible.value = false
    const socket = connectTerminalSession(
      { identitySource, username: username === '' ? undefined : username },
      {
        onFrame(frame) {
          const tab = findTab(id)
          if (tab === undefined) return
          if (frame.type === 'ready') {
            tab.status = 'connected'
            tab.sessionId = frame.data.sessionId
            tab.runUser = frame.data.identity.runUser
            tab.limits = frame.data.limits
            concurrentLimit.value = frame.data.limits.maxConcurrent
          } else if (frame.type === 'output') {
            writers.get(id)?.(base64ToBytes(frame.data), frame.truncated === true)
          } else {
            markClosed(id, frame.data)
          }
        },
        onInvalidFrame() {
          // I-06 前端镜像：协议违规 → 本地按 protocol_error 断开（服务端同理会回收）
          markClosed(id, localClosed('protocol_error', createdAt))
          sockets.get(id)?.close()
        },
        onClose(info) {
          handleSocketClose(id, info.readyReceived, info.reason, createdAt)
        }
      }
    )
    sockets.set(id, socket)
  }

  /** 关闭标签：连接中/已连接=断开会话（WS close → 服务端 self_close 清理）；closed=纯前端移除 */
  function removeTab(id: string): void {
    sockets.get(id)?.close()
    sockets.delete(id)
    writers.delete(id)
    const idx = tabs.value.findIndex((t) => t.id === id)
    if (idx >= 0) tabs.value.splice(idx, 1)
    if (activeTabId.value === id) {
      activeTabId.value = tabs.value.length > 0 ? tabs.value[tabs.value.length - 1].id : null
    }
  }

  /** SessionPane 挂载时注册 output 直写通道 */
  function attachTerminal(id: string, write: TerminalOutputWriter): void {
    writers.set(id, write)
  }

  function detachTerminal(id: string): void {
    writers.delete(id)
  }

  /** 键入（流式无需回车确认）：xterm onData → input 帧（data=Base64，I-06 ≤16KB 天然细粒度） */
  function sendInput(id: string, data: string): void {
    const tab = findTab(id)
    if (tab === undefined || tab.status !== 'connected') return
    sockets.get(id)?.send({ type: 'input', data: bytesToBase64(encoder.encode(data)) })
  }

  /** 尺寸同步：xterm onResize → resize 帧（cols/rows；两帧均更新服务端 lastActiveAt） */
  function sendResize(id: string, cols: number, rows: number): void {
    const tab = findTab(id)
    if (tab === undefined || tab.status !== 'connected') return
    sockets.get(id)?.send({ type: 'resize', cols, rows })
  }

  /** 页面卸载/路由离开：全部活跃 WS 尽力 close（孤儿由服务端心跳/空闲兜底回收，api D-03/I-02） */
  function disposeAll(): void {
    for (const socket of sockets.values()) socket.close()
    sockets.clear()
    writers.clear()
    tabs.value = []
    activeTabId.value = null
    dialogVisible.value = false
  }

  return {
    tabs,
    activeTabId,
    dialogVisible,
    userOptionsData,
    userOptionsLoading,
    userOptionsError,
    handshakeFailure,
    concurrentLimit,
    connecting,
    activeCount,
    loadUserOptions,
    openConnectDialog,
    closeConnectDialog,
    createSession,
    removeTab,
    attachTerminal,
    detachTerminal,
    sendInput,
    sendResize,
    disposeAll
  }
})
