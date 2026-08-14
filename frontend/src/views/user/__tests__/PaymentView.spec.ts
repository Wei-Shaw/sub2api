import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'
import PaymentView from '../PaymentView.vue'
import { PAYMENT_RECOVERY_STORAGE_KEY } from '@/components/payment/paymentFlow'
import { formatPaymentAmount } from '@/components/payment/currency'
import SubscriptionPlanCard from '@/components/payment/SubscriptionPlanCard.vue'
import type { CheckoutInfoResponse, MethodLimit, SubscriptionPlan } from '@/types/payment'

const routeState = vi.hoisted(() => ({
  path: '/purchase',
  query: {} as Record<string, unknown>,
}))

const routerReplace = vi.hoisted(() => vi.fn())
const routerPush = vi.hoisted(() => vi.fn())
const routerResolve = vi.hoisted(() => vi.fn(() => ({ href: '/payment/stripe?mock=1' })))
const createOrder = vi.hoisted(() => vi.fn())
const refreshUser = vi.hoisted(() => vi.fn())
const fetchActiveSubscriptions = vi.hoisted(() => vi.fn().mockResolvedValue(undefined))
const showError = vi.hoisted(() => vi.fn())
const showInfo = vi.hoisted(() => vi.fn())
const showWarning = vi.hoisted(() => vi.fn())
const getCheckoutInfo = vi.hoisted(() => vi.fn())
const bridgeInvoke = vi.hoisted(() => vi.fn())

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => routeState,
    useRouter: () => ({
      replace: routerReplace,
      push: routerPush,
      resolve: routerResolve,
    }),
  }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      // `NumCell` reads `locale` off `useI18n()`. Left undefined on purpose:
      // `Intl` then falls back to the system default, which is exactly what the
      // view's own `localeCode` computed resolves to for an empty locale — so the
      // amount expectations below can keep comparing against
      // `formatPaymentAmount(...)` called with no locale at all.
      locale: { value: undefined as unknown as string },
    }),
  }
})

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    user: {
      username: 'demo-user',
      balance: 0,
    },
    refreshUser,
  }),
}))

vi.mock('@/stores/payment', () => ({
  usePaymentStore: () => ({
    createOrder,
  }),
}))

vi.mock('@/stores/subscriptions', () => ({
  useSubscriptionStore: () => ({
    activeSubscriptions: [],
    fetchActiveSubscriptions,
  }),
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
    showInfo,
    showWarning,
  }),
}))

vi.mock('@/api/payment', () => ({
  paymentAPI: {
    getCheckoutInfo,
  },
}))

vi.mock('@/utils/device', () => ({
  isMobileDevice: () => true,
}))

function checkoutInfoFixture(overrides: Partial<CheckoutInfoResponse> = {}) {
  const sepayMethod: MethodLimit = {
    daily_limit: 0,
    daily_used: 0,
    daily_remaining: 0,
    single_min: 0,
    single_max: 0,
    fee_rate: 0,
    available: true,
  }
  const data: CheckoutInfoResponse = {
    methods: {
      sepay: sepayMethod,
    },
    global_min: 0,
    global_max: 0,
    plans: [],
    balance_disabled: false,
    balance_recharge_multiplier: 1,
    subscription_usd_to_vnd_rate: 0,
    recharge_fee_rate: 0,
    help_text: '',
    help_image_url: '',
  }

  return {
    data: { ...data, ...overrides },
  }
}

function checkoutInfoWithPlansFixture(options: {
  checkout?: Partial<CheckoutInfoResponse>
  method?: Partial<MethodLimit>
  plan?: Partial<SubscriptionPlan>
} = {}) {
  const base = checkoutInfoFixture(options.checkout).data
  const plan: SubscriptionPlan = {
    id: 7,
    group_id: 3,
    name: 'Starter',
    description: '',
    price: 128,
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
    ...options.plan,
  }

  return {
    data: {
      ...base,
      methods: {
        ...base.methods,
        sepay: {
          ...base.methods.sepay,
          ...options.method,
        },
      },
      plans: [plan],
    },
  }
}



