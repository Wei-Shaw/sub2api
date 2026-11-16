import { beforeEach, describe, expect, it } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAppStore } from '@/stores/app'
import { formatQuotaPercentage, useQuotaUsageDisplay, useUnitPrice } from '../useSubscriptionUnit'

describe('subscription quota display', () => {
 beforeEach(() => setActivePinia(createPinia()))
 it('uses independent display modes for groups on the same page', () => {
  const store = useAppStore()
  store.cachedPublicSettings = { subscription_units: [{ id: 'credits', name: 'Credits', units_per_usd: 100 }] } as typeof store.cachedPublicSettings
  const format = useQuotaUsageDisplay()
  const percentageGroup = { pricing_unit: 'credits', subscription_usage_display: 'percentage' as const }
  const amountGroup = { pricing_unit: 'credits', subscription_usage_display: 'amount' as const }
  expect(format(0.35, 1, percentageGroup)).toBe('35%')
  expect(format(0.35, 1, amountGroup)).toBe('35 Credits / 100 Credits')
  expect(format(0.35, null, percentageGroup)).toBe('—')
  expect(format(0.35, 1, { pricing_unit: 'usd' })).toBe('$0.35 / $1')
 })
 it('handles unused, over-limit and unbounded quotas', () => {
  expect(formatQuotaPercentage(0, 100)).toBe('0%')
  expect(formatQuotaPercentage(1, 3)).toBe('33.3%')
  expect(formatQuotaPercentage(125, 100)).toBe('125%')
  expect(formatQuotaPercentage(1, 0)).toBe('—')
 })
 it('converts request and token prices for multiple units', () => {
  const store = useAppStore()
  store.cachedPublicSettings = { subscription_units: [{ id: 'credits', name: 'Credits', units_per_usd: 100 }] } as typeof store.cachedPublicSettings
  const price = useUnitPrice()
  expect(price(0.15, 1, ['usd', 'credits'])).toBe('$0.15 · 15 Credits')
  expect(price(0.000003, 1000000, ['credits'])).toBe('300 Credits')
 })
})
