import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import MonitorSettingsPanel from '../MonitorSettingsPanel.vue'
import { getConfig, updateConfig } from '@/api/channelMonitorV2'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, te: () => false }) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ cachedPublicSettings: {}, showSuccess: vi.fn(), showError: vi.fn() }) }))
vi.mock('@/utils/featureFlags', () => ({ getChannelMonitorMode: () => 'v2', isChannelMonitorV2Mode: () => true }))
vi.mock('@/api/admin', () => ({ adminAPI: { groups: { getAllIncludingInactive: vi.fn().mockResolvedValue([]) } } }))
vi.mock('@/api/channelMonitorV2', () => ({ getConfig: vi.fn(), updateConfig: vi.fn(), MONITOR_ERROR_CATEGORIES: [] }))

const config = {
  version: 1, enabled: true, refresh_interval_seconds: 60 as const,
  platforms: [], group_ids: [],
  health_thresholds: {
    minimum_sample: 30, minimum_sample_by_range: { '90m': 10, '24h': 30, '7d': 50, '30d': 100 },
    warning_error_rate: 0.05, critical_error_rate: 0.2,
    target_ttft_ms: 3000, warning_ttft_ms: 8000, critical_ttft_ms: 20000,
    warning_cache_rate: 0.85, critical_cache_rate: 0.6,
    error_weight: 0.6, ttft_weight: 0.2, cache_weight: 0.2,
  },
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(getConfig).mockResolvedValue(structuredClone(config))
  vi.mocked(updateConfig).mockImplementation(async value => structuredClone(value))
})

describe('per-view sample minimum settings', () => {
  it('saves independent values and removes a cleared override without changing the common minimum', async () => {
    const wrapper = mount(MonitorSettingsPanel, { global: { stubs: { Icon: true, 'router-link': true } } })
    await flushPromises()
    const input90 = wrapper.get<HTMLInputElement>('[data-sample-range="90m"]')
    expect(input90.element.value).toBe('10')
    expect(wrapper.get<HTMLInputElement>('[data-sample-range="30d"]').element.value).toBe('100')
    await input90.setValue('12')
    await wrapper.get('[data-sample-range="7d"]').setValue('')
    await wrapper.get('header button').trigger('click')
    await flushPromises()
    expect(updateConfig).toHaveBeenCalledTimes(1)
    const saved = vi.mocked(updateConfig).mock.calls[0][0]
    expect(saved.health_thresholds.minimum_sample).toBe(30)
    expect(saved.health_thresholds.minimum_sample_by_range).toEqual({ '90m': 12, '24h': 30, '30d': 100 })
    wrapper.unmount()
  })

  it('keeps legacy configs on their common minimum until an override is entered', async () => {
    const legacy = structuredClone(config)
    delete (legacy.health_thresholds as { minimum_sample_by_range?: unknown }).minimum_sample_by_range
    vi.mocked(getConfig).mockResolvedValue(legacy)
    const wrapper = mount(MonitorSettingsPanel, { global: { stubs: { Icon: true, 'router-link': true } } })
    await flushPromises()
    const input = wrapper.get<HTMLInputElement>('[data-sample-range="90m"]')
    expect(input.element.value).toBe('')
    expect(input.attributes('placeholder')).toBe('30')
    await input.setValue('10')
    await wrapper.get('header button').trigger('click')
    await flushPromises()
    expect(vi.mocked(updateConfig).mock.calls[0][0].health_thresholds.minimum_sample_by_range).toEqual({ '90m': 10 })
    wrapper.unmount()
  })
})
