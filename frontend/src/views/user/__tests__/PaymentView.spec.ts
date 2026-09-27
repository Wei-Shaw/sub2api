import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'
import PaymentView from '../PaymentView.vue'
import { PAYMENT_RECOVERY_STORAGE_KEY } from '@/components/payment/paymentFlow'
import { formatPaymentAmount } from '@/components/payment/currency'
import AmountInput from '@/components/payment/AmountInput.vue'
import SubscriptionPlanCard from '@/components/payment/SubscriptionPlanCard.vue'
import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'
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
const getExchangeRate = vi.hoisted(() => vi.fn())
const bridgeInvoke = vi.hoisted(() => vi.fn())
const translate = vi.hoisted(() => vi.fn((key: string) => key))
// Public settings live in a reactive holder so tests can flip feature flags after mount
// and exercise the watchers that react to them.
const appStoreState = vi.hoisted(() => ({
  setPublicSettings: (_value: Record<string, unknown> | undefined) => {},
}))

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
      t: translate,
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

vi.mock('@/stores', async () => {
  const { reactive } = await import('vue')
  const state = reactive({ cachedPublicSettings: undefined as Record<string, unknown> | undefined })
  appStoreState.setPublicSettings = (value) => {
    state.cachedPublicSettings = value
  }
  return {
    useAppStore: () => ({
      showError,
      showInfo,
      showWarning,
      get cachedPublicSettings() {
        return state.cachedPublicSettings
      },
    }),
  }
})

vi.mock('@/api/payment', () => ({
  paymentAPI: {
    getCheckoutInfo,
    getExchangeRate,
  },
}))

vi.mock('@/utils/device', () => ({
  isMobileDevice: () => true,
}))

