import { describe, expect, it } from 'vitest'
import {
  cloneServiceTierConfig,
  defaultServiceTierConfig,
  validateServiceTierConfig,
} from '../serviceTier'

describe('channel service tier form helpers', () => {
  it('uses backward-compatible defaults', () => {
    expect(defaultServiceTierConfig()).toEqual({
      standard: { enabled: true, multiplier: 1 },
      priority: { enabled: true, multiplier: 2 },
      flex: { enabled: true, multiplier: 0.5 },
      use_outbound_tier_for_billing: true,
    })
  })

  it('clones nested options instead of sharing form state', () => {
    const source = defaultServiceTierConfig()
    const clone = cloneServiceTierConfig(source)
    clone.priority.multiplier = 3
    clone.use_outbound_tier_for_billing = false
    expect(source.priority.multiplier).toBe(2)
    expect(source.use_outbound_tier_for_billing).toBe(true)
  })

  it('defaults an omitted legacy billing switch to enabled', () => {
    const legacy = {
      standard: { enabled: true, multiplier: 1 },
      priority: { enabled: true, multiplier: 2 },
      flex: { enabled: true, multiplier: 0.5 },
    } as unknown as Parameters<typeof cloneServiceTierConfig>[0]

    expect(cloneServiceTierConfig(legacy).use_outbound_tier_for_billing).toBe(true)
  })

  it('requires at least one enabled tier', () => {
    const config = defaultServiceTierConfig()
    config.standard.enabled = false
    config.priority.enabled = false
    config.flex.enabled = false
    expect(validateServiceTierConfig(config)).toBe('no_enabled_tier')
  })

  it.each([0, 100.01, Number.NaN, Number.POSITIVE_INFINITY])('rejects multiplier %s', multiplier => {
    const config = defaultServiceTierConfig()
    config.priority.multiplier = multiplier
    expect(validateServiceTierConfig(config)).toBe('invalid_multiplier')
  })

  it.each([0.01, 1, 100])('accepts boundary multiplier %s', multiplier => {
    const config = defaultServiceTierConfig()
    config.flex.multiplier = multiplier
    expect(validateServiceTierConfig(config)).toBeNull()
  })
})
