/**
 * 二次确认的触发按钮不得自带 onClick —— 全仓 .vue 源码扫描（vitest）。
 *
 * 背景：容器行 / Compose 行的按钮把执行同时绑在「按钮自己的 onClick」和「确认框的
 * onPositiveClick」上，点一下就直接执行，二次确认从上线起没有拦过任何操作。入口
 * 可达性测试只核对「按钮 → 处理函数」是否连通，这种接法反而被判成可达；真实浏览器
 * 里点一遍才暴露。这里从结构上守住：每个 NPopconfirm 的触发按钮里不允许出现 onClick。
 *
 * 扫描器自测在先（已知有问题的写法必须被识别、正确写法不得误报），再扫全仓。
 */
import { readdirSync, readFileSync, statSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { scanSource } from './support/scanConfirmTrigger'

const SRC = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')

function vueFiles(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    const p = path.join(dir, name)
    if (statSync(p).isDirectory()) {
      if (name !== '__tests__') vueFiles(p, out)
    } else if (p.endsWith('.vue')) out.push(p)
  }
  return out
}

describe('扫描器自测：按结构识别二次确认的触发按钮', () => {
  /** 容器行 / Compose 行的原始写法（跨行 onClick） */
  const buggyVariableForm = `
function render(row) {
  const btn = h(
    NButton,
    {
      size: 'small',
      disabled: inFlight,
      onClick: () => {
        void execContainerAction(row, def.action)
      }
    },
    { default: () => '停止' }
  )
  buttons.push(
    def.confirm === null
      ? btn
      : h(
          NPopconfirm,
          {
            onPositiveClick: () => {
              void execContainerAction(row, def.action)
            }
          },
          { trigger: () => btn, default: () => def.confirm }
        )
  )
}`

  it('识别：变量写法，触发按钮带跨行 onClick（容器行 / Compose 行的原始缺陷形态）', () => {
    const r = scanSource(buggyVariableForm, 'x.vue')
    expect(r.counts).toEqual({ renderFn: 1, template: 0 })
    expect(r.findings).toHaveLength(1)
    expect(r.findings[0].form).toBe('渲染函数')
    expect(r.findings[0].snippet).toContain('onClick')
    expect(r.findings[0].line).toBe(8)
  })

  it('识别：内联写法，trigger: () => h(NButton, { onClick })', () => {
    const r = scanSource(
      `h(NPopconfirm, { onPositiveClick: () => doDelete(r) }, { trigger: () => h(NButton, { onClick: () => doDelete(r) }, { default: () => '删除' }) })`,
      'x.vue'
    )
    expect(r.counts.renderFn).toBe(1)
    expect(r.findings).toHaveLength(1)
    expect(r.findings[0].form).toBe('渲染函数')
  })

  it('不误报：变量写法，触发按钮不带 onClick（执行只在 onPositiveClick）', () => {
    const r = scanSource(
      `
const btn = h(NButton, { size: 'small', disabled }, { default: () => '停止' })
nodes.push(h(NPopconfirm, { onPositiveClick: () => void exec(row) }, { trigger: () => btn, default: () => '确认停止？' }))
`,
      'x.vue'
    )
    expect(r.counts.renderFn).toBe(1)
    expect(r.findings).toEqual([])
  })

  it('不误报：无确认的动作按钮（直接执行）与确认框按钮同名时，各认各的定义', () => {
    // 两个函数里都有 const btn：第一个带 onClick（应报），第二个不带（不应报）
    const r = scanSource(buggyVariableForm + buggyVariableForm.replace(/onClick: \(\) => \{\n\s*void execContainerAction\(row, def.action\)\n\s*\}/, ''), 'x.vue')
    expect(r.counts.renderFn).toBe(2)
    expect(r.findings).toHaveLength(1)
    expect(r.findings[0].line).toBe(8)
  })

  it('不误报：只阻止冒泡的空操作触发器（stopAnd(() => undefined)）', () => {
    const r = scanSource(
      `h(NPopconfirm, { onPositiveClick: () => remove(row) }, {
  trigger: () => h(NButton, { size: 'small', onClick: stopAnd(() => undefined) }, { default: () => '移除' }),
  default: () => '确认移除？'
})`,
      'x.vue'
    )
    expect(r.counts.renderFn).toBe(1)
    expect(r.findings).toEqual([])
  })

  it('识别：stopAnd 里带了真正的动作仍算问题', () => {
    const r = scanSource(
      `h(NPopconfirm, { onPositiveClick: () => remove(row) }, { trigger: () => h(NButton, { onClick: stopAnd(() => remove(row)) }, { default: () => '移除' }) })`,
      'x.vue'
    )
    expect(r.findings).toHaveLength(1)
  })

  it('识别：模板 #trigger 里的按钮带 @click', () => {
    const r = scanSource(
      `<n-popconfirm @positive-click="doDelete(row)">
  <template #trigger>
    <n-button size="small" @click="doDelete(row)">删除</n-button>
  </template>
  确认删除？
</n-popconfirm>`,
      'x.vue'
    )
    expect(r.counts.template).toBe(1)
    expect(r.findings).toHaveLength(1)
    expect(r.findings[0].form).toBe('模板')
    expect(r.findings[0].line).toBe(3)
  })

  it('识别：模板默认插槽当触发器，按钮带 @click', () => {
    const r = scanSource(
      `<NPopconfirm :on-positive-click="doDelete"><NButton type="error" @click="doDelete">删除</NButton></NPopconfirm>`,
      'x.vue'
    )
    expect(r.counts.template).toBe(1)
    expect(r.findings).toHaveLength(1)
    expect(r.findings[0].form).toBe('模板')
  })

  it('不误报：模板写法，触发按钮不带 @click（确认后的内容里可以有别的按钮）', () => {
    const r = scanSource(
      `<n-popconfirm @positive-click="doDelete(row)">
  <template #trigger>
    <n-button size="small">删除</n-button>
  </template>
  确认删除？<n-button @click="openHelp">帮助</n-button>
</n-popconfirm>`,
      'x.vue'
    )
    expect(r.counts.template).toBe(1)
    expect(r.findings).toEqual([])
  })

  it('自闭合的 NPopconfirm（无触发插槽）不计入', () => {
    const r = scanSource(`h(NButton, { onClick: () => open() }, { default: () => 'x' })\n<NPopconfirm :on-positive-click="fn" />`, 'x.vue')
    expect(r.counts).toEqual({ renderFn: 0, template: 0 })
    expect(r.findings).toEqual([])
  })
})

describe('全仓：二次确认的触发按钮都不得自带 onClick', () => {
  const files = vueFiles(SRC)
  const findings = files.flatMap((f) => scanSource(readFileSync(f, 'utf8'), path.relative(SRC, f).split(path.sep).join('/')).findings)
  const counts = files
    .map((f) => scanSource(readFileSync(f, 'utf8'), '').counts)
    .reduce((acc, c) => ({ renderFn: acc.renderFn + c.renderFn, template: acc.template + c.template }), { renderFn: 0, template: 0 })

  it('扫描器没有失明：全仓确认框数量在已知量级之上', () => {
    expect(files.length).toBeGreaterThan(50)
    expect(counts.renderFn).toBeGreaterThanOrEqual(20)
    expect(counts.template).toBeGreaterThanOrEqual(10)
  })

  it('触发按钮自带 onClick 的位置为 0', () => {
    const detail = findings.map((f) => `  ${f.file}:${f.line}  [${f.form}]  ${f.snippet}`).join('\n')
    expect(findings, `以下二次确认的触发按钮点一下就会直接执行：\n${detail}`).toEqual([])
  })
})
