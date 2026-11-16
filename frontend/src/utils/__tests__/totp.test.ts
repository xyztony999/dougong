import { describe, expect, it } from 'vitest'
import { buildOtpauthUri, generateTotpSecret } from '../totp'

const BASE32 = /^[A-Z2-7]+$/

describe('generateTotpSecret（init-wizard 前端生成 secret，PageSpec 5.1）', () => {
  it('默认 20 字节 → 32 字符 Base32（RFC 6238 常规强度）', () => {
    const secret = generateTotpSecret()
    expect(secret).toHaveLength(32)
    expect(BASE32.test(secret)).toBe(true)
  })

  it('两次生成互不相同（crypto 随机）', () => {
    const a = generateTotpSecret()
    const b = generateTotpSecret()
    expect(a).not.toBe(b)
  })
})

describe('buildOtpauthUri', () => {
  it('含 secret/issuer 与标准 TOTP 参数（SHA1/6 位/30 秒，api 3.2）', () => {
    const uri = buildOtpauthUri('JBSWY3DPEHPK3PXP', 'ops@example')
    expect(uri.startsWith('otpauth://totp/')).toBe(true)
    expect(uri).toContain('secret=JBSWY3DPEHPK3PXP')
    expect(uri).toContain('issuer=%E6%96%97%E6%8B%B1%E9%9D%A2%E6%9D%BF')
    expect(uri).toContain('algorithm=SHA1')
    expect(uri).toContain('digits=6')
    expect(uri).toContain('period=30')
    // label = issuer:account，account 特殊字符编码
    expect(uri).toContain('%E6%96%97%E6%8B%B1%E9%9D%A2%E6%9D%BF%3Aops%40example')
  })
})
