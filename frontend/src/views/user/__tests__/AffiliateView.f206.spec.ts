import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AffiliateView from '../AffiliateView.vue'

const f206Mocks = vi.hoisted(() => ({
  getAffiliateDetail: vi.fn(),
  transferAffiliateQuota: vi.fn(),
  refreshUser: vi.fn(),
}))

vi.mock('@/api/user', () => ({
  default: {
    getAffiliateDetail: f206Mocks.getAffiliateDetail,
    transferAffiliateQuota: f206Mocks.transferAffiliateQuota,
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ refreshUser: f206Mocks.refreshUser }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

describe('AffiliateView post-transfer refresh (F2-06)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    f206Mocks.getAffiliateDetail.mockResolvedValue({
      user_id: 1,
      aff_code: 'code-1',
      inviter_id: null,
      aff_count: 0,
      aff_quota: 5,
      aff_frozen_quota: 0,
      aff_history_quota: 5,
      effective_rebate_rate_percent: 10,
      invitees: [],
    })
    f206Mocks.transferAffiliateQuota.mockResolvedValue({ transferred_quota: 5 })
    f206Mocks.refreshUser.mockResolvedValue({})
  })

  it('forces a fresh /auth/me after moving the quota into the balance', async () => {
    const wrapper = mount(AffiliateView, {
      global: { stubs: { AppLayout: { template: '<main><slot /></main>' }, Icon: true } },
    })
    await flushPromises()

    const button = wrapper.findAll('button').find((b) => b.text() === 'affiliate.transfer.button')
    expect(button).toBeDefined()
    await button!.trigger('click')
    await flushPromises()

    expect(f206Mocks.transferAffiliateQuota).toHaveBeenCalledTimes(1)
    expect(f206Mocks.refreshUser).toHaveBeenCalledWith({ force: true })
  })
})
