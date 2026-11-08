import { describe, expect, it } from 'vitest'
import { formatMultiplier } from '../formatters'

describe('formatMultiplier', () => {
  it('keeps significant decimals instead of rounding to 2 places', () => {
    expect(formatMultiplier(0.035)).toBe('0.035')
    expect(formatMultiplier(0.125)).toBe('0.125')
    expect(formatMultiplier(0.015)).toBe('0.015')
    expect(formatMultiplier(0.0625)).toBe('0.0625')
  })

  it('pads to at least 2 decimals for round values', () => {
    expect(formatMultiplier(0.3)).toBe('0.30')
    expect(formatMultiplier(1)).toBe('1.00')
    expect(formatMultiplier(1.5)).toBe('1.50')
    expect(formatMultiplier(2)).toBe('2.00')
  })

  it('handles small values down to 4 decimals', () => {
    expect(formatMultiplier(0.001)).toBe('0.001')
    expect(formatMultiplier(0.0001)).toBe('0.0001')
  })

  it('falls back to 2 significant digits below 0.0001', () => {
    expect(formatMultiplier(0.00005)).toBe('0.000050')
  })
})
