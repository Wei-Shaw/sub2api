import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, shallowMount } from '@vue/test-utils'
import OpsDashboardHeader from '../OpsDashboardHeader.vue'
import type { OpsDashboardOverview } from '@/api/admin/ops'
import { useAdminSettingsStore } from '@/stores/adminSettings'

const { getAllGroups, getRealtimeTrafficSummary } = vi.hoisted(() => ({
  getAllGroups: vi.fn(),
  getRealtimeTrafficSummary: vi.fn()
}))

vi.mock('@/api', () => ({
  adminAPI: {
    groups: {
      getAll: getAllGroups
    }
  }
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getRealtimeTrafficSummary
  }
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key} ${Object.values(params).join(' ')}` : key
    })
  }
})

function makeOverview(overrides: Partial<OpsDashboardOverview> = {}): OpsDashboardOverview {
  return {
    start_time: '2026-07-28T12:00:00Z',
    end_time: '2026-07-28T13:00:00Z',
    platform: '',
    group_id: null,
    health_score: 100,
    system_metrics: null,
    job_heartbeats: [],
    success_count: 0,
    error_count_total: 0,
    business_limited_count: 0,
    error_count_sla: 0,
    request_count_total: 0,
    request_count_sla: 0,
    availability_available: false,
    token_consumed: 0,
    sla: 0,
    error_rate: 0,
    upstream_error_rate: 0,
    upstream_error_count_excl_429_529: 0,
    upstream_429_count: 0,
    upstream_529_count: 0,
    qps: { current: 0, peak: 0, avg: 0 },
    tps: { current: 0, peak: 0, avg: 0 },
    duration: {},
    ttft: {},
    ...overrides
  }
}

function mountHeader(overview: OpsDashboardOverview) {
  return shallowMount(OpsDashboardHeader, {
    props: {
      overview,
      platform: '',
      groupId: null,
      timeRange: '1h',
      queryMode: 'auto',
      loading: false,
      lastUpdated: new Date('2026-07-28T13:00:00Z')
    },
    global: {
      stubs: {
        Select: true,
        HelpTooltip: true,
        BaseDialog: true,
        Icon: true
      }
    }
  })
}

describe('OpsDashboardHeader health score breakdown', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    useAdminSettingsStore().setOpsRealtimeMonitoringEnabledLocal(true)
    getAllGroups.mockReset().mockResolvedValue([])
    getRealtimeTrafficSummary.mockReset().mockResolvedValue({
      enabled: true,
      summary: null
    })
  })

  it('shows RPM, K TPS and two-decimal TTFT seconds and opens TTFT-sorted details', async () => {
    getRealtimeTrafficSummary.mockResolvedValue({
      enabled: true,
      summary: {
        qps: { current: 2, peak: 3, avg: 1.5 },
        tps: { current: 12345, peak: 20000, avg: 10000 },
		actual_cost_total: 1.2345,
		points: [
		  { time: '2026-07-28T12:59:50Z', rpm: 60, tokens_per_second: 10000, actual_cost: 0.4 },
		  { time: '2026-07-28T12:59:55Z', rpm: 120, tokens_per_second: 12345, actual_cost: 0.8345 }
		]
      }
    })
    const wrapper = mountHeader(makeOverview({
      qps: { current: 2, peak: 3, avg: 1.5 },
      tps: { current: 12345, peak: 20000, avg: 10000 },
      ttft: {
        p99_ms: 1234,
        p95_ms: 1000,
        p90_ms: 900,
        p50_ms: 500,
        avg_ms: 750,
        max_ms: 2345
      }
    }))
    await flushPromises()

    expect(wrapper.text()).toContain('120.0')
    expect(wrapper.text()).toContain('12.35')
    expect(wrapper.text()).toContain('RPM')
	expect(wrapper.get('[data-testid="realtime-traffic-card"]').text()).toContain('K token/s')
	expect(wrapper.get('[data-testid="realtime-metric-cost"]').text()).toContain('$1.23')
	expect(wrapper.get('[data-testid="realtime-traffic-chart"] polyline').attributes('points')).toContain('720.00,10.00')
	expect(wrapper.find('[data-testid="realtime-traffic-chart"] animate').exists()).toBe(false)

	await wrapper.get('[data-testid="realtime-metric-cost"]').trigger('click')
	expect(wrapper.get('[data-testid="realtime-traffic-chart"]').text()).toContain('$0.8345')
    expect(wrapper.get('[data-testid="ttft-card"]').text()).toContain('1.23')
    expect(wrapper.get('[data-testid="ttft-card"]').text()).toContain('2.35')
    expect(wrapper.get('[data-testid="ttft-card"]').text()).toContain('s (P99)')

    await wrapper.get('[data-testid="ttft-details-button"]').trigger('click')
    expect(wrapper.emitted('openRequestDetails')?.at(-1)?.[0]).toEqual({
      title: 'admin.ops.ttftLabel',
      kind: 'success',
      sort: 'ttft_desc',
      has_ttft: true
    })
  })

  it('keeps the latest realtime result when filters change during an in-flight request', async () => {
    let resolveInitial!: (value: unknown) => void
    let resolveFiltered!: (value: unknown) => void
    getRealtimeTrafficSummary
      .mockImplementationOnce(() => new Promise((resolve) => { resolveInitial = resolve }))
      .mockImplementationOnce(() => new Promise((resolve) => { resolveFiltered = resolve }))

    const wrapper = mountHeader(makeOverview())
    await flushPromises()
    expect(getRealtimeTrafficSummary).toHaveBeenCalledTimes(1)

    await wrapper.setProps({ platform: 'openai' })
    await flushPromises()
    expect(getRealtimeTrafficSummary).toHaveBeenCalledTimes(2)

    resolveFiltered({
      enabled: true,
      summary: {
        qps: { current: 4, peak: 4, avg: 4 },
        tps: { current: 1000, peak: 1000, avg: 1000 },
        actual_cost_total: 2,
        points: []
      }
    })
    await flushPromises()
    expect(wrapper.get('[data-testid="realtime-metric-rpm"]').text()).toContain('240.0')

    resolveInitial({
      enabled: true,
      summary: {
        qps: { current: 1, peak: 1, avg: 1 },
        tps: { current: 500, peak: 500, avg: 500 },
        actual_cost_total: 1,
        points: []
      }
    })
    await flushPromises()

    expect(wrapper.get('[data-testid="realtime-metric-rpm"]').text()).toContain('240.0')
  })

  it('does not let an in-flight response overwrite the disabled realtime state', async () => {
    let resolveRequest!: (value: unknown) => void
    getRealtimeTrafficSummary.mockImplementationOnce(
      () => new Promise((resolve) => { resolveRequest = resolve })
    )
    const wrapper = mountHeader(makeOverview())
    await flushPromises()
    expect(getRealtimeTrafficSummary).toHaveBeenCalledTimes(1)

    useAdminSettingsStore().setOpsRealtimeMonitoringEnabledLocal(false)
    await flushPromises()
    expect(wrapper.get('[data-testid="realtime-metric-rpm"]').text()).toContain('0.0')

    resolveRequest({
      enabled: true,
      summary: {
        qps: { current: 5, peak: 5, avg: 5 },
        tps: { current: 1000, peak: 1000, avg: 1000 },
        actual_cost_total: 2,
        points: []
      }
    })
    await flushPromises()

    expect(wrapper.get('[data-testid="realtime-metric-rpm"]').text()).toContain('0.0')
  })

  it('renders backend-provided deduction reasons instead of the legacy smart diagnosis', async () => {
    const wrapper = mountHeader(makeOverview({
      health_score: 94,
      health_score_breakdown: {
        mode: 'infrastructure_only',
        business_included: false,
        score: 94,
        components: [
          {
            key: 'infrastructure_storage',
            score: 100,
            weight: 0.4,
            max_points: 40,
            earned_points: 40,
            deduction_points: 0,
            reasons: []
          },
          {
            key: 'infrastructure_compute',
            score: 100,
            weight: 0.3,
            max_points: 30,
            earned_points: 30,
            deduction_points: 0,
            reasons: []
          },
          {
            key: 'infrastructure_jobs',
            score: 80,
            weight: 0.3,
            max_points: 30,
            earned_points: 24,
            deduction_points: 6,
            reasons: [
              {
                code: 'job_heartbeat_stale',
                job_name: 'ops_alert_evaluator',
                age_seconds: 240,
                max_age_seconds: 180
              }
            ]
          }
        ]
      }
    }))
    await flushPromises()

    const breakdown = wrapper.get('[data-testid="health-score-breakdown"]')
    expect(breakdown.text()).toContain('admin.ops.healthBreakdown.title')
    expect(breakdown.text()).toContain('admin.ops.healthBreakdown.totalDeduction 6')
    expect(breakdown.text()).toContain('admin.ops.healthBreakdown.components.infrastructure_jobs')
    expect(breakdown.text()).toContain('admin.ops.healthBreakdown.reasons.jobStale')
    expect(breakdown.text()).toContain('ops_alert_evaluator')
    expect(wrapper.findAll('[data-testid="health-score-deduction"]')).toHaveLength(1)
    expect(wrapper.find('[data-testid="health-score-idle-mode"]').exists()).toBe(true)
    expect(breakdown.text()).not.toContain('admin.ops.diagnosis')
  })

  it('shows a safe unavailable state when an older backend omits the breakdown', async () => {
    const wrapper = mountHeader(makeOverview({ health_score_breakdown: null }))
    await flushPromises()

    expect(wrapper.get('[data-testid="health-score-breakdown"]').text())
      .toContain('admin.ops.healthBreakdown.unavailable')
  })

  it('keeps fractional score deductions consistent and does not warn for disabled jobs', async () => {
    const wrapper = mountHeader(makeOverview({
      health_score: 98.5,
      disabled_job_names: ['ops_cleanup'],
      job_heartbeats: [{
        job_name: 'ops_cleanup',
        last_success_at: '2026-07-14T12:00:00Z',
        updated_at: '2026-07-14T12:00:00Z'
      }],
      health_score_breakdown: {
        mode: 'business_and_infrastructure',
        business_included: true,
        score: 98.5,
        deduction_points: 1.5,
        components: [{
          key: 'business_quality',
          score: 95.7,
          weight: 0.35,
          max_points: 35,
          earned_points: 33.5,
          deduction_points: 1.5,
          reasons: [{
            code: 'request_error_rate_high',
            value: 1,
            threshold: 0.8
          }]
        }]
      }
    }))
    await flushPromises()

    expect(wrapper.get('[data-testid="health-score-breakdown"]').text())
      .toContain('admin.ops.healthBreakdown.totalDeduction 1.5')
    expect(wrapper.text()).toContain('98.5')
    expect(wrapper.text()).toContain('common.warning 0')
    expect(wrapper.text()).toContain('admin.ops.disabled 1')
  })
})
