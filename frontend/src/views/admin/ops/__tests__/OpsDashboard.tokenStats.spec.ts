import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import OpsDashboard from '../OpsDashboard.vue'

const mocks = vi.hoisted(() => ({
  route: { query: {} as Record<string, string> },
  routerReplace: vi.fn(),
  displayTokenStats: true,
  fetchAdminSettings: vi.fn(),
  getAdvancedSettings: vi.fn(),
  getMetricThresholds: vi.fn(),
  getDashboardSnapshotV2: vi.fn(),
  getThroughputTrend: vi.fn(),
  getLatencyHistogram: vi.fn(),
  getErrorDistribution: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRoute: () => mocks.route,
  useRouter: () => ({ replace: mocks.routerReplace }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

vi.mock('@/stores', () => ({
  useAppStore: () => ({ showError: vi.fn() }),
  useAdminSettingsStore: () => ({
    opsMonitoringEnabled: true,
    opsQueryModeDefault: 'auto',
    fetch: mocks.fetchAdminSettings,
  }),
}))

vi.mock('@/api/admin/ops', () => {
  const opsAPI = {
    getAdvancedSettings: mocks.getAdvancedSettings,
    getMetricThresholds: mocks.getMetricThresholds,
    getDashboardSnapshotV2: mocks.getDashboardSnapshotV2,
    getThroughputTrend: mocks.getThroughputTrend,
    getLatencyHistogram: mocks.getLatencyHistogram,
    getErrorDistribution: mocks.getErrorDistribution,
  }
  return { opsAPI, default: opsAPI }
})

const AppLayoutStub = defineComponent({ template: '<main><slot /></main>' })
const TokenStatsCardStub = defineComponent({
  name: 'OpsTokenStatsCard',
  props: {
    platformFilter: { type: String, default: '' },
    groupIdFilter: { type: Number, default: null },
    refreshToken: { type: Number, required: true },
  },
  template: '<div data-testid="token-stats-card" :data-platform="platformFilter" />',
})

function mountDashboard() {
  return mount(OpsDashboard, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        BaseDialog: true,
        OpsDashboardHeader: true,
        OpsDashboardSkeleton: true,
        OpsConcurrencyCard: true,
        OpsSwitchRateTrendChart: true,
        OpsThroughputTrendChart: true,
        OpsLatencyChart: true,
        OpsErrorDistributionChart: true,
        OpsErrorTrendChart: true,
        OpsAlertEventsCard: true,
        OpsTokenStatsCard: TokenStatsCardStub,
        OpsSystemLogTable: true,
        OpsSettingsDialog: true,
        OpsAlertRulesCard: true,
        OpsErrorDetailsModal: true,
        OpsErrorDetailModal: true,
        OpsRequestDetailsModal: true,
      },
    },
  })
}

describe('OpsDashboard token stats card', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.route.query = {}
    mocks.displayTokenStats = true
    mocks.fetchAdminSettings.mockResolvedValue(undefined)
    mocks.getAdvancedSettings.mockImplementation(async () => ({
      display_alert_events: false,
      display_openai_token_stats: mocks.displayTokenStats,
      auto_refresh_enabled: false,
      auto_refresh_interval_seconds: 30,
    }))
    mocks.getMetricThresholds.mockResolvedValue({})
    mocks.getDashboardSnapshotV2.mockResolvedValue({
      overview: null,
      throughput_trend: { points: [] },
      error_trend: { points: [] },
    })
    mocks.getThroughputTrend.mockResolvedValue({ points: [] })
    mocks.getLatencyHistogram.mockResolvedValue(null)
    mocks.getErrorDistribution.mockResolvedValue(null)
  })

  it.each([
    ['all platforms', undefined, ''],
    ['OpenAI', 'openai', 'openai'],
    ['DeepSeek', 'deepseek', 'deepseek'],
  ])('renders one card for %s and forwards the platform filter', async (_label, platform, expected) => {
    if (platform) mocks.route.query = { platform }

    const wrapper = mountDashboard()
    await flushPromises()
    await flushPromises()

    const cards = wrapper.findAll('[data-testid="token-stats-card"]')
    expect(cards).toHaveLength(1)
    expect(cards[0].attributes('data-platform')).toBe(expected)
    wrapper.unmount()
  })

  it('does not render the card when the existing dashboard setting is disabled', async () => {
    mocks.displayTokenStats = false

    const wrapper = mountDashboard()
    await flushPromises()
    await flushPromises()

    expect(wrapper.find('[data-testid="token-stats-card"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
