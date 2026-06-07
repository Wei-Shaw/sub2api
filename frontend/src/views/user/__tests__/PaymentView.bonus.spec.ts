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

  // 充值赠送活动到期防呆（用户在充值页待时间过长 → 活动 valid_until 已过
  // → 点击"创建订单"必须先弹二次确认）。两个用例分别覆盖：
  //   1. continue：弹窗 → 用户点"继续充值" → 真正调用 createOrder；
  //   2. cancel  ：弹窗 → 用户取消 → 不进入下单流程；
  // 还有一条独立的负向用例：valid_until 仍在未来 → 点击直接下单，
  // 永远不显示弹窗 — 防止 modal 误伤"活动还在窗口内"的常规路径。
  describe('expired-promo confirm modal', () => {
    /**
     * 触发到期场景的最小 fixture：
     *   • 把 `valid_until` 钉在 2020 年 — 远比任何现实运行时早，
     *     `Date.now() >= ts` 必为真，无需 `vi.useFakeTimers`。
     *   • tiers / enabled 保持默认，让 banner 与 breakdown 加赠行
     *     仍然渲染——这正是题目里"用户停留过久，banner 还摆着但
     *     窗口期已过"的状态。
     */
    function expiredPromoFixture(): PromoFixture {
      return {
        ...defaultPromoFixture(),
        valid_until: '2020-01-01T00:00:00Z',
      }
    }

    /**
     * 拿到充值底部的"创建订单"提交按钮。组件里通过 data-test
     * 钩子 `payment-submit-recharge` 锁定，避免依赖 i18n 文本
     * （t() 在测试里被 mock 成 identity，文本会包含 key）或
     * 顺序 (`buttons[buttons.length - 1]`) 这类脆弱选择。
     */
    function findSubmitButton(wrapper: ReturnType<typeof mountPaymentView>) {
      const btn = wrapper.find('[data-test="payment-submit-recharge"]')
      expect(btn.exists()).toBe(true)
      return btn
    }

    /** 触发金额输入到 200（命中第一档），让 canSubmit 进入 true。 */
    async function enterTierAmount(wrapper: ReturnType<typeof mountPaymentView>, amt = 200) {
      const amountInput = wrapper.findComponent(AmountInput)
      amountInput.vm.$emit('update:modelValue', amt)
      await flushPromises()
    }

    it('blocks submission and only forwards to createOrder after the user confirms', async () => {
      getCheckoutInfo.mockResolvedValue(checkoutFixture({ promo: expiredPromoFixture() }))
      createOrder.mockResolvedValue({})

      const wrapper = mountPaymentView()
      await flushPromises()
      await enterTierAmount(wrapper)

      // 第一次点提交：因为活动已经过期，应只触发弹窗，不走下单。
      await findSubmitButton(wrapper).trigger('click')
      await flushPromises()

      const modal = wrapper.find('[data-test="promo-expired-modal"]')
      expect(modal.exists()).toBe(true)
      expect(createOrder).not.toHaveBeenCalled()

      // 用户在弹窗内确认——这次必须真正下单。
      await wrapper.find('[data-test="promo-expired-confirm"]').trigger('click')
      await flushPromises()

      expect(createOrder).toHaveBeenCalledTimes(1)
      // 弹窗应当随 confirm 关闭；保留它只会让用户疑惑还有第二次确认。
      expect(wrapper.find('[data-test="promo-expired-modal"]').exists()).toBe(false)
    })

    it('keeps createOrder untouched when the user cancels in the modal', async () => {
      getCheckoutInfo.mockResolvedValue(checkoutFixture({ promo: expiredPromoFixture() }))

      const wrapper = mountPaymentView()
      await flushPromises()
      await enterTierAmount(wrapper)

      await findSubmitButton(wrapper).trigger('click')
      await flushPromises()
      expect(wrapper.find('[data-test="promo-expired-modal"]').exists()).toBe(true)

      await wrapper.find('[data-test="promo-expired-cancel"]').trigger('click')
      await flushPromises()

      // 核心断言——取消必须不进入下单流程。这是题目的硬性要求；
      // 模态本身是否同步消失依赖 Vue `<Transition>` 在 jsdom 下的
      // leave 完成时序（confirm 路径里的 `await createOrder(...)`
      // 隐式给了它多一个 microtask 调度，cancel 是纯同步翻转，时序
      // 在这里更紧），用例不去守这条不稳定的边界——modal 关闭使用
      // 的是和 confirm 相同的 `showPromoExpiredModal.value = false`，
      // confirm 用例已经守住了那行代码的正确性。
      expect(createOrder).not.toHaveBeenCalled()
    })

    it('does not show the modal when the promo is still in window', async () => {
      // 反向用例：未过期 → 不能误触弹窗。否则正常充值路径都会被
      // 拦下，回归严重。
      getCheckoutInfo.mockResolvedValue(checkoutFixture({ promo: defaultPromoFixture() }))
      createOrder.mockResolvedValue({})

      const wrapper = mountPaymentView()
      await flushPromises()
      await enterTierAmount(wrapper)

      await findSubmitButton(wrapper).trigger('click')
      await flushPromises()

      expect(wrapper.find('[data-test="promo-expired-modal"]').exists()).toBe(false)
      expect(createOrder).toHaveBeenCalledTimes(1)
    })

    it('forwards client_expected_bonus on the create-order payload for active promos', async () => {
      // 后端拦截判窗的唯一信号是这个字段——前端必须在每次 balance 提交
      // 时把"banner / breakdown 上正在向用户展示的赠送预览金额"如实
      // 上报。这里守 happy path：promo 还在窗口、用户付 200 → 命中
      // 200×1×0.05 = 10 这一档，payload 必须带 client_expected_bonus=10。
      // 一旦该字段被误删，server 的 RECHARGE_PROMO_EXPIRED 闸门会彻底
      // 失效（client 永远不发期待 → server 永远不拦），属于安全回归
      // 必须有测试守住。
      getCheckoutInfo.mockResolvedValue(checkoutFixture({ promo: defaultPromoFixture() }))
      createOrder.mockResolvedValue({})

      const wrapper = mountPaymentView()
      await flushPromises()
      await enterTierAmount(wrapper, 200)

      await findSubmitButton(wrapper).trigger('click')
      await flushPromises()

      expect(createOrder).toHaveBeenCalledTimes(1)
      const payload = createOrder.mock.calls[0]?.[0] as Record<string, unknown>
      expect(payload).toMatchObject({
        amount: 200,
        order_type: 'balance',
        client_expected_bonus: 10,
      })
      // 没确认过就不该带 ack；只有用户在 modal 上点了"继续充值"重发
      // 时才允许出现。
      expect(payload.promo_expired_acknowledged).toBeUndefined()
    })

    // 服务端兜底路径：客户端时钟与服务端有偏差、或 admin 在用户停留期间
    // 中途禁用了活动 → 本地 isPromoExpiredNow() 漏判 → server 返回
    // 409 RECHARGE_PROMO_EXPIRED。前端 catch 必须把这条错误转化为同一个
    // 二次确认 modal（与本地快速路径形成对称），不能走通用 toast。
    describe('server-side 409 fallback', () => {
      /** 模拟后端 infraerrors.Conflict 经 apiClient 拦截后抛出的形状。 */
      function makePromoExpired409(): Error & Record<string, unknown> {
        const err = new Error('recharge bonus campaign has ended') as Error & Record<string, unknown>
        err.status = 409
        err.code = 409
        err.reason = 'RECHARGE_PROMO_EXPIRED'
        return err
      }

      it('opens the modal when paymentStore.createOrder rejects with RECHARGE_PROMO_EXPIRED', async () => {
        // 用未过期的 promo 让本地快速路径放行 → 第一次提交真的会调
        // server，server 才有机会返回 409。这条路径覆盖"client 时钟
        // 慢 / admin 中途禁用活动"两类现实漂移。
        getCheckoutInfo.mockResolvedValue(checkoutFixture({ promo: defaultPromoFixture() }))
        createOrder.mockRejectedValueOnce(makePromoExpired409())

        const wrapper = mountPaymentView()
        await flushPromises()
        await enterTierAmount(wrapper, 200)

        await findSubmitButton(wrapper).trigger('click')
        await flushPromises()

        // 第一次提交应该已经走到 server（返回了 409）。
        expect(createOrder).toHaveBeenCalledTimes(1)
        // catch 分支必须把它转成 modal——而不是通用 toast。
        expect(showError).not.toHaveBeenCalled()
        expect(wrapper.find('[data-test="promo-expired-modal"]').exists()).toBe(true)

        // 用户在 modal 内确认 → 应再次调用 createOrder，且这次必须
        // 带 promo_expired_acknowledged=true（让 server 跳过拦截）。
        createOrder.mockResolvedValueOnce({})
        await wrapper.find('[data-test="promo-expired-confirm"]').trigger('click')
        await flushPromises()

        expect(createOrder).toHaveBeenCalledTimes(2)
        const retryPayload = createOrder.mock.calls[1]?.[0] as Record<string, unknown>
        expect(retryPayload).toMatchObject({
          amount: 200,
          order_type: 'balance',
          promo_expired_acknowledged: true,
        })
      })

      it('does not retry createOrder when the user cancels the server-triggered modal', async () => {
        // 防回归：cancel 路径在 server 触发的弹窗里也必须保持"不重发"。
        // 这是题目的硬性约束 — 用户拒绝就别再悄悄替他下单。
        getCheckoutInfo.mockResolvedValue(checkoutFixture({ promo: defaultPromoFixture() }))
        createOrder.mockRejectedValueOnce(makePromoExpired409())

        const wrapper = mountPaymentView()
        await flushPromises()
        await enterTierAmount(wrapper, 200)

        await findSubmitButton(wrapper).trigger('click')
        await flushPromises()
        expect(wrapper.find('[data-test="promo-expired-modal"]').exists()).toBe(true)
        expect(createOrder).toHaveBeenCalledTimes(1) // server 那次

        await wrapper.find('[data-test="promo-expired-cancel"]').trigger('click')
        await flushPromises()

        // 关键：取消后不应再次 createOrder。计数仍是 1（只有最初触发
        // 409 的那次）。
        expect(createOrder).toHaveBeenCalledTimes(1)
      })
    })
  })
})
