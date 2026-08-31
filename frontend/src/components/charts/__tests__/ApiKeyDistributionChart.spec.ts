import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import ApiKeyDistributionChart from '../ApiKeyDistributionChart.vue'
import { getApiKeysRanking } from '@/api/admin/dashboard'
import { getMyApiKeysRanking } from '@/api/usage'
import type { ApiKeyUsageRankingItem } from '@/types'

const messages: Record<string, string> = {
  'admin.dashboard.apiKeyDistribution': 'API Key Distribution',
  'admin.dashboard.apiKeyDeletedBadge': 'Deleted',
  'admin.dashboard.apiKeyPrefix': 'Key #{id}',
  'admin.dashboard.apiKeyCount': '{count} keys',
  'admin.dashboard.apiKeyOtherHint': '{count} unranked keys',
  'admin.dashboard.metricTokens': 'By Tokens',
  'admin.dashboard.metricActualCost': 'By Actual Cost',
  'admin.dashboard.metricRequests': 'By Requests',
  'admin.dashboard.totalCost': 'Total Cost',
  'admin.dashboard.totalRequests': 'Total Requests',
  'admin.dashboard.totalTokens': 'Total Tokens',
  'admin.dashboard.spendingRankingOther': 'Others',
  'admin.dashboard.noDataAvailable': 'No data available',
  'admin.dashboard.failedToLoad': 'Failed to load dashboard statistics',
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        const message = messages[key] ?? key
        return params
          ? message.replace(/\{(\w+)\}/g, (_, name: string) => String(params[name]))
          : message
      },
    }),
  }
})

vi.mock('vue-chartjs', () => ({
  Doughnut: {
    props: ['data'],
    template: '<div class="chart-data">{{ JSON.stringify(data) }}</div>',
  },
}))

vi.mock('@/api/admin/dashboard', () => ({
  getApiKeysRanking: vi.fn(),
}))

vi.mock('@/api/usage', () => ({
  getMyApiKeysRanking: vi.fn(),
}))

const mockedGetApiKeysRanking = vi.mocked(getApiKeysRanking)
const mockedGetMyApiKeysRanking = vi.mocked(getMyApiKeysRanking)

const makeItem = (overrides: Partial<ApiKeyUsageRankingItem>): ApiKeyUsageRankingItem => ({
  api_key_id: 1,
  key_name: 'key',
  key_deleted: false,
  user_id: 1,
  email: 'owner@example.com',
  username: 'owner',
  requests: 0,
  input_tokens: 0,
  output_tokens: 0,
  cache_tokens: 0,
  total_tokens: 0,
  cost: 0,
  actual_cost: 0,
  ...overrides,
})

const rankingResponse = {
  ranking: [
    makeItem({ api_key_id: 11, key_name: 'prod-main', requests: 10, total_tokens: 1000, actual_cost: 12 }),
    makeItem({ api_key_id: 12, key_name: '', key_deleted: true, email: '', username: '', requests: 6, total_tokens: 600, actual_cost: 8 }),
  ],
  total_actual_cost: 30,
  total_requests: 20,
  total_tokens: 2000,
  total_keys: 35,
  start_date: '2025-01-01',
  end_date: '2025-01-07',
}

const mountChart = async () => {
  const wrapper = mount(ApiKeyDistributionChart, {
    props: {
      startDate: '2025-01-01',
      endDate: '2025-01-07',
    },
    global: {
      stubs: {
        LoadingSpinner: true,
      },
    },
  })
  await flushPromises()
  return wrapper
}

