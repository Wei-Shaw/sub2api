import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, shallowMount } from '@vue/test-utils'
import OpsDashboardHeader from '../OpsDashboardHeader.vue'
import type { OpsDashboardOverview } from '@/api/admin/ops'

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
    getAllGroups.mockReset().mockResolvedValue([])
    getRealtimeTrafficSummary.mockReset().mockResolvedValue({
      enabled: true,
      summary: null
    })
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
})
