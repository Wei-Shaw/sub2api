import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import UpstreamBillingRateHistoryDialog from '../UpstreamBillingRateHistoryDialog.vue'
import {
  getUpstreamBillingRateHistoryCache,
  invalidateUpstreamBillingRateHistoryCache,
  setUpstreamBillingRateHistoryCache
} from '../upstreamBillingRateHistoryCache'
import { adminAPI } from '@/api/admin'
import type { Account, UpstreamBillingRateHistoryResponse } from '@/types'

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getUpstreamBillingRateHistoryWithEtag: vi.fn()
    }
  }
}))

vi.mock('@/utils/apiError', () => ({
  extractApiErrorMessage: (_error: unknown, fallback: string) => fallback
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}:${Object.values(params).join(',')}` : key
    })
  }
})

vi.mock('vue-chartjs', () => ({
  Line: {
    name: 'Line',
    props: ['data', 'options'],
    template: '<div data-testid="upstream-rate-history-chart" />'
  }
}))

const account: Account = {
  id: 7,
  name: 'upstream',
  platform: 'openai',
  type: 'apikey',
  credentials: { base_url: 'https://upstream.example/v1' },
  proxy_id: null,
  concurrency: 1,
  priority: 1,
  status: 'active',
  error_message: null,
  last_used_at: null,
  expires_at: null,
  auto_pause_on_expired: false,
  created_at: '2026-07-01T00:00:00Z',
  updated_at: '2026-07-01T00:00:00Z',
  schedulable: true,
  rate_limited_at: null,
  rate_limit_reset_at: null,
  overload_until: null,
  temp_unschedulable_until: null,
  temp_unschedulable_reason: null,
  session_window_start: null,
  session_window_end: null,
  session_window_status: null
}

const history: UpstreamBillingRateHistoryResponse = {
  account_id: 7,
  range_days: 7,
  truncated: false,
  events: [
    {
      id: 1,
      detected_at: '2026-07-19T00:00:00Z',
      interval_end: '2026-07-19T12:00:00Z',
      carried_in: false,
      group_rate_multiplier: 1,
      user_rate_multiplier: null,
      peak_rate_enabled: false,
      peak_start: null,
      peak_end: null,
      peak_timezone: null,
      peak_rate_multiplier: null,
      resolved_rate_multiplier: 1,
      effective_rate_multiplier: 1
    },
    {
      id: 2,
      detected_at: '2026-07-19T12:00:00Z',
      interval_end: null,
      carried_in: false,
      group_rate_multiplier: 1,
      user_rate_multiplier: null,
      peak_rate_enabled: true,
      peak_start: '09:00',
      peak_end: '18:00',
      peak_timezone: 'Asia/Shanghai',
      peak_rate_multiplier: 1,
      resolved_rate_multiplier: 1,
      effective_rate_multiplier: 1
    }
  ]
}

const mountDialog = () => mount(UpstreamBillingRateHistoryDialog, {
  props: { show: true, account },
  global: {
    stubs: {
      BaseDialog: {
        props: ['show', 'title'],
        template: '<div v-if="show"><slot /><slot name="footer" /></div>'
      },
      LoadingSpinner: true,
      Icon: true
    }
  }
})

describe('UpstreamBillingRateHistoryDialog', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-07-20T00:00:00Z'))
    invalidateUpstreamBillingRateHistoryCache()
    vi.mocked(adminAPI.accounts.getUpstreamBillingRateHistoryWithEtag).mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('loads on demand and builds a real-time stepped series with every event marker', async () => {
    vi.mocked(adminAPI.accounts.getUpstreamBillingRateHistoryWithEtag).mockResolvedValue({
      notModified: false,
      etag: '"history-v1"',
      data: history
    })
    const wrapper = mountDialog()
    await flushPromises()

    expect(adminAPI.accounts.getUpstreamBillingRateHistoryWithEtag).toHaveBeenCalledWith(
      7,
      7,
      expect.objectContaining({ etag: undefined, signal: expect.any(AbortSignal) })
    )
    const chart = wrapper.findComponent({ name: 'Line' })
    const dataset = chart.props('data').datasets[0]
    expect(dataset.stepped).toBe('before')
    expect(dataset.pointRadius).toEqual([3, 3, 0])
    expect(dataset.pointHitRadius).toEqual([8, 8, 0])
    expect(dataset.data).toEqual([
      { x: Date.parse('2026-07-19T00:00:00Z'), y: 1 },
      { x: Date.parse('2026-07-19T12:00:00Z'), y: 1 },
      { x: Date.now(), y: 1 }
    ])
    expect(chart.props('options').scales.x.min).toBe(Date.parse('2026-07-19T00:00:00Z'))
    expect(chart.props('options').scales.x.max).toBe(Date.now())
    expect(chart.props('options').interaction).toEqual({ intersect: true, mode: 'nearest' })
    expect(wrapper.findAll('tbody tr')).toHaveLength(2)
    const firstRowCells = wrapper.findAll('tbody tr')[0].findAll('td')
    expect(firstRowCells).toHaveLength(7)
    expect(firstRowCells[0].text()).toContain('~')
    expect(wrapper.findAll('thead th')[0].text()).toBe('admin.accounts.upstreamBilling.historyPeriod')
    expect(getUpstreamBillingRateHistoryCache(7, 7)?.etag).toBe('"history-v1"')
  })

  it('keeps the full seven-day axis when a carried-in event covers the range start', async () => {
    vi.mocked(adminAPI.accounts.getUpstreamBillingRateHistoryWithEtag).mockResolvedValue({
      notModified: false,
      etag: '"carried-v1"',
      data: {
        ...history,
        events: [{
          ...history.events[0],
          detected_at: '2026-07-01T00:00:00Z',
          interval_end: null,
          carried_in: true
        }]
      }
    })

    const wrapper = mountDialog()
    await flushPromises()

    const chart = wrapper.findComponent({ name: 'Line' })
    const rangeStart = Date.parse('2026-07-13T00:00:00Z')
    expect(chart.props('options').scales.x.min).toBe(rangeStart)
    expect(chart.props('data').datasets[0].data[0].x).toBe(rangeStart)
  })

  it('renders cached data immediately and keeps it after a 304', async () => {
    setUpstreamBillingRateHistoryCache(7, 7, {
      data: history,
      etag: '"history-v1"'
    })
    let resolveRequest!: (value: {
      notModified: boolean
      etag: string | null
      data: UpstreamBillingRateHistoryResponse | null
    }) => void
    vi.mocked(adminAPI.accounts.getUpstreamBillingRateHistoryWithEtag).mockReturnValue(
      new Promise(resolve => { resolveRequest = resolve })
    )

    const wrapper = mountDialog()
    await flushPromises()
    expect(wrapper.findAll('tbody tr')).toHaveLength(2)
    expect(adminAPI.accounts.getUpstreamBillingRateHistoryWithEtag).toHaveBeenCalledWith(
      7,
      7,
      expect.objectContaining({ etag: '"history-v1"' })
    )

    resolveRequest({ notModified: true, etag: '"history-v1"', data: null })
    await flushPromises()
    expect(wrapper.findAll('tbody tr')).toHaveLength(2)
    expect(wrapper.find('[data-testid="upstream-rate-history-cache-warning"]').exists()).toBe(false)
  })

  it('keeps cached data with a warning when revalidation fails', async () => {
    setUpstreamBillingRateHistoryCache(7, 7, {
      data: history,
      etag: '"history-v1"'
    })
    vi.mocked(adminAPI.accounts.getUpstreamBillingRateHistoryWithEtag).mockRejectedValue(new Error('offline'))

    const wrapper = mountDialog()
    await flushPromises()
    expect(wrapper.findAll('tbody tr')).toHaveLength(2)
    expect(wrapper.get('[data-testid="upstream-rate-history-cache-warning"]').text()).toContain(
      'admin.accounts.upstreamBilling.historyLoadFailed'
    )
  })

  it('bounds cache entries and invalidates all ranges for one account', () => {
    for (let accountID = 1; accountID <= 21; accountID += 1) {
      setUpstreamBillingRateHistoryCache(accountID, 90, {
        data: { ...history, account_id: accountID },
        etag: `"${accountID}"`
      })
    }
    expect(getUpstreamBillingRateHistoryCache(1, 90)).toBeUndefined()
    expect(getUpstreamBillingRateHistoryCache(21, 90)).toBeDefined()

    invalidateUpstreamBillingRateHistoryCache(21)
    expect(getUpstreamBillingRateHistoryCache(21, 90)).toBeUndefined()
  })
})
