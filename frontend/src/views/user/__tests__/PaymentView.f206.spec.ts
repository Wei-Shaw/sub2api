import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'
import PaymentView from '../PaymentView.vue'
import { PAYMENT_RECOVERY_STORAGE_KEY } from '@/components/payment/paymentFlow'
import type { CheckoutInfoResponse } from '@/types/payment'

const f206RouteState = vi.hoisted(() => ({
  path: '/purchase',
  query: {} as Record<string, unknown>,
}))
const f206Mocks = vi.hoisted(() => ({
  refreshUser: vi.fn(),
  getCheckoutInfo: vi.fn(),
  routerPush: vi.fn(),
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => f206RouteState,
    useRouter: () => ({
      replace: vi.fn().mockResolvedValue(undefined),
      push: f206Mocks.routerPush,
      resolve: vi.fn(() => ({ href: '/' })),
    }),
  }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    user: { id: 5, username: 'buyer', balance: 0 },
    refreshUser: f206Mocks.refreshUser,
  }),
}))

vi.mock('@/stores/payment', () => ({
  usePaymentStore: () => ({ createOrder: vi.fn() }),
}))

vi.mock('@/stores/subscriptions', () => ({
  useSubscriptionStore: () => ({
    activeSubscriptions: [],
    fetchActiveSubscriptions: vi.fn().mockResolvedValue(undefined),
  }),
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showInfo: vi.fn(),
    showWarning: vi.fn(),
    cachedPublicSettings: undefined,
  }),
}))

vi.mock('@/api/payment', () => ({
  paymentAPI: {
    getCheckoutInfo: f206Mocks.getCheckoutInfo,
    getExchangeRate: vi.fn(),
  },
}))

vi.mock('@/utils/device', () => ({
  isMobileDevice: () => false,
}))

function f206Checkout(): { data: CheckoutInfoResponse } {
  return {
    data: {
      methods: {
        wxpay: {
          daily_limit: 0,
          daily_used: 0,
          daily_remaining: 0,
          single_min: 0,
          single_max: 0,
          fee_rate: 0,
          available: true,
        },
      },
      global_min: 0,
      global_max: 0,
      plans: [],
      balance_disabled: false,
      balance_recharge_multiplier: 1,
      subscription_usd_to_cny_rate: 0,
      recharge_fee_rate: 0,
      help_text: '',
      help_image_url: '',
      stripe_publishable_key: '',
    },
  }
}

describe('PaymentView post-payment refresh (F2-06)', () => {
  beforeEach(() => {
    vi.useRealTimers()
    vi.clearAllMocks()
    f206RouteState.query = {}
    f206Mocks.refreshUser.mockResolvedValue({})
    f206Mocks.routerPush.mockResolvedValue(undefined)
    f206Mocks.getCheckoutInfo.mockResolvedValue(f206Checkout())
    window.localStorage.clear()
    window.localStorage.setItem(PAYMENT_RECOVERY_STORAGE_KEY, JSON.stringify({
      orderId: 321,
      amount: 10,
      qrCode: 'qr-321',
      expiresAt: '2099-01-01T00:10:00.000Z',
      paymentType: 'wxpay',
      payUrl: '',
      outTradeNo: 'sub2_321',
      currency: '',
      paymentEnv: '',
      payAmount: 10,
      orderType: 'balance',
      paymentMode: 'qrcode',
      resumeToken: '',
      createdAt: Date.now(),
      userId: 5,
    }))
  })

  it('forces a fresh /auth/me when the panel reports a paid order', async () => {
    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          PaymentStatusPanel: {
            template: '<button data-test="f206-paid" @click="$emit(\'success\')" />',
          },
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-test="f206-paid"]').trigger('click')
    await flushPromises()

    expect(f206Mocks.refreshUser).toHaveBeenCalledWith({ force: true })
  })
})
