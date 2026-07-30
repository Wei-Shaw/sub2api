import { describe, expect, it } from 'vitest'
import type { ModelPlazaGroup } from '@/api/modelPlaza'
import {
  effectiveRate,
  filterPlazaGroups,
  formatCatalogPrice,
  groupBillingLabel,
  isImageModel,
  sortPlazaModels
} from '../modelPlazaCustom'

function group(overrides: Partial<ModelPlazaGroup> = {}): ModelPlazaGroup {
  return {
    id: 1,
    name: 'GPT Plus',
    description: '日常模型',
    platform: 'OpenAI',
    subscription_type: 'standard',
    rate_multiplier: 0.12,
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 0,
    is_exclusive: false,
    models: [],
    ...overrides
  }
}

function model(name: string, output = 0.000003) {
  return {
    name,
    platform: 'OpenAI',
    pricing: {
      billing_mode: 'token' as const,
      input_price: 0.000001,
      output_price: output,
      cache_write_price: null,
      cache_read_price: null,
      image_input_price: null,
      image_output_price: null,
      per_request_price: null,
      intervals: []
    },
    official_pricing: {
      input_price: 0.000001,
      output_price: output,
      cache_write_price: null,
      cache_read_price: null
    }
  }
}

describe('custom model plaza view model', () => {
  it('uses a user-specific multiplier and keeps the public billing explanation', () => {
    const subscription = group({
      subscription_type: 'subscription',
      user_rate_multiplier: 1.8
    })
    const balance = group()

    expect(effectiveRate(subscription)).toBe(1.8)
    expect(groupBillingLabel(subscription)).toBe('订阅 1:10')
    expect(groupBillingLabel(balance)).toBe('余额 1:1')
  })

  it('filters groups by platform, group, rate, and model search', () => {
    const groups = [
      group({ id: 1, platform: 'OpenAI', models: [model('gpt-5.6-sol')] }),
      group({ id: 2, name: 'Claude', platform: 'Anthropic', models: [model('claude-sonnet')] })
    ]

    expect(filterPlazaGroups(groups, { platform: 'Anthropic' }).map((item) => item.id)).toEqual([2])
    expect(filterPlazaGroups(groups, { groupId: 1 }).map((item) => item.id)).toEqual([1])
    expect(filterPlazaGroups(groups, { query: 'sonnet' }).map((item) => item.id)).toEqual([2])
  })

  it('sorts models by official output price and identifies image billing', () => {
    const expensive = model('expensive', 0.00001)
    const cheap = model('cheap', 0.000001)
    expect(sortPlazaModels([cheap, expensive]).map((item) => item.name)).toEqual(['expensive', 'cheap'])
    expect(isImageModel({ ...cheap, pricing: { ...cheap.pricing, billing_mode: 'image' } })).toBe(true)
  })

  it('formats customer-facing prices without dropping small values', () => {
    expect(formatCatalogPrice(0.09)).toBe('0.09')
    expect(formatCatalogPrice(0.0003)).toBe('0.0003')
    expect(formatCatalogPrice(null)).toBe('—')
  })
})
