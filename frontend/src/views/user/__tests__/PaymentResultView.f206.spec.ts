import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const f206RouteState = vi.hoisted(() => ({
  query: {} as Record<string, unknown>,
}))
const f206Mocks = vi.hoisted(() => ({
  pollOrderStatus: vi.fn(),
  verifyOrder: vi.fn(),
  verifyOrderPublic: vi.fn(),
  resolveOrderPublicByResumeToken: vi.fn(),
  refreshUser: vi.fn(),
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => f206RouteState,
    useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

vi.mock('@/stores/payment', () => ({
  usePaymentStore: () => ({ pollOrderStatus: f206Mocks.pollOrderStatus }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ refreshUser: f206Mocks.refreshUser }),
}))

vi.mock('@/api/payment', () => ({
  paymentAPI: {
    verifyOrder: f206Mocks.verifyOrder,
    verifyOrderPublic: f206Mocks.verifyOrderPublic,
    resolveOrderPublicByResumeToken: f206Mocks.resolveOrderPublicByResumeToken,
  },
}))

import PaymentResultView from '../PaymentResultView.vue'

describe('PaymentResultView post-payment refresh (F2-06)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    window.localStorage.clear()
    f206RouteState.query = { resume_token: 'resume-f206' }
    f206Mocks.refreshUser.mockResolvedValue({})
    f206Mocks.resolveOrderPublicByResumeToken.mockResolvedValue({
      data: {
        id: 42,
        user_id: 9,
        amount: 88,
        pay_amount: 88,
        fee_rate: 0,
        payment_type: 'alipay',
        out_trade_no: 'sub2_f206',
        status: 'COMPLETED',
        order_type: 'balance',
        created_at: '2026-04-20T12:00:00Z',
        expires_at: '2026-04-20T12:30:00Z',
        refund_amount: 0,
      },
    })
  })

  it('forces a fresh /auth/me once the top-up is completed', async () => {
    mount(PaymentResultView, { global: { stubs: { OrderStatusBadge: true } } })
    await flushPromises()

    expect(f206Mocks.resolveOrderPublicByResumeToken).toHaveBeenCalledWith('resume-f206')
    expect(f206Mocks.refreshUser).toHaveBeenCalledWith({ force: true })
  })
})
