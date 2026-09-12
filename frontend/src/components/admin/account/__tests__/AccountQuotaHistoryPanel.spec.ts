import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccountQuotaHistoryPanel from '../AccountQuotaHistoryPanel.vue'
import AccountStatsModal from '../AccountStatsModal.vue'
import type { Account, AccountWindowHistoryResponse, AccountWindowUsageEntry } from '@/types'

const { getWindowHistory, getStats } = vi.hoisted(() => ({ getWindowHistory: vi.fn(), getStats: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { accounts: { getWindowHistory, getStats } } }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const { default: messages } = await import('@/i18n/locales/en')
  return { ...actual, useI18n: () => ({ t: (key: string, values: Record<string, unknown> = {}) => {
    let message: unknown = messages
    for (const part of key.split('.')) message = (message as Record<string, unknown>)?.[part]
    return typeof message === 'string'
      ? message.replace(/\{(\w+)\}/g, (_, name: string) => String(values[name] ?? `{${name}}`))
      : key
  } }) }
})


function entry(overrides: Partial<AccountWindowUsageEntry> = {}): AccountWindowUsageEntry {
  return {
    window_start: '2026-09-01T00:00:00Z', window_end: '2026-09-01T05:00:00Z',
    first_observed_at: '2026-09-01T00:00:00Z', last_sample_at: '2026-09-01T04:55:00Z',
    peak_used_percent: 100, last_used_percent: 100, final_used_percent: 100,
    sample_count: 8, finalized: true, end_reason: 'expired', requests: 40, tokens_total: 1000,
    api_reference_cost: 50, priced_requests: 40, missing_pricing_requests: 0,
    estimated_reference_limit: 50, estimate_reference_cost: 50, estimate_used_percent: 100,
    estimate_observed_at: '2026-09-01T04:55:00Z', quality_flags: [], ...overrides
  }
}

const globalOptions = () => ({
  stubs: {
    LoadingSpinner: true, Icon: true, Line: true, ModelDistributionChart: true, EndpointDistributionChart: true,
    BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' }
  }
})

function panel(accountId = 1) {
  return mount(AccountQuotaHistoryPanel, { props: { accountId }, global: globalOptions() })
}

