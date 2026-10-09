import { describe, expect, it } from 'vitest'
import { summarizeModelPricing } from '../pricing'
import type { UserSupportedModelPricing } from '@/api/channels'

function pricing(partial: Partial<UserSupportedModelPricing>): UserSupportedModelPricing {
  return {
    billing_mode: 'token',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_input_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: [],
    ...partial
  }
}

describe('summarizeModelPricing', () => {
  it('returns empty string when pricing is missing', () => {
    expect(summarizeModelPricing(null)).toBe('')
    expect(summarizeModelPricing(undefined)).toBe('')
  })

  it('formats token input/output per 1M tokens', () => {
    expect(
      summarizeModelPricing(
        pricing({ billing_mode: 'token', input_price: 3e-6, output_price: 15e-6 })
      )
    ).toBe('$3 / $15')
  })

  it('falls back to first interval for token pricing', () => {
    expect(
      summarizeModelPricing(
        pricing({
          billing_mode: 'token',
          intervals: [
            {
              min_tokens: 0,
              max_tokens: null,
              input_price: 2e-6,
              output_price: 10e-6,
              cache_write_price: null,
              cache_read_price: null,
              per_request_price: null
            }
          ]
        })
      )
    ).toBe('$2 / $10')
  })

  it('formats per-request and image prices', () => {
    expect(
      summarizeModelPricing(pricing({ billing_mode: 'per_request', per_request_price: 0.05 }))
    ).toBe('$0.05')
    expect(
      summarizeModelPricing(pricing({ billing_mode: 'image', image_output_price: 0.04 }))
    ).toBe('$0.04')
  })

  it('formats video prices per second', () => {
    expect(
      summarizeModelPricing(pricing({ billing_mode: 'video', per_request_price: 0.12 }))
    ).toBe('$0.12/s')
  })
})
