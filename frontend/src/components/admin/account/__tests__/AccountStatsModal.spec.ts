import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AccountStatsModal from '../AccountStatsModal.vue'
import type { Account, AccountUsageStatsResponse } from '@/types'

const { getStats } = vi.hoisted(() => ({
  getStats: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getStats
    }
  }
}))

vi.mock('vue-chartjs', () => ({
  Line: {
    props: ['data', 'options'],
    template: '<div class="chart-data">{{ JSON.stringify(data) }}</div>'
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

function makeStats(withUpstreamErrors: boolean): AccountUsageStatsResponse {
  const errors = (value: number) => (withUpstreamErrors ? { upstream_errors: value } : {})
  return {
    history: [
      { date: '2026-11-10', label: '11/10', requests: 5, tokens: 100, cost: 1, actual_cost: 1, user_cost: 1, ...errors(0) },
      { date: '2026-11-11', label: '11/11', requests: 0, tokens: 0, cost: 0, actual_cost: 0, user_cost: 0, ...errors(4) },
      { date: '2026-11-12', label: '11/12', requests: 3, tokens: 50, cost: 1, actual_cost: 1, user_cost: 1, ...errors(2) }
    ],
    summary: {
      days: 30,
      actual_days_used: 2,
      total_cost: 2,
      total_user_cost: 2,
      total_standard_cost: 2,
      total_requests: 8,
      total_tokens: 150,
      avg_daily_cost: 1,
      avg_daily_user_cost: 1,
      avg_daily_requests: 4,
      avg_daily_tokens: 75,
      avg_duration_ms: 100,
      ...(withUpstreamErrors ? { total_upstream_errors: 6, today_upstream_errors: 2 } : {}),
      today: { date: '2026-11-12', cost: 1, user_cost: 1, requests: 3, tokens: 50 },
      highest_cost_day: null,
      highest_request_day: null
    },
    models: [],
    endpoints: [],
    upstream_endpoints: []
  } as AccountUsageStatsResponse
}

const mountModal = async () => {
  const wrapper = mount(AccountStatsModal, {
    props: { show: false, account: { id: 1, name: 'acc', status: 'active' } as Account },
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        ModelDistributionChart: true,
        EndpointDistributionChart: true,
        LoadingSpinner: true,
        Icon: true
      }
    }
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  return wrapper
}

describe('AccountStatsModal upstream errors', () => {
  beforeEach(() => {
    getStats.mockReset()
  })

  it('adds an upstream error trend line and summary rows when reported', async () => {
    getStats.mockResolvedValue(makeStats(true))
    const wrapper = await mountModal()

    const chartData = JSON.parse(wrapper.get('.chart-data').text()) as {
      datasets: Array<{ label: string; data: number[]; yAxisID: string }>
    }
    const errorDataset = chartData.datasets.find((d) => d.label === 'admin.accounts.stats.upstreamErrors')
    expect(errorDataset).toBeDefined()
    expect(errorDataset!.data).toEqual([0, 4, 2])
    expect(errorDataset!.yAxisID).toBe('y1')

    expect(wrapper.get('[data-testid="today-upstream-errors"]').text()).toContain('2')
    expect(wrapper.get('[data-testid="total-upstream-errors"]').text()).toContain('6')
  })

  it('hides upstream errors when the backend omits them', async () => {
    getStats.mockResolvedValue(makeStats(false))
    const wrapper = await mountModal()

    const chartData = JSON.parse(wrapper.get('.chart-data').text()) as {
      datasets: Array<{ label: string }>
    }
    expect(chartData.datasets.map((d) => d.label)).not.toContain('admin.accounts.stats.upstreamErrors')
    expect(wrapper.find('[data-testid="today-upstream-errors"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="total-upstream-errors"]').exists()).toBe(false)
  })
})
