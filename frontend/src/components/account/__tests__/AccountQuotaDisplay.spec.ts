import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, reactive } from 'vue'
import type { Account, AccountQuotaResult } from '@/types'
import AccountQuotaDisplay from '../AccountQuotaDisplay.vue'
import { fetchAccountQuota } from '../accountQuotaCache'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

vi.mock('../accountQuotaCache', () => ({
  fetchAccountQuota: vi.fn()
}))

const initialQuota: AccountQuotaResult = {
  mode: 'upstream',
  provider: 'sub2api',
  status: 'ok',
  source: 'active',
  metrics: [
    {
      key: 'daily',
      label: 'Daily',
      period: 'day',
      unit: 'USD',
      limit: 10,
      used: 2,
      remaining: 8,
      utilization: 20,
      reset_at: '2030-01-11T00:00:00Z'
    },
    {
      key: 'weekly',
      label: 'Weekly',
      period: 'week',
      unit: 'USD',
      limit: 0,
      used: 0,
      remaining: 0
    }
  ]
}

beforeEach(() => {
  vi.mocked(fetchAccountQuota).mockReset().mockResolvedValue(initialQuota)
})

describe('AccountQuotaDisplay', () => {
  it('keeps today usage stats above upstream quota windows', async () => {
    const wrapper = mount(AccountQuotaDisplay, {
      props: {
        account: { id: 7, extra: { quota_provider: 'sub2api' } } as Account,
        todayStats: {
          requests: 12,
          tokens: 3500,
          cost: 1.25,
          user_cost: 2.5
        }
      },
      global: {
        stubs: {
          Icon: true,
          QuotaBadge: true,
          CapacityBadge: true,
          UsageProgressBar: {
            props: ['label', 'utilization', 'resetsAt', 'color'],
            template: '<div data-testid="quota-window">{{ label }}|{{ utilization }}|{{ resetsAt }}</div>'
          }
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('12 req')
    expect(wrapper.text()).toContain('3.5K')
    expect(wrapper.text()).toContain('A $1.25')
    expect(wrapper.text()).toContain('U $2.50')
    expect(wrapper.get('[data-testid="quota-window"]').text()).toContain('1d|20|2030-01-11T00:00:00Z')
    expect(wrapper.emitted('loaded')).toHaveLength(1)
  })

  it('hides zero limits and syncs refreshed quota into the compact capacity display', async () => {
    const refreshedQuota: AccountQuotaResult = {
      ...initialQuota,
      metrics: [
        { ...initialQuota.metrics[0], limit: 20, used: 4, remaining: 16, utilization: 20 },
        initialQuota.metrics[1]
      ]
    }
    vi.mocked(fetchAccountQuota)
      .mockResolvedValueOnce(initialQuota)
      .mockResolvedValueOnce(initialQuota)
      .mockResolvedValueOnce(refreshedQuota)

    const account = reactive({
      id: 7,
      extra: {
        quota_provider: 'sub2api',
        upstream_quota_snapshot: initialQuota
      }
    }) as Account
    const Harness = defineComponent({
      setup() {
        const handleLoaded = (quota: AccountQuotaResult) => {
          account.extra = { ...account.extra, upstream_quota_snapshot: quota }
        }
        return () => h('div', [
          h(AccountQuotaDisplay, { account, compact: true, onLoaded: handleLoaded }),
          h(AccountQuotaDisplay, { account, onLoaded: handleLoaded })
        ])
      }
    })
    const wrapper = mount(Harness, {
      global: {
        stubs: {
          Icon: true,
          CapacityBadge: true,
          UsageProgressBar: true,
          QuotaBadge: {
            props: ['label', 'used', 'limit'],
            template: '<div data-testid="quota-badge">{{ label }}|{{ used }}|{{ limit }}</div>'
          }
        }
      }
    })

    await flushPromises()

    expect(wrapper.findAll('[data-testid="quota-badge"]')).toHaveLength(1)
    expect(wrapper.get('[data-testid="quota-badge"]').text()).toBe('D|2|10')
    expect(wrapper.text()).not.toContain('W|')

    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="quota-badge"]').text()).toBe('D|4|20')
    expect(wrapper.text()).not.toContain('W|')
  })

  it('shows balance-only quota in usage but not in compact capacity', async () => {
    const balanceQuota: AccountQuotaResult = {
      mode: 'upstream',
      provider: 'newapi',
      status: 'ok',
      source: 'active',
      metrics: [{
        key: 'balance',
        label: 'Account balance',
        period: 'total',
        unit: 'USD',
        remaining: 25
      }]
    }
    vi.mocked(fetchAccountQuota).mockResolvedValue(balanceQuota)
    const account = {
      id: 8,
      extra: { quota_provider: 'newapi', upstream_quota_snapshot: balanceQuota }
    } as Account
    const global = {
      stubs: {
        Icon: true,
        QuotaBadge: true,
        UsageProgressBar: true
      }
    }

    const compact = mount(AccountQuotaDisplay, { props: { account, compact: true }, global })
    const usage = mount(AccountQuotaDisplay, { props: { account }, global })
    await flushPromises()

    expect(compact.text()).not.toContain('USD 25.00')
    expect(usage.text()).toContain('admin.accounts.quotaSource.accountBalance：USD 25.00')
    expect(usage.text()).not.toContain('∞')
    expect(usage.get('[data-testid="account-balance"]').classes()).toContain('text-emerald-700')
  })

  it('does not present a stale zero snapshot as an account balance', async () => {
    const staleQuota: AccountQuotaResult = {
      mode: 'upstream',
      provider: 'newapi',
      status: 'stale',
      source: 'cache',
      error: 'refresh failed',
      metrics: [{
        key: 'balance',
        label: 'Account balance',
        period: 'total',
        unit: 'USD',
        remaining: 0
      }]
    }
    vi.mocked(fetchAccountQuota).mockResolvedValue(staleQuota)
    const wrapper = mount(AccountQuotaDisplay, {
      props: { account: { id: 9, extra: { quota_provider: 'newapi' } } as Account },
      global: {
        stubs: { Icon: true, QuotaBadge: true, UsageProgressBar: true }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('refresh failed')
    expect(wrapper.text()).not.toContain('USD 0.00')
  })

  it('shows balances below 10 in red', async () => {
    const lowBalanceQuota: AccountQuotaResult = {
      mode: 'upstream',
      provider: 'newapi',
      status: 'ok',
      source: 'active',
      metrics: [{
        key: 'balance',
        label: 'Account balance',
        period: 'total',
        unit: 'USD',
        remaining: 9.99
      }]
    }
    vi.mocked(fetchAccountQuota).mockResolvedValue(lowBalanceQuota)
    const wrapper = mount(AccountQuotaDisplay, {
      props: { account: { id: 10, extra: { quota_provider: 'newapi' } } as Account },
      global: {
        stubs: { Icon: true, QuotaBadge: true, UsageProgressBar: true }
      }
    })

    await flushPromises()

    const balance = wrapper.get('[data-testid="account-balance"]')
    expect(balance.text()).toContain('admin.accounts.quotaSource.accountBalance：USD 9.99')
    expect(balance.classes()).toContain('text-red-700')
    expect(balance.classes()).not.toContain('text-emerald-700')
  })
})
