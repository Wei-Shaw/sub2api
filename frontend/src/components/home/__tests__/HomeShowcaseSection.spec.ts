import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

// 测试目标 + 依赖：先 mock plazaAPI，再 import 组件，避免组件 onMounted 时
// 真的去打 axios 请求。
vi.mock('@/api/plaza', async () => {
  const actual = await vi.importActual<typeof import('@/api/plaza')>('@/api/plaza')
  return {
    ...actual,
    plazaAPI: {
      getRechargePromo: vi.fn(),
      listPlans: vi.fn(),
    },
  }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

import HomeShowcaseSection from '../HomeShowcaseSection.vue'
import { plazaAPI, type PlazaPlanCard, type PublicRechargePromo } from '@/api/plaza'
import { useAppStore } from '@/stores/app'

const mockedGetPromo = vi.mocked(plazaAPI.getRechargePromo)
const mockedListPlans = vi.mocked(plazaAPI.listPlans)

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/login', component: { template: '<div />' } },
      { path: '/purchase', component: { template: '<div />' } },
      { path: '/plaza/plans', component: { template: '<div />' } },
    ],
  })
}

function makePromo(over: Partial<PublicRechargePromo> = {}): PublicRechargePromo {
  return {
    name: '充值赠送活动',
    valid_from: '2026-01-01T00:00:00Z',
    valid_until: '2026-02-01T00:00:00Z',
    tiers: [{ min_amount: 100, bonus_rate: 0.05 }],
    version: '1:1700000000',
    ...over,
  }
}

function makeCard(id: number): PlazaPlanCard {
  return {
    id,
    name: `Plan ${id}`,
    description: '',
    price: 99,
    validity_days: 30,
    validity_unit: 'days',
    features: '',
    group_id: 1,
    group_name: 'Default',
    platform: 'claude',
    models: ['claude-3-5-sonnet'],
    models_overflow: 0,
  }
}

async function mountSection(opts: {
  paymentEnabled: boolean | undefined
  promoResponse?: PublicRechargePromo | null
  promoReject?: unknown
  plansResponse?: PlazaPlanCard[]
  plansReject?: unknown
}) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const app = useAppStore()
  // 直接覆盖 cachedPublicSettings —— 组件读 `payment_enabled === true`，
  // undefined / false 都视为关闭。
  if (opts.paymentEnabled === undefined) {
    app.cachedPublicSettings = null
  } else {
    app.cachedPublicSettings = {
      payment_enabled: opts.paymentEnabled,
    } as never
  }

  if (opts.promoReject !== undefined) {
    mockedGetPromo.mockRejectedValueOnce(opts.promoReject)
  } else {
    mockedGetPromo.mockResolvedValueOnce({ promo: opts.promoResponse ?? null })
  }
  if (opts.plansReject !== undefined) {
    mockedListPlans.mockRejectedValueOnce(opts.plansReject)
  } else {
    mockedListPlans.mockResolvedValueOnce({
      cards: opts.plansResponse ?? [],
      currency_meta: {
        balance_recharge_multiplier: 1,
        model_native: 'USD',
        plan_native: 'CNY',
      },
    })
  }

  const wrapper = mount(HomeShowcaseSection, {
    global: {
      plugins: [pinia, makeRouter()],
      stubs: {
        // 子组件渲染细节由各自的 spec 覆盖；这里只关心 HomeShowcaseSection
        // 的「画不画 banner / 画不画 cards」行为。Stub 暴露 data-test 钩子。
        HomePromoBanner: {
          name: 'HomePromoBanner',
          props: ['promo'],
          template: '<div data-test="stub-promo-banner">{{ promo.name }}</div>',
        },
        PlanPlazaCards: {
          name: 'PlanPlazaCards',
          props: ['cards', 'loading', 'maxItems', 'viewAllHref'],
          template:
            '<div data-test="stub-plan-cards" :data-cards="cards.length" :data-max="maxItems" :data-href="viewAllHref" />',
        },
      },
    },
  })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  mockedGetPromo.mockReset()
  mockedListPlans.mockReset()
  // 默认压住 console.warn 噪音；个别用例需要时再 spyOn 拿 mock。
  vi.spyOn(console, 'warn').mockImplementation(() => {})
})

