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
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key} ${JSON.stringify(params)}` : key
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
  subscription_tier_raw: 'Muse Code Power Usage',
  subscription_tier_id: '27681631238169137'
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
    const tier = wrapper.get('[data-testid="meta-usage-tier"]')
    expect(tier.text()).toBe('Muse Code Power Usage')
    expect(tier.attributes('title')).toContain('27681631238169137')
  })

  it('keeps an unnamed tier id out of the badge text', async () => {
    getUsage.mockResolvedValue({
      ...sampledUsage(),
      subscription_tier_raw: undefined,
      subscription_tier_id: '28367280949604159'
    })
    const wrapper = mount(MetaUsageCell, { props: { account: account() } })
    await flushPromises()

    const tier = wrapper.get('[data-testid="meta-usage-tier"]')
    expect(tier.text()).toBe('admin.accounts.metaUsage.tierUnknown')
    expect(tier.attributes('title')).toContain('28367280949604159')
    expect(wrapper.text()).not.toContain('28367280949604159')
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

  it('queries the upstream actively and renders the windows it returns', async () => {
    getUsage
      .mockResolvedValueOnce({
        updated_at: null,
        source: 'passive',
        five_hour: { utilization: 0, resets_at: null, remaining_seconds: 0 },
        seven_day: null,
        seven_day_sonnet: null
      } as unknown as AccountUsageInfo)
      .mockResolvedValueOnce({ ...sampledUsage(), source: 'active' })
    const wrapper = mount(MetaUsageCell, { props: { account: account() } })
    await flushPromises()
    expect(wrapper.findAllComponents(UsageProgressBar)).toHaveLength(0)

    await wrapper.get('[data-testid="meta-usage-refresh"]').trigger('click')
    await flushPromises()

    expect(getUsage).toHaveBeenLastCalledWith(7, 'active', true)
    expect(wrapper.findAllComponents(UsageProgressBar).map((bar) => bar.props('label'))).toEqual(['5h', '7d'])
    expect(wrapper.text()).toContain('admin.accounts.metaUsage.activeSampled')
  })

  it('shows the upstream error when the active query fails', async () => {
    getUsage
      .mockResolvedValueOnce(sampledUsage())
      .mockResolvedValueOnce({ ...sampledUsage(), error: 'meta usage probe returned 429: quota' })
    const wrapper = mount(MetaUsageCell, { props: { account: account() } })
    await flushPromises()
    await wrapper.get('[data-testid="meta-usage-refresh"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="meta-usage-error"]').text()).toContain('429')
  })
})
