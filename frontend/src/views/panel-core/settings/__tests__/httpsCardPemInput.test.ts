/**
 * HTTPS 卡片的证书 / 私钥 PEM 输入必须是多行文本域。
 *
 * 背景：v0.5.0 前私钥用 type="password" 单行框承载（为了遮罩），单行框粘贴多行 PEM 时
 * 浏览器把换行换成空格，PEM 格式必坏、导入报「解析失败或不配对」。PEM 必须多行；
 * 「零回显」靠导入后清空 + 无查看入口保证，不靠输入时遮罩。这里从结构上守住，防退回单行。
 */
import { readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const HERE = path.dirname(fileURLToPath(import.meta.url))
const SRC = readFileSync(path.resolve(HERE, '../components/HttpsCard.vue'), 'utf8')

/** 取出某个 path 的 n-form-item 块里的 n-input 标签 */
function inputBlock(field: string): string {
  const re = new RegExp(`<n-form-item[^>]*path="${field}"[\\s\\S]*?</n-form-item>`)
  const m = SRC.match(re)
  return m ? m[0] : ''
}

describe('HttpsCard PEM 输入框结构', () => {
  it('证书 PEM 是多行文本域', () => {
    const block = inputBlock('certPem')
    expect(block).toContain('type="textarea"')
  })

  it('私钥 PEM 是多行文本域（不是单行 password）', () => {
    const block = inputBlock('keyPem')
    expect(block).toContain('type="textarea"')
    expect(block).not.toContain('type="password"')
  })

  it('两个 PEM 框都不带 rows 以外的单行特征', () => {
    for (const field of ['certPem', 'keyPem']) {
      const block = inputBlock(field)
      expect(block, field).toContain(':rows=')
      expect(block, field).not.toContain('show-password-on')
    }
  })
})
