/**
 * TOTP 前端工具（PageSpec 5.1 设计决策：init-wizard 的 secret 由前端生成，
 * 随 initWizard 提交 totpSecret+totpCode；字段名与 api 4.4.4 req 对齐，Gate 1 W4 修正）。
 * 参数与 api 3.2 对齐：RFC 6238、30 秒窗口、6 位数字码（SHA1/6/30 为标准 otpauth 参数）。
 */

const BASE32_ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'

/** 生成 Base32 随机密钥（默认 20 字节 → 32 字符，RFC 6238 常规强度） */
export function generateTotpSecret(byteLength = 20): string {
  const bytes = new Uint8Array(byteLength)
  crypto.getRandomValues(bytes)
  let bits = 0
  let value = 0
  let out = ''
  for (const b of bytes) {
    value = (value << 8) | b
    bits += 8
    while (bits >= 5) {
      out += BASE32_ALPHABET[(value >>> (bits - 5)) & 31]
      bits -= 5
    }
  }
  if (bits > 0) {
    out += BASE32_ALPHABET[(value << (5 - bits)) & 31]
  }
  return out
}

/** 构造 otpauth:// URI（认证器 App 扫码导入） */
export function buildOtpauthUri(secret: string, account: string, issuer = '斗拱面板'): string {
  const label = encodeURIComponent(`${issuer}:${account}`)
  const issuerParam = encodeURIComponent(issuer)
  return `otpauth://totp/${label}?secret=${secret}&issuer=${issuerParam}&algorithm=SHA1&digits=6&period=30`
}
