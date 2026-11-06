import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import OpsDashboardHeader from '../OpsDashboardHeader.vue'
import OpsTrafficCards from '../OpsTrafficCards.vue'
import { opsAPI, type OpsDashboardOverview, type OpsTrafficSummary } from '@/api/admin/ops'

vi.mock('@/api', () => ({ adminAPI: { groups: { getAll: vi.fn().mockResolvedValue([]) } } }))
vi.mock('@/api/admin/ops', () => ({ opsAPI: { getRealtimeTrafficSummary: vi.fn().mockResolvedValue({ enabled: true, summary: null }) } }))
vi.mock('@/stores', () => ({ useAdminSettingsStore: () => ({ opsRealtimeMonitoringEnabled: true, setOpsRealtimeMonitoringEnabledLocal: vi.fn() }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key })
}))
enableAutoUnmount(afterEach)

// Upstream carries a directional split; downstream keeps only side totals, so the
// header integration covers both the directional view and the legacy fallback.
const traffic: OpsTrafficSummary = {
  source: 'http_requests',
  request_count: 2,
  upstream: {
    original_bytes: 2048, actual_bytes: 1024, original_sample_count: 4, actual_sample_count: 4, paired_sample_count: 4, compression_percent: 50,
    request: { original_bytes: 1024, actual_bytes: 512, original_sample_count: 2, actual_sample_count: 2, compression_percent: 50 },
    response: { original_bytes: 1024, actual_bytes: 512, original_sample_count: 2, actual_sample_count: 2, compression_percent: 25 }
  },
  downstream: { original_bytes: null, actual_bytes: null, original_sample_count: 0, actual_sample_count: 0, paired_sample_count: 0, compression_percent: null }
}
const overview = {
  start_time: '2026-06-01T00:00:00Z',
  end_time: '2026-06-01T01:00:00Z',
  tps: { avg: 242.7 }, request_count_total: 99, traffic
} as OpsDashboardOverview
const HelpTooltipStub = { template: '<span><slot name="trigger" /></span>' }

function mountHeader() {
  return mount(OpsDashboardHeader, {
    props: { overview, platform: 'openai', groupId: 7, timeRange: '6h', queryMode: 'preagg', loading: false, lastUpdated: null },
    global: { stubs: { Select: true, HelpTooltip: HelpTooltipStub, BaseDialog: true, Icon: true } }
  })
}

describe('OpsDashboardHeader traffic integration', () => {
  it('places two traffic cards as direct output-TPS siblings and uses traffic ledger counts, not usage/error counts', async () => {
    const wrapper = mountHeader()
    await flushPromises()
    const cards = wrapper.getComponent(OpsTrafficCards)
    expect(cards.props('traffic')).toEqual(traffic)
    expect(cards.props('startTime')).toBe(overview.start_time)
    expect(cards.props('endTime')).toBe(overview.end_time)
    expect(cards.findAll('article')).toHaveLength(2)

    const tps = wrapper.get('[data-testid="output-tps-card"]')
    const upstream = wrapper.get('[data-testid="traffic-upstream-card"]')
    const downstream = wrapper.get('[data-testid="traffic-downstream-card"]')
    const grid = tps.element.parentElement
    expect(grid?.querySelectorAll(':scope > [style]').length).toBe(9)
    for (const card of [upstream, downstream]) {
      expect(card.element.parentElement).toBe(tps.element.parentElement)
      expect(card.classes()).toEqual(expect.arrayContaining(tps.classes()))
    }
    expect(upstream.attributes('style')).toContain('order: 8')
    expect(downstream.attributes('style')).toContain('order: 9')
    expect(wrapper.find('[data-testid="traffic-composition-card"]').exists()).toBe(false)
    expect(wrapper.html().indexOf('data-testid="traffic-upstream-card"')).toBeGreaterThan(wrapper.html().indexOf('data-testid="output-tps-card"'))
    expect(wrapper.html().indexOf('data-testid="traffic-downstream-card"')).toBeLessThan(wrapper.html().indexOf('admin.ops.memory'))
    // Upstream shows the request body on top and the response body below.
    expect(wrapper.get('[data-testid="traffic-upstream-request"]').text()).toBe('512 B')
    expect(wrapper.get('[data-testid="traffic-upstream-response"]').text()).toBe('512 B')
    expect(wrapper.get('[data-testid="traffic-upstream-request-compression"]').text()).toBe('50.0%')
    expect(wrapper.get('[data-testid="traffic-upstream-response-compression"]').text()).toBe('25.0%')
    // Downstream has no directional split and falls back to the side totals.
    expect(wrapper.get('[data-testid="traffic-downstream-combined"]').text()).toBe('—')
    // The overview usage/error population can differ from traffic's HTTP ledger.
    expect(cards.props('traffic')?.request_count).toBe(2)
    expect(cards.props('traffic')?.request_count).not.toBe(overview.request_count_total)
  })

  it('calculates bandwidth from the returned overview times, not the toolbar or realtime window', async () => {
    const wrapper = mountHeader()
    await flushPromises()
    // Toolbar is 6h and realtime defaults to 1min, but the returned overview covers 1h.
    expect(wrapper.get('[data-testid="traffic-upstream-request-bandwidth"]').text()).toBe('1.14 bps')
    expect(wrapper.get('[data-testid="traffic-upstream-response-bandwidth"]').text()).toBe('1.14 bps')
    expect(wrapper.get('[data-testid="traffic-downstream-combined-bandwidth"]').text()).toBe('—')

    await wrapper.setProps({
      timeRange: 'custom',
      customStartTime: '2026-06-01T00:00:00Z',
      customEndTime: '2026-06-01T00:10:00Z',
      overview: { ...overview, end_time: '2026-06-01T00:30:00Z' }
    })
    expect(wrapper.get('[data-testid="traffic-upstream-request-bandwidth"]').text()).toBe('2.28 bps')
    expect(wrapper.get('[data-testid="traffic-upstream-response-bandwidth"]').text()).toBe('2.28 bps')
  })

  it('reacts to the existing overview/loading props without an independent fetch, including fullscreen', async () => {
    const wrapper = mountHeader()
    await flushPromises()
    vi.mocked(opsAPI.getRealtimeTrafficSummary).mockClear()
    await wrapper.setProps({ loading: true, fullscreen: true })
    expect(wrapper.getComponent(OpsTrafficCards).props('loading')).toBe(true)
    expect(wrapper.find('[data-testid="traffic-upstream-request"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="traffic-upstream-request-bandwidth"]').exists()).toBe(false)

    const nextTraffic = {
      ...traffic,
      upstream: {
        ...traffic.upstream,
        request: { original_bytes: 1024, actual_bytes: 768, original_sample_count: 2, actual_sample_count: 2, compression_percent: 75 }
      }
    }
    await wrapper.setProps({ loading: false, overview: { ...overview, traffic: nextTraffic } })
    await flushPromises()
    expect(wrapper.get('[data-testid="traffic-upstream-request"]').text()).toBe('768 B')
    expect(wrapper.get('[data-testid="traffic-upstream-request-compression"]').text()).toBe('75.0%')
    expect(wrapper.get('[data-testid="traffic-upstream-request-bandwidth"]').text()).toBe('1.71 bps')
    expect(opsAPI.getRealtimeTrafficSummary).not.toHaveBeenCalled()

    await wrapper.setProps({ overview: { ...overview, traffic: undefined } })
    expect(wrapper.get('[data-testid="traffic-upstream-combined"]').text()).toBe('—')
    expect(wrapper.get('[data-testid="traffic-downstream-combined-compression"]').text()).toBe('—')
  })
})
