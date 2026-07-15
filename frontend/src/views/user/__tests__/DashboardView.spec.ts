import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const {
  refreshUser,
  getDashboardStats,
  getDashboardTrend,
  getDashboardModels,
  getByDateRange,
  getMyPlatformQuotas,
} = vi.hoisted(() => ({
  refreshUser: vi.fn(),
  getDashboardStats: vi.fn(),
  getDashboardTrend: vi.fn(),
  getDashboardModels: vi.fn(),
  getByDateRange: vi.fn(),
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
    getByDateRange,
  },
}))

vi.mock('@/api/user', () => ({ getMyPlatformQuotas }))

const simpleStub = { template: '<div><slot /></div>' }
const originalTimezone = process.env.TZ

describe('user DashboardView date range', () => {
  beforeEach(() => {
    vi.resetModules()
    vi.useFakeTimers()
    process.env.TZ = 'America/New_York'
    vi.setSystemTime(new Date(2026, 2, 9, 0, 30))

    refreshUser.mockReset().mockResolvedValue(undefined)
    getDashboardStats.mockReset().mockResolvedValue({})
    getDashboardTrend.mockReset().mockResolvedValue({ trend: [] })
    getDashboardModels.mockReset().mockResolvedValue({ models: [] })
    getByDateRange.mockReset().mockResolvedValue({ items: [] })
    getMyPlatformQuotas.mockReset().mockResolvedValue({ platform_quotas: [] })
  })

  afterEach(() => {
    vi.useRealTimers()
    if (originalTimezone === undefined) {
      delete process.env.TZ
    } else {
      process.env.TZ = originalTimezone
    }
  })

  it('uses a seven-calendar-day range and sends the IANA timezone to every usage request', async () => {
    const { default: DashboardView } = await import('../DashboardView.vue')
    const wrapper = mount(DashboardView, {
      global: {
        stubs: {
          AppLayout: simpleStub,
          LoadingSpinner: true,
          UserDashboardStats: true,
          UserDashboardCharts: true,
          UserDashboardRecentUsage: true,
          UserDashboardQuickActions: true,
        },
      },
    })

    await flushPromises()

    expect(getDashboardTrend).toHaveBeenCalledWith({
      start_date: '2026-03-03',
      end_date: '2026-03-09',
      granularity: 'day',
      timezone: 'America/New_York',
    })
    expect(getDashboardModels).toHaveBeenCalledWith({
      start_date: '2026-03-03',
      end_date: '2026-03-09',
      timezone: 'America/New_York',
    })
    expect(getByDateRange).toHaveBeenCalledWith(
      '2026-03-03',
      '2026-03-09',
      undefined,
      'America/New_York'
    )

    wrapper.unmount()
  })
})
