import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

import type { DashboardStats } from '@/types'
import DashboardView from '../DashboardView.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Select from '@/components/common/Select.vue'

enableAutoUnmount(afterEach)

const authStore = vi.hoisted(() => ({ user: { id: 1 } as { id: number } | null }))

vi.mock('@/stores/auth', () => ({ useAuthStore: () => authStore }))
vi.mock('@/composables/useBatchImageAccess', () => ({
  useBatchImageAccess: () => ({ canUseBatchImage: false, refreshBatchImageAccess: vi.fn() })
}))

const { getSnapshotV2, getUserUsageTrend, getUserSpendingRanking } = vi.hoisted(() => ({
  getSnapshotV2: vi.fn(),
  getUserUsageTrend: vi.fn(),
  getUserSpendingRanking: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    dashboard: {
      getSnapshotV2,
      getUserUsageTrend,
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
    push: vi.fn()
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      locale: { value: 'en' }
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

const mountDashboard = (stubDatePicker = true) => mount(DashboardView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      LoadingSpinner: true,
      Icon: true,
      DateRangePicker: stubDatePicker,
      Select: true,
      ModelDistributionChart: true,
      TokenUsageTrend: true,
      Line: true
    }
  }
})

const changeRange = async (wrapper: ReturnType<typeof mountDashboard>, range: {
  startDate: string; endDate: string; preset: string | null
}) => {
  const picker = wrapper.getComponent(DateRangePicker)
  picker.vm.$emit('update:startDate', range.startDate)
  picker.vm.$emit('update:endDate', range.endDate)
  picker.vm.$emit('change', range)
  await flushPromises()
}

const changeGranularity = async (wrapper: ReturnType<typeof mountDashboard>, value: 'day' | 'hour') => {
  const select = wrapper.getComponent(Select)
  select.vm.$emit('update:modelValue', value)
  select.vm.$emit('change', value)
  await flushPromises()
}

const preferenceKey = (userId = 1) => `admin-dashboard-preferences:${userId}`

describe('admin DashboardView', () => {
  beforeEach(() => {
    localStorage.clear()
    authStore.user = { id: 1 }
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date(2026, 8, 20, 12))
    setActivePinia(createPinia())

    getSnapshotV2.mockReset()
    getUserUsageTrend.mockReset()
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

  afterEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it('restores the selected range and granularity before the first request on remount', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    await changeRange(wrapper, { startDate: '2026-09-14', endDate: '2026-09-20', preset: '7days' })
    expect(wrapper.getComponent(Select).props('modelValue')).toBe('day')
    await changeGranularity(wrapper, 'hour')
    wrapper.unmount()
    getSnapshotV2.mockClear()
    getUserUsageTrend.mockClear()
    getUserSpendingRanking.mockClear()

    const restored = mountDashboard()
    await flushPromises()

    expect(restored.getComponent(DateRangePicker).props()).toMatchObject({
      startDate: '2026-09-14', endDate: '2026-09-20'
    })
    expect(restored.getComponent(Select).props('modelValue')).toBe('hour')
    for (const request of [getSnapshotV2, getUserUsageTrend, getUserSpendingRanking]) {
      expect(request).toHaveBeenCalledTimes(1)
      expect(request).toHaveBeenCalledWith(expect.objectContaining({
        start_date: '2026-09-14', end_date: '2026-09-20'
      }))
    }
    expect(getSnapshotV2).toHaveBeenCalledWith(expect.objectContaining({ granularity: 'hour' }))
  })

  it('persists selections applied through the real date picker', async () => {
    const wrapper = mountDashboard(false)
    await flushPromises()
    const picker = wrapper.getComponent(DateRangePicker)
    await picker.get('.date-picker-trigger').trigger('click')
    const preset = picker.findAll('.date-picker-preset').find((button) => button.text() === 'dates.last7Days')
    await preset!.trigger('click')
    await picker.get('.date-picker-apply').trigger('click')
    await flushPromises()
    await changeGranularity(wrapper, 'hour')
    wrapper.unmount()
    getSnapshotV2.mockClear()

    const restored = mountDashboard(false)
    await flushPromises()

    expect(restored.getComponent(DateRangePicker).text()).toContain('dates.last7Days')
    expect(getSnapshotV2).toHaveBeenCalledTimes(1)
    expect(getSnapshotV2).toHaveBeenCalledWith(expect.objectContaining({
      start_date: '2026-09-14', end_date: '2026-09-20', granularity: 'hour'
    }))
  })

  it.each([
    ['thisMonth', '2026-10-01', '2026-10-01'],
    ['lastMonth', '2026-09-01', '2026-09-30']
  ])('recalculates %s across a month boundary', async (preset, startDate, endDate) => {
    localStorage.setItem(preferenceKey(), JSON.stringify({ version: 1, preset, granularity: 'day' }))
    vi.setSystemTime(new Date(2026, 9, 1, 12))
    mountDashboard()
    await flushPromises()
    expect(getSnapshotV2).toHaveBeenCalledWith(expect.objectContaining({
      start_date: startDate, end_date: endDate, granularity: 'day'
    }))
  })

  it.each([
    ['today', '2026-09-20', '2026-09-20', '2026-09-21', '2026-09-21'],
    ['yesterday', '2026-09-19', '2026-09-19', '2026-09-20', '2026-09-20'],
    ['last24Hours', '2026-09-19', '2026-09-20', '2026-09-20', '2026-09-21'],
    ['7days', '2026-09-14', '2026-09-20', '2026-09-15', '2026-09-21'],
    ['14days', '2026-09-07', '2026-09-20', '2026-09-08', '2026-09-21'],
    ['30days', '2026-08-22', '2026-09-20', '2026-08-23', '2026-09-21'],
    ['thisMonth', '2026-09-01', '2026-09-20', '2026-09-01', '2026-09-21'],
    ['lastMonth', '2026-08-01', '2026-08-31', '2026-08-01', '2026-08-31']
  ])('recalculates %s on the next day', async (preset, startDate, endDate, nextStart, nextEnd) => {
    const wrapper = mountDashboard()
    await flushPromises()
    await changeRange(wrapper, { preset, startDate, endDate })
    const saved = localStorage.getItem(preferenceKey())
    wrapper.unmount()
    vi.setSystemTime(new Date(2026, 8, 21, 12))
    getSnapshotV2.mockClear()

    mountDashboard()
    await flushPromises()

    expect(getSnapshotV2).toHaveBeenCalledTimes(1)
    expect(getSnapshotV2).toHaveBeenCalledWith(expect.objectContaining({
      start_date: nextStart, end_date: nextEnd
    }))
    expect(localStorage.getItem(preferenceKey())).toBe(saved)
  })

  it('restores custom dates without turning them into a relative preset', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    await changeRange(wrapper, { startDate: '2026-09-20', endDate: '2026-09-20', preset: null })
    await changeGranularity(wrapper, 'day')
    wrapper.unmount()
    vi.setSystemTime(new Date(2026, 9, 3, 12))

    const restored = mountDashboard()
    await flushPromises()

    expect(restored.getComponent(DateRangePicker).props()).toMatchObject({
      startDate: '2026-09-20', endDate: '2026-09-20'
    })
    expect(restored.getComponent(Select).props('modelValue')).toBe('day')
    await changeRange(restored, { startDate: '2026-10-03', endDate: '2026-10-03', preset: 'today' })
    expect(restored.getComponent(Select).props('modelValue')).toBe('hour')
  })

  it('keeps preferences isolated when switching accounts', async () => {
    const first = mountDashboard()
    await flushPromises()
    await changeRange(first, { startDate: '2026-09-01', endDate: '2026-09-10', preset: null })
    first.unmount()
    authStore.user = { id: 2 }

    const second = mountDashboard()
    await flushPromises()
    expect(second.getComponent(DateRangePicker).props('startDate')).toBe('2026-09-19')
    await changeRange(second, { startDate: '2026-09-20', endDate: '2026-09-20', preset: 'today' })
    second.unmount()
    authStore.user = { id: 1 }

    const restored = mountDashboard()
    await flushPromises()
    expect(restored.getComponent(DateRangePicker).props()).toMatchObject({
      startDate: '2026-09-01', endDate: '2026-09-10'
    })
    expect(restored.getComponent(Select).props('modelValue')).toBe('day')
  })

  it('does not persist without a logged-in user', async () => {
    authStore.user = null
    const wrapper = mountDashboard()
    await flushPromises()
    await changeGranularity(wrapper, 'day')
    expect(localStorage.length).toBe(0)
  })

  it('persists granularity changes alone with the default relative range', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    await changeGranularity(wrapper, 'day')
    wrapper.unmount()
    vi.setSystemTime(new Date(2026, 8, 21, 12))

    const restored = mountDashboard()
    await flushPromises()
    expect(restored.getComponent(DateRangePicker).props()).toMatchObject({
      startDate: '2026-09-20', endDate: '2026-09-21'
    })
    expect(restored.getComponent(Select).props('modelValue')).toBe('day')
  })

  it.each([
    'not json', 'null', '[]', '{}',
    JSON.stringify({ version: 2, preset: 'today', granularity: 'day' }),
    JSON.stringify({ version: 1, preset: 'unknown', granularity: 'day' }),
    JSON.stringify({ version: 1, preset: 'today', granularity: 'week' }),
    ...[
      ['2026-02-30', '2026-03-01'],
      ['2026-09-20', '2026-09-10'],
      ['2026-9-01', '2026-09-20'],
      ['', '2026-09-20'],
      [null, '2026-09-20'],
      ['2026-09-01', 'invalid']
    ].map(([startDate, endDate]) => JSON.stringify({
      version: 1, preset: null, startDate, endDate, granularity: 'day'
    }))
  ])('falls back safely for invalid preference %s', async (value) => {
    localStorage.setItem(preferenceKey(), value)
    const wrapper = mountDashboard()
    await flushPromises()

    expect(wrapper.getComponent(Select).props('modelValue')).toBe('hour')
    expect(getSnapshotV2).toHaveBeenCalledTimes(1)
    expect(getSnapshotV2).toHaveBeenCalledWith(expect.objectContaining({
      start_date: '2026-09-19', end_date: '2026-09-20', granularity: 'hour'
    }))
    expect(localStorage.getItem(preferenceKey())).toBe(value)
  })

  it('still loads and accepts changes when storage reads and writes throw', async () => {
    vi.spyOn(localStorage, 'getItem').mockImplementation(() => { throw new Error('unavailable') })
    vi.spyOn(localStorage, 'setItem').mockImplementation(() => { throw new Error('quota exceeded') })
    const wrapper = mountDashboard()
    await flushPromises()

    expect(getSnapshotV2).toHaveBeenCalledTimes(1)
    expect(wrapper.getComponent(Select).props('modelValue')).toBe('hour')
    await changeRange(wrapper, { startDate: '2026-09-14', endDate: '2026-09-20', preset: '7days' })
    await changeGranularity(wrapper, 'hour')
    expect(getSnapshotV2).toHaveBeenLastCalledWith(expect.objectContaining({
      start_date: '2026-09-14', end_date: '2026-09-20', granularity: 'hour'
    }))
  })

  it('uses last 24 hours as default dashboard range', async () => {
    mountDashboard()

    await flushPromises()

    const now = new Date()
    const yesterday = new Date(now.getTime() - 24 * 60 * 60 * 1000)

    expect(getSnapshotV2).toHaveBeenCalledTimes(1)
    expect(getSnapshotV2).toHaveBeenCalledWith(expect.objectContaining({
      start_date: formatLocalDate(yesterday),
      end_date: formatLocalDate(now),
      granularity: 'hour'
    }))
    expect(localStorage.getItem(preferenceKey())).toBeNull()
  })
})
