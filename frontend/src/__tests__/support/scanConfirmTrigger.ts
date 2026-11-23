/**
 * scanConfirmTrigger —— 找出「二次确认（NPopconfirm）的触发按钮上同时绑了直接执行的
 * onClick / @click」的位置。
 *
 * 那种写法点一下就直接执行：确认气泡只是事后才出现，点「确认」还会再执行一遍。
 * 容器行的停止 / 重启 / 暂停 / 删除、Compose 行的 up / restart / down / 删除项目从上线起
 * 就是这样写的，源码断言只核对了「按钮 → 处理函数」连不连通，反而把错误的接法也判成了
 * 可达；直到 v0.4.0 重组时在真实浏览器里点了一遍才发现。
 *
 * 只看每个确认框自己的触发按钮：
 *   渲染函数  h(NPopconfirm, { onPositiveClick }, { trigger: () => btn })          btn 由 const btn = h(…) 定义
 *             h(NPopconfirm, { onPositiveClick }, { trigger: () => h(NButton, { … }) })
 *   模板      <n-popconfirm @positive-click="…"><template #trigger>…</template>…</n-popconfirm>
 *             <n-popconfirm @positive-click="…"><n-button @click="…"/></n-popconfirm>（默认插槽即触发器）
 */

export interface ConfirmFinding {
  file: string
  line: number
  form: '渲染函数' | '模板'
  /** 触发按钮上绑定的处理（源码片段） */
  snippet: string
}

export interface ConfirmScan {
  findings: ConfirmFinding[]
  /** 识别出的确认框数量（渲染函数写法 / 模板写法）——用来确认扫描器没有失明 */
  counts: { renderFn: number; template: number }
}

/** 从 openIdx（指向开括号）起做括号配对，返回闭括号下标；找不到返回 -1。跳过字符串与注释里的括号。 */
export function matchParen(src: string, openIdx: number): number {
  const open = src[openIdx]
  const close = open === '(' ? ')' : open === '{' ? '}' : ']'
  let depth = 0
  for (let i = openIdx; i < src.length; i++) {
    const c = src[i]
    if (c === '"' || c === "'" || c === '`') {
      i = skipString(src, i)
      continue
    }
    if (c === '/' && src[i + 1] === '/') {
      i = src.indexOf('\n', i)
      if (i < 0) return -1
      continue
    }
    if (c === '/' && src[i + 1] === '*') {
      i = src.indexOf('*/', i + 2)
      if (i < 0) return -1
      continue
    }
    if (c === open) depth++
    else if (c === close) {
      depth--
      if (depth === 0) return i
    }
  }
  return -1
}

function skipString(src: string, start: number): number {
  const quote = src[start]
  for (let j = start + 1; j < src.length; j++) {
    if (src[j] === '\\') {
      j++
      continue
    }
    if (src[j] === quote) return j
  }
  return src.length - 1
}

const lineAt = (src: string, index: number) => src.slice(0, index).split('\n').length
const firstLine = (text: string) => text.split('\n').find((l) => l.trim() !== '')?.trim().slice(0, 90) ?? ''

