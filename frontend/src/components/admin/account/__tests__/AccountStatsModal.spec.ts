import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { getStats, getUsage, getWindowUsage } = vi.hoisted(() => ({
  getStats: vi.fn(),
  getUsage: vi.fn(),
  getWindowUsage: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: { accounts: { getStats, getUsage, getWindowUsage } }
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

import AccountStatsModal from '../AccountStatsModal.vue'
import type {
  Account,
  AccountUsageInfo,
  AccountUsageStatsResponse,
  AccountWindowUsageResponse
} from '@/types'

const now = '2026-08-11T10:30:00.000Z'

const account = (id: number): Account => ({
  id,
  name: `account-${id}`,
  platform: 'openai',
  type: 'oauth',
  status: 'active',
  session_window_start: '2026-08-11T08:00:00.000Z',
  session_window_end: '2026-08-11T13:00:00.000Z'
} as Account)

const usageInfo = (): AccountUsageInfo => ({
  updated_at: new Date().toISOString(),
  five_hour: {
    utilization: 50,
    resets_at: new Date(Date.now() + 2 * 60 * 60 * 1000).toISOString(),
    remaining_seconds: 7200,
    window_minutes: 300
  },
  seven_day: null,
  seven_day_sonnet: null
} as AccountUsageInfo)

const statsResponse = (requests: number): AccountUsageStatsResponse => ({
  history: [],
  summary: {
    days: 30,
    actual_days_used: 1,
    total_cost: 1,
    total_user_cost: 1.2,
    total_standard_cost: 0.8,
    total_requests: requests,
    total_tokens: requests * 10,
    avg_daily_cost: 1,
    avg_daily_user_cost: 1.2,
    avg_daily_requests: requests,
    avg_daily_tokens: requests * 10,
    avg_duration_ms: 100,
    today: null,
    highest_cost_day: null,
    highest_request_day: null
  },
  models: [],
  endpoints: [],
  upstream_endpoints: []
})

const windowResponse = (requests: number): AccountWindowUsageResponse => ({
  generated_at: now,
  items: [
    {
      window_key: 'five_hour', period: 'current', start_time: '2026-08-11T08:00:00.000Z', end_time: now,
      matched: true, total_requests: requests, success_calls: requests, failure_calls: 0,
      total_tokens: requests * 10, account_cost: requests / 10, standard_cost: requests / 10,
      user_cost: requests / 10, success_rate: 100, success_rate_status: 'available'
    },
    {
      window_key: 'five_hour', period: 'previous', start_time: '2026-08-11T03:00:00.000Z', end_time: '2026-08-11T08:00:00.000Z',
      matched: true, total_requests: requests - 1, success_calls: requests - 1, failure_calls: 0,
      total_tokens: (requests - 1) * 10, account_cost: (requests - 1) / 10,
      standard_cost: (requests - 1) / 10, user_cost: (requests - 1) / 10,
      success_rate: 100, success_rate_status: 'available'
    }
  ]
})

const globalStubs = {
  BaseDialog: {
    props: ['show'],
    template: '<article v-if="show"><slot /><slot name="footer" /></article>'
  },
  AccountQuotaWindowSection: {
    name: 'AccountQuotaWindowSection',
    props: ['windows', 'loading', 'error'],
    emits: ['retry'],
    template: '<div data-testid="quota-stub">quota</div>'
  },
  LoadingSpinner: true,
  ModelDistributionChart: true,
  EndpointDistributionChart: true,
  Icon: true,
  Line: true
}

const mountModal = (id = 1, cachedUsage: AccountUsageInfo | null = usageInfo()) => mount(AccountStatsModal, {
  props: { show: true, account: account(id), usageInfo: cachedUsage },
  global: { stubs: globalStubs }
})

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

describe('AccountStatsModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
    getStats.mockResolvedValue(statsResponse(111))
    getUsage.mockResolvedValue(usageInfo())
    getWindowUsage.mockResolvedValue(windowResponse(12))
  })

  afterEach(() => {
    vi.mocked(console.error).mockRestore()
  })

  it('reuses a fresh loaded usage snapshot and loads history independently', async () => {
    const wrapper = mountModal()
    await flushPromises()

    expect(getUsage).not.toHaveBeenCalled()
    expect(getStats).toHaveBeenCalledWith(1, 30)
    expect(getWindowUsage).toHaveBeenCalledTimes(1)
    expect(getWindowUsage.mock.calls[0][1].windows).toHaveLength(2)
    const quota = wrapper.findComponent({ name: 'AccountQuotaWindowSection' })
    expect(quota.props('error')).toBeNull()
    expect(quota.props('windows')[0].current.total_requests).toBe(12)
    expect(wrapper.text()).toContain('111')
  })

  it('falls back to the existing usage endpoint when no cached snapshot is available', async () => {
    mountModal(1, null)
    await flushPromises()
    expect(getUsage).toHaveBeenCalledWith(1)
    expect(getWindowUsage).toHaveBeenCalledTimes(1)
  })

  it('reloads only quota windows when a refreshed provider snapshot arrives', async () => {
    const wrapper = mountModal()
    await flushPromises()

    const refreshedUsage = usageInfo()
    refreshedUsage.five_hour = {
      ...refreshedUsage.five_hour!,
      utilization: 5,
      resets_at: new Date(Date.now() + 5 * 60 * 60 * 1000).toISOString()
    }
    await wrapper.setProps({ usageInfo: refreshedUsage })
    await flushPromises()

    expect(getStats).toHaveBeenCalledTimes(1)
    expect(getWindowUsage).toHaveBeenCalledTimes(2)
  })

  it('keeps quota-window data when historical statistics fail', async () => {
    getStats.mockRejectedValueOnce(new Error('history failed'))
    const wrapper = mountModal()
    await flushPromises()

    expect(wrapper.find('[data-testid="account-stats-error"]').exists()).toBe(true)
    const quota = wrapper.findComponent({ name: 'AccountQuotaWindowSection' })
    expect(quota.props('error')).toBeNull()
    expect(quota.props('windows')[0].current.total_requests).toBe(12)
  })

  it('keeps historical statistics when quota-window aggregation fails', async () => {
    getWindowUsage.mockRejectedValueOnce(new Error('window failed'))
    const wrapper = mountModal()
    await flushPromises()

    expect(wrapper.text()).toContain('111')
    expect(wrapper.find('[data-testid="account-stats-error"]').exists()).toBe(false)
    expect(wrapper.findComponent({ name: 'AccountQuotaWindowSection' }).props('error')).toBe('load_failed')
  })

  it('ignores stale responses after switching accounts', async () => {
    const oldStats = deferred<AccountUsageStatsResponse>()
    const newStats = deferred<AccountUsageStatsResponse>()
    const oldWindow = deferred<AccountWindowUsageResponse>()
    const newWindow = deferred<AccountWindowUsageResponse>()
    getStats.mockImplementationOnce(() => oldStats.promise).mockImplementationOnce(() => newStats.promise)
    getWindowUsage.mockImplementationOnce(() => oldWindow.promise).mockImplementationOnce(() => newWindow.promise)

    const wrapper = mountModal(1)
    await flushPromises()
    await wrapper.setProps({ account: account(2), usageInfo: usageInfo() })
    await flushPromises()

    newStats.resolve(statsResponse(222))
    newWindow.resolve(windowResponse(22))
    await flushPromises()
    oldStats.resolve(statsResponse(111))
    oldWindow.resolve(windowResponse(11))
    await flushPromises()

    expect(wrapper.text()).toContain('222')
    expect(wrapper.text()).not.toContain('111')
    expect(wrapper.findComponent({ name: 'AccountQuotaWindowSection' }).props('windows')[0].current.total_requests).toBe(22)
  })

  it('invalidates in-flight writes as soon as close is requested', async () => {
    const pendingStats = deferred<AccountUsageStatsResponse>()
    getStats.mockReturnValueOnce(pendingStats.promise)
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.find('button').trigger('click')
    pendingStats.resolve(statsResponse(333))
    await flushPromises()
    expect(wrapper.emitted('close')).toHaveLength(1)
    expect(wrapper.text()).not.toContain('333')
  })
})