async function mountSubscriptionConfirm(options: Parameters<typeof checkoutInfoWithPlansFixture>[0] = {}) {
  vi.useRealTimers()
  routeState.path = '/purchase'
  routeState.query = {
    tab: 'subscription',
    group: '3',
  }
  routerReplace.mockReset().mockResolvedValue(undefined)
  routerPush.mockReset().mockResolvedValue(undefined)
  routerResolve.mockClear()
  createOrder.mockReset()
  refreshUser.mockReset()
  fetchActiveSubscriptions.mockReset().mockResolvedValue(undefined)
  showError.mockReset()
  showInfo.mockReset()
  showWarning.mockReset()
  getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoWithPlansFixture(options))
  bridgeInvoke.mockReset()
  window.localStorage.clear()
  ;(window as Window & { WeixinJSBridge?: { invoke: typeof bridgeInvoke } }).WeixinJSBridge = undefined

  const wrapper = shallowMount(PaymentView, {
    global: {
      stubs: {
        AppLayout: {
          template: '<div><slot /></div>',
        },
        // Money is rendered as a currency-symbol `<span>` plus a `NumCell`, so
        // `NumCell` has to render for the amounts to reach `wrapper.text()` at
        // all. The rest of the tree stays shallow.
        NumCell: false,
        /*
         * `Button` is stubbed to a real `<button>` that renders its slot rather
         * than being un-stubbed with `Button: false`. The primitive's root is
         * `<component :is="tag">`, and VTU's shallow mode intercepts dynamic
         * components — it stubs the resolved `'button'` tag as a component named
         * `Button`, which then has no render function at all. A one-line stub
         * keeps `findAll('button')` and `.text()` working without asking
         * `shallowMount` to special-case a dynamic root.
         */
        Button: {
          template: '<button><slot /></button>',
        },
        Teleport: true,
        Transition: false,
      },
    },
  })
  await flushPromises()
  await flushPromises()
  return wrapper
}

async function mountSubscriptionPlanList(planCount: number) {
  vi.useRealTimers()
  routeState.path = '/purchase'
  routeState.query = { tab: 'subscription' }
  routerReplace.mockReset().mockResolvedValue(undefined)
  routerPush.mockReset().mockResolvedValue(undefined)
  routerResolve.mockClear()
  createOrder.mockReset()
  refreshUser.mockReset()
  fetchActiveSubscriptions.mockReset().mockResolvedValue(undefined)
  showError.mockReset()
  showInfo.mockReset()
  showWarning.mockReset()
  const basePlan = checkoutInfoWithPlansFixture().data.plans[0]
  const plans = Array.from({ length: planCount }, (_, index) => ({
    ...basePlan,
    id: index + 1,
    name: `Plan ${index + 1}`,
  }))
  getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture({ plans }))
  bridgeInvoke.mockReset()
  window.localStorage.clear()
  ;(window as Window & { WeixinJSBridge?: { invoke: typeof bridgeInvoke } }).WeixinJSBridge = undefined

  const wrapper = shallowMount(PaymentView, {
    global: {
      stubs: {
        AppLayout: {
          template: '<div><slot /></div>',
        },
        Teleport: true,
        Transition: false,
      },
    },
  })
  await flushPromises()
  await flushPromises()
  return wrapper
}

describe('PaymentView subscription plan grid', () => {
  it.each([3, 4, 6])('keeps %i plans on the existing mobile/tablet/desktop grid', async (planCount) => {
    const wrapper = await mountSubscriptionPlanList(planCount)
    const cards = wrapper.findAllComponents(SubscriptionPlanCard)

    expect(cards).toHaveLength(planCount)
    expect([...(cards[0].element.parentElement?.classList ?? [])]).toEqual(expect.arrayContaining([
      'grid',
      'grid-cols-1',
      'sm:grid-cols-2',
      'lg:grid-cols-3',
    ]))
  })
})

