import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import GrokQuotaProbeCell from '../GrokQuotaProbeCell.vue'
import type { Account } from '@/types'

const { queryQuota, getUsage } = vi.hoisted(() => ({
  queryQuota: vi.fn(),
  getUsage: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    grok: { queryQuota }
  }
}))

vi.mock('@/api/userAccounts', () => ({
  userAccountsAPI: { getUsage }
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
    getUsage.mockReset()
  })

  it('keeps billing data while exposing a failed Free quota fallback', async () => {
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

    expect(wrapper.text()).toContain('upstream returned 402 for probe model "grok-4.5"')
    expect(wrapper.emitted('probed')?.[0]?.[0]).toMatchObject({
      billing: { period_type: 'weekly', usage_percent: null },
      probe_error: 'upstream returned 402 for probe model "grok-4.5"'
    })
  })

  it('uses userAccountsAPI when usageApi is user', async () => {
    getUsage.mockResolvedValue({
      grok_billing: { period_type: 'weekly', usage_percent: 45 },
      grok_request_quota: { remaining: 10, limit: 20 },
      grok_token_quota: { remaining: 500, limit: 1000 }
    })
    const wrapper = mount(GrokQuotaProbeCell, { props: { account, usageApi: 'user' } })

    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(getUsage).toHaveBeenCalledWith(99, 'active', true)
    expect(queryQuota).not.toHaveBeenCalled()
    expect(wrapper.emitted('probed')?.[0]?.[0]).toMatchObject({
      source: 'billing_probe',
      billing: { period_type: 'weekly', usage_percent: 45 }
    })
  })
})
