import { describe, expect, it } from 'vitest'

import { calculateOutputTokensPerSecond } from '../usageMetrics'

describe('calculateOutputTokensPerSecond', () => {
  it('uses total request duration to match the existing operations metric', () => {
    expect(calculateOutputTokensPerSecond({
      output_tokens: 100,
      duration_ms: 2000,
    })).toBe(50)
  })

  it.each([
    { output_tokens: 0, duration_ms: 1000 },
    { output_tokens: 10, duration_ms: 0 },
    { output_tokens: 10, duration_ms: null },
    { output_tokens: Number.NaN, duration_ms: 1000 },
    { output_tokens: 10, duration_ms: Number.POSITIVE_INFINITY },
  ])('returns null when token or duration data is unusable', (usage) => {
    expect(calculateOutputTokensPerSecond(usage)).toBeNull()
  })

  it.each([
    { image_count: 1, image_output_tokens: 0 },
    { image_count: 1, image_output_tokens: 100 },
    { image_count: 0, image_output_tokens: 100 },
  ])('does not report text token speed for image generation output', (imageUsage) => {
    expect(calculateOutputTokensPerSecond({
      output_tokens: 100,
      duration_ms: 2000,
      ...imageUsage,
    })).toBeNull()
  })
})