describe('AccountQuotaHistoryPanel', () => {
  beforeEach(() => {
    getWindowHistory.mockReset().mockResolvedValue({ windows: {} })
    getStats.mockReset().mockResolvedValue(null)
  })

  it('explains why old unobserved cycles cannot be displayed', async () => {
    const wrapper = panel()
    await flushPromises()
    expect(getWindowHistory).toHaveBeenCalledWith(1, 90, expect.any(AbortSignal))
    expect(wrapper.get('[data-test="current"]').text()).toContain('No current cycle')
    expect(wrapper.get('[data-test="history"]').text()).toContain('unobserved past cycles cannot be reconstructed')
    wrapper.unmount()
  })

  it('keeps 100% observations approximate and uses the saved estimate, not the latest cumulative cost', async () => {
    getWindowHistory.mockResolvedValue({ windows: { '5h': [entry({
      api_reference_cost: 60, estimated_reference_limit: 50, estimate_used_percent: 80,
      estimate_reference_cost: 40, quality_flags: ['missing_pricing', 'observation_gap'], missing_pricing_requests: 2
    })] } })
    const wrapper = panel()
    await flushPromises()
    expect(wrapper.get('[data-test="reference-cost"]').text()).toBe('$60.00')
    expect(wrapper.get('[data-test="estimated-limit"]').text()).toBe('Approx. $50.00')
    expect(wrapper.text()).toContain('Exhaustion observed')
    expect(wrapper.text()).toContain('does not establish an official dollar allowance')
    expect(wrapper.text()).toContain('$40.00 / 80%')
    expect(wrapper.text()).toContain('lack reference prices')
    expect(wrapper.text()).toContain('Gaps between observations')
    wrapper.unmount()
  })

  it('shows unavailable amounts without inventing zero or a full-cycle estimate', async () => {
    getWindowHistory.mockResolvedValue({ windows: { '5h': [entry({
      api_reference_cost: null, estimated_reference_limit: null, quality_flags: ['pending_usage'],
      finalized: false, final_used_percent: null, end_reason: null
    })] } })
    const wrapper = panel()
    await flushPromises()
    expect(wrapper.get('[data-test="reference-cost"]').text()).toBe('—')
    expect(wrapper.get('[data-test="estimated-limit"]').text()).toBe('Insufficient data')
    expect(wrapper.get('[data-test="current"]').text()).toContain('Collecting observations')
    expect(wrapper.text()).toContain('Waiting for asynchronous usage records')
    wrapper.unmount()
  })

  it('shows 10 completed cycles at a time and switches windows using one response', async () => {
    const completed = Array.from({ length: 12 }, (_, index) => entry({
      window_start: `2026-09-${String(index + 1).padStart(2, '0')}T00:00:00Z`,
      window_end: `2026-09-${String(index + 1).padStart(2, '0')}T05:00:00Z`,
      api_reference_cost: index + 1
    }))
    getWindowHistory.mockResolvedValue({ windows: {
      '5h': completed,
      '7d': [entry({ api_reference_cost: 777, finalized: false, final_used_percent: null, end_reason: null })]
    } })
    const wrapper = panel()
    await flushPromises()
    expect(wrapper.findAll('[data-test="history-entry"]')).toHaveLength(10)
    expect(wrapper.findAll('[data-test="reference-cost"]')[0].text()).toBe('$12.00')
    expect(wrapper.findAll('[data-test="reference-cost"]')[9].text()).toBe('$3.00')
    await wrapper.get('[data-test="show-more"]').trigger('click')
    expect(wrapper.findAll('[data-test="history-entry"]')).toHaveLength(12)
    expect(wrapper.find('[data-test="show-more"]').exists()).toBe(false)
    await wrapper.get('[data-test="window-7d"]').trigger('click')
    expect(wrapper.get('[data-test="current"] [data-test="reference-cost"]').text()).toBe('$777.00')
    expect(wrapper.get('[data-test="window-7d"]').attributes('aria-pressed')).toBe('true')
    expect(getWindowHistory).toHaveBeenCalledTimes(1)
    await wrapper.get('[data-test="window-5h"]').trigger('click')
    expect(wrapper.findAll('[data-test="history-entry"]')).toHaveLength(10)
    wrapper.unmount()
  })

  it('ignores a previous account response even if cancellation is not respected', async () => {
    let resolveFirst!: (value: AccountWindowHistoryResponse) => void
    getWindowHistory.mockImplementationOnce(() => new Promise(resolve => { resolveFirst = resolve }))
      .mockResolvedValueOnce({ windows: { '5h': [entry({ api_reference_cost: 22 })] } })
    const wrapper = panel(1)
    const firstSignal = getWindowHistory.mock.calls[0][2] as AbortSignal
    await wrapper.setProps({ accountId: 2 })
    await flushPromises()
    expect(firstSignal.aborted).toBe(true)
    resolveFirst({ windows: { '5h': [entry({ api_reference_cost: 999 })] } })
    await flushPromises()
    expect(wrapper.get('[data-test="reference-cost"]').text()).toBe('$22.00')
    wrapper.unmount()
  })

  it('offers retry after an API error', async () => {
    getWindowHistory.mockRejectedValueOnce(new Error('network')).mockResolvedValueOnce({ windows: {} })
    const wrapper = panel()
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('Failed to load quota cycles')
    await wrapper.get('[data-test="retry"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(getWindowHistory).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('only loads history when its tab opens and cancels when the tab closes', async () => {
    const account = { id: 1, name: 'Codex account', platform: 'openai', type: 'oauth', status: 'active', quota_dimension: ' GLOBAL ' } as Account
    const wrapper = mount(AccountStatsModal, { props: { show: false, account }, global: globalOptions() })
    expect(getStats).not.toHaveBeenCalled()
    expect(getWindowHistory).not.toHaveBeenCalled()
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect(getStats).toHaveBeenCalledWith(1, 30)
    expect(getWindowHistory).not.toHaveBeenCalled()
    await wrapper.get('#account-quota-tab').trigger('click')
    await flushPromises()
    expect(getWindowHistory).toHaveBeenCalledTimes(1)
    const signal = getWindowHistory.mock.calls[0][2] as AbortSignal
    await wrapper.get('#account-stats-tab').trigger('click')
    expect(signal.aborted).toBe(true)
    expect(wrapper.findComponent(AccountQuotaHistoryPanel).exists()).toBe(false)
    await wrapper.get('#account-quota-tab').trigger('click')
    const closeSignal = getWindowHistory.mock.calls[1][2] as AbortSignal
    await wrapper.setProps({ show: false })
    expect(closeSignal.aborted).toBe(true)
    await wrapper.setProps({ show: true })
    expect(wrapper.get('#account-stats-tab').attributes('aria-selected')).toBe('true')
    expect(getWindowHistory).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it.each([
    { platform: 'anthropic', type: 'oauth' },
    { platform: 'openai', type: 'apikey' },
    { platform: 'openai', type: 'oauth', parent_account_id: 9 },
    { platform: 'openai', type: 'oauth', quota_dimension: 'spark' },
    { platform: 'openai', type: 'oauth', credentials: { auth_mode: 'agent_identity' } },
    { platform: 'openai', type: 'oauth', credentials: { auth_mode: 'personalAccessToken' } },
    { platform: 'openai', type: 'oauth', credentials: { auth_mode: 'personal_access_token' } },
    { platform: 'openai', type: 'oauth', credentials: { openai_auth_mode: 'agent_identity' } },
    { platform: 'openai', type: 'oauth', credentials: { openai_auth_mode: ' PERSONALACCESSTOKEN ' } },
    { platform: 'openai', type: 'oauth', credentials: { auth_mode: 'oauth', openai_auth_mode: 'personal_access_token' } }
  ])('does not offer quota history for unsupported accounts %o', async overrides => {
    const wrapper = mount(AccountStatsModal, {
      props: { show: true, account: { id: 1, name: 'Other account', status: 'active', ...overrides } as Account },
      global: globalOptions()
    })
    await flushPromises()
    expect(wrapper.find('#account-quota-tab').exists()).toBe(false)
    expect(getWindowHistory).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
