/**
 * 开着安全入口时，面板发出的每个请求都要带入口路径前缀（api D-P03）——不带前缀的请求会被
 * 入口网关拦掉，页面就是一片空白。这里核对所有真正发出请求的位置：集中请求出口的 5 个函数、
 * 审计日志导出、终端和日志跟随的两个 WebSocket。用替身接住 fetch / WebSocket，只看它们收到的地址。
 */
import { afterEach, describe, expect, it, vi } from 'vitest'
import { exportAuditLog } from '../auth'
import { connectComposeLogsFollow, connectContainerLogsFollow } from '../docker'
import { downloadFile, postDownload, postForm, request, requestText } from '../http'
import { connectTerminalSession } from '../terminal'

const ENTRY = '/abc12345/'

/** 记录地址的 WebSocket 替身（握手后不触发任何事件） */
class FakeWebSocket {
  static OPEN = 1
  static urls: string[] = []
  readyState = 0
  onmessage: unknown = null
  onclose: unknown = null
  constructor(url: string | URL) {
    FakeWebSocket.urls.push(String(url))
  }
  send(): void {}
  close(): void {}
}

interface FakeBrowser {
  /** fetch 收到的地址，按先后 */
  urls: () => string[]
  /** 页面上被点击的下载链接地址 */
  clicked: () => string[]
}

/** 模拟浏览器环境：页面里的 <base>、fetch、WebSocket、地址栏（127.0.0.1:18697，http） */
function fakeBrowser(baseHref: string | null): FakeBrowser {
  const clicked: string[] = []
  const fetchMock = vi.fn(async (_url: string, init?: RequestInit) => {
    const accept = String((init?.headers as Record<string, string> | undefined)?.Accept ?? '')
    if (accept.includes('application/octet-stream')) {
      return new Response('file-bytes', { status: 200, headers: { 'Content-Type': 'application/octet-stream' } })
    }
    if (accept.includes('text/plain')) {
      return new Response('log line', { status: 200, headers: { 'Content-Type': 'text/plain' } })
    }
    if (accept.includes('text/csv')) {
      return new Response('a,b', { status: 200, headers: { 'Content-Type': 'text/csv' } })
    }
    return new Response(JSON.stringify({ code: 0, message: '', data: { ok: true } }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' }
    })
  })
  vi.stubGlobal('fetch', fetchMock)
  vi.stubGlobal('WebSocket', FakeWebSocket)
  FakeWebSocket.urls = []
  vi.stubGlobal('window', { location: { protocol: 'http:', host: '127.0.0.1:18697' } })
  vi.stubGlobal('document', {
    querySelector: (selector: string) =>
      selector === 'base[href]' && baseHref !== null ? { getAttribute: () => baseHref } : null,
    createElement: () => ({ href: '', download: '', click: () => clicked.push('clicked') })
  })
  return { urls: () => fetchMock.mock.calls.map((call) => String(call[0])), clicked: () => clicked }
}

/** 集中请求出口的 5 个函数各发一次（请求体形态不同：JSON、表单、文件流、纯文本） */
async function callEveryRequestHelper(): Promise<void> {
  await request('/auth/session/current')
  await request('/docker/container/list', { method: 'POST', body: { state: 'running' } })
  await postForm('/files/entry/upload', new FormData())
  await downloadFile('/files/entry/download', { path: 'a.txt' })
  await postDownload('/db/backup/export', { instanceId: 1, database: 'shop' })
  await requestText('/cron/log/tail', { lines: 50 })
}

/** 上面 6 次请求不带前缀时的地址 */
const ROOT_URLS = [
  '/auth/session/current',
  '/docker/container/list',
  '/files/entry/upload',
  '/files/entry/download?path=a.txt',
  '/db/backup/export',
  '/cron/log/tail?lines=50'
]

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('集中请求出口：发出的地址带应用基址', () => {
  it('安全入口开启：5 个函数发出的地址都带入口路径', async () => {
    const browser = fakeBrowser(ENTRY)
    await callEveryRequestHelper()
    expect(browser.urls()).toEqual(ROOT_URLS.map((url) => ENTRY.slice(0, -1) + url))
  })

  it('安全入口没开启（<base href="/">）：地址和以前一样', async () => {
    const browser = fakeBrowser('/')
    await callEveryRequestHelper()
    expect(browser.urls()).toEqual(ROOT_URLS)
  })

  it('页面里没有 <base>（开发服务器）：地址和以前一样', async () => {
    const browser = fakeBrowser(null)
    await callEveryRequestHelper()
    expect(browser.urls()).toEqual(ROOT_URLS)
  })
})

describe('审计日志导出：不经集中请求出口，地址同样要带前缀', () => {
  it('安全入口开启', async () => {
    const browser = fakeBrowser(ENTRY)
    await exportAuditLog({ page: 1, pageSize: 20 } as never)
    expect(browser.urls()).toEqual([`${ENTRY}auth/audit-log/export?page=1&pageSize=20`])
    expect(browser.clicked()).toEqual(['clicked'])
  })

  it('安全入口没开启：地址和以前一样', async () => {
    const browser = fakeBrowser('/')
    await exportAuditLog({ page: 1, pageSize: 20 } as never)
    expect(browser.urls()).toEqual(['/auth/audit-log/export?page=1&pageSize=20'])
  })
})

describe('WebSocket：连接地址带应用基址', () => {
  const noEvents = {
    onFrame: () => undefined,
    onInvalidFrame: () => undefined,
    onClose: () => undefined
  }
  const noLogEvents = {
    onLog: () => undefined,
    onEnd: () => undefined,
    onInvalidFrame: () => undefined,
    onClose: () => undefined
  }

  it('安全入口开启：终端和日志跟随都连到入口路径之下', () => {
    fakeBrowser(ENTRY)
    connectTerminalSession({ identitySource: 'panel' as never }, noEvents).close()
    connectContainerLogsFollow({ containerId: 'c1', tail: 100 }, noLogEvents).close()
    connectComposeLogsFollow({ project: 'shop' }, noLogEvents).close()
    expect(FakeWebSocket.urls).toEqual([
      `ws://127.0.0.1:18697${ENTRY}terminal/session/connect?identitySource=panel`,
      `ws://127.0.0.1:18697${ENTRY}docker/container/logs-follow?containerId=c1&tail=100`,
      `ws://127.0.0.1:18697${ENTRY}docker/compose/logs-follow?project=shop`
    ])
  })

  it('安全入口没开启：地址和以前一样', () => {
    fakeBrowser('/')
    connectTerminalSession({ identitySource: 'panel' as never }, noEvents).close()
    connectContainerLogsFollow({ containerId: 'c1', tail: 100 }, noLogEvents).close()
    connectComposeLogsFollow({ project: 'shop' }, noLogEvents).close()
    expect(FakeWebSocket.urls).toEqual([
      'ws://127.0.0.1:18697/terminal/session/connect?identitySource=panel',
      'ws://127.0.0.1:18697/docker/container/logs-follow?containerId=c1&tail=100',
      'ws://127.0.0.1:18697/docker/compose/logs-follow?project=shop'
    ])
  })
})
