/**
 * 面板自己的地址都要带应用基址 —— 全仓源码扫描（vitest）。
 *
 * 背景：安全入口开启后面板的一切都在 /{entryPath}/ 之下，写成根路径的请求（'/auth/me'、
 * src="/favicon.svg'）不带前缀，被入口网关拦掉——面板在浏览器里一片空白。修复前整个前端
 * 都是这样写的，而当时的验收只用接口请求核对了网关本身。入口路径不是默认开启的设置，以后
 * 新加的页面、接口模块在日常使用里也用不出来。这里从结构上守住。
 *
 * 扫描器自测在先（已知有问题的写法必须被识别、正确写法不得误报），再扫全仓。
 */
import { readdirSync, readFileSync, statSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { scanSource, stripComments } from './support/scanSitePaths'

const SRC = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')

function sourceFiles(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    const p = path.join(dir, name)
    if (statSync(p).isDirectory()) {
      if (name !== '__tests__') sourceFiles(p, out)
    } else if ((p.endsWith('.ts') || p.endsWith('.vue')) && !p.endsWith('.d.ts') && !p.endsWith('.test.ts')) {
      out.push(p)
    }
  }
  return out
}

const kindsOf = (src: string, file: string): string[] => scanSource(src, file).findings.map((f) => f.kind)

describe('扫描器自测：按结构识别没带应用基址的地址', () => {
  it('识别：fetch 的参数是根路径字面量（修复前 api/auth.ts 的写法）', () => {
    const r = scanSource(
      'const res = await fetch(`/auth/audit-log/export${buildQuery(filter)}`, {\n  method: \'GET\'\n})',
      'src/api/auth.ts'
    )
    expect(r.findings).toHaveLength(1)
    expect(r.findings[0]).toMatchObject({ kind: 'fetch-root-literal', file: 'api/auth.ts', line: 1 })
    expect(r.findings[0].snippet).toContain('/auth/audit-log/export')
  })

  it('不误报：fetch 的参数经 withBase（修复后的集中请求出口）', () => {
    const src = "res = await fetch(withBase(path + buildQuery(options.query)), {\n  method\n})"
    expect(scanSource(src, 'src/api/http.ts').findings).toEqual([])
  })

  it('不误报：fetch 的参数是变量（由浏览器按 <base> 解析的入口脚本地址）', () => {
    expect(scanSource('const res = await fetch(src, { method: \'HEAD\' })', 'src/router/navFailure.ts').findings).toEqual([])
  })

  it('识别：白名单之外的文件里直接 fetch', () => {
    const r = scanSource('const res = await fetch(url)', 'src/views/demo/index.ts')
    expect(r.findings.map((f) => f.kind)).toEqual(['fetch-outside-allowlist'])
  })

  it('识别：fetch 的第二个参数里有根路径字面量不算（只看地址）', () => {
    expect(scanSource("fetch(url, { headers: { Referer: '/x' } })", 'src/api/http.ts').findings).toEqual([])
  })

  it('识别：同源 WebSocket 地址里没有经 withBase（修复前 api/terminal.ts 的写法）', () => {
    const src = 'function url() {\n  return `${proto}://${window.location.host}/terminal/session/connect${query}`\n}\nconst ws = new WebSocket(url())'
    const r = scanSource(src, 'src/api/terminal.ts')
    expect(r.findings).toHaveLength(1)
    expect(r.findings[0]).toMatchObject({ kind: 'same-origin-url-without-base', file: 'api/terminal.ts', line: 2 })
  })

  it('识别：用 location.origin 拼的完整地址同理', () => {
    expect(kindsOf('window.open(`${window.location.origin}/docs`)', 'src/views/demo/index.vue')).toEqual([
      'same-origin-url-without-base'
    ])
  })

  it('不误报：WebSocket 地址经 withBase（先在小函数里拼好，再传给 new WebSocket）', () => {
    const src = 'function url(path, query) {\n  return `${proto}://${window.location.host}${withBase(path)}${buildQuery(query)}`\n}\nconst ws = new WebSocket(url(path, query))'
    expect(scanSource(src, 'src/api/docker.ts').findings).toEqual([])
  })

  it('不误报：只用到主机名、不拼面板路径', () => {
    expect(scanSource('const label = `主机：${window.location.hostname}`', 'src/views/demo/index.vue').findings).toEqual([])
  })

  it('识别：白名单之外的文件里建立 WebSocket', () => {
    expect(kindsOf('const ws = new WebSocket(withBase(url))', 'src/stores/terminal.ts')).toEqual([
      'websocket-outside-allowlist'
    ])
  })

  it('识别：模板里的 src 写了根路径（修复前登录页的写法）', () => {
    const src = '<script setup lang="ts">\nconst a = 1\n</script>\n\n<template>\n  <div class="page">\n    <img class="login-logo" src="/favicon.svg" alt="x" />\n  </div>\n</template>\n'
    const r = scanSource(src, 'src/views/login/index.vue')
    expect(r.findings).toHaveLength(1)
    expect(r.findings[0]).toMatchObject({ kind: 'template-root-path', line: 7 })
  })

  it('识别：多行标签里单独一行的 href 写了根路径', () => {
    const tpl = '<template>\n  <a\n    href="/docs"\n  >帮助</a>\n</template>'
    expect(kindsOf(tpl, 'src/views/x/index.vue')).toEqual(['template-root-path'])
  })

  it('不误报：模板里用变量，脚本里经 withBase 取图标地址', () => {
    const src = '<script setup lang="ts">\nconst logoSrc = withBase(\'/favicon.svg\')\n</script>\n\n<template>\n  <img :src="logoSrc" />\n  <!-- <img src="/old.png" /> -->\n</template>\n'
    expect(scanSource(src, 'src/layouts/MainLayout.vue').findings).toEqual([])
  })

  it('识别：整页跳转写了根路径字面量', () => {
    expect(kindsOf("window.location.assign('/settings')", 'src/views/panel-core/settings/components/EntryPathCard.vue')).toEqual([
      'location-root-literal'
    ])
  })

  it('不误报：整页跳转的地址经 withBase / joinBase / 变量', () => {
    const src = "window.location.assign(withBase(target))\nwindow.location.assign(joinBase(base, '/settings'))\nwindow.location.assign(url)"
    expect(scanSource(src, 'src/router/navFailure.ts').findings).toEqual([])
  })

  it('注释里的写法不算', () => {
    const src = "// fetch('/auth/me')\n/* new WebSocket(`${proto}://${host}/x`) */\nconst url = 'https://x//y' // 不是注释\nconst re = /^\\/\//\n"
    expect(scanSource(src, 'src/views/demo/index.ts').findings).toEqual([])
  })
})

