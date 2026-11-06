import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import AccountUsageCell from '../AccountUsageCell.vue'
import type { Account } from '@/types'

const { getUsage } = vi.hoisted(() => ({
  getUsage: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getUsage
    }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      locale: { value: 'en' }
    })
  }
})

function makeAccount(overrides: Partial<Account>): Account {
  return {
    id: 1,
    name: 'antigravity-account',
    platform: 'antigravity',
    type: 'oauth',
    proxy_id: null,
    concurrency: 10,
    priority: 1,
    status: 'active',
    error_message: null,
    last_used_at: null,
    expires_at: null,
    auto_pause_on_expired: true,
    created_at: '2026-11-04T00:00:00Z',
    updated_at: '2026-11-04T00:00:00Z',
    schedulable: true,
    rate_limited_at: null,
    rate_limit_reset_at: null,
    overload_until: null,
    temp_unschedulable_until: null,
    temp_unschedulable_reason: null,
    session_window_start: null,
    session_window_end: null,
    session_window_status: null,
    ...overrides
  }
}

async function expandModels(wrapper: VueWrapper) {
  for (const button of wrapper.findAll('[data-test="quota-model-toggle"][aria-expanded="false"]')) {
    await button.trigger('click')
  }
}

describe('AccountUsageCell Antigravity OAuth', () => {
  beforeEach(() => {
    getUsage.mockReset()
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: vi.fn().mockImplementation(() => ({
        matches: true,
        media: '(min-width: 768px)',
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn()
      }))
    })
  })

  it('keeps Gemini and Argon pools separate from local 5h and 7d usage', async () => {
    getUsage.mockResolvedValue({
      antigravity_quota_groups: [
        { name: 'Gemini Models', buckets: [
          { id: 'gemini-5h', window: '5h', utilization: 16, resets_at: null },
          { id: 'gemini-weekly', window: 'weekly', utilization: 25, resets_at: null }
        ] },
        { name: 'Gemini 4 Argon', buckets: [
          { id: 'gemini-argon-wtu-weekly', window: 'weekly', utilization: 42, resets_at: null }
        ] }
      ],
      five_hour: {
        utilization: 42,
        resets_at: '2026-11-04T16:00:00Z',
        remaining_seconds: 14400,
        window_stats: {
          requests: 38,
          tokens: 1_250_000,
          cost: 3.5,
          standard_cost: 3.5,
          user_cost: 3.5
        }
      },
      seven_day: {
        utilization: 25,
        resets_at: '2026-11-10T00:00:00Z',
        remaining_seconds: 432000,
        window_stats: {
          requests: 420,
          tokens: 18_500_000,
          cost: 40,
          standard_cost: 40,
          user_cost: 40
        }
      },
      antigravity_quota: {
        'gemini-4-argon-high': {
          utilization: 42,
          reset_time: '2026-11-04T16:00:00Z'
        },
        'gemini-3.8-flash-high': {
          utilization: 16,
          reset_time: '2026-11-04T15:30:00Z'
        },
        'claude-opus-5-5-thinking': {
          utilization: 8,
          reset_time: '2026-11-04T17:00:00Z'
        }
      }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 8101 })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'resetsAt', 'windowStats', 'estimatedTotalCost'],
            template:
              '<div class="usage-bar">{{ label }}|{{ utilization }}|{{ resetsAt }}|{{ windowStats?.requests ?? "no-stats" }}|{{ estimatedTotalCost ?? "no-est" }}</div>'
          },
          AccountQuotaInfo: true
        }
      }
    })

    await flushPromises()

    const groups = wrapper.findAll('[data-test="quota-group"]')
    expect(groups).toHaveLength(2)
    expect(groups[0].text()).toContain('antigravityQuota.fiveHour|16')
    expect(groups[0].text()).toContain('antigravityQuota.weekly|25')
    expect(groups[1].text()).toContain('antigravityQuota.weekly|42')
    expect(groups[1].text()).not.toContain('antigravityQuota.fiveHour')
    expect(groups[0].text()).not.toContain('38 req')
    expect(wrapper.text()).not.toContain('38 req')
    expect(wrapper.text()).not.toContain('420 req')
    wrapper.unmount()
  })

  it('does not render fake 0% 7d progress bar when upstream 7d quota is unknown', async () => {
    getUsage.mockResolvedValue({
      five_hour: {
        utilization: 30,
        resets_at: '2026-11-04T16:00:00Z',
        remaining_seconds: 14400,
        window_stats: {
          requests: 10,
          tokens: 200_000,
          cost: 1.2,
          standard_cost: 1.2,
          user_cost: 1.2
        }
      },
      seven_day: {
        utilization: 0,
        resets_at: null,
        remaining_seconds: 0,
        window_stats: {
          requests: 95,
          tokens: 4_500_000,
          cost: 15.6,
          standard_cost: 15.6,
          user_cost: 12.3
        }
      },
      antigravity_quota: {
        'gemini-4-argon': {
          utilization: 30,
          reset_time: '2026-11-04T16:00:00Z'
        }
      }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 8103 })
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

    await expandModels(wrapper)
    expect(wrapper.get('[data-model="gemini-4-argon"]').text()).toContain('30%')
    expect(wrapper.text()).not.toContain('7d|0')
    expect(wrapper.text()).not.toContain('95 req')
    expect(wrapper.text()).not.toContain('A $15.60')
    expect(wrapper.text()).not.toContain('U $12.30')
    wrapper.unmount()
  })

  it('keeps active query available on degraded upstream error without local stats', async () => {
    getUsage
      .mockResolvedValueOnce({
        error: 'usage API error: HTTP 429',
        error_code: 'rate_limited',
        five_hour: {
          utilization: 0,
          resets_at: null,
          remaining_seconds: 0,
          window_stats: {
            requests: 12,
            tokens: 340_000,
            cost: 2.1,
            standard_cost: 2.1,
            user_cost: 2.1
          }
        }
      })
      .mockResolvedValueOnce({
        antigravity_quota: {
          'gemini-4-argon': {
            utilization: 55,
            reset_time: '2026-11-04T16:00:00Z'
          }
        }
      })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 8104 })
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
    expect(wrapper.text()).toContain('admin.accounts.rateLimited')
    expect(wrapper.text()).not.toContain('12 req')

    await wrapper.get('[data-test="antigravity-active-query"]').trigger('click')
    await flushPromises()

    expect(getUsage).toHaveBeenLastCalledWith(8104, 'active', true)
    await expandModels(wrapper)
    expect(wrapper.get('[data-model="gemini-4-argon"]').text()).toContain('55%')
    wrapper.unmount()
  })

  it('does not leak cached usage across remounted component instances with the same account id', async () => {
    getUsage.mockResolvedValueOnce({
      five_hour: { utilization: 0, resets_at: null, remaining_seconds: 0 }
    })
    const first = mount(AccountUsageCell, {
      props: { account: makeAccount({ id: 2004, platform: 'openai', type: 'oauth' }) },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}</div>'
          },
          OpenAIQuotaResetCell: true
        }
      }
    })
    await flushPromises()
    expect(first.text()).toContain('5h|0')
    first.unmount()

    getUsage.mockResolvedValueOnce({
      five_hour: { utilization: 100, resets_at: '2026-11-04T16:00:00Z', remaining_seconds: 3600 }
    })
    const second = mount(AccountUsageCell, {
      props: { account: makeAccount({ id: 2004, platform: 'openai', type: 'oauth' }) },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}</div>'
          },
          OpenAIQuotaResetCell: true
        }
      }
    })
    await flushPromises()
    expect(getUsage).toHaveBeenCalledTimes(2)
    expect(second.text()).toContain('5h|100')
  })

  it('ignores a response for a row whose account has changed', async () => {
    let resolveOld!: (value: unknown) => void
    getUsage.mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve }))
      .mockResolvedValueOnce({ antigravity_quota: { 'new-account': { utilization: 20 } } })
    const wrapper = mount(AccountUsageCell, {
      props: { account: makeAccount({ id: 3001 }) },
      global: { stubs: { UsageProgressBar: true, AccountQuotaInfo: true } }
    })
    await flushPromises()
    await wrapper.setProps({ account: makeAccount({ id: 3002 }) })
    await flushPromises()
    await expandModels(wrapper)
    expect(wrapper.text()).toContain('new-account')
    resolveOld({ antigravity_quota: { 'old-account': { utilization: 90 } } })
    await flushPromises()
    expect(wrapper.text()).not.toContain('old-account')
    expect(wrapper.text()).toContain('new-account')
    wrapper.unmount()
  })

  it('clears query loading and ignores stale results when a refresh supersedes a query', async () => {
    let resolveActive!: (value: unknown) => void
    getUsage.mockResolvedValueOnce({})
      .mockImplementationOnce(() => new Promise(resolve => { resolveActive = resolve }))
      .mockResolvedValueOnce({ antigravity_quota: { fresh: { utilization: 20 } } })
    const wrapper = mount(AccountUsageCell, {
      props: { account: makeAccount({ id: 3003 }), manualRefreshToken: 0 },
      global: { stubs: { UsageProgressBar: true, AccountQuotaInfo: true } }
    })
    await flushPromises()
    await wrapper.get('[data-test="antigravity-active-query"]').trigger('click')
    expect(wrapper.get('[data-test="antigravity-active-query"]').attributes('disabled')).toBeDefined()
    await wrapper.setProps({ manualRefreshToken: 1 })
    await flushPromises()
    await expandModels(wrapper)
    expect(wrapper.get('[data-test="antigravity-active-query"]').attributes('disabled')).toBeUndefined()
    resolveActive({ antigravity_quota: { stale: { utilization: 90 } } })
    await flushPromises()
    expect(wrapper.text()).toContain('fresh')
    expect(wrapper.text()).not.toContain('stale')
    wrapper.unmount()
  })

  it.each([{ is_forbidden: true }, { needs_reauth: true }])('keeps retry available after an auth failure: %j', async flags => {
    getUsage.mockResolvedValueOnce({
      ...flags,
      five_hour: { window_stats: { requests: 2, tokens: 100, cost: 0.5 } }
    })
    const wrapper = mount(AccountUsageCell, {
      props: { account: makeAccount({ id: 3004 }) },
      global: { stubs: { UsageProgressBar: true, AccountQuotaInfo: true } }
    })
    await flushPromises()
    expect(wrapper.find('[data-test="antigravity-active-query"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('2 req')
    wrapper.unmount()
  })

  it('forces a backend refresh when the parent manual refresh token changes', async () => {
    getUsage.mockResolvedValue({ antigravity_quota_groups: [] })
    const wrapper = mount(AccountUsageCell, {
      props: { account: makeAccount({ id: 3005 }), manualRefreshToken: 0 }
    })
    await flushPromises()
    await wrapper.setProps({ manualRefreshToken: 1 })
    await flushPromises()
    expect(getUsage).toHaveBeenLastCalledWith(3005, undefined, true)
    wrapper.unmount()
  })

  it.each([
    ['g1-ultra-lite-tier', 'Ultra 5x', 'text-purple-700'],
    ['g1-ultra-tier', 'Ultra 20x', 'text-amber-800']
  ])('updates both the usage badge and the account row from the precise tier %s', async (tier, label, color) => {
    getUsage.mockResolvedValueOnce({ subscription_tier_raw: tier })
    const account = makeAccount({ id: 4100, credentials: { plan_type: 'Ultra', project_id: 'preserve-project' } })
    const wrapper = mount(AccountUsageCell, { props: { account } })
    await flushPromises()
    expect(wrapper.text()).toContain(label)
    expect(wrapper.find(`.${color}`).exists()).toBe(true)
    const updated = wrapper.emitted('account-updated')?.[0]?.[0] as Account
    expect(updated.id).toBe(account.id)
    expect(updated.credentials).toEqual({ plan_type: tier, project_id: 'preserve-project' })
    expect(account.credentials?.plan_type).toBe('Ultra')
    wrapper.unmount()
  })
})
