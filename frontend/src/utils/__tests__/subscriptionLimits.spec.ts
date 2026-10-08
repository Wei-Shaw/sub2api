import { describe, expect, it } from 'vitest'
import type { UserSubscription } from '@/types'
import {
  getEffectiveSubscriptionLimit,
  hasAnyEffectiveSubscriptionLimit,
  parseOptionalSubscriptionLimit
} from '@/utils/subscriptionLimits'

function subscription(overrides: Partial<UserSubscription> = {}): UserSubscription {
  return {
    id: 1,
    user_id: 2,
    group_id: 3,
    status: 'active',
    starts_at: '2026-08-14T00:00:00Z',
    expires_at: '2026-09-14T00:00:00Z',
    daily_usage_usd: 0,
    weekly_usage_usd: 0,
    monthly_usage_usd: 0,
    daily_window_start: null,
    weekly_window_start: null,
    monthly_window_start: null,
    created_at: '2026-08-14T00:00:00Z',
    updated_at: '2026-08-14T00:00:00Z',
    ...overrides
  }
}

describe('subscription limits', () => {
  it('parses values emitted by number inputs', () => {
    expect(parseOptionalSubscriptionLimit(800)).toBe(800)
    expect(parseOptionalSubscriptionLimit(' 800 ')).toBe(800)
    expect(parseOptionalSubscriptionLimit('')).toBeNull()
    expect(parseOptionalSubscriptionLimit(null)).toBeNull()
  })

  it('rejects non-positive and non-numeric values', () => {
    expect(() => parseOptionalSubscriptionLimit(0)).toThrow(RangeError)
    expect(() => parseOptionalSubscriptionLimit('invalid')).toThrow(RangeError)
  })

  it('prefers the user override', () => {
    const sub = subscription({
      daily_limit_usd: 10,
      group: { daily_limit_usd: 100 } as UserSubscription['group']
    })

    expect(getEffectiveSubscriptionLimit(sub, 'daily')).toBe(10)
  })

  it('falls back to the group limit', () => {
    const sub = subscription({
      group: { weekly_limit_usd: 50 } as UserSubscription['group']
    })

    expect(getEffectiveSubscriptionLimit(sub, 'weekly')).toBe(50)
    expect(hasAnyEffectiveSubscriptionLimit(sub)).toBe(true)
  })

  it('reports unlimited when neither level has a positive limit', () => {
    const sub = subscription({
      monthly_limit_usd: null,
      group: { monthly_limit_usd: null } as UserSubscription['group']
    })

    expect(getEffectiveSubscriptionLimit(sub, 'monthly')).toBeNull()
    expect(hasAnyEffectiveSubscriptionLimit(sub)).toBe(false)
  })
})
