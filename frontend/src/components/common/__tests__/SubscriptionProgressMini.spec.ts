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
describe('subscription details panel', () => {
  it('exposes the open state and pins the panel inside the viewport on phones', async () => {
    store.activeSubscriptions = [{ id: 1, group_id: 1, group: { name: 'Plan' } }]
    const w = mount(SubscriptionProgressMini, { global: { stubs: { Icon: true, RouterLink: true } } })
    const trigger = w.get('button')
    expect(trigger.attributes('aria-expanded')).toBe('false')
    await trigger.trigger('click')
    expect(trigger.attributes('aria-expanded')).toBe('true')
    const panel = w.get('.shadow-overlay').classes()
    // 375px 下 absolute right-0 w-[340px] 会越过左边缘；小屏改为贴视口两侧，sm 起恢复原来的下拉定位。
    expect(panel).toEqual(expect.arrayContaining(['fixed', 'inset-x-2', 'sm:absolute', 'sm:right-0', 'sm:w-[340px]']))
    expect(panel).not.toContain('w-[340px]')
  })
})
