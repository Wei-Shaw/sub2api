import { describe, expect, it } from 'vitest'
import { isBytePlusEndpoint, packTokens } from '../byteplusPrepaid'

describe('prepaid provider inputs', () => {
  it('converts provider K/M displays without losing fractional pack units', () => {
    expect(packTokens('1234.567015625', 3)).toBe('1234567.015625')
    expect(packTokens('1,234,567.015625', 0)).toBe('1234567.015625')
    expect(packTokens('1.200000', 6)).toBe('1200000')
    expect(packTokens('0.015625', 0)).toBe('0.015625')
    for (const value of ['1e6', '-1', '1,00', '', 'NaN', '0.0000001']) {
      expect(() => packTokens(value, 0)).toThrow()
    }
  })
  it('recognizes only the official HTTPS inference bases', () => {
    expect(isBytePlusEndpoint('https://ark.ap-southeast.bytepluses.com/api/v3/')).toBe(true)
    expect(isBytePlusEndpoint('https://ark.eu-west.bytepluses.com:443/api/v3')).toBe(true)
    for (const value of ['http://ark.ap-southeast.bytepluses.com/api/v3', 'https://ark.ap-southeast.bytepluses.com.evil.invalid/api/v3', 'https://secret@ark.ap-southeast.bytepluses.com/api/v3', 'https://ark.ap-southeast.bytepluses.com/api/v3?key=secret', 'https://api.openai.com/v1']) {
      expect(isBytePlusEndpoint(value)).toBe(false)
    }
  })
})
