import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'
import PaymentView from '../PaymentView.vue'
import { PAYMENT_RECOVERY_STORAGE_KEY } from '@/components/payment/paymentFlow'
import type { CheckoutInfoResponse } from '@/types/payment'

const f202RouteState = vi.hoisted(() => ({
  path: '/purchase',
  query: {} as Record<string, unknown>,
}))
const f202CreateOrder = vi.hoisted(() => vi.fn())
const f202GetCheckoutInfo = vi.hoisted(() => vi.fn())
const f202AuthState = vi.hoisted(() => ({
  user: { id: 2, username: 'user-b', balance: 0 } as Record<string, unknown>,
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => f202RouteState,
    useRouter: () => ({
      replace: vi.fn().mockResolvedValue(undefined),
      push: vi.fn().mockResolvedValue(undefined),
      resolve: vi.fn(() => ({ href: '/' })),
    }),
  }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    get user() {
      return f202AuthState.user
    },
    refreshUser: vi.fn().mockResolvedValue(undefined),
  }),
}))

vi.mock('@/stores/payment', () => ({
  usePaymentStore: () => ({ createOrder: f202CreateOrder }),
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
    getCheckoutInfo: f202GetCheckoutInfo,
    getExchangeRate: vi.fn(),
  },
}))

vi.mock('@/utils/device', () => ({
  isMobileDevice: () => false,
}))

function f202Checkout(): { data: CheckoutInfoResponse } {
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
      plans: [{
        id: 7,
        group_id: 3,
        name: 'Starter',
        description: '',
        price: 10,
        original_price: 0,
        validity_days: 30,
        validity_unit: 'day',
        rate_multiplier: 1,
        daily_limit_usd: null,
        weekly_limit_usd: null,
        monthly_limit_usd: null,
        features: [],
        group_platform: 'openai',
        sort_order: 1,
        for_sale: true,
        group_name: 'OpenAI',
      }],
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

function f202Snapshot(overrides: Record<string, unknown> = {}): string {
  return JSON.stringify({
    orderId: 123,
    amount: 10,
    qrCode: 'user-a-qr',
    expiresAt: '2099-01-01T00:10:00.000Z',
    paymentType: 'wxpay',
    payUrl: '',
    outTradeNo: 'sub2_123',
    currency: '',
    paymentEnv: '',
    payAmount: 10,
    orderType: 'balance',
    paymentMode: 'qrcode',
    resumeToken: '',
    createdAt: Date.now(),
    ...overrides,
  })
}

async function f202Mount() {
  const wrapper = shallowMount(PaymentView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        Teleport: true,
        Transition: false,
      },
    },
  })
  await flushPromises()
  await flushPromises()
  return wrapper
}

describe('PaymentView recovery snapshot ownership (F2-02)', () => {
  beforeEach(() => {
    vi.useRealTimers()
    f202RouteState.path = '/purchase'
    f202RouteState.query = {}
    f202AuthState.user = { id: 2, username: 'user-b', balance: 0 }
    f202CreateOrder.mockReset()
    f202GetCheckoutInfo.mockReset().mockResolvedValue(f202Checkout())
    window.localStorage.clear()
  })

  it("does not restore another user's pending order and drops the snapshot", async () => {
    window.localStorage.setItem(PAYMENT_RECOVERY_STORAGE_KEY, f202Snapshot({ userId: 1 }))

    const wrapper = await f202Mount()

    expect(wrapper.findComponent({ name: 'PaymentStatusPanel' }).exists()).toBe(false)
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toBeNull()
  })

  it('restores the pending order for the user who created it', async () => {
    window.localStorage.setItem(PAYMENT_RECOVERY_STORAGE_KEY, f202Snapshot({ userId: 2 }))

    const wrapper = await f202Mount()

    const panel = wrapper.findComponent({ name: 'PaymentStatusPanel' })
    expect(panel.exists()).toBe(true)
    expect(panel.props('orderId')).toBe(123)
  })

  it('records the current user id in the snapshot it saves', async () => {
    f202RouteState.query = { tab: 'subscription', group: '3' }
    f202CreateOrder.mockResolvedValue({
      order_id: 456,
      amount: 10,
      pay_amount: 10,
      fee_rate: 0,
      expires_at: '2099-01-01T00:10:00.000Z',
      qr_code: 'user-b-qr',
      payment_mode: 'qrcode',
    })
    const wrapper = await f202Mount()

    const submit = wrapper.findAll('button').find(button => button.text().includes('payment.createOrder'))
    expect(submit).toBeDefined()
    await submit!.trigger('click')
    await flushPromises()

    expect(f202CreateOrder).toHaveBeenCalledTimes(1)
    const saved = JSON.parse(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY) || '{}')
    expect(saved.orderId).toBe(456)
    expect(saved.userId).toBe(2)
  })
})
