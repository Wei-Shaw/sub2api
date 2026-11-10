import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import MetaUsageCell from '../MetaUsageCell.vue'
import UsageProgressBar from '../UsageProgressBar.vue'
import type { Account, AccountUsageInfo } from '@/types'

const { getUsage } = vi.hoisted(() => ({ getUsage: vi.fn() }))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: { getUsage }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const account = (): Account => ({
  id: 7,
  name: 'muse-account',
  platform: 'meta',
  type: 'oauth',
  proxy_id: null,
  concurrency: 1,
  priority: 1,
  status: 'active',
  error_message: null,
  last_used_at: null,
  expires_at: null,
  auto_pause_on_expired: false,
  extra: {},
  created_at: '2026-07-01T00:00:00Z',
  updated_at: '2026-07-01T00:00:00Z'
} as unknown as Account)

const sampledUsage = (): AccountUsageInfo => ({
  updated_at: '2026-07-22T12:00:00Z',
  source: 'passive',
  five_hour: { utilization: 25, resets_at: '2026-07-22T17:00:00Z', remaining_seconds: 18000 },
  seven_day: { utilization: 80, resets_at: '2026-07-29T12:00:00Z', remaining_seconds: 600000 },
  seven_day_sonnet: null,
  subscription_tier_raw: 'tier-1'
} as AccountUsageInfo)

describe('MetaUsageCell', () => {
  beforeEach(() => {
    getUsage.mockReset()
  })

  it('renders sampled 5h / 7d windows from the passive endpoint', async () => {
    getUsage.mockResolvedValue(sampledUsage())
    const wrapper = mount(MetaUsageCell, { props: { account: account() } })
    await flushPromises()

    expect(getUsage).toHaveBeenCalledWith(7, 'passive')
    const bars = wrapper.findAllComponents(UsageProgressBar)
    expect(bars.map((bar) => bar.props('label'))).toEqual(['5h', '7d'])
    expect(wrapper.get('[data-testid="meta-usage-window"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="meta-usage-weekly"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('tier-1')
  })

  it('shows an empty state before Meta has sampled usage', async () => {
    getUsage.mockResolvedValue({
      updated_at: null,
      five_hour: { utilization: 0, resets_at: null, remaining_seconds: 0 },
      seven_day: null,
      seven_day_sonnet: null
    } as unknown as AccountUsageInfo)
    const wrapper = mount(MetaUsageCell, { props: { account: account() } })
    await flushPromises()

    expect(wrapper.findAllComponents(UsageProgressBar)).toHaveLength(0)
    expect(wrapper.text()).toContain('-')
  })

  it('re-reads the passive snapshot when the refresh control is used', async () => {
    getUsage.mockResolvedValue(sampledUsage())
    const wrapper = mount(MetaUsageCell, { props: { account: account() } })
    await flushPromises()
    await wrapper.get('[data-testid="meta-usage-refresh"]').trigger('click')
    await flushPromises()

    expect(getUsage).toHaveBeenCalledTimes(2)
  })
})