function checkoutInfoFixture(overrides: Partial<CheckoutInfoResponse> = {}) {
  const wxpayMethod: MethodLimit = {
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
      wxpay: wxpayMethod,
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
        wxpay: {
          ...base.methods.wxpay,
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

describe('PaymentView help text', () => {
  beforeEach(() => {
    vi.useRealTimers()
    routeState.path = '/purchase'
    routeState.query = {}
    createOrder.mockReset()
    window.localStorage.clear()
  })

  async function mountHelp(help_text: string, help_image_url = '') {
    getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture({ help_text, help_image_url }))
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
    return wrapper
  }

  it('renders headings, emphasis, links, and lists in payment help without starting checkout', async () => {
    const wrapper = await mountHelp('## Recharge help\n\n**Read first**\n\n- [Contact support](https://example.com/help)')
    const help = wrapper.get('.markdown-body')
    expect(help.get('h2').text()).toBe('Recharge help')
    expect(help.get('strong').text()).toBe('Read first')
    expect(help.get('li a').attributes('href')).toBe('https://example.com/help')
    expect(createOrder).not.toHaveBeenCalled()
  })

  it('removes scripts, event handlers, and unsafe URLs from rendered help', async () => {
    const wrapper = await mountHelp([
      '<script>alert(1)</script>',
      '<img src="https://example.com/help.png" onerror="alert(1)">',
      '[Unsafe](javascript:alert%281%29)',
      '[Support](https://example.com/help)',
    ].join('\n\n'))
    const help = wrapper.get('.markdown-body')
    expect(help.find('script').exists()).toBe(false)
    expect(help.get('img').attributes('onerror')).toBeUndefined()
    expect(help.findAll('a').map(link => link.attributes('href'))).toEqual([undefined, 'https://example.com/help'])
  })

  it('keeps plain-text soft line breaks and the separate help image preview', async () => {
    const wrapper = await mountHelp('First line\nSecond line', 'https://example.com/help.png')
    const help = wrapper.get('.markdown-body')
    expect(help.get('p').text()).toBe('First line\nSecond line')
    expect(help.find('br').exists()).toBe(false)
    await wrapper.get('img').trigger('click')
    expect(wrapper.findAll('img')).toHaveLength(2)
    expect(wrapper.findAll('img')[1].attributes('src')).toBe('https://example.com/help.png')
  })

  it('keeps image-only help without an empty Markdown container', async () => {
    const wrapper = await mountHelp('', 'https://example.com/help.png')
    expect(wrapper.find('.markdown-body').exists()).toBe(false)
    expect(wrapper.get('img').attributes('src')).toBe('https://example.com/help.png')
  })
})

describe('PaymentView checkout load failure', () => {
  it('shows the load error with a retry instead of claiming top-up is unavailable', async () => {
    vi.useRealTimers()
    routeState.path = '/purchase'
    routeState.query = {}
    showError.mockReset()
    window.localStorage.clear()
    getCheckoutInfo.mockReset()
      .mockRejectedValueOnce({ status: 500, message: 'Internal Server Error' })
      .mockResolvedValueOnce(checkoutInfoFixture())

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

    expect(wrapper.text()).toContain('Internal Server Error')
    expect(wrapper.text()).not.toContain('payment.notAvailable')
    expect(wrapper.text()).not.toContain('payment.rechargeAccount')
    expect(showError).toHaveBeenCalledWith('Internal Server Error')

    const retry = wrapper.findAll('button').find(button => button.text() === 'common.refresh')
    expect(retry).toBeDefined()
    await retry!.trigger('click')
    await flushPromises()

    expect(getCheckoutInfo).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).not.toContain('Internal Server Error')
    expect(wrapper.text()).toContain('payment.rechargeAccount')
    wrapper.unmount()
  })
})

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

describe('PaymentView recharge rate preview', () => {
  it('uses the selected payment method currency in both locale templates', async () => {
    translate.mockClear()
    routeState.path = '/purchase'
    routeState.query = {}
    getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture({
      balance_recharge_multiplier: 0.5,
      methods: {
        stripe: {
          ...checkoutInfoFixture().data.methods.wxpay,
          currency: 'USD',
        },
      },
    }))

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
    wrapper.getComponent(AmountInput).vm.$emit('update:modelValue', 10)
    await flushPromises()

    expect(translate).toHaveBeenCalledWith('payment.rechargeRatePreview', {
      currency: 'USD',
      usd: '0.50',
    })
    expect(en.payment.rechargeRatePreview).toBe('Current rate: 1 {currency} = {usd} USD')
    expect(zh.payment.rechargeRatePreview).toBe('当前倍率：1 {currency} = {usd} USD')
  })
})

