import { describe, it, expect } from 'vitest'
import {
  isTokenSubscription,
  windowMetric,
  hasWindowLimit,
  windowPercentage,
  formatWindowUsage,
  isSubscriptionUnlimited,
  progressBarClass,
  progressBarWidth
} from '../subscriptionMetric'
import type { UserSubscription } from '@/types'

function makeSub(overrides: Partial<UserSubscription> = {}): UserSubscription {
  return {
    id: 1,
    user_id: 1,
    group_id: 1,
    status: 'active',
    starts_at: '2026-01-01',
    daily_usage_usd: 0,
    weekly_usage_usd: 0,
    monthly_usage_usd: 0,
    daily_window_start: null,
    weekly_window_start: null,
    monthly_window_start: null,
    created_at: '',
    updated_at: '',
    expires_at: null,
    ...overrides
  } as UserSubscription
}

describe('subscriptionMetric', () => {
  // token 型：配了日 token 限额、已用 30%
  const tokenSub = makeSub({
    daily_usage_tokens: 30000,
    group: { subscription_type: 'subscription_token', daily_limit_tokens: 100000 } as UserSubscription['group']
  })
  // USD 型：配了日 USD 限额、已用 50%
  const usdSub = makeSub({
    daily_usage_usd: 5,
    group: { subscription_type: 'subscription', daily_limit_usd: 10 } as UserSubscription['group']
  })

  it('isTokenSubscription 按 group.subscription_type 判断', () => {
    expect(isTokenSubscription(tokenSub)).toBe(true)
    expect(isTokenSubscription(usdSub)).toBe(false)
  })

  it('windowMetric token 型取 *_usage_tokens/*_limit_tokens', () => {
    expect(windowMetric(tokenSub, 'daily')).toEqual({ used: 30000, limit: 100000 })
    expect(windowMetric(tokenSub, 'weekly')).toEqual({ used: 0, limit: null })
  })

  it('windowMetric USD 型取 *_usage_usd/*_limit_usd', () => {
    expect(windowMetric(usdSub, 'daily')).toEqual({ used: 5, limit: 10 })
  })

  it('hasWindowLimit / windowPercentage 按对应计量计算', () => {
    expect(hasWindowLimit(tokenSub, 'daily')).toBe(true)
    expect(hasWindowLimit(tokenSub, 'weekly')).toBe(false)
    expect(windowPercentage(tokenSub, 'daily')).toBe(30)
    expect(windowPercentage(usdSub, 'daily')).toBe(50)
  })

  it('formatWindowUsage：token 型千分位整数，USD 型两位小数 + $', () => {
    expect(formatWindowUsage(tokenSub, 'daily')).toBe('30,000 / 100,000')
    expect(formatWindowUsage(usdSub, 'daily')).toBe('$5.00 / $10.00')
  })

  it('isSubscriptionUnlimited：配了限额不算无限', () => {
    expect(isSubscriptionUnlimited(tokenSub)).toBe(false)
    // token 型什么限额都没配 → 无限
    const tokenNoLimit = makeSub({
      group: { subscription_type: 'subscription_token' } as UserSubscription['group']
    })
    expect(isSubscriptionUnlimited(tokenNoLimit)).toBe(true)
    // USD 型配了 USD 限额 → 不是无限
    expect(isSubscriptionUnlimited(usdSub)).toBe(false)
  })

  it('progressBarClass / progressBarWidth 按百分比返回样式', () => {
    expect(progressBarClass(30)).toBe('bg-green-500')
    expect(progressBarClass(70)).toBe('bg-orange-500')
    expect(progressBarClass(90)).toBe('bg-red-500')
    expect(progressBarWidth(30)).toBe('30%')
    expect(progressBarWidth(150)).toBe('100%')
  })
})
