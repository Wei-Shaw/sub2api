import { describe, expect, it } from 'vitest'
import { currencySymbol, formatPaymentAmount } from '../currency'

describe('formatPaymentAmount', () => {
  it('uses the currency default fraction digits', () => {
    expect(formatPaymentAmount(100, 'JPY', 'en-US')).not.toContain('.00')
    expect(formatPaymentAmount(100, 'KRW', 'en-US')).not.toContain('.00')
    expect(formatPaymentAmount(100, 'HKD', 'en-US')).toContain('.00')
  })
})

describe('currencySymbol', () => {
  it('maps common payment currencies and falls back safely', () => {
    expect(currencySymbol('USD')).toBe('$')
    expect(currencySymbol('cny')).toBe('￥')
    expect(currencySymbol('EUR')).toBe('€')
    expect(currencySymbol('')).toBe('￥')
    expect(currencySymbol('XYZ')).toBe('XYZ')
  })

  it('keeps the RMB symbol stable across locales', () => {
    expect(formatPaymentAmount(100, 'CNY', 'en-US')).toBe('￥100.00')
    expect(formatPaymentAmount(100, 'CNY', 'zh-CN')).toBe('￥100.00')
  })
})