/** 在 `before` 里找某个名字的定义位置（最近的一个），返回定义表达式所在区域及其起点 */
function definitionRegion(src: string, name: string, before: number): { start: number; text: string } | null {
  const prefix = src.slice(0, before)
  const re = new RegExp('(?:const|let) ' + name + '\\s*=\\s*', 'g')
  let m: RegExpExecArray | null
  let last: RegExpExecArray | null = null
  while ((m = re.exec(prefix))) last = m
  if (!last) return null
  const exprStart = last.index + last[0].length
  if (/^h\(/.test(src.slice(exprStart, exprStart + 2))) {
    const close = matchParen(src, exprStart + 1)
    return close >= 0 && close < before ? { start: exprStart, text: src.slice(exprStart, close + 1) } : null
  }
  // 其它初始化式：按缩进取到语句结束（遇到缩进不深于定义行且非空的行为止）
  const defLineStart = prefix.lastIndexOf('\n', last.index) + 1
  const indent = prefix.slice(defLineStart).match(/^\s*/)?.[0].length ?? 0
  const tail = src.slice(exprStart)
  const lines = tail.split('\n')
  let end = lines[0].length
  for (let i = 1; i < lines.length; i++) {
    const l = lines[i]
    if (l.trim() !== '' && (l.match(/^\s*/)?.[0].length ?? 0) <= indent) break
    end += 1 + l.length
  }
  return { start: exprStart, text: tail.slice(0, end) }
}

/** 扫描一份源码（.vue 全文） */
export function scanSource(src: string, file: string): ConfirmScan {
  const findings: ConfirmFinding[] = []
  const counts = { renderFn: 0, template: 0 }

  // ---- 渲染函数写法 ----
  let from = 0
  for (;;) {
    const at = src.indexOf('NPopconfirm', from)
    if (at < 0) break
    from = at + 'NPopconfirm'.length
    const before = src.slice(Math.max(0, at - 60), at)
    const head = /(?:\bh|\bcreateVNode|\b_createVNode)\(\s*$/.exec(before)
    if (!head) continue
    // head[0] 形如 "h(" 或 "h(\n          "（跨行写法）：左括号是其中的那个 "("
    const callOpen = at - before.length + head.index + head[0].indexOf('(')
    const callClose = matchParen(src, callOpen)
    if (callClose < 0) continue
    counts.renderFn++
    const call = src.slice(callOpen, callClose + 1)
    const trig = /trigger:\s*\(\)\s*=>\s*/.exec(call)
    if (!trig) continue
    const trigStart = callOpen + trig.index + trig[0].length
    let region: { start: number; text: string } | null = null
    if (/^h\(/.test(src.slice(trigStart, trigStart + 2))) {
      const close = matchParen(src, trigStart + 1)
      if (close >= 0) region = { start: trigStart, text: src.slice(trigStart, close + 1) }
    } else {
      const name = /^(\w+)/.exec(src.slice(trigStart, trigStart + 40))?.[1]
      region = name ? definitionRegion(src, name, trigStart) : null
    }
    if (!region) continue
    // 「只阻止冒泡、什么都不做」的触发器是允许的（数据库管理页为了让行内按钮不触发整行点击）
    const NO_OP_TRIGGER = /onClick:\s*stopAnd\(\(\)\s*=>\s*undefined\)/g
    const onClick = /\bonClick\b/.exec(region.text.replace(NO_OP_TRIGGER, ''))
    if (onClick) {
      const raw = region.text.replace(NO_OP_TRIGGER, (m) => ' '.repeat(m.length))
      const at = region.start + raw.indexOf(onClick[0])
      findings.push({ file, line: lineAt(src, at), form: '渲染函数', snippet: firstLine(raw.slice(raw.indexOf(onClick[0]))) })
    }
  }

  // ---- 模板写法 ----
  const tagRe = /<(n-popconfirm|NPopconfirm)\b([^>]*)>/g
  let m: RegExpExecArray | null
  while ((m = tagRe.exec(src))) {
    if (/\/>$/.test(m[0])) continue
    const closeTag = '</' + m[1] + '>'
    const innerStart = m.index + m[0].length
    let end = -1
    let depth = 1
    const nested = /<(\/?)(?:n-popconfirm|NPopconfirm)\b[^>]*?>/g
    nested.lastIndex = innerStart
    let n: RegExpExecArray | null
    while ((n = nested.exec(src))) {
      depth += n[1] ? -1 : 1
      if (depth === 0) {
        end = n.index
        break
      }
    }
    if (end < 0) {
      // 找不到配对的结束标签就按第一个结束标签算
      end = src.indexOf(closeTag, innerStart)
      if (end < 0) continue
    }
    const inner = src.slice(innerStart, end)
    tagRe.lastIndex = end
    counts.template++
    const slot = /<template\s+#trigger[^>]*>([\s\S]*?)<\/template>/.exec(inner)
    const region = slot ? slot[1] : inner
    const at = region.search(/@(?:click|click\.stop|click\.prevent)\b|v-on:click/)
    if (at >= 0) {
      const idx = innerStart + (slot ? slot.index + slot[0].indexOf(slot[1]) : 0) + at
      findings.push({ file, line: lineAt(src, idx), form: '模板', snippet: firstLine(region.slice(at)) })
    }
  }
  return { findings, counts }
}
