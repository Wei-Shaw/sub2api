import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import AccountWindowHistoryChart from '../AccountWindowHistoryChart.vue'

const messages: Record<string, string> = {
  'admin.accounts.stats.windowHistory.empty': 'No window usage data yet',
  'admin.accounts.stats.windowHistory.chartTokens': 'Window Tokens',
  'admin.accounts.stats.windowHistory.chartPeak': 'Peak Utilization',
  'admin.accounts.stats.windowHistory.chartFinal': 'Final Utilization',
  'admin.accounts.stats.windowHistory.chartImplied': 'Implied Limit',
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

vi.mock('vue-chartjs', () => ({
  Chart: {
    props: ['type', 'data', 'options'],
    template: '<div class="chart-data">{{ JSON.stringify(data) }}</div>',
  },
}))

const entry = (overrides: Record<string, unknown> = {}) => ({
  window_start: '2026-08-10T00:00:00Z',
  window_end: '2026-08-10T05:00:00Z',
  requests: 10,
  tokens_total: 1000,
  tokens_input: 600,
  tokens_output: 400,
  tokens_cache_creation: 0,
  tokens_cache_read: 0,
  peak_used_percent: 80,
  final_used_percent: 50,
  sample_count: 3,
  finalized: true,
  ...overrides,
})

const datasetByLabel = (wrapper: ReturnType<typeof mount>, label: string) =>
  JSON.parse(wrapper.find('.chart-data').text()).datasets.find(
    (ds: { label: string }) => ds.label === label
  )

describe('AccountWindowHistoryChart', () => {
  it('builds token bars and utilization lines from entries', () => {
    const wrapper = mount(AccountWindowHistoryChart, {
      props: {
        entries: [
          entry(),
          entry({
            window_start: '2026-08-10T05:00:00Z',
            window_end: '2026-08-10T10:00:00Z',
            tokens_total: 2000,
            peak_used_percent: 90,
            final_used_percent: 75,
          }),
        ],
      },
      global: {
        stubs: { LoadingSpinner: true },
      },
    })

    const tokens = datasetByLabel(wrapper, 'Window Tokens')
    expect(tokens.data).toEqual([1000, 2000])
    expect(tokens.type).toBe('bar')

    const peak = datasetByLabel(wrapper, 'Peak Utilization')
    expect(peak.data).toEqual([80, 90])

    const final = datasetByLabel(wrapper, 'Final Utilization')
    expect(final.data).toEqual([50, 75])
  })

  it('derives the implied limit from final utilization (tokens ÷ final%)', () => {
    // 1000 tokens at 50% final → implied limit 2000
    const wrapper = mount(AccountWindowHistoryChart, {
      props: { entries: [entry()] },
      global: { stubs: { LoadingSpinner: true } },
    })

    const implied = datasetByLabel(wrapper, 'Implied Limit')
    expect(implied.data).toEqual([2000])
  })

  it('breaks the implied limit on open windows (null final/tokens)', () => {
    const wrapper = mount(AccountWindowHistoryChart, {
      props: {
        entries: [
          entry(),
          entry({
            // open window: nothing rebuilt yet, no final utilization
            requests: null,
            tokens_total: null,
            final_used_percent: null,
            finalized: false,
          }),
        ],
      },
      global: { stubs: { LoadingSpinner: true } },
    })

    const implied = datasetByLabel(wrapper, 'Implied Limit')
    expect(implied.data).toEqual([2000, null])
    // token bar also gaps on the open window
    const tokens = datasetByLabel(wrapper, 'Window Tokens')
    expect(tokens.data).toEqual([1000, null])
  })

  it('skips implied limit when final utilization is too low to derive', () => {
    // 4% final: derived limit would be 25x tokens — error too large to be meaningful
    const wrapper = mount(AccountWindowHistoryChart, {
      props: { entries: [entry({ final_used_percent: 4 })] },
      global: { stubs: { LoadingSpinner: true } },
    })

    const implied = datasetByLabel(wrapper, 'Implied Limit')
    expect(implied.data).toEqual([null])
  })

  it('renders the empty state without entries', () => {
    const wrapper = mount(AccountWindowHistoryChart, {
      props: { entries: [] },
      global: { stubs: { LoadingSpinner: true } },
    })

    expect(wrapper.find('.chart-data').exists()).toBe(false)
    expect(wrapper.text()).toContain('No window usage data yet')
  })
})
