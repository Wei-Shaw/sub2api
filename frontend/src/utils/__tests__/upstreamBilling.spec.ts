import { describe, expect, it } from 'vitest'
import { formatBillingAmount, hasNonZeroBillingAmount, isBillingMonth } from '../upstreamBilling'

describe('upstream billing display', () => {
  it('preserves exact decimal amounts, including tiny adjustments and large amounts', () => {
    expect(formatBillingAmount('12345678901234567890.12345678')).toBe('12,345,678,901,234,567,890.12345678')
    expect(formatBillingAmount('-0.00000001')).toBe('-0.00000001')
    expect(formatBillingAmount('42.50000000')).toBe('42.50')
    expect(formatBillingAmount('0')).toBe('0.00')
  })

  it('accepts only complete supported billing months', () => {
    expect(isBillingMonth('2026-09')).toBe(true)
    for (const value of ['2026-00', '2026-13', '2026-1', '2026-09-01', 'oops']) {
      expect(isBillingMonth(value)).toBe(false)
    }
  })

  it('detects nonzero signed adjustments without floating-point conversion', () => {
    expect(hasNonZeroBillingAmount('-0.000000000000000000000000000000001')).toBe(true)
    expect(hasNonZeroBillingAmount('9999999999999999999999.00')).toBe(true)
    for (const value of ['0', '-0.00000000', '', undefined, 'unknown']) expect(hasNonZeroBillingAmount(value)).toBe(false)
  })
})
