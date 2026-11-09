import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

import type { DashboardStats } from '@/types'
import DashboardView from '../DashboardView.vue'
import Select from '@/components/common/Select.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import ModelDistributionChart from '@/components/charts/ModelDistributionChart.vue'

const { getSnapshotV2, getUserUsageTrend, getUserSpendingRanking, getApiKeyUsageTrend, push } = vi.hoisted(() => ({
  push: vi.fn(),
  getSnapshotV2: vi.fn(),
  getUserUsageTrend: vi.fn(),
  getApiKeyUsageTrend: vi.fn(),
  getUserSpendingRanking: vi.fn()
}))

vi.mock('vue-chartjs', () => ({
  Line: { name: 'Line', props: ['data', 'options'], template: '<div class="line-chart" />' }
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    dashboard: {
      getSnapshotV2,
      getUserUsageTrend,
      getApiKeyUsageTrend,
      getUserSpendingRanking
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn()
  })
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push
  })
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

const formatLocalDate = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

const createDashboardStats = (): DashboardStats => ({
  total_users: 0,
  today_new_users: 0,
  active_users: 0,
  hourly_active_users: 0,
  stats_updated_at: '',
  stats_stale: false,
  total_api_keys: 0,
  active_api_keys: 0,
  total_accounts: 0,
  normal_accounts: 0,
  error_accounts: 0,
  ratelimit_accounts: 0,
  overload_accounts: 0,
  total_requests: 0,
  total_input_tokens: 0,
  total_output_tokens: 0,
  total_cache_creation_tokens: 0,
  total_cache_read_tokens: 0,
  total_tokens: 0,
  total_cost: 0,
  total_actual_cost: 0,
  today_requests: 0,
  today_input_tokens: 0,
  today_output_tokens: 0,
  today_cache_creation_tokens: 0,
  today_cache_read_tokens: 0,
  today_tokens: 0,
  today_cost: 0,
  today_actual_cost: 0,
  average_duration_ms: 0,
  uptime: 0,
  rpm: 0,
  tpm: 0
})

