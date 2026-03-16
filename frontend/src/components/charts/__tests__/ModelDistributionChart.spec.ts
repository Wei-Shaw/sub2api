import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import ModelDistributionChart from '../ModelDistributionChart.vue'

const messages: Record<string, string> = {
  'admin.dashboard.modelDistribution': 'Model Distribution',
  'admin.dashboard.spendingRankingTitle': 'User Spending Ranking',
  'admin.dashboard.viewModelDistribution': 'Model Distribution',
  'admin.dashboard.viewSpendingRanking': 'User Spending Ranking',
  'admin.dashboard.spendingRankingUser': 'User',
  'admin.dashboard.spendingRankingRequests': 'Requests',
  'admin.dashboard.spendingRankingTokens': 'Tokens',
  'admin.dashboard.spendingRankingSpend': 'Spend',
  'admin.dashboard.spendingRankingOther': 'Others',
  'admin.dashboard.model': 'Model',
  'admin.dashboard.requests': 'Requests',
  'admin.dashboard.tokens': 'Tokens',
  'admin.dashboard.actual': 'Actual',
  'admin.dashboard.standard': 'Standard',
  'admin.dashboard.metricTokens': 'By Tokens',
  'admin.dashboard.metricActualCost': 'By Actual Cost',
  'admin.dashboard.noDataAvailable': 'No data available',
  'admin.redeem.userPrefix': 'User #{id}',
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
  Doughnut: {
    props: ['data'],
    template: '<div class="chart-data">{{ JSON.stringify(data) }}</div>',
  },
}))

describe('ModelDistributionChart', () => {
  const modelStats = [
    {
      model: 'model-a',
      requests: 8,
      input_tokens: 100,
      output_tokens: 50,
      cache_creation_tokens: 0,
      cache_read_tokens: 0,
      total_tokens: 1000,
      cost: 1.5,
      actual_cost: 0.2,
    },
    {
      model: 'model-b',
      requests: 3,
      input_tokens: 40,
      output_tokens: 20,
      cache_creation_tokens: 0,
      cache_read_tokens: 0,
      total_tokens: 500,
      cost: 0.5,
      actual_cost: 1.4,
    },
  ]

  it('uses total_tokens and token ordering by default', () => {
    const wrapper = mount(ModelDistributionChart, {
      props: {
        modelStats,
      },
      global: {
        stubs: {
          LoadingSpinner: true,
        },
      },
    })

    const chartData = JSON.parse(wrapper.find('.chart-data').text())
    expect(chartData.labels).toEqual(['model-a', 'model-b'])
    expect(chartData.datasets[0].data).toEqual([1000, 500])

    const rows = wrapper.findAll('tbody tr')
    expect(rows[0].text()).toContain('model-a')
    expect(rows[1].text()).toContain('model-b')

    const options = (wrapper.vm as any).$?.setupState.doughnutOptions
    const label = options.plugins.tooltip.callbacks.label({
      label: 'model-a',
      raw: 1000,
      dataset: { data: [1000, 500] },
    })
    expect(label).toBe('model-a: 1.00K (66.7%)')
  })

  it('uses actual_cost and reorders rows in actual cost mode', () => {
    const wrapper = mount(ModelDistributionChart, {
      props: {
        modelStats,
        metric: 'actual_cost',
      },
      global: {
        stubs: {
          LoadingSpinner: true,
        },
      },
    })

    const chartData = JSON.parse(wrapper.find('.chart-data').text())
    expect(chartData.labels).toEqual(['model-b', 'model-a'])
    expect(chartData.datasets[0].data).toEqual([1.4, 0.2])

    const rows = wrapper.findAll('tbody tr')
    expect(rows[0].text()).toContain('model-b')
    expect(rows[1].text()).toContain('model-a')

    const options = (wrapper.vm as any).$?.setupState.doughnutOptions
    const label = options.plugins.tooltip.callbacks.label({
      label: 'model-b',
      raw: 1.4,
      dataset: { data: [1.4, 0.2] },
    })
    expect(label).toBe('model-b: $1.40 (87.5%)')
  })

  it('renders Others in the spending ranking table and uses a dedicated chart color', async () => {
    const wrapper = mount(ModelDistributionChart, {
      props: {
        modelStats: [],
        enableRankingView: true,
        rankingItems: [
          { user_id: 1, email: 'alpha@example.com', actual_cost: 12, requests: 10, tokens: 1000 },
          { user_id: 2, email: 'beta@example.com', actual_cost: 8, requests: 6, tokens: 600 },
        ],
        rankingTotalActualCost: 30,
        rankingTotalRequests: 20,
        rankingTotalTokens: 2000,
      },
      global: {
        stubs: {
          LoadingSpinner: true,
        },
      },
    })

    const rankingButton = wrapper.findAll('button').find((button) => button.text() === 'User Spending Ranking')
    expect(rankingButton).toBeTruthy()
    await rankingButton!.trigger('click')

    const chartData = JSON.parse(wrapper.find('.chart-data').text())
    expect(chartData.labels).toEqual([
      '#1 alpha@example.com',
      '#2 beta@example.com',
      'Others',
    ])
    expect(chartData.datasets[0].data).toEqual([12, 8, 10])
    expect(chartData.datasets[0].backgroundColor[0]).toBe('#3b82f6')
    expect(chartData.datasets[0].backgroundColor[2]).toBe('#94a3b8')
    expect(chartData.datasets[0].backgroundColor[2]).not.toBe(chartData.datasets[0].backgroundColor[0])

    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(3)
    expect(rows[2].text()).toContain('Others')
    expect(rows[2].text()).toContain('4')
    expect(rows[2].text()).toContain('400')
    expect(rows[2].text()).toContain('$10.00')
  })
})
