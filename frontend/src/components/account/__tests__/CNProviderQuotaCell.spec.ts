import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import CNProviderQuotaCell from '../CNProviderQuotaCell.vue'
import type { Account } from '@/types'

const { queryQuota } = vi.hoisted(() => ({
  queryQuota: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    cnProviders: { queryQuota }
  }
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

const account = {
  id: 7,
  platform: 'zhipu',
  type: 'apikey',
  credentials: { account_mode: 'coding' },
  extra: {
    zhipu_5h_used_percent: 0,
    zhipu_weekly_used_percent: 27,
    zhipu_5h_reset_at: '2026-08-18T12:30:00+08:00',
    zhipu_weekly_reset_at: '2026-08-22T00:00:00+08:00',
    zhipu_usage_updated_at: new Date().toISOString()
  }
} as Account

describe('CNProviderQuotaCell', () => {
  beforeEach(() => {
    queryQuota.mockReset()
  })

  it('keeps the compact quota stack readable inside the account table cell', async () => {
    queryQuota.mockResolvedValue({
      success: true,
      tiers: [
        { window: '5h', used_percent: 0, reset_at: '2026-08-18T12:30:00+08:00' },
        { window: 'weekly', used_percent: 27, reset_at: '2026-08-22T00:00:00+08:00' }
      ]
    })
    const wrapper = mount(CNProviderQuotaCell, { props: { account } })

    const root = wrapper.get('[data-test="cn-provider-quota"]')
    expect(root.classes()).toContain('min-w-[220px]')

    const probeButton = root.get('button')
    expect(probeButton.classes()).toContain('whitespace-nowrap')
    expect(probeButton.classes()).toContain('leading-4')
    await probeButton.trigger('click')
    await flushPromises()

    const tiers = root.findAll('[data-test="cn-provider-quota-tier"]')
    expect(tiers).toHaveLength(2)
    for (const tier of tiers) {
      expect(tier.classes()).toContain('min-w-0')
      expect(tier.classes()).toContain('leading-4')
    }

    const labels = root.findAll('[data-test="cn-provider-quota-label"]')
    expect(labels).toHaveLength(2)
    for (const label of labels) {
      expect(label.classes()).toContain('w-14')
      expect(label.classes()).toContain('whitespace-nowrap')
    }

    expect(queryQuota).toHaveBeenCalledWith(account.id)
  })
})
