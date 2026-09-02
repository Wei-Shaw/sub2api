import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import TemporaryBalanceCard from '../TemporaryBalanceCard.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, string>) => {
      const labels: Record<string, string> = {
        'dashboard.temporaryBalance': 'Temporary balance',
        'dashboard.temporaryBalanceActive': 'Active until {time}',
        'dashboard.temporaryBalanceExpired': 'Expired',
        'dashboard.temporaryBalanceExpiredAt': 'Expired {time}',
      }
      return (labels[key] ?? key).replace(/\{(\w+)\}/g, (_, name) => params?.[name] ?? '')
    }
  })
}))

describe('TemporaryBalanceCard', () => {
  it('shows the active grant and expiry without replacing permanent balance', () => {
    const wrapper = mount(TemporaryBalanceCard, {
      props: {
        amount: 12.5,
        expiresAt: '2026-09-03T00:00:00Z',
        now: new Date('2026-09-02T12:00:00Z')
      },
      global: { stubs: { Icon: true } }
    })

    expect(wrapper.get('[data-testid="temporary-balance-card"]').text()).toContain('$12.50')
    expect(wrapper.text()).toContain('Active until')
    expect(wrapper.text()).not.toContain('Expired')
  })

  it('labels an expired amount as unavailable', () => {
    const wrapper = mount(TemporaryBalanceCard, {
      props: {
        amount: 5,
        expiresAt: '2026-09-01T00:00:00Z',
        now: new Date('2026-09-02T12:00:00Z')
      },
      global: { stubs: { Icon: true } }
    })

    expect(wrapper.get('[data-testid="temporary-balance-card"]').text()).toContain('$5.00')
    expect(wrapper.text()).toContain('Expired')
    expect(wrapper.get('[data-testid="temporary-balance-card"]').attributes('data-status')).toBe('expired')
  })

  it('updates to expired while the page remains open', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-02T12:00:00Z'))
    const wrapper = mount(TemporaryBalanceCard, {
      props: { amount: 3, expiresAt: '2026-09-02T12:01:00Z' },
      global: { stubs: { Icon: true } }
    })

    expect(wrapper.get('[data-testid="temporary-balance-card"]').attributes('data-status')).toBe('active')
    await vi.advanceTimersByTimeAsync(60_000)
    expect(wrapper.get('[data-testid="temporary-balance-card"]').attributes('data-status')).toBe('expired')

    wrapper.unmount()
    vi.useRealTimers()
  })
})
