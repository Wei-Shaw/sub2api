import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'
import PaymentView from '../PaymentView.vue'
import AmountInput from '@/components/payment/AmountInput.vue'

// 把所有外部依赖 mock 到本测试不关心的极简形态：
//   - vue-router / vue-i18n / 各 store / paymentAPI 都被替换为 vi.fn / 常量返回值
//   - 这样我们可以 100% 控制 PaymentView 看到的 checkout-info 与登录态，
//     专注断言 promo banner、breakdown 加赠行、preset 点击红点 dismiss。
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
const setRechargePromo = vi.hoisted(() => vi.fn())

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
    }),
  }
})

// useRechargePromoDot 通过 user.value?.id 取 userId，必须给一个真实的数字 id，
// 否则 storageKey 计算为 null，dismiss() 静默 no-op，第三个用例就拿不到 localStorage 写入。
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    user: {
      id: 42,
      username: 'demo-user',
      balance: 0,
    },
    refreshUser,
  }),
}))

vi.mock('@/stores/payment', () => ({
  usePaymentStore: () => ({
    createOrder,
    setRechargePromo,
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

// 桌面端：避免触发移动端微信回退或自动跳转分支。
vi.mock('@/utils/device', () => ({
  isMobileDevice: () => false,
}))

interface PromoTierFixture {
  min_amount: number
  bonus_rate: number
}

interface PromoFixture {
  enabled: boolean
  version: string
  valid_from?: string | null
  valid_until?: string | null
  tiers: PromoTierFixture[]
}

function defaultPromoFixture(): PromoFixture {
  return {
    enabled: true,
    version: 'v1',
    valid_from: null,
    valid_until: null,
    tiers: [
      { min_amount: 100, bonus_rate: 0.05 },
      { min_amount: 500, bonus_rate: 0.08 },
    ],
  }
}

interface CheckoutFixtureOptions {
  /** undefined → 不下发 recharge_promo 字段；null → 字段为 null；object → 携带 promo。 */
  promo?: PromoFixture | null
  balance_recharge_multiplier?: number
}

function checkoutFixture(opts: CheckoutFixtureOptions = {}) {
  const data: Record<string, unknown> = {
    methods: {
      // wxpay 是 visible 方法（normalizeVisibleMethod 命中），single_min/max=0 表示无限制，
      // available=true 让 enabledMethods.length > 0，从而展开充值 UI（含 AmountInput / breakdown）。
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
    balance_recharge_multiplier: opts.balance_recharge_multiplier ?? 1,
    recharge_fee_rate: 0,
    help_text: '',
    help_image_url: '',
    stripe_publishable_key: '',
  }
  if ('promo' in opts) {
    data.recharge_promo = opts.promo
  }
  return { data }
}

function mountPaymentView() {
  return shallowMount(PaymentView, {
    global: {
      stubs: {
        // AppLayout 默认 shallowMount 会留下空 stub，这里透明化以便子内容（含 banner / breakdown）渲染。
        AppLayout: { template: '<div class="app-layout-stub"><slot /></div>' },
        Teleport: true,
        Transition: false,
      },
    },
  })
}

describe('PaymentView · recharge bonus promo', () => {
  beforeEach(() => {
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
    getCheckoutInfo.mockReset()
    setRechargePromo.mockReset()
    window.localStorage.clear()
  })

  describe('campaign banner', () => {
    it('omits the promo banner when checkout-info has no recharge_promo', async () => {
      getCheckoutInfo.mockResolvedValue(checkoutFixture())
      const wrapper = mountPaymentView()
      await flushPromises()

      const html = wrapper.html()
      expect(html).not.toContain('payment.promo.banner')
      expect(html).not.toContain('payment.promo.bannerNoExpiry')
      expect(html).not.toContain('payment.promo.tier')
    })

    it('renders the no-expiry banner copy plus tier list when recharge_promo is present without valid_until', async () => {
      getCheckoutInfo.mockResolvedValue(checkoutFixture({ promo: defaultPromoFixture() }))
      const wrapper = mountPaymentView()
      await flushPromises()

      const html = wrapper.html()
      // t() 被 mock 成 identity，所以 i18n key 直接出现在 DOM 里。
      expect(html).toContain('payment.promo.bannerNoExpiry')
      expect(html).not.toContain('payment.promo.banner ') // 防止 `banner` 误判：实际渲染的是 bannerNoExpiry
      expect(html).toContain('payment.promo.tier')
    })
  })

  describe('breakdown bonus row', () => {
    it('hides the bonus and total-credited rows when no amount is entered', async () => {
      getCheckoutInfo.mockResolvedValue(checkoutFixture({ promo: defaultPromoFixture() }))
      const wrapper = mountPaymentView()
      await flushPromises()

      const html = wrapper.html()
      // 整张 breakdown 卡片都要求 validAmount > 0；金额为空时这两行根本不会出现。
      expect(html).not.toContain('payment.promo.bonusLine')
      expect(html).not.toContain('payment.promo.totalCredited')
    })

    it('keeps the bonus row hidden when the amount is below the lowest tier', async () => {
      getCheckoutInfo.mockResolvedValue(checkoutFixture({ promo: defaultPromoFixture() }))
      const wrapper = mountPaymentView()
      await flushPromises()

      const amountInput = wrapper.findComponent(AmountInput)
      expect(amountInput.exists()).toBe(true)
      // 50 < 100（最低档），bonusForAmount → 0，bonus 行应保持隐藏。
      amountInput.vm.$emit('update:modelValue', 50)
      await flushPromises()

      expect(wrapper.html()).not.toContain('payment.promo.bonusLine')
    })

    it('renders the bonus / total-credited rows once a tier-eligible amount is entered', async () => {
      getCheckoutInfo.mockResolvedValue(checkoutFixture({ promo: defaultPromoFixture() }))
      const wrapper = mountPaymentView()
      await flushPromises()

      const amountInput = wrapper.findComponent(AmountInput)
      // 200 命中 (min_amount=100, bonus_rate=0.05)；
      // bonus = ceil(200 × 1 × 0.05 × 100) / 100 = 10；total = 200 + 10 = 210。
      amountInput.vm.$emit('update:modelValue', 200)
      await flushPromises()

      const html = wrapper.html()
      expect(html).toContain('payment.promo.bonusLine')
      expect(html).toContain('payment.promo.totalCredited')
      // 模板里 `+$` 与 `$` 是硬编码字符串，跟 i18n / formatPaymentAmount 解耦，断言稳定。
      expect(html).toContain('+$10.00')
      expect(html).toContain('$210.00')
    })
  })

  describe('preset click red-dot dismissal', () => {
    it('writes the seen localStorage key when AmountInput emits bonusPresetClicked', async () => {
      getCheckoutInfo.mockResolvedValue(checkoutFixture({ promo: defaultPromoFixture() }))
      const wrapper = mountPaymentView()
      await flushPromises()

      const seenKey = 'recharge-promo-seen:42:v1'
      // 初始：用户从未 dismiss，红点应允许显示，localStorage 无该 key。
      expect(window.localStorage.getItem(seenKey)).toBeNull()

      const amountInput = wrapper.findComponent(AmountInput)
      // AmountInput 在用户点击命中赠送档位的 preset 时会 emit 这个事件 —
      // 这里直接模拟该 emit，验证 PaymentView 的 onPromoPresetClicked 调用 dismiss()。
      amountInput.vm.$emit('bonusPresetClicked', 200)
      await flushPromises()

      expect(window.localStorage.getItem(seenKey)).toBe('1')
    })

    it('does not write the seen key when the promo is absent (handler is a no-op)', async () => {
      getCheckoutInfo.mockResolvedValue(checkoutFixture())
      const wrapper = mountPaymentView()
      await flushPromises()

      const amountInput = wrapper.findComponent(AmountInput)
      amountInput.vm.$emit('bonusPresetClicked', 200)
      await flushPromises()

      // 没有 promo.version，buildKey → null，dismiss 应该早返回；localStorage 保持空。
      expect(window.localStorage.length).toBe(0)
    })
  })
})
