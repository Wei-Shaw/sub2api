import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import type { UserMonitorView } from '@/api/channelMonitor'
import MonitorCard from '../monitor/MonitorCard.vue'

const { isQuotaVisible } = vi.hoisted(() => ({
  isQuotaVisible: vi.fn(() => false),
}))

vi.mock('@/utils/featureFlags', () => ({
  isChannelMonitorQuotaVisible: () => isQuotaVisible(),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key, te: () => true }),
  }
})

function makeItem(overrides: Partial<UserMonitorView> = {}): UserMonitorView {
  return {
    id: 1,
    name: 'claude-main',
    provider: 'kimi',
    group_name: '',
    primary_model: 'quota',
    primary_status: 'operational',
    primary_latency_ms: null,
    primary_ping_latency_ms: null,
    availability_7d: 100,
    extra_models: [],
    timeline: [],
    ...overrides,
  }
}

function mountCard(item: UserMonitorView) {
  return mount(MonitorCard, {
    props: {
      item,
      window: '7d',
      availabilityValue: 100,
      countdownSeconds: 0,
    },
    global: {
      stubs: {
        MonitorMetricPair: true,
        MonitorAvailabilityRow: true,
        MonitorTimeline: true,
      },
    },
  })
}

describe('MonitorCard quota snapshot visibility', () => {
  it('hides the quota block when the system switch is off even if data exists', () => {
    isQuotaVisible.mockReturnValue(false)
    const wrapper = mountCard(
      makeItem({
        latest_quota: { source: 'cn_quota', success: true, fetched_at: '2026-08-18T00:00:00Z' },
      }),
    )
    expect(wrapper.find('[data-testid="monitor-quota-view"]').exists()).toBe(false)
  })

  it('renders the quota block when the switch is on and a snapshot exists', () => {
    isQuotaVisible.mockReturnValue(true)
    const wrapper = mountCard(
      makeItem({
        latest_quota: {
          source: 'cn_quota',
          success: true,
          plan_level: 'kimi-plus',
          tiers: [{ window: 'daily', label: 'requests', used_percent: 60 }],
          fetched_at: '2026-08-18T00:00:00Z',
        },
      }),
    )
    expect(wrapper.find('[data-testid="monitor-quota-view"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('kimi-plus')
  })

  it('never renders the quota block without a snapshot', () => {
    isQuotaVisible.mockReturnValue(true)
    const wrapper = mountCard(makeItem())
    expect(wrapper.find('[data-testid="monitor-quota-view"]').exists()).toBe(false)
  })

  // 占位符 "quota" 是主模型存储值，不得作为假模型名直接透出到用户端。
  it('shows the localized quota label instead of the raw placeholder model', () => {
    const wrapper = mountCard(makeItem())
    expect(wrapper.text()).toContain('monitorCommon.checkMode.quota')
  })

  it('keeps the real model name for probe monitors', () => {
    const wrapper = mountCard(makeItem({ primary_model: 'claude-sonnet-4-5' }))
    expect(wrapper.text()).toContain('claude-sonnet-4-5')
    expect(wrapper.text()).not.toContain('monitorCommon.checkMode.quota')
  })
})
