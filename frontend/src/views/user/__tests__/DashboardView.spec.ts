import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import DashboardView from '../DashboardView.vue'

const { refreshUser, getDashboardStats, getDashboardTrend, getDashboardModels, query, getMyPlatformQuotas } = vi.hoisted(() => ({
  refreshUser: vi.fn(),
  getDashboardStats: vi.fn(),
  getDashboardTrend: vi.fn(),
  getDashboardModels: vi.fn(),
  query: vi.fn(),
  getMyPlatformQuotas: vi.fn(),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    user: { balance: 0 },
    isSimpleMode: false,
    refreshUser,
  }),
}))

vi.mock('@/api/usage', () => ({
  usageAPI: {
    getDashboardStats,
    getDashboardTrend,
    getDashboardModels,
    query,
  },
}))

vi.mock('@/api/user', () => ({
  getMyPlatformQuotas,
}))

const UserDashboardChartsStub = {
  template: '<button data-test="last24hours" @click="emitPreset">last24hours</button>',
  methods: {
    emitPreset() {
      this.$emit('update:startDate', '2026-03-07')
      this.$emit('update:endDate', '2026-03-08')
      this.$emit('dateRangeChange', {
        startDate: '2026-03-07',
        endDate: '2026-03-08',
        preset: 'last24Hours'
      })
    }
  }
}

describe('user DashboardView', () => {
  beforeEach(() => {
    refreshUser.mockReset()
    getDashboardStats.mockReset()
    getDashboardTrend.mockReset()
    getDashboardModels.mockReset()
    query.mockReset()
    getMyPlatformQuotas.mockReset()

    refreshUser.mockResolvedValue(undefined)
    getDashboardStats.mockResolvedValue({})
    getDashboardTrend.mockResolvedValue({ trend: [], start_date: '', end_date: '', granularity: 'day' })
    getDashboardModels.mockResolvedValue({ models: [], start_date: '', end_date: '' })
    query.mockResolvedValue({ items: [], total: 0, pages: 0 })
    getMyPlatformQuotas.mockResolvedValue({ platform_quotas: [] })
  })

  it('uses rolling last24hours period for charts and recent usage when selected', async () => {
    const wrapper = mount(DashboardView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          LoadingSpinner: true,
          UserDashboardStats: true,
          UserDashboardCharts: UserDashboardChartsStub,
          UserDashboardRecentUsage: true,
          UserDashboardQuickActions: true,
        },
      },
    })

    await flushPromises()
    getDashboardTrend.mockClear()
    getDashboardModels.mockClear()
    query.mockClear()

    await wrapper.get('[data-test="last24hours"]').trigger('click')
    await flushPromises()

    expect(getDashboardTrend).toHaveBeenCalledWith(expect.objectContaining({
      period: 'last24hours',
      start_date: undefined,
      end_date: undefined,
      granularity: 'hour'
    }))
    expect(getDashboardModels).toHaveBeenCalledWith(expect.objectContaining({
      period: 'last24hours',
      start_date: undefined,
      end_date: undefined,
    }))
    expect(query).toHaveBeenCalledWith(expect.objectContaining({
      period: 'last24hours',
      start_date: undefined,
      end_date: undefined,
      page_size: 5
    }))
  })
})
