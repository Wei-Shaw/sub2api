import { flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import OpsDashboard from '../OpsDashboard.vue'

const { opsAPI } = vi.hoisted(() => ({
  opsAPI: {
    getAdvancedSettings: vi.fn(),
    getMetricThresholds: vi.fn(),
    getDashboardSnapshotV2: vi.fn(),
    getThroughputTrend: vi.fn(),
    getLatencyHistogram: vi.fn(),
    getErrorDistribution: vi.fn(),
  },
}))

vi.mock('@/api/admin/ops', () => ({ opsAPI, default: opsAPI }))
vi.mock('@/stores', () => ({
  useAppStore: () => ({ showError: vi.fn() }),
  useAdminSettingsStore: () => ({
    opsMonitoringEnabled: true,
    opsQueryModeDefault: 'auto',
    fetch: vi.fn().mockResolvedValue(undefined),
  }),
}))
vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ replace: vi.fn() }),
}))
vi.mock('vue-i18n', async () => ({
  ...(await vi.importActual<typeof import('vue-i18n')>('vue-i18n')),
  useI18n: () => ({ t: (key: string) => key }),
}))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))

function setHidden(hidden: boolean) {
  Object.defineProperty(document, 'hidden', { configurable: true, get: () => hidden })
}

let wrapper: ReturnType<typeof shallowMount> | undefined

beforeEach(() => {
  vi.useFakeTimers()
  opsAPI.getAdvancedSettings.mockResolvedValue({
    display_alert_events: false,
    display_openai_token_stats: false,
    auto_refresh_enabled: true,
    auto_refresh_interval_seconds: 30,
  })
  opsAPI.getMetricThresholds.mockResolvedValue(null)
  opsAPI.getDashboardSnapshotV2.mockResolvedValue({ overview: null, throughput_trend: null, error_trend: null })
  opsAPI.getThroughputTrend.mockResolvedValue(null)
  opsAPI.getLatencyHistogram.mockResolvedValue(null)
  opsAPI.getErrorDistribution.mockResolvedValue(null)
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  delete (document as { hidden?: boolean }).hidden
  vi.clearAllMocks()
  vi.useRealTimers()
})

describe('OpsDashboard auto refresh', () => {
  it('sends no dashboard queries while the tab is hidden and resumes when shown', async () => {
    wrapper = shallowMount(OpsDashboard)
    await flushPromises()
    expect(opsAPI.getDashboardSnapshotV2).toHaveBeenCalledTimes(1)

    setHidden(true)
    await vi.advanceTimersByTimeAsync(10 * 60 * 1000)
    expect(opsAPI.getDashboardSnapshotV2).toHaveBeenCalledTimes(1)

    setHidden(false)
    await vi.advanceTimersByTimeAsync(31 * 1000)
    expect(opsAPI.getDashboardSnapshotV2).toHaveBeenCalledTimes(2)
  })
})