describe('admin DashboardView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())

    push.mockReset()
    getSnapshotV2.mockReset()
    getUserUsageTrend.mockReset()
    getApiKeyUsageTrend.mockReset()
    getApiKeyUsageTrend.mockResolvedValue({ trend: [] })
    getUserSpendingRanking.mockReset()

    getSnapshotV2.mockResolvedValue({
      stats: createDashboardStats(),
      trend: [],
      models: []
    })
    getUserUsageTrend.mockResolvedValue({
      trend: [],
      start_date: '',
      end_date: '',
      granularity: 'hour'
    })
    getUserSpendingRanking.mockResolvedValue({
      ranking: [],
      total_actual_cost: 0,
      total_requests: 0,
      total_tokens: 0,
      start_date: '',
      end_date: ''
    })
  })

  it('switches metrics and ignores a stale tokens response', async () => {
    let resolveTokens!: (value: any) => void
    getUserUsageTrend.mockImplementationOnce(() => new Promise(resolve => { resolveTokens = resolve }))
    getUserUsageTrend.mockResolvedValueOnce({ trend: [{ date: '2026-01-01', user_id: 2, username: 'spender', email: '', requests: 1, tokens: 10, cost: 5, actual_cost: 5 }] })
    const wrapper = mount(DashboardView, {
      global: { stubs: {
        AppLayout: { template: '<div><slot /></div>' }, LoadingSpinner: true,
        Icon: true, DateRangePicker: true, Select: true, ModelDistributionChart: true,
        TokenUsageTrend: true
      } }
    })
    await flushPromises()
    expect(getUserUsageTrend).toHaveBeenCalledWith(expect.objectContaining({ metric: 'tokens', limit: 12 }))
    await wrapper.findAll('button').find(button => button.text() === 'admin.dashboard.actualSpending')!.trigger('click')
    expect(getUserUsageTrend).toHaveBeenCalledWith(expect.objectContaining({ metric: 'actual_cost', limit: 12 }))
    await flushPromises()
    const chart = wrapper.findComponent({ name: 'Line' })
    expect(chart.props('data').datasets[0].data).toEqual([5])
    expect(chart.props('options').scales.y.ticks.callback(5)).toBe('$5.00')
    resolveTokens({ trend: [{ date: '2026-01-01', user_id: 1, username: 'tokens', email: '', requests: 1, tokens: 100, cost: 1, actual_cost: 1 }] })
    await flushPromises()
    expect(chart.props('data').datasets[0].data).toEqual([5])
  })

  it('uses last 24 hours as default dashboard range', async () => {
    mount(DashboardView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          LoadingSpinner: true,
          Icon: true,
          DateRangePicker: true,
          Select: true,
          ModelDistributionChart: true,
          TokenUsageTrend: true,
          Line: true
        }
      }
    })

    await flushPromises()

    const now = new Date()
    const yesterday = new Date(now.getTime() - 24 * 60 * 60 * 1000)

    expect(getApiKeyUsageTrend).not.toHaveBeenCalled()
    expect(getSnapshotV2).toHaveBeenCalledTimes(1)
    expect(getSnapshotV2).toHaveBeenCalledWith(expect.objectContaining({
      start_date: formatLocalDate(yesterday),
      end_date: formatLocalDate(now),
      granularity: 'hour'
    }))
  })

  const mountDashboard = () => mount(DashboardView, {
    global: { stubs: {
      AppLayout: { template: '<div><slot /></div>' }, LoadingSpinner: true,
      Icon: true, DateRangePicker: true, Select: true, ModelDistributionChart: true,
      TokenUsageTrend: true
    } }
  })

  const selectDimension = async (wrapper: ReturnType<typeof mountDashboard>, dimension: string) => {
    const select = wrapper.findAllComponents(Select).find(component =>
      component.props('ariaLabel') === 'admin.dashboard.usageDimension'
    )!
    select.vm.$emit('update:modelValue', dimension)
    select.vm.$emit('change', dimension)
    await flushPromises()
  }

  it('switches the dashboard dimension, metrics and usage navigation', async () => {
    getSnapshotV2.mockResolvedValue({
      stats: { ...createDashboardStats(), today_new_users: 3, active_users: 4,
        total_api_keys: 14, today_new_api_keys: 2, today_active_api_keys: 8 },
      trend: [], models: []
    })
    const key = { user_id: 0, api_key_id: 9, key_name: 'shared', email: '', username: '', actual_cost: 4, requests: 2, tokens: 30 }
    getUserSpendingRanking.mockResolvedValue({ ranking: [key], total_actual_cost: 5, total_requests: 3, total_tokens: 50 })
    const points = [
      { date: '2026-01-01', api_key_id: 1, key_name: 'shared', requests: 1, tokens: 100, actual_cost: 2 },
      { date: '2026-01-02', api_key_id: 2, key_name: 'shared', requests: 2, tokens: 200, actual_cost: 5 }
    ]
    getApiKeyUsageTrend.mockResolvedValue({ trend: points })
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.text()).toContain('+3')
    await selectDimension(wrapper, 'api_key')
    expect(getApiKeyUsageTrend).toHaveBeenLastCalledWith(expect.objectContaining({ metric: 'tokens', limit: 12 }))
    let chart = wrapper.findComponent({ name: 'Line' })
    expect(chart.props('data').datasets.map((dataset: any) => dataset.label)).toEqual(['shared (#1)', 'shared (#2)'])
    expect(chart.props('data').datasets.map((dataset: any) => dataset.data)).toEqual([[100, 0], [0, 200]])

    await wrapper.findAll('button').find(button => button.text() === 'admin.dashboard.actualSpending')!.trigger('click')
    await flushPromises()
    expect(getApiKeyUsageTrend).toHaveBeenLastCalledWith(expect.objectContaining({ metric: 'actual_cost', limit: 12 }))
    chart = wrapper.findComponent({ name: 'Line' })
    expect(chart.props('data').datasets.map((dataset: any) => dataset.data)).toEqual([[2, 0], [0, 5]])
    expect(chart.props('options').scales.y.ticks.callback(5)).toBe('$5.00')
    // Key-only activity is opt-in; aggregate totals retain their scope.
    expect(getSnapshotV2).toHaveBeenCalledTimes(2)
    expect(getSnapshotV2.mock.calls[0][0]).not.toHaveProperty('include_api_key_activity')
    expect(getSnapshotV2).toHaveBeenLastCalledWith(expect.objectContaining({ include_api_key_activity: true }))
    expect(getUserSpendingRanking).toHaveBeenCalledTimes(2)
    expect(getUserSpendingRanking).toHaveBeenLastCalledWith(expect.objectContaining({ group_by: 'api_key' }))
    expect(wrapper.text()).toContain('admin.dashboard.newApiKeysToday')
    expect(wrapper.text()).toContain('+2')
    expect(wrapper.text()).toContain('8 admin.dashboard.todayActiveApiKeys')
    const distribution = wrapper.findComponent(ModelDistributionChart)
    expect(distribution.props('dimension')).toBe('api_key')
    expect(distribution.props('rankingTotalActualCost')).toBe(5)
    distribution.vm.$emit('ranking-click', key)
    expect(push).toHaveBeenCalledWith({ path: '/admin/usage', query: expect.objectContaining({ api_key_id: '9' }) })
    expect(push.mock.calls[0][0].query).not.toHaveProperty('user_id')
    await selectDimension(wrapper, 'user')
    expect(getUserUsageTrend).toHaveBeenLastCalledWith(expect.objectContaining({ metric: 'actual_cost', limit: 12 }))
    expect(wrapper.text()).toContain('+3')
    expect(distribution.props('dimension')).toBe('user')
    expect(getUserSpendingRanking.mock.lastCall![0]).not.toHaveProperty('group_by')
    wrapper.unmount()
  })

  it('ignores late responses when switching dimensions in either direction', async () => {
    let resolveUser!: (value: any) => void
    let resolveKey!: (value: any) => void
    getUserUsageTrend.mockImplementationOnce(() => new Promise(resolve => { resolveUser = resolve }))
    getApiKeyUsageTrend.mockResolvedValueOnce({ trend: [
      { date: '2026-01-01', api_key_id: 7, key_name: '', requests: 1, tokens: 7, actual_cost: 1 }
    ] })
    const wrapper = mountDashboard()
    await flushPromises()
    await selectDimension(wrapper, 'api_key')
    resolveUser({ trend: [] })
    await flushPromises()
    expect(wrapper.findComponent({ name: 'Line' }).props('data').datasets[0].label).toBe('keys.apiKey (#7)')

    getApiKeyUsageTrend.mockImplementationOnce(() => new Promise(resolve => { resolveKey = resolve }))
    await wrapper.findAll('button').find(button => button.text() === 'admin.dashboard.actualSpending')!.trigger('click')
    getUserUsageTrend.mockResolvedValueOnce({ trend: [
      { date: '2026-01-01', user_id: 3, username: 'user', email: '', requests: 1, tokens: 3, cost: 4, actual_cost: 4 }
    ] })
    await selectDimension(wrapper, 'user')
    resolveKey({ trend: [] })
    await flushPromises()
    expect(wrapper.findComponent({ name: 'Line' }).props('data').datasets[0].label).toBe('user')
    wrapper.unmount()
  })

  it('retains the key dimension for date, granularity and refresh changes', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    await selectDimension(wrapper, 'api_key')
    const picker = wrapper.findComponent(DateRangePicker)
    picker.vm.$emit('update:startDate', '2026-01-01')
    picker.vm.$emit('update:endDate', '2026-01-07')
    picker.vm.$emit('change', { startDate: '2026-01-01', endDate: '2026-01-07', preset: null })
    await flushPromises()
    expect(getApiKeyUsageTrend).toHaveBeenLastCalledWith(expect.objectContaining({
      start_date: '2026-01-01', end_date: '2026-01-07', granularity: 'day'
    }))
    const granularity = wrapper.findAllComponents(Select).find(component => component.props('modelValue') === 'day')!
    granularity.vm.$emit('update:modelValue', 'hour')
    granularity.vm.$emit('change', 'hour')
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'common.refresh')!.trigger('click')
    await flushPromises()
    expect(getApiKeyUsageTrend).toHaveBeenLastCalledWith(expect.objectContaining({ granularity: 'hour' }))
    expect(getApiKeyUsageTrend).toHaveBeenCalledTimes(4)
    expect(getUserUsageTrend).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('clears the previous series for empty or failed key queries', async () => {
    getUserUsageTrend.mockResolvedValue({ trend: [
      { date: '2026-01-01', user_id: 1, username: 'user', email: '', requests: 1, tokens: 1, cost: 1, actual_cost: 1 }
    ] })
    const wrapper = mountDashboard()
    await flushPromises()
    await selectDimension(wrapper, 'api_key')
    expect(wrapper.findComponent({ name: 'Line' }).exists()).toBe(false)
    await selectDimension(wrapper, 'user')
    const error = vi.spyOn(console, 'error').mockImplementation(() => {})
    getApiKeyUsageTrend.mockRejectedValueOnce(new Error('query failed'))
    await selectDimension(wrapper, 'api_key')
    expect(wrapper.findComponent({ name: 'Line' }).exists()).toBe(false)
    expect(wrapper.text()).toContain('admin.dashboard.noDataAvailable')
    error.mockRestore()
    wrapper.unmount()
  })

  it('does not overwrite a key ranking with a delayed user ranking', async () => {
    let resolveUser!: (value: any) => void
    getUserSpendingRanking.mockImplementationOnce(() => new Promise(resolve => { resolveUser = resolve }))
    const key = { user_id: 0, api_key_id: 9, key_name: 'shared', email: '', username: '', actual_cost: 4, requests: 2, tokens: 30 }
    getUserSpendingRanking.mockResolvedValueOnce({ ranking: [key], total_actual_cost: 4 })
    const wrapper = mountDashboard()
    await flushPromises()
    await selectDimension(wrapper, 'api_key')
    resolveUser({ ranking: [], total_actual_cost: 100 })
    await flushPromises()
    expect(wrapper.findComponent(ModelDistributionChart).props('rankingItems')).toEqual([key])
    expect(wrapper.findComponent(ModelDistributionChart).props('rankingTotalActualCost')).toBe(4)
    wrapper.unmount()
  })

  it('keeps key activity stats when a date-only snapshot supersedes its charts', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    let resolveKeyStats!: (value: any) => void
    getSnapshotV2.mockImplementationOnce(() => new Promise(resolve => { resolveKeyStats = resolve }))
    await selectDimension(wrapper, 'api_key')
    getSnapshotV2.mockResolvedValueOnce({ trend: [], models: [] })
    const picker = wrapper.findComponent(DateRangePicker)
    picker.vm.$emit('update:startDate', '2026-01-01')
    picker.vm.$emit('change', { startDate: '2026-01-01', endDate: '2026-01-07', preset: null })
    await flushPromises()
    expect(getSnapshotV2).toHaveBeenLastCalledWith(expect.objectContaining({ include_stats: false }))
    resolveKeyStats({ stats: { ...createDashboardStats(), today_active_api_keys: 8 }, trend: [], models: [] })
    await flushPromises()
    expect(wrapper.text()).toContain('8 admin.dashboard.todayActiveApiKeys')
    wrapper.unmount()
  })

})
