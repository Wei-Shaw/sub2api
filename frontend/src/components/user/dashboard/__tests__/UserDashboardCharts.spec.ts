import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import UserDashboardCharts from '../UserDashboardCharts.vue'
import type { ModelStat, TrendDataPoint } from '@/types'

const messages: Record<string, string> = {
  'dashboard.timeRange': '时间范围',
  'dashboard.granularity': '粒度',
  'dashboard.day': '按天',
  'dashboard.hour': '按小时',
  'dashboard.modelDistribution': '模型分布',
  'dashboard.noDataAvailable': '暂无数据',
  'dashboard.model': '模型',
  'dashboard.requests': '请求',
  'dashboard.tokens': 'Token',
  'dashboard.actual': '实际',
  'dashboard.standard': '标准',
  'common.refresh': '刷新',
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
    props: ['data', 'options'],
    template: '<div data-testid="model-doughnut"></div>',
  },
}))

vi.mock('@/components/charts/TokenUsageTrend.vue', () => ({
  default: {
    props: ['trendData', 'loading'],
    template: '<section data-testid="token-trend">Token 使用趋势</section>',
  },
}))

const trend: TrendDataPoint[] = [
  {
    date: '2026-06-21',
    requests: 1,
    input_tokens: 100,
    output_tokens: 50,
    cache_creation_tokens: 0,
    cache_read_tokens: 0,
    total_tokens: 150,
    cost: 0.01,
    actual_cost: 0.005,
  },
]

const models: ModelStat[] = [
  {
    model: 'gpt-5.5',
    requests: 3,
    input_tokens: 120,
    output_tokens: 80,
    cache_creation_tokens: 0,
    cache_read_tokens: 0,
    total_tokens: 200,
    cost: 0.02,
    actual_cost: 0.01,
    account_cost: 0,
  },
]

describe('UserDashboardCharts', () => {
  it('移除模型拆分卡片但保留筛选和 Token 趋势', () => {
    const wrapper = mount(UserDashboardCharts, {
      props: {
        loading: false,
        startDate: '2026-06-15',
        endDate: '2026-06-21',
        granularity: 'day',
        trend,
        models,
      },
      global: {
        stubs: {
          DateRangePicker: {
            template: '<div data-testid="date-range-picker"></div>',
          },
          Select: {
            template: '<div data-testid="granularity-select"></div>',
          },
          LoadingSpinner: true,
        },
      },
    })

    expect(wrapper.text()).toContain('时间范围')
    expect(wrapper.text()).toContain('粒度')
    expect(wrapper.text()).toContain('Token 使用趋势')
    expect(wrapper.text()).not.toContain('模型分布')
    expect(wrapper.text()).not.toContain('gpt-5.5')
    expect(wrapper.find('[data-testid="token-trend"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="model-doughnut"]').exists()).toBe(false)
  })
})
