import { getUsage, makeAccount } from './accountUsageCellTestFixtures'
import { describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccountUsageCell from '../AccountUsageCell.vue'

describe('AccountUsageCell Grok billing', () => {
  it('Grok OAuth compact UI drops local chips and header quota bars', async () => {
    getUsage.mockResolvedValue({
      grok_local_usage: {
        requests: 4,
        tokens: 1200,
        cost: 0.12,
        standard_cost: 0.12,
        user_cost: 0.34
      },
      grok_request_quota: {
        limit: 10,
        remaining: -2,
        reset_at: '2026-07-09T16:00:00Z'
      },
      grok_quota_snapshot_state: 'observed'
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 3861, platform: 'grok', type: 'oauth', extra: {} })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'resetsAt', 'color'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}|{{ resetsAt }}</div>'
          },
          AccountQuotaInfo: true
        }
      }
    })

    await flushPromises()
    expect(getUsage).toHaveBeenCalledWith(3861)
    expect(wrapper.text()).not.toContain('4 req')
    expect(wrapper.text()).not.toContain('admin.accounts.usageWindow.grokRequests|')
  })

  it('Grok paid monthly limits show 30d bar without free 24h', async () => {
    getUsage.mockResolvedValue({
      grok_billing: {
        period_type: 'weekly',
        usage_percent: null,
        used_percent: 12,
        monthly_limit_cents: 25_000,
        used_cents: 3_000,
        plan: ''
      },
      grok_entitlement_status: 'free',
      grok_token_quota: { limit: 1_000, remaining: 250 }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 4402, platform: 'grok', type: 'oauth', extra: {} })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}</div>'
          },
          AccountQuotaInfo: true
        }
      }
    })

    await flushPromises()
    expect(wrapper.text()).toContain('30d|')
    expect(wrapper.text()).not.toContain('24h|')
  })

  it('Grok OAuth uses the official weekly billing percentage when available', async () => {
    getUsage.mockResolvedValue({
      grok_billing: {
        period_type: 'weekly',
        usage_percent: 37,
        period_end: '2026-07-16T03:25:00Z',
        plan: 'SuperGrok'
      },
      grok_local_usage: {
        requests: 5,
        tokens: 2_200_000,
        cost: 4.42,
        standard_cost: 4.42,
        user_cost: 0.44
      },
      grok_request_quota: { limit: 100, remaining: 100 },
      grok_token_quota: { limit: 2_000_000, remaining: 2_000_000 }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 4201, platform: 'grok', type: 'oauth', extra: {} })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'resetsAt', 'remainingCapacity'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}|{{ resetsAt }}|{{ remainingCapacity }}</div>'
          },
          AccountQuotaInfo: true,
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('7d|37|2026-07-16T03:25:00Z')
    expect(wrapper.text()).not.toContain('admin.accounts.usageWindow.grokRequests|')
    expect(wrapper.text()).not.toContain('admin.accounts.usageWindow.grokTokens|')
    expect(wrapper.text()).not.toContain('2M|')
  })

  it.each([
    { tokens: 0, expected: 0 },
    { tokens: 500_000, expected: 50 },
    { tokens: 1_000_000, expected: 100 },
    { tokens: 1_100_000, expected: 100 }
  ])('Grok Free derives its 1M quota from local tokens: $tokens -> $expected%', async ({ tokens, expected }) => {
    getUsage.mockResolvedValue({
      grok_free_token_limit: 1_000_000,
      grok_billing: {
        period_type: 'weekly',
        usage_percent: null,
        plan: ''
      },
      grok_local_usage_24h: {
        requests: 5,
        tokens,
        cost: 0,
        standard_cost: 0,
        user_cost: 0
      },
      grok_request_quota: { limit: 100, remaining: 100 },
      grok_token_quota: { limit: 1_000_000, remaining: 1_000_000 }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 4300 + expected, platform: 'grok', type: 'oauth', extra: {} })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}</div>'
          },
          AccountQuotaInfo: true,
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain(`24h|${expected}`)
    expect(wrapper.findAll('.usage-bar')).toHaveLength(1)
    expect(wrapper.text()).not.toContain('admin.accounts.usageWindow.grokRequests|')
    expect(wrapper.text()).not.toContain('admin.accounts.usageWindow.grokTokens|')
    expect(wrapper.text()).not.toContain('7d|')
  })

  it('Grok SuperGrok Lite stays on paid 7d bar, not free 24h', async () => {
    getUsage.mockResolvedValue({
      subscription_tier: 'supergrok_lite',
      grok_billing: {
        period_type: 'weekly',
        plan: 'SuperGrok',
        usage_percent: 20
      },
      grok_local_usage_24h: {
        requests: 1,
        tokens: 100,
        cost: 0,
        standard_cost: 0
      }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 4405, platform: 'grok', type: 'oauth', extra: {} })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}</div>'
          },
          AccountQuotaInfo: true
        }
      }
    })

    await flushPromises()
    expect(wrapper.text()).toContain('7d|')
    expect(wrapper.text()).not.toContain('24h|')
  })

  it('Grok SuperGrok and Heavy bars show period-aligned local 7d and 30d usage', async () => {
    getUsage.mockResolvedValue({
      subscription_tier: 'SuperGrok Heavy',
      grok_billing: {
        period_type: 'weekly',
        usage_percent: 37,
        used_percent: 12,
        monthly_limit_cents: 150_000,
        period_end: '2026-07-16T03:25:00Z',
        billing_period_end: '2026-08-01T00:00:00Z',
        plan: 'SuperGrok Heavy'
      },
      grok_local_usage: {
        requests: 1,
        tokens: 99,
        cost: 0,
        standard_cost: 0
      },
      grok_local_usage_24h: {
        requests: 2,
        tokens: 100,
        cost: 0,
        standard_cost: 0
      },
      grok_local_usage_7d: {
        requests: 8,
        tokens: 2_200_000,
        cost: 4.42,
        standard_cost: 4.42
      },
      grok_local_usage_monthly: {
        requests: 20,
        tokens: 8_000_000,
        cost: 18.5,
        standard_cost: 18.5
      }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 4411, platform: 'grok', type: 'oauth', extra: {} })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'windowStats'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}|{{ windowStats?.tokens }}</div>'
          },
          AccountQuotaInfo: true
        }
      }
    })

    await flushPromises()
    expect(wrapper.text()).toContain('7d|37|2200000')
    expect(wrapper.text()).toContain('30d|12|8000000')
    expect(wrapper.text()).not.toContain('|99')
    expect(wrapper.text()).not.toContain('|100')
    expect(wrapper.text()).not.toContain('24h|')
  })

  it('Grok paid bars fall back to official seven_day and thirty_day window_stats', async () => {
    getUsage.mockResolvedValue({
      subscription_tier: 'SuperGrok',
      grok_billing: {
        period_type: 'weekly',
        usage_percent: 20,
        used_percent: 8,
        monthly_limit_cents: 25_000,
        plan: 'SuperGrok'
      },
      seven_day: {
        utilization: 20,
        window_stats: {
          requests: 6,
          tokens: 1_500_000,
          cost: 3.1,
          standard_cost: 3.1
        }
      },
      thirty_day: {
        utilization: 8,
        window_stats: {
          requests: 14,
          tokens: 4_400_000,
          cost: 9.2,
          standard_cost: 9.2
        }
      }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 4412, platform: 'grok', type: 'oauth', extra: {} })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'windowStats'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}|{{ windowStats?.tokens }}</div>'
          },
          AccountQuotaInfo: true
        }
      }
    })

    await flushPromises()
    expect(wrapper.text()).toContain('7d|20|1500000')
    expect(wrapper.text()).toContain('30d|8|4400000')
  })

  it('Grok paid hides zero prepaid and hides used/limit when monthly limit is 0', async () => {
    getUsage.mockResolvedValue({
      subscription_tier: 'SuperGrok',
      grok_billing: {
        period_type: 'weekly',
        usage_percent: 20,
        prepaid_balance: 0,
        monthly_limit: 0,
        monthly_used: 3.5,
        plan: 'SuperGrok'
      }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 4413, platform: 'grok', type: 'oauth', extra: {} })
      },
      global: {
        stubs: {
          UsageProgressBar: true,
          AccountQuotaInfo: true
        }
      }
    })

    await flushPromises()
    expect(wrapper.text()).not.toContain('admin.accounts.usageWindow.grokPrepaid')
    expect(wrapper.text()).not.toContain('admin.accounts.usageWindow.grokUsed')
    expect(wrapper.text()).not.toContain('3.5/0')
  })

  it('Grok paid shows used/limit without prepaid, and prepaid without a zero monthly limit', async () => {
    getUsage.mockResolvedValueOnce({
      subscription_tier: 'SuperGrok',
      grok_billing: {
        period_type: 'weekly',
        usage_percent: 20,
        monthly_limit: 25,
        monthly_used: 3.5,
        plan: 'SuperGrok'
      }
    })

    const usedOnly = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 4414, platform: 'grok', type: 'oauth', extra: {} })
      },
      global: {
        stubs: {
          UsageProgressBar: true,
          AccountQuotaInfo: true
        }
      }
    })
    await flushPromises()
    expect(usedOnly.text()).not.toContain('admin.accounts.usageWindow.grokPrepaid')
    expect(usedOnly.text()).toContain('admin.accounts.usageWindow.grokUsed')
    expect(usedOnly.text()).toContain('3.50/25.0')

    getUsage.mockResolvedValueOnce({
      subscription_tier: 'SuperGrok Heavy',
      grok_billing: {
        period_type: 'weekly',
        usage_percent: 20,
        prepaid_balance: 12.5,
        monthly_limit: 0,
        monthly_used: 8,
        plan: 'SuperGrok Heavy'
      }
    })
    const prepaidOnly = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 4415, platform: 'grok', type: 'oauth', extra: {} })
      },
      global: {
        stubs: {
          UsageProgressBar: true,
          AccountQuotaInfo: true
        }
      }
    })
    await flushPromises()
    expect(prepaidOnly.text()).toContain('admin.accounts.usageWindow.grokPrepaid')
    expect(prepaidOnly.text()).toContain('$12.5')
    expect(prepaidOnly.text()).not.toContain('admin.accounts.usageWindow.grokUsed')
    expect(prepaidOnly.text()).not.toContain('8.00/0')
  })

})