describe('HomeShowcaseSection', () => {
  it('payment_enabled=false 时整段不渲染（不发起任何 API 调用）', async () => {
    const wrapper = await mountSection({ paymentEnabled: false })
    expect(wrapper.find('[data-test="home-showcase-section"]').exists()).toBe(false)
    expect(mockedGetPromo).not.toHaveBeenCalled()
    expect(mockedListPlans).not.toHaveBeenCalled()
  })

  it('cachedPublicSettings 缺失时（首次访问尚未水合）也不渲染 + 不调用 API', async () => {
    const wrapper = await mountSection({ paymentEnabled: undefined })
    expect(wrapper.find('[data-test="home-showcase-section"]').exists()).toBe(false)
    expect(mockedGetPromo).not.toHaveBeenCalled()
  })

  it('payment_enabled=true、promo=null 时只渲染 plan cards + view-all 链接', async () => {
    const wrapper = await mountSection({
      paymentEnabled: true,
      promoResponse: null,
      plansResponse: [makeCard(1), makeCard(2), makeCard(3), makeCard(4)],
    })
    expect(wrapper.find('[data-test="home-showcase-section"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="stub-promo-banner"]').exists()).toBe(false)
    const cards = wrapper.find('[data-test="stub-plan-cards"]')
    expect(cards.exists()).toBe(true)
    // 透传 maxItems=3 / viewAllHref=/plaza/plans
    expect(cards.attributes('data-max')).toBe('3')
    expect(cards.attributes('data-href')).toBe('/plaza/plans')
    // 实际 cards 数量交给子组件按 maxItems 切割；本层把全部传下去。
    expect(cards.attributes('data-cards')).toBe('4')
  })

  it('promo 存在时同时渲染 banner + cards', async () => {
    const wrapper = await mountSection({
      paymentEnabled: true,
      promoResponse: makePromo(),
      plansResponse: [makeCard(1)],
    })
    const banner = wrapper.find('[data-test="stub-promo-banner"]')
    expect(banner.exists()).toBe(true)
    expect(banner.text()).toContain('充值赠送活动')
    expect(wrapper.find('[data-test="stub-plan-cards"]').exists()).toBe(true)
  })

  it('promo 存在时 banner 上方渲染 “活动专区” section header（与 plans 块视觉对齐）', async () => {
    const wrapper = await mountSection({
      paymentEnabled: true,
      promoResponse: makePromo(),
      plansResponse: [makeCard(1)],
    })
    const title = wrapper.find('[data-test="home-promo-section-title"]')
    expect(title.exists()).toBe(true)
    // i18n mock 透传 key 本身
    expect(title.text()).toBe('home.promo.title')
    // header 必须出现在 banner 之前（DOM 顺序），不能反过来
    const html = wrapper.html()
    expect(html.indexOf('home-promo-section-title')).toBeLessThan(
      html.indexOf('stub-promo-banner'),
    )
  })

  it('promo 不存在时不渲染 “活动专区” header（防止空 header 占位）', async () => {
    const wrapper = await mountSection({
      paymentEnabled: true,
      promoResponse: null,
      plansResponse: [makeCard(1)],
    })
    expect(wrapper.find('[data-test="home-promo-section-title"]').exists()).toBe(false)
  })

  it('promo API 失败时 silent skip（仅 console.warn），plan cards 仍渲染', async () => {
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const wrapper = await mountSection({
      paymentEnabled: true,
      promoReject: new Error('boom'),
      plansResponse: [makeCard(1)],
    })
    expect(wrapper.find('[data-test="stub-promo-banner"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="stub-plan-cards"]').exists()).toBe(true)
    expect(warnSpy).toHaveBeenCalled()
    // 不能弹 toast：匿名首页不应有任何 ARIA live region 或 toast 容器。
    expect(wrapper.html()).not.toContain('toast')
  })

  it('plans API 失败时 plans 块整块隐藏（不显示空 header / 空卡片占位），banner 仍按 promo 渲染', async () => {
    // 行为契约：首页是营销面板，"暂无套餐" 的 empty state 会读作
    // "这个产品没东西卖"，反而拉低转化。约定是 cards=[] 直接隐藏整个
    // plans block（header + grid 一起），让 promo banner 独自承担页面。
    // /plaza/plans 上下文不同（用户主动点进去），那里的 empty 文案
    // 仍由 PlanPlazaCards 自己处理，不需要为本次改动改它。
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const wrapper = await mountSection({
      paymentEnabled: true,
      promoResponse: makePromo(),
      plansReject: new Error('plans down'),
    })
    expect(wrapper.find('[data-test="stub-promo-banner"]').exists()).toBe(true)
    // plans 块的两个标志都应消失：section header + 卡片组件本身。
    expect(wrapper.find('[data-test="home-plans-section-header"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="stub-plan-cards"]').exists()).toBe(false)
    expect(warnSpy).toHaveBeenCalled()
  })

  it('promo 与 plans 都为空时整段 section 不渲染（不留 mb-16 空白）', async () => {
    // 边界行为：admin 把 payment_enabled 打开但还没配任何套餐 / 活动时，
    // 不该在首页留一个只有底部 margin 的空 wrapper —— 那会在 features
    // grid 与 footer 之间撕开一道无内容的间距，视觉上像是渲染失败。
    // 约定：promo=null AND cards=[] AND !loading → 整段不画。
    const wrapper = await mountSection({
      paymentEnabled: true,
      promoResponse: null,
      plansResponse: [],
    })
    expect(wrapper.find('[data-test="home-showcase-section"]').exists()).toBe(false)
  })

  it('promo 存在但 plans 为空时仅渲染 promo 区块（plans header / 卡片均不出现）', async () => {
    // 与上一条配合：仍有营销内容（活动）就保留 section，但 plans 块
    // 必须独立隐藏，不能因为 promo 还在就强行把 plans header 也带出来
    // —— 用户会下拉到 plans header 然后看到空白，体验更差。
    const wrapper = await mountSection({
      paymentEnabled: true,
      promoResponse: makePromo(),
      plansResponse: [],
    })
    expect(wrapper.find('[data-test="home-showcase-section"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="stub-promo-banner"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="home-plans-section-header"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="stub-plan-cards"]').exists()).toBe(false)
  })
})