describe('PaymentView subscription confirmation amounts', () => {
  it('shows converted VND pay amount using the subscription rate, not the balance multiplier', async () => {
    const wrapper = await mountSubscriptionConfirm({
      checkout: {
        balance_recharge_multiplier: 0.14,
        subscription_usd_to_vnd_rate: 26000,
      },
      method: {
        currency: 'VND',
      },
      plan: {
        price: 9.99,
        original_price: 12.99,
      },
    })

    const text = wrapper.text()
    const convertedPrice = formatPaymentAmount(259740, 'VND')
    const convertedOriginalPrice = formatPaymentAmount(337740, 'VND')

    expect(text).toContain(convertedPrice)
    expect(text).toContain(convertedOriginalPrice)
    expect(text).not.toContain(formatPaymentAmount(9.99, 'VND'))
    // The conversion must use the subscription rate, not the balance multiplier.
    expect(text).not.toContain(formatPaymentAmount(71, 'VND'))
    expect(wrapper.findAll('button').some(button => button.text().includes(convertedPrice))).toBe(true)
  })

  it('keeps plan price when the subscription rate is not configured or payment currency is not VND', async () => {
    // Opt-in lock: with no subscription rate configured, a dong channel still
    // charges the plan price as-is even though a balance multiplier exists.
    const vndWrapper = await mountSubscriptionConfirm({
      checkout: {
        balance_recharge_multiplier: 0.14,
        subscription_usd_to_vnd_rate: 0,
      },
      method: {
        currency: 'VND',
      },
      plan: {
        price: 7.99,
      },
    })

    expect(vndWrapper.text()).toContain(formatPaymentAmount(7.99, 'VND'))
    expect(vndWrapper.text()).not.toContain(formatPaymentAmount(207740, 'VND'))

    const usdWrapper = await mountSubscriptionConfirm({
      checkout: {
        subscription_usd_to_vnd_rate: 26000,
      },
      method: {
        currency: 'USD',
      },
      plan: {
        price: 7.99,
        original_price: 9.99,
      },
    })

    expect(usdWrapper.text()).toContain(formatPaymentAmount(7.99, 'USD'))
    expect(usdWrapper.text()).toContain(formatPaymentAmount(9.99, 'USD'))
  })

  it('adds fee rate after VND rate conversion to match backend pay_amount', async () => {
    const wrapper = await mountSubscriptionConfirm({
      checkout: {
        subscription_usd_to_vnd_rate: 26000,
        recharge_fee_rate: 2.5,
      },
      method: {
        currency: 'VND',
      },
      plan: {
        price: 9.99,
      },
    })

    const text = wrapper.text()
    const convertedPrice = formatPaymentAmount(259740, 'VND')
    const fee = formatPaymentAmount(6494, 'VND')
    const total = formatPaymentAmount(266234, 'VND')

    expect(text).toContain(convertedPrice)
    expect(text).toContain(fee)
    expect(text).toContain(total)
    expect(wrapper.findAll('button').some(button => button.text().includes(total))).toBe(true)
  })
})

async function mountRecharge(amount: number, options: {
  checkout?: Partial<CheckoutInfoResponse>
  method?: Partial<MethodLimit>
} = {}) {
  vi.useRealTimers()
  routeState.path = '/purchase'
  routeState.query = {}
  routerReplace.mockReset().mockResolvedValue(undefined)
  routerPush.mockReset().mockResolvedValue(undefined)
  routerResolve.mockClear()
  createOrder.mockReset()
  refreshUser.mockReset()
  fetchActiveSubscriptions.mockReset().mockResolvedValue(undefined)
  showError.mockReset()
  showInfo.mockReset()
  showWarning.mockReset()
  const base = checkoutInfoFixture(options.checkout).data
  getCheckoutInfo.mockReset().mockResolvedValue({
    data: {
      ...base,
      methods: {
        ...base.methods,
        sepay: { ...base.methods.sepay, ...options.method },
      },
    },
  })
  bridgeInvoke.mockReset()
  window.localStorage.clear()
  ;(window as Window & { WeixinJSBridge?: { invoke: typeof bridgeInvoke } }).WeixinJSBridge = undefined

  const wrapper = shallowMount(PaymentView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        NumCell: false,
        Button: { template: '<button><slot /></button>' },
        Teleport: true,
        Transition: false,
      },
    },
  })
  await flushPromises()
  await flushPromises()
  // `AmountInput` is stubbed, so the typed amount arrives as its update event.
  wrapper.findComponent({ name: 'AmountInput' }).vm.$emit('update:modelValue', amount)
  await flushPromises()
  return wrapper
}

