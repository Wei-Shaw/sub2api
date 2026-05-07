import { describe, expect, it } from 'vitest'

import { validateIntervals } from '../types'
import type { IntervalFormEntry } from '../types'

function imageTier(label: string, price: number): IntervalFormEntry {
  return {
    min_tokens: 0,
    max_tokens: null,
    tier_label: label,
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    per_request_price: price,
    sort_order: 0,
  }
}

describe('channel pricing interval validation', () => {
  it('allows image tiers to share open-ended token ranges', () => {
    expect(validateIntervals([
      imageTier('1K', 0.04),
      imageTier('2K', 0.08),
      imageTier('4K', 0.16),
    ], 'image')).toBeNull()
  })

  it('keeps open-ended overlap validation for token intervals', () => {
    expect(validateIntervals([
      imageTier('A', 0.04),
      imageTier('B', 0.08),
    ], 'token')).toContain('无上限区间')
  })

  it('rejects duplicate image tier labels', () => {
    expect(validateIntervals([
      imageTier('1K', 0.04),
      imageTier('1k', 0.08),
    ], 'image')).toContain('分辨率 1K 重复')
  })
})
