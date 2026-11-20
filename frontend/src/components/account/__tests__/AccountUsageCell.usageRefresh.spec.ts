import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
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
      t: (key: string) => key
    })
  }
})

function makeAccount(overrides: Partial<Account>): Account {
  return {
    id: 1,
    name: 'account',
    platform: 'anthropic',
    type: 'oauth',
    proxy_id: null,
    concurrency: 1,
    priority: 1,
    status: 'active',
    error_message: null,
    last_used_at: null,
    expires_at: null,
    auto_pause_on_expired: true,
    created_at: '2026-10-25T00:00:00Z',
    updated_at: '2026-10-25T00:00:00Z',
    schedulable: true,
    rate_limited_at: null,
    rate_limit_reset_at: null,
    overload_until: null,
    temp_unschedulable_until: null,
    temp_unschedulable_reason: null,
    session_window_start: null,
    session_window_end: null,
    session_window_status: null,
    ...overrides,
  }
}

const passiveUsage = (fiveHour: number, sevenDay: number) => ({
  source: 'passive',
  five_hour: { utilization: fiveHour, resets_at: null, remaining_seconds: 0 },
  seven_day: { utilization: sevenDay, resets_at: null, remaining_seconds: 0 }
})

const stubs = {
  UsageProgressBar: {
    props: ['label', 'utilization'],
    template: '<div class="usage-bar">{{ label }}|{{ utilization }}</div>'
  },
  ClaudeResetCreditsCell: { template: '<div><slot name="pre-actions" /></div>' },
  AccountQuotaInfo: true
}

const sampledClaudeAccount = (id: number, sampledAt: string, fiveHour: number) => makeAccount({
  id,
  extra: {
    passive_usage_sampled_at: sampledAt,
    session_window_utilization: fiveHour,
    passive_usage_7d_utilization: 0.2
  }
})

describe('AccountUsageCell usage refresh on row updates', () => {
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
        dispatchEvent: vi.fn(),
      }))
    })
  })

  it('refetches passive usage past its cache when the Claude row carries a new sample', async () => {
    getUsage
      .mockResolvedValueOnce(passiveUsage(10, 20))
      .mockResolvedValueOnce(passiveUsage(55, 20))

    const wrapper = mount(AccountUsageCell, {
      props: { account: sampledClaudeAccount(4101, '2026-10-25T10:00:00Z', 0.1) },
      global: { stubs }
    })
    await flushPromises()
    expect(getUsage).toHaveBeenCalledTimes(1)
    expect(getUsage).toHaveBeenLastCalledWith(4101, 'passive', false)
    expect(wrapper.text()).toContain('5h|10')

    // A replaced row whose usage inputs are unchanged keeps the cached usage.
    await wrapper.setProps({
      account: { ...sampledClaudeAccount(4101, '2026-10-25T10:00:00Z', 0.1), current_concurrency: 3 }
    })
    await flushPromises()
    expect(getUsage).toHaveBeenCalledTimes(1)

    await wrapper.setProps({ account: sampledClaudeAccount(4101, '2026-10-25T10:01:00Z', 0.55) })
    await flushPromises()
    expect(getUsage).toHaveBeenCalledTimes(2)
    expect(getUsage).toHaveBeenLastCalledWith(4101, 'passive', false)
    expect(wrapper.text()).toContain('5h|55')
  })

  it('asks the batch loader to bypass its cache when a batch-managed Claude row carries a new sample', async () => {
    const requestBatchedUsage = vi.fn()
    const wrapper = mount(AccountUsageCell, {
      props: {
        account: sampledClaudeAccount(4102, '2026-10-25T10:00:00Z', 0.1),
        requestBatchedUsage
      },
      global: { stubs }
    })
    await flushPromises()
    expect(requestBatchedUsage).toHaveBeenCalledTimes(1)

    await wrapper.setProps({
      account: { ...sampledClaudeAccount(4102, '2026-10-25T10:00:00Z', 0.1), current_concurrency: 3 }
    })
    await flushPromises()
    expect(requestBatchedUsage).toHaveBeenCalledTimes(1)

    await wrapper.setProps({ account: sampledClaudeAccount(4102, '2026-10-25T10:01:00Z', 0.55) })
    await flushPromises()
    expect(requestBatchedUsage).toHaveBeenCalledTimes(2)
    expect(requestBatchedUsage).toHaveBeenLastCalledWith(expect.objectContaining({ id: 4102 }), undefined)
    expect(getUsage).not.toHaveBeenCalled()
  })

  it('refetches Gemini usage when the row reports new use', async () => {
    getUsage.mockResolvedValue({})
    const account = makeAccount({
      id: 4103,
      platform: 'gemini',
      type: 'oauth',
      last_used_at: '2026-10-25T10:00:00Z',
      credentials: { oauth_type: 'google_one' }
    })
    const wrapper = mount(AccountUsageCell, {
      props: { account },
      global: { stubs }
    })
    await flushPromises()
    expect(getUsage).toHaveBeenCalledTimes(1)
    expect(getUsage).toHaveBeenLastCalledWith(4103)

    await wrapper.setProps({ account: { ...account, last_used_at: '2026-10-25T10:01:00Z' } })
    await flushPromises()
    expect(getUsage).toHaveBeenCalledTimes(2)
    expect(getUsage).toHaveBeenLastCalledWith(4103)
  })
})