describe('ApiKeyDistributionChart', () => {
  beforeEach(() => {
    mockedGetApiKeysRanking.mockReset()
    mockedGetApiKeysRanking.mockResolvedValue({ ...rankingResponse })
    mockedGetMyApiKeysRanking.mockReset()
    mockedGetMyApiKeysRanking.mockResolvedValue({ ...rankingResponse })
  })

  it('fetches ranking sorted by actual cost and renders Top-N plus a computed Others slice', async () => {
    const wrapper = await mountChart()

    expect(mockedGetApiKeysRanking).toHaveBeenCalledWith({
      start_date: '2025-01-01',
      end_date: '2025-01-07',
      limit: 12,
      sort_by: 'actual_cost',
    })

    const chartData = JSON.parse(wrapper.find('.chart-data').text())
    expect(chartData.labels).toEqual(['#1 prod-main', '#2 Key #12', 'Others'])
    // Others = window totals minus ranked rows: 30 - 12 - 8 = 10
    expect(chartData.datasets[0].data).toEqual([12, 8, 10])
    expect(chartData.datasets[0].backgroundColor[0]).toBe('#3b82f6')
    expect(chartData.datasets[0].backgroundColor[2]).toBe('#94a3b8')

    // 环中心合计
    expect(wrapper.text()).toContain('Total Cost')
    expect(wrapper.text()).toContain('$30.00')
    expect(wrapper.text()).toContain('35 keys')

    const rows = wrapper.findAll('[data-testid="key-dist-row"]')
    expect(rows).toHaveLength(3)
    expect(rows[0].text()).toContain('prod-main')
    expect(rows[0].text()).toContain('owner@example.com')
    expect(rows[0].text()).toContain('$12.00')
    expect(rows[0].text()).toContain('40.0%')
    expect(rows[1].text()).toContain('Key #12')
    expect(rows[1].text()).toContain('Deleted')
    expect(rows[2].text()).toContain('Others')
    expect(rows[2].text()).toContain('33 unranked keys')
    expect(rows[2].text()).toContain('$10.00')
    expect(rows[2].text()).toContain('33.3%')
  })

  it('refetches with the mapped sort_by when the metric toggles and charts that metric', async () => {
    const wrapper = await mountChart()

    const tokensButton = wrapper.findAll('button').find((button) => button.text() === 'By Tokens')
    expect(tokensButton).toBeTruthy()
    await tokensButton!.trigger('click')
    await flushPromises()

    expect(mockedGetApiKeysRanking).toHaveBeenLastCalledWith({
      start_date: '2025-01-01',
      end_date: '2025-01-07',
      limit: 12,
      sort_by: 'total_tokens',
    })
    let chartData = JSON.parse(wrapper.find('.chart-data').text())
    // Others tokens = 2000 - 1000 - 600 = 400
    expect(chartData.datasets[0].data).toEqual([1000, 600, 400])

    const requestsButton = wrapper.findAll('button').find((button) => button.text() === 'By Requests')
    await requestsButton!.trigger('click')
    await flushPromises()

    expect(mockedGetApiKeysRanking).toHaveBeenLastCalledWith(
      expect.objectContaining({ sort_by: 'requests' })
    )
    chartData = JSON.parse(wrapper.find('.chart-data').text())
    expect(chartData.datasets[0].data).toEqual([10, 6, 4])
  })

  it('emits key-click for ranked rows but not for the Others row', async () => {
    const wrapper = await mountChart()

    const rows = wrapper.findAll('[data-testid="key-dist-row"]')
    await rows[0].trigger('click')
    await rows[2].trigger('click')

    const emitted = wrapper.emitted('key-click')
    expect(emitted).toHaveLength(1)
    expect((emitted![0][0] as ApiKeyUsageRankingItem).api_key_id).toBe(11)
  })

  it('scopes the request to a user and skips the Others slice when totals match ranked rows', async () => {
    mockedGetApiKeysRanking.mockResolvedValue({
      ...rankingResponse,
      total_actual_cost: 20,
      total_requests: 16,
      total_tokens: 1600,
      total_keys: 2,
    })
    const wrapper = mount(ApiKeyDistributionChart, {
      props: {
        startDate: '2025-01-01',
        endDate: '2025-01-07',
        userId: 7,
      },
      global: {
        stubs: {
          LoadingSpinner: true,
        },
      },
    })
    await flushPromises()

    expect(mockedGetApiKeysRanking).toHaveBeenCalledWith(
      expect.objectContaining({ user_id: 7 })
    )
    const chartData = JSON.parse(wrapper.find('.chart-data').text())
    expect(chartData.labels).toEqual(['#1 prod-main', '#2 Key #12'])
    expect(wrapper.findAll('[data-testid="key-dist-row"]')).toHaveLength(2)
  })

  it('uses the user-scoped endpoint without user_id and hides owner emails in user scope', async () => {
    const wrapper = mount(ApiKeyDistributionChart, {
      props: {
        startDate: '2025-01-01',
        endDate: '2025-01-07',
        scope: 'user',
        userId: 7,
      },
      global: {
        stubs: {
          LoadingSpinner: true,
        },
      },
    })
    await flushPromises()

    expect(mockedGetApiKeysRanking).not.toHaveBeenCalled()
    expect(mockedGetMyApiKeysRanking).toHaveBeenCalledWith({
      start_date: '2025-01-01',
      end_date: '2025-01-07',
      limit: 12,
      sort_by: 'actual_cost',
    })

    const rows = wrapper.findAll('[data-testid="key-dist-row"]')
    expect(rows[0].text()).toContain('prod-main')
    expect(rows[0].text()).not.toContain('owner@example.com')
    // Others 行的未上榜提示仍然显示
    expect(rows[2].text()).toContain('33 unranked keys')

    // 用户视角：已删除的 Key(第 2 行)不可下钻——用户端接口按活跃 Key 校验会 404
    await rows[1].trigger('click')
    expect(wrapper.emitted('key-click')).toBeUndefined()
    await rows[0].trigger('click')
    expect(wrapper.emitted('key-click')).toHaveLength(1)
  })

  it('shows the error state when the request fails', async () => {
    mockedGetApiKeysRanking.mockRejectedValue(new Error('boom'))
    const wrapper = await mountChart()

    expect(wrapper.text()).toContain('Failed to load dashboard statistics')
  })
})
