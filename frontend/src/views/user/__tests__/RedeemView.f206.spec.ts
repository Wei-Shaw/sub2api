import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'
import RedeemView from '../RedeemView.vue'

const f206Mocks = vi.hoisted(() => ({
  redeem: vi.fn(),
  getHistory: vi.fn(),
  refreshUser: vi.fn(),
}))

vi.mock('@/api', () => ({
  redeemAPI: { redeem: f206Mocks.redeem, getHistory: f206Mocks.getHistory },
}))
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ user: { balance: 10, concurrency: 2 }, refreshUser: f206Mocks.refreshUser }),
}))
vi.mock('@/stores/subscriptions', () => ({
  useSubscriptionStore: () => ({ fetchActiveSubscriptions: vi.fn().mockResolvedValue([]) }),
}))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showWarning: vi.fn(),
    showSuccess: vi.fn(),
    fetchPublicSettings: vi.fn().mockResolvedValue({}),
  }),
}))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

describe('RedeemView post-redeem refresh (F2-06)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    f206Mocks.redeem.mockResolvedValue({ type: 'balance', value: 20, message: 'ok' })
    f206Mocks.getHistory.mockResolvedValue({ items: [], total: 0 })
    f206Mocks.refreshUser.mockResolvedValue({ balance: 30, concurrency: 2 })
  })

  it('forces a fresh /auth/me instead of joining an in-flight one', async () => {
    const wrapper = mount(RedeemView, {
      global: {
        stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true, RouterLink: RouterLinkStub },
      },
    })
    await flushPromises()
    await wrapper.get('input#code').setValue('REDEEM-CODE')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(f206Mocks.redeem).toHaveBeenCalledWith('REDEEM-CODE')
    expect(f206Mocks.refreshUser).toHaveBeenCalledWith({ force: true })
    wrapper.unmount()
  })
})