describe('stripComments：注释换成空白，行号与字符串不变', () => {
  it('行注释与块注释被清空，字符串里的 // 保留', () => {
    const src = "const a = 'http://x' // 注释\n/* 多行\n注释 */ const b = 2"
    const out = stripComments(src)
    expect(out).toHaveLength(src.length)
    expect(out.split('\n')).toHaveLength(3)
    expect(out).toContain("const a = 'http://x'")
    expect(out).not.toContain('注释')
    expect(out).toContain('const b = 2')
  })
})

describe('全仓：面板自己的地址都带应用基址', () => {
  const files = sourceFiles(SRC)
  const rel = (f: string): string => path.relative(SRC, f).split(path.sep).join('/')
  const scans = files.map((f) => scanSource(readFileSync(f, 'utf8'), 'src/' + rel(f)))
  const findings = scans.flatMap((s) => s.findings)
  const counts = scans.reduce(
    (acc, c) => ({ fetch: acc.fetch + c.counts.fetch, websocket: acc.websocket + c.counts.websocket }),
    { fetch: 0, websocket: 0 }
  )

  it('扫描器没有失明：全仓 fetch 与 WebSocket 调用数在已知量之上', () => {
    expect(files.length).toBeGreaterThan(150)
    // api/http.ts 5 处 + api/auth.ts 1 处 + router/navFailure.ts 1 处
    expect(counts.fetch).toBeGreaterThanOrEqual(7)
    // api/terminal.ts + api/docker.ts
    expect(counts.websocket).toBeGreaterThanOrEqual(2)
  })

  it('没带应用基址的地址为 0', () => {
    const detail = findings.map((f) => `  ${f.file}:${f.line}  [${f.kind}]  ${f.snippet}`).join('\n')
    expect(findings, `以下地址不带入口路径前缀，开着安全入口时会被拦掉：\n${detail}`).toEqual([])
  })
})
