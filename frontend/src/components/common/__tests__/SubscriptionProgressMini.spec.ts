import { enableAutoUnmount, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import SubscriptionProgressMini from '../SubscriptionProgressMini.vue'
const store = vi.hoisted(() => ({ activeSubscriptions: [] as unknown[], hasActiveSubscriptions: true, fetchActiveSubscriptions: vi.fn().mockResolvedValue(undefined) }))
vi.mock('@/stores', () => ({ useSubscriptionStore: () => store }))
vi.mock('@/utils/featureFlags', () => ({ FeatureFlags: { subscription: 'subscription' }, isFeatureFlagEnabled: () => true }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
beforeEach(() => { vi.useFakeTimers(); vi.setSystemTime(new Date(2026, 8, 22, 12)) })
afterEach(() => vi.useRealTimers())
describe('subscription expiry calendar labels', () => {
  it.each([
    [new Date(2026, 8, 22, 18), 'expiresToday'],
    [new Date(2026, 8, 23, 18), 'expiresTomorrow'],
    [new Date(2026, 8, 22, 12), 'expired'],
    [new Date(2026, 8, 25, 12), 'daysRemaining'],
  ])('labels %s as %s', async (expires, label) => {
    store.activeSubscriptions = [{ id: 1, group_id: 1, expires_at: expires.toISOString(), group: { name: 'Plan' } }]
    const w = mount(SubscriptionProgressMini, { global: { stubs: { Icon: true, RouterLink: true } } })
    await w.get('button').trigger('click')
    expect(w.text()).toContain('subscriptionProgress.' + label)
  })
})
