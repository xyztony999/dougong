import { describe, expect, it } from 'vitest'
import { certIssuerText } from '../panel'

describe('certIssuerText', () => {
  it('空值显示破折号', () => {
    expect(certIssuerText(null)).toBe('—')
    expect(certIssuerText(undefined)).toBe('—')
    expect(certIssuerText('')).toBe('—')
    expect(certIssuerText('  ')).toBe('—')
  })

  it('非空颁发者原样返回', () => {
    expect(certIssuerText("Let's Encrypt")).toBe("Let's Encrypt")
    expect(certIssuerText('R11')).toBe('R11')
  })
})