describe('PaymentView recharge amounts', () => {
  it('converts the recharge amount for a dong channel instead of charging the USD figure as dong', async () => {
    const wrapper = await mountRecharge(10, {
      checkout: { subscription_usd_to_vnd_rate: 26000 },
      method: { currency: 'VND' },
    })

    const text = wrapper.text()
    const converted = formatPaymentAmount(260000, 'VND')

    expect(text).toContain(converted)
    expect(text).not.toContain(formatPaymentAmount(10, 'VND'))
    expect(wrapper.findAll('button').some(button => button.text().includes(converted))).toBe(true)
  })

  it('adds the fee after conversion, matching the backend pay amount', async () => {
    const wrapper = await mountRecharge(10, {
      checkout: { subscription_usd_to_vnd_rate: 26000, recharge_fee_rate: 2.5 },
      method: { currency: 'VND' },
    })

    const text = wrapper.text()
    // 2.5% of ₫260,000 is ₫6,500, charged on top of the converted amount.
    expect(text).toContain(formatPaymentAmount(6500, 'VND'))
    expect(text).toContain(formatPaymentAmount(266500, 'VND'))
  })

  it('charges the amount as-is when no rate is configured', async () => {
    const wrapper = await mountRecharge(10, {
      checkout: { subscription_usd_to_vnd_rate: 0 },
      method: { currency: 'VND' },
    })

    expect(wrapper.text()).toContain(formatPaymentAmount(10, 'VND'))
  })

  // Once the summary converts, every figure in it is dong. The credited row is
  // the only line naming the USD credit, so a 1× multiplier must not hide it.
  it('states the USD credit even on a 1x multiplier when the gateway settles in dong', async () => {
    const wrapper = await mountRecharge(10, {
      checkout: { subscription_usd_to_vnd_rate: 26000, balance_recharge_multiplier: 1 },
      method: { currency: 'VND' },
    })

    expect(wrapper.text()).toContain(formatPaymentAmount(10, 'USD'))
  })

  it('leaves the credited row out on a dollar gateway with a 1x multiplier', async () => {
    const wrapper = await mountRecharge(10, {
      checkout: { balance_recharge_multiplier: 1 },
      method: { currency: 'USD' },
    })

    expect(wrapper.text()).not.toContain('payment.creditedBalance')
  })
})

describe('PaymentView payment recovery', () => {
  beforeEach(() => {
    vi.useRealTimers()
    routeState.path = '/purchase'
    routeState.query = {}
    routerReplace.mockReset().mockResolvedValue(undefined)
    routerPush.mockReset().mockResolvedValue(undefined)
    routerResolve.mockClear()
    createOrder.mockReset()
    refreshUser.mockReset()
    fetchActiveSubscriptions.mockReset().mockResolvedValue(undefined)
    showError.mockReset()
    showInfo.mockReset()
    showWarning.mockReset()
    bridgeInvoke.mockReset()
    window.localStorage.clear()
    ;(window as Window & { WeixinJSBridge?: { invoke: typeof bridgeInvoke } }).WeixinJSBridge = undefined
  })

  it('restores the stored payment method when a pending snapshot is found', async () => {
    getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture())
    window.localStorage.setItem(PAYMENT_RECOVERY_STORAGE_KEY, JSON.stringify({
      orderId: 42,
      amount: 100,
      qrCode: '00020101021238...6304ABCD',
      expiresAt: '2099-01-01T00:10:00.000Z',
      paymentType: 'sepay',
      payUrl: '',
      outTradeNo: 'SUB220260101ABCD1234',
      intentId: '',
      currency: 'VND',
      payAmount: 100,
      orderType: 'balance',
      paymentMode: 'qrcode',
      resumeToken: '',
      createdAt: 1,
    }))

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
        },
      },
    })
    await flushPromises()

    expect(wrapper.findComponent({ name: 'PaymentStatusPanel' }).props('paymentType')).toBe('sepay')
  })
})
