/**
 * scanSitePaths —— 找出「面板自己的地址没带应用基址」的写法。
 *
 * 开着安全入口时，面板的页面、脚本、接口都在 /{entryPath}/ 之下。地址写成根路径（'/auth/me'、
 * src="/favicon.svg"）的请求不带前缀，被入口网关拦掉。入口路径不是默认开启的设置，这种漏点
 * 平时用不出来，要到开着安全入口的那台机器上才暴露成一片空白。面板路径一律经 utils/base 的
 * withBase()；真正发请求的地方只有固定的几处。
 *
 * 规则（只看代码，不看注释）：
 *   1  fetch( 的第一个参数是根路径字面量                 fetch('/auth/me')
 *   2  fetch( 只允许出现在白名单文件里                   api/http.ts、api/auth.ts、router/navFailure.ts
 *   3  用「协议 + 主机」拼同源完整地址时没有经 withBase(   `${proto}://${location.host}/x`
 *   4  new WebSocket( 只允许出现在白名单文件里           api/terminal.ts、api/docker.ts
 *   5  模板里的 src / href / action 写了根路径           <img src="/favicon.svg">
 *   6  整页跳转写了根路径字面量                         window.location.assign('/settings')
 */
import { matchParen } from './scanConfirmTrigger'

export type SitePathKind =
  | 'fetch-root-literal'
  | 'fetch-outside-allowlist'
  | 'same-origin-url-without-base'
  | 'websocket-outside-allowlist'
  | 'template-root-path'
  | 'location-root-literal'

export interface SitePathFinding {
  file: string
  line: number
  kind: SitePathKind
  /** 源码片段 */
  snippet: string
}

export interface SitePathScan {
  findings: SitePathFinding[]
  /** 识别出的 fetch( / new WebSocket( 调用数——用来确认扫描器没有失明 */
  counts: { fetch: number; websocket: number }
}

/** 允许直接发请求的文件（相对 src/，统一用 / 分隔） */
export const FETCH_FILES: ReadonlySet<string> = new Set([
  'api/http.ts',
  'api/auth.ts',
  'router/navFailure.ts'
])
export const WEBSOCKET_FILES: ReadonlySet<string> = new Set(['api/terminal.ts', 'api/docker.ts'])

const lineAt = (src: string, index: number): number => src.slice(0, index).split('\n').length

/** 该行里第一个非空片段 */
function snippetAt(src: string, index: number): string {
  return (src.slice(src.lastIndexOf('\n', index - 1) + 1, src.indexOf('\n', index) < 0 ? undefined : src.indexOf('\n', index)).trim().slice(0, 100))
}

/** 把注释换成等长的空白（行号不变）。字符串里的 // 与 /* 不算注释。 */
export function stripComments(src: string): string {
  const out = src.split('')
  const blank = (from: number, to: number): void => {
    for (let i = from; i < to; i++) {
      if (out[i] !== '\n') out[i] = ' '
    }
  }
  let i = 0
  while (i < src.length) {
    const c = src[i]
    if (c === '"' || c === "'" || c === '`') {
      i++
      while (i < src.length && src[i] !== c) {
        i += src[i] === '\\' ? 2 : 1
      }
      i++
      continue
    }
    if (c === '/' && src[i + 1] === '/') {
      const end = src.indexOf('\n', i)
      blank(i, end < 0 ? src.length : end)
      i = end < 0 ? src.length : end
      continue
    }
    if (c === '/' && src[i + 1] === '*') {
      const end = src.indexOf('*/', i + 2)
      const stop = end < 0 ? src.length : end + 2
      blank(i, stop)
      i = stop
      continue
    }
    i++
  }
  return out.join('')
}

/** 取出 call( … ) 括号里的第一个参数源码；括号不配对时返回 null */
function firstArgument(code: string, openIdx: number): string | null {
  const close = matchParen(code, openIdx)
  if (close < 0) return null
  const args = code.slice(openIdx + 1, close)
  let depth = 0
  for (let i = 0; i < args.length; i++) {
    const c = args[i]
    if (c === '(' || c === '[' || c === '{') depth++
    else if (c === ')' || c === ']' || c === '}') depth--
    else if (c === ',' && depth === 0) return args.slice(0, i)
  }
  return args
}

const isRootLiteral = (arg: string): boolean => /^\s*['"`]\//.test(arg)

/** .vue 文件顶层 <template>…</template> 的范围；没有返回 null */
function templateRange(src: string): { start: number; end: number } | null {
  const open = /^<template>\s*$/m.exec(src)
  if (open === null) return null
  const close = /^<\/template>/m.exec(src.slice(open.index + open[0].length))
  if (close === null) return null
  return { start: open.index, end: open.index + open[0].length + close.index + close[0].length }
}

export function scanSource(raw: string, file: string): SitePathScan {
  const findings: SitePathFinding[] = []
  const rel = file.split('\\').join('/').replace(/^src\//, '')
  const isVue = rel.endsWith('.vue')
  const tpl = isVue ? templateRange(raw) : null
  // 代码部分：注释换成空白；.vue 的模板部分不参与（模板另有一套规则）
  const code = stripComments(tpl === null ? raw : raw.slice(0, tpl.start) + ' '.repeat(tpl.end - tpl.start) + raw.slice(tpl.end))
  const add = (kind: SitePathKind, index: number): void => {
    findings.push({ file: rel, line: lineAt(code, index), kind, snippet: snippetAt(code, index) })
  }

  for (const m of code.matchAll(/\bfetch\(/g)) {
    const arg = firstArgument(code, m.index + 'fetch'.length)
    if (arg !== null && isRootLiteral(arg)) add('fetch-root-literal', m.index)
    if (!FETCH_FILES.has(rel)) add('fetch-outside-allowlist', m.index)
  }
  // 同源完整地址（WebSocket 等）：主机后面接的路径必须经 withBase()
  for (const m of code.matchAll(/`(?:[^`\\]|\\.)*`/g)) {
    if (/location\.(?:host|origin)\}/.test(m[0]) && !m[0].includes('withBase(')) {
      add('same-origin-url-without-base', m.index)
    }
  }
  for (const m of code.matchAll(/\bnew\s+WebSocket\(/g)) {
    if (!WEBSOCKET_FILES.has(rel)) add('websocket-outside-allowlist', m.index)
  }
  for (const m of code.matchAll(/\blocation\.(?:assign|replace)\(\s*['"`]\//g)) {
    add('location-root-literal', m.index)
  }

  let template = ''
  if (tpl !== null) {
    // 模板里的 HTML 注释不算
    template = raw.slice(tpl.start, tpl.end).replace(/<!--[\s\S]*?-->/g, (c) => c.replace(/[^\n]/g, ' '))
    for (const m of template.matchAll(/(?:^|[\s<])(?:src|href|action)\s*=\s*"\//g)) {
      findings.push({
        file: rel,
        line: lineAt(raw, tpl.start + m.index),
        kind: 'template-root-path',
        snippet: snippetAt(raw, tpl.start + m.index)
      })
    }
  }

  return {
    findings,
    counts: {
      fetch: [...code.matchAll(/\bfetch\(/g)].length,
      websocket: [...code.matchAll(/\bnew\s+WebSocket\(/g)].length
    }
  }
}
