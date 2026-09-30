import { describe, expect, it } from 'vitest'
import { formatScaled, MODEL_PLAZA_USD_TO_VND } from '../pricing'

describe('Model Plaza display pricing', () => {
  it('uses the fixed 1 USD = 1,000 VND display conversion without changing default USD output', () => {
    expect(MODEL_PLAZA_USD_TO_VND).toBe(1_000)
    expect(formatScaled(3e-6, 1_000_000)).toBe('$3')
    expect(formatScaled(3e-6, 1_000_000, 2, { currency: 'VND' })).toBe('3.000₫')
  })

  it('preserves small per-request VND values instead of adding USD formatting', () => {
    expect(formatScaled(0.04 * 0.5, 1, 2, { currency: 'VND' })).toBe('20₫')
    expect(formatScaled(null, 1_000_000, 2, { currency: 'VND' })).toBe('-')
  })
})
