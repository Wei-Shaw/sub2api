import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import GrokQuotaProbeCell from '../GrokQuotaProbeCell.vue'
import type { Account } from '@/types'

const { queryQuota } = vi.hoisted(() => ({
  queryQuota: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    grok: { queryQuota }
  }
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params?.percent == null ? key : `${key}:${params.percent}`
  })
}))

const account = {
  id: 99,
  platform: 'grok',
  type: 'oauth'
} as Account

describe('GrokQuotaProbeCell', () => {
  beforeEach(() => {
    queryQuota.mockReset()
  })

  it('does not publish a failed Free quota fallback as a fresh result', async () => {
    queryQuota.mockResolvedValue({
      source: 'hybrid_probe',
      billing: { period_type: 'weekly', usage_percent: null },
      headers_observed: false,
      reset_supported: false,
      fetched_at: 1,
      probe_error: 'upstream returned 402 for probe model "grok-4.5"'
    })
    const wrapper = mount(GrokQuotaProbeCell, { props: { account } })

    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(wrapper.get('.text-red-600').attributes('title')).toContain('upstream returned 402 for probe model "grok-4.5"')
    expect(wrapper.emitted('probed')).toBeUndefined()
    wrapper.unmount()
  })
  it('retains the previous successful summary and only emits once when a later probe fails', async () => {
    queryQuota.mockResolvedValueOnce({
      billing: { period_type: 'weekly', usage_percent: 24 },
      persisted: true, fetched_at: 1
    }).mockResolvedValueOnce({ probe_error: 'rate limited', fetched_at: 2 })
    const wrapper = mount(GrokQuotaProbeCell, { props: { account } })
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('24')
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('24')
    expect(wrapper.text()).toContain('showingLastData')
    expect(wrapper.emitted('probed')).toHaveLength(1)
    wrapper.unmount()
  })

})
