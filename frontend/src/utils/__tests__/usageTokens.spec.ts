import { describe, expect, it } from 'vitest'

import { buildUsageTokenDisplay } from '../usageTokens'

describe('buildUsageTokenDisplay', () => {
  it('computes display input and total tokens including cache tokens', () => {
    const result = buildUsageTokenDisplay({
      input_tokens: 100,
      output_tokens: 20,
      cache_read_tokens: 30,
      cache_creation_tokens: 10,
    })

    expect(result.netInputTokens).toBe(100)
    expect(result.displayInputTokens).toBe(140)
    expect(result.displayTotalTokens).toBe(160)
  })

  it('keeps zero-safe behaviour', () => {
    const result = buildUsageTokenDisplay({
      input_tokens: 0,
      output_tokens: 0,
      cache_read_tokens: 0,
      cache_creation_tokens: 0,
    })

    expect(result.netInputTokens).toBe(0)
    expect(result.displayInputTokens).toBe(0)
    expect(result.displayTotalTokens).toBe(0)
  })
})
