import { mount, flushPromises, enableAutoUnmount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia } from 'pinia'
import { defineComponent, h } from 'vue'
import { useChannelMonitorFormat } from '../useChannelMonitorFormat'
import { useAppStore } from '@/stores/app'
import MonitorRunResultDialog from '@/components/admin/monitor/MonitorRunResultDialog.vue'
import MonitorTimeline from '@/components/user/monitor/MonitorTimeline.vue'
import { getPublicSettings } from '@/api/auth'
import type { PublicSettings } from '@/types'

vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key }),
}))
vi.mock('@/api/auth', () => ({ getPublicSettings: vi.fn() }))
vi.mock('@/api/admin/system', () => ({ checkUpdates: vi.fn() }))

const initial = { first_token_thresholds_ms: { warn: 1000, slow: 3000, critical: 6000 } } as PublicSettings
enableAutoUnmount(afterEach)
beforeEach(() => {
  delete window.__APP_CONFIG__
  vi.mocked(getPublicSettings).mockReset().mockResolvedValue(initial)
})

const Host = defineComponent({
  setup() {
    const { statusBadgeClass, statusLabel, latencyTextClass } = useChannelMonitorFormat()
    return () => h('div', [
      h('span', { class: statusBadgeClass('operational', 1500), 'data-testid': 'badge' }, statusLabel('operational', 1500)),
      h('span', { class: latencyTextClass(1500), 'data-testid': 'number' }, '1500'),
      h('span', { class: statusBadgeClass('failed', 1500), 'data-testid': 'failure' }, statusLabel('failed', 1500)),
      h('span', { class: statusBadgeClass('degraded', null), 'data-testid': 'quota' }, statusLabel('degraded', null)),
    ])
  },
})

describe('configured channel monitor latency colors', () => {
  it('refreshes stale injected settings and preserves failures and quota warnings', async () => {
    window.__APP_CONFIG__ = { first_token_thresholds_ms: { warn: 10000, slow: 30000, critical: 60000 } } as PublicSettings
    const pinia = createPinia()
    useAppStore(pinia).initFromInjectedConfig()
    const wrapper = mount(Host, { global: { plugins: [pinia] } })
    expect(wrapper.get('[data-testid="badge"]').classes()).toContain('bg-emerald-100')
    await flushPromises()
    expect(getPublicSettings).toHaveBeenCalledOnce()
    expect(wrapper.get('[data-testid="badge"]').classes()).toContain('bg-amber-100')
    expect(wrapper.get('[data-testid="badge"]').text()).toBe('monitorCommon.status.degraded')
    expect(wrapper.get('[data-testid="number"]').classes()).toContain('text-amber-600')
    expect(wrapper.get('[data-testid="failure"]').classes()).toContain('bg-red-100')
    expect(wrapper.get('[data-testid="failure"]').text()).toBe('monitorCommon.status.failed')
    expect(wrapper.get('[data-testid="quota"]').classes()).toContain('bg-amber-100')
  })

  it('recolors existing timeline entries using the current thresholds without changing history', async () => {
    const wrapper = mount(MonitorTimeline, {
      props: {
        length: 3,
        countdownSeconds: 0,
        buckets: [
          { status: 'failed', latency_ms: 500, ping_latency_ms: null, checked_at: new Date().toISOString() },
          { status: 'operational', latency_ms: 1500, ping_latency_ms: null, checked_at: new Date().toISOString() },
          { status: 'operational', latency_ms: 999, ping_latency_ms: null, checked_at: new Date().toISOString() },
        ],
      },
      global: { plugins: [createPinia()] },
    })
    await flushPromises()
    const bars = wrapper.findAll('[title]')
    expect(bars[0].classes()).toContain('bg-emerald-500')
    expect(bars[1].classes()).toContain('bg-amber-400')
    expect(bars[2].classes()).toContain('bg-red-500')
  })

  it('uses inclusive yellow, orange and red boundaries in manual detection results', async () => {
    const wrapper = mount(MonitorRunResultDialog, {
      props: { show: true, results: [999, 1000, 3000, 6000].map((ms) => ({ model: `model-${ms}`, status: 'operational', latency_ms: ms, ping_latency_ms: null, checked_at: new Date().toISOString(), message: '' })) },
      global: {
        plugins: [createPinia()],
        stubs: { BaseDialog: { template: '<div><slot /></div>' }, MonitorQuotaView: true },
      },
    })
    await flushPromises()
    expect(wrapper.findAll('.rounded-full').map(badge => badge.classes().find(value => /^bg-/.test(value)))).toEqual(['bg-emerald-100', 'bg-amber-100', 'bg-orange-100', 'bg-red-100'])
  })
})