describe('PaymentView subscription confirmation amounts', () => {
  it('shows the converted pay amount using the subscription rate, not the balance multiplier', async () => {
    const wrapper = await mountSubscriptionConfirm({
      checkout: {
        balance_recharge_multiplier: 0.14,
        subscription_usd_to_cny_rate: 25000,
      },
      method: {
        currency: 'VND',
      },
      plan: {
        price: 10,
        original_price: 12,
      },
    })

    const text = wrapper.text()
    const convertedPrice = formatPaymentAmount(250000, 'VND')
    const convertedOriginalPrice = formatPaymentAmount(300000, 'VND')

    expect(text).toContain(convertedPrice)
    expect(text).toContain(convertedOriginalPrice)
    // 换算必须使用订阅汇率（×25000），而不是余额倍率（÷0.14）
    expect(text).not.toContain(formatPaymentAmount(71, 'VND'))
    expect(wrapper.findAll('button').some(button => button.text().includes(convertedPrice))).toBe(true)
  })

  it('falls back to the live rate when the subscription rate is unset, never to the raw price', async () => {
    // 回归锁：未配置订阅汇率时用实时牌价换算，绝不按面值直付
    // （8000 USD 的套餐不能变成 8000 VND）。
    getExchangeRate.mockResolvedValue({ data: { rate: '26000', gateway_currency: 'VND' } })
    const gatewayWrapper = await mountSubscriptionConfirm({
      checkout: {
        balance_recharge_multiplier: 0.14,
        subscription_usd_to_cny_rate: 0,
      },
      method: {
        currency: 'VND',
      },
      plan: {
        price: 2,
      },
    })

    expect(gatewayWrapper.text()).toContain(formatPaymentAmount(52000, 'VND'))
    expect(gatewayWrapper.text()).not.toContain(formatPaymentAmount(2, 'VND'))

    const usdWrapper = await mountSubscriptionConfirm({
      checkout: {
        subscription_usd_to_cny_rate: 25000,
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

  it('adds the fee rate after rate conversion to match the backend pay_amount', async () => {
    const wrapper = await mountSubscriptionConfirm({
      checkout: {
        subscription_usd_to_cny_rate: 25000,
        recharge_fee_rate: 2.5,
      },
      method: {
        currency: 'VND',
      },
      plan: {
        price: 10,
      },
    })

    const text = wrapper.text()
    const convertedPrice = formatPaymentAmount(250000, 'VND')
    const fee = formatPaymentAmount(6250, 'VND')
    const total = formatPaymentAmount(256250, 'VND')

    expect(text).toContain(convertedPrice)
    expect(text).toContain(fee)
    expect(text).toContain(total)
    expect(wrapper.findAll('button').some(button => button.text().includes(total))).toBe(true)
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

  it('restores a custom EasyPay method as the selected payment method', async () => {
    getCheckoutInfo.mockResolvedValue(checkoutInfoFixture({
      methods: {
        wxpay: checkoutInfoFixture().data.methods.wxpay,
        ldc: {
          daily_limit: 0,
          daily_used: 0,
          daily_remaining: 0,
          single_min: 0,
          single_max: 0,
          fee_rate: 0,
          available: true,
          display_name: 'LDC Pay',
        },
      },
    }))
    window.localStorage.setItem(PAYMENT_RECOVERY_STORAGE_KEY, JSON.stringify({
      orderId: 888,
      amount: 66,
      qrCode: 'ldc-qr',
      expiresAt: '2099-01-01T00:10:00.000Z',
      paymentType: 'ldc',
      payUrl: 'https://pay.example.com/ldc',
      outTradeNo: 'sub2_ldc_888',
      clientSecret: '',
      intentId: '',
      currency: '',
      countryCode: '',
      paymentEnv: '',
      payAmount: 66,
      orderType: 'balance',
      paymentMode: 'popup',
      resumeToken: '',
      createdAt: Date.now(),
    }))

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: {
            template: '<div><slot /></div>',
          },
          PaymentStatusPanel: {
            template: '<button data-test="payment-done" @click="$emit(\'done\')" />',
          },
          PaymentMethodSelector: {
            props: ['selected'],
            template: '<div data-test="method-selector">{{ selected }}</div>',
          },
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()
    await wrapper.find('[data-test="payment-done"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-test="method-selector"]').text()).toBe('ldc')
  })
})

describe('PaymentView gateway return', () => {
  it('hands the gateway status to the panel when the user is sent back here', async () => {
    // The result page redirects a gateway return to this page so the panel that
    // was already tracking the order reports the outcome. It has to pick the
    // status up from the URL — otherwise a cancelled checkout sits on
    // "waiting for payment" until the order expires.
    vi.useRealTimers()
    routeState.path = '/purchase'
    routeState.query = { resume_token: 'resume-7', order_id: '7', status: 'cancelled' }
    getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture())
    window.localStorage.clear()
    window.localStorage.setItem(PAYMENT_RECOVERY_STORAGE_KEY, JSON.stringify({
      orderId: 7,
      amount: 100000,
      qrCode: '',
      expiresAt: '2099-01-01T00:10:00.000Z',
      paymentType: 'sepay_bank_transfer',
      payUrl: 'https://pay.example.com/session/7',
      outTradeNo: 'sub2_20260904abcd1234',
      clientSecret: '',
      intentId: '',
      currency: 'VND',
      countryCode: '',
      paymentEnv: '',
      payAmount: 100000,
      orderType: 'balance',
      paymentMode: 'redirect',
      resumeToken: 'resume-7',
      createdAt: Date.UTC(2099, 0, 1, 0, 0, 0),
    }))

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

    const panel = wrapper.findComponent({ name: 'PaymentStatusPanel' })
    expect(panel.exists()).toBe(true)
    expect(panel.props('gatewayReturnStatus')).toBe('cancelled')
  })
})

describe('PaymentView subscription feature flag', () => {
  afterEach(() => {
    appStoreState.setPublicSettings(undefined)
  })

  function tabLabels(wrapper: Awaited<ReturnType<typeof mountSubscriptionPlanList>>) {
    return wrapper
      .findAll('button')
      .map((button) => button.text())
      .filter((text) => text === 'payment.tabTopUp' || text === 'payment.tabSubscribe')
  }

  it('keeps the top-up / subscribe switcher when subscription_enabled is absent (opt-out default)', async () => {
    const wrapper = await mountSubscriptionPlanList(2)

    expect(tabLabels(wrapper)).toEqual(['payment.tabTopUp', 'payment.tabSubscribe'])
    expect(wrapper.findAllComponents(SubscriptionPlanCard)).toHaveLength(2)
  })

  it('drops the subscribe tab, hides the switcher and ignores ?tab=subscription when subscriptions are disabled', async () => {
    appStoreState.setPublicSettings({ subscription_enabled: false })
    const wrapper = await mountSubscriptionPlanList(2)

    expect(tabLabels(wrapper)).toEqual([])
    expect(wrapper.findAllComponents(SubscriptionPlanCard)).toHaveLength(0)
    expect(wrapper.text()).toContain('payment.rechargeAccount')
  })

  it('shows an unavailable notice instead of a doomed top-up form when balance recharge is disabled too', async () => {
    appStoreState.setPublicSettings({ subscription_enabled: false })
    const wrapper = await mountSubscriptionConfirm({ checkout: { balance_disabled: true } })

    expect(tabLabels(wrapper)).toEqual([])
    expect(wrapper.findAllComponents(SubscriptionPlanCard)).toHaveLength(0)
    expect(wrapper.text()).not.toContain('payment.confirmSubscription')
    expect(wrapper.text()).not.toContain('payment.rechargeAccount')
    expect(wrapper.text()).toContain('payment.billingUnavailable')
    wrapper.unmount()
  })

  it('falls back from the subscribe tab to top-up when the flag flips off after mount', async () => {
    const wrapper = await mountSubscriptionPlanList(2)
    expect(wrapper.findAllComponents(SubscriptionPlanCard)).toHaveLength(2)

    appStoreState.setPublicSettings({ subscription_enabled: false })
    await flushPromises()

    expect(tabLabels(wrapper)).toEqual([])
    expect(wrapper.findAllComponents(SubscriptionPlanCard)).toHaveLength(0)
    expect(wrapper.text()).toContain('payment.rechargeAccount')
    wrapper.unmount()
  })

  it('enters the subscribe tab when a subscription-only site turns subscriptions back on', async () => {
    appStoreState.setPublicSettings({ subscription_enabled: false })
    const wrapper = await mountSubscriptionConfirm({ checkout: { balance_disabled: true } })
    expect(wrapper.text()).toContain('payment.billingUnavailable')

    appStoreState.setPublicSettings({ subscription_enabled: true })
    await flushPromises()

    expect(wrapper.text()).not.toContain('payment.billingUnavailable')
    expect(wrapper.text()).not.toContain('payment.rechargeAccount')
    expect(wrapper.findAllComponents(SubscriptionPlanCard).length).toBeGreaterThan(0)
    wrapper.unmount()
  })
})
