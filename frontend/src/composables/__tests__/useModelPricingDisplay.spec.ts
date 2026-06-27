import { describe, expect, it } from 'vitest'
import { ref } from 'vue'
import {
  formatContextTokens,
  formatModelPrice,
  formatRateMultiplier,
  pricePartVisible,
  useModelPricingSummary,
} from '@/composables/useModelPricingDisplay'

describe('useModelPricingDisplay', () => {
  it('formats model prices across ranges', () => {
    expect(formatModelPrice(null)).toBe('—')
    expect(formatModelPrice(0)).toBe('$0.0000')
    expect(formatModelPrice(0.00001)).toBe('$1.00e-5')
    expect(formatModelPrice(0.125)).toBe('$0.125000')
    expect(formatModelPrice(1.23456)).toBe('$1.2346')
  })

  it('formats rate multipliers and context tokens', () => {
    expect(formatRateMultiplier(null)).toBe('1x')
    expect(formatRateMultiplier(1)).toBe('1x')
    expect(formatRateMultiplier(0.8)).toBe('0.8x')
    expect(formatRateMultiplier(1.25)).toBe('1.25x')

    expect(formatContextTokens(null)).toBe('—')
    expect(formatContextTokens(0)).toBe('—')
    expect(formatContextTokens(999)).toBe('999')
    expect(formatContextTokens(1_500)).toBe('2K')
    expect(formatContextTokens(1_500_000)).toBe('1.5M')
    expect(formatContextTokens(87_000_000)).toBe('87M')
  })

  it('detects visible price parts', () => {
    expect(pricePartVisible(null)).toBe(false)
    expect(pricePartVisible(undefined)).toBe(false)
    expect(pricePartVisible({ per_token: 0, per_1m: 0, display_usd: 0 })).toBe(false)
    expect(pricePartVisible({ per_token: 0.000001, per_1m: 0, display_usd: 0 })).toBe(true)
  })

  it('summarizes model counts by billing mode and provider', () => {
    const items = ref([
      {
        model: 'gpt-5.5',
        provider: 'openai',
        billing_mode: 'token',
      },
      {
        model: 'gpt-image-1',
        provider: 'openai',
        billing_mode: 'image',
      },
      {
        model: 'claude-sonnet-4',
        provider: 'anthropic',
        billing_mode: 'token',
      },
    ] as any)

    const summary = useModelPricingSummary(items)

    expect(summary.totalModels.value).toBe(3)
    expect(summary.tokenModels.value).toBe(2)
    expect(summary.imageModels.value).toBe(1)
    expect(summary.providerCount.value).toBe(2)
  })
})
