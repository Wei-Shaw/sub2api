import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import PlanPlazaCards from '../PlanPlazaCards.vue'
import type { PlazaPlanCard } from '@/api/plaza'

// 项目约定：在组件单测里 mock vue-i18n，避免引入完整 i18n 实例。
// `t(key)` 直接回显 key，便于断言文案是否被引用、以及非翻译数据是否正确渲染。
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

function makeCard(over: Partial<PlazaPlanCard> = {}): PlazaPlanCard {
  return {
    id: 1,
    name: 'Pro',
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
    ...over,
  }
}

const formatCny = (amount: number) =>
  `¥${amount.toLocaleString(undefined, { minimumFractionDigits: 0, maximumFractionDigits: 2 })}`

// 组件内部使用了 useAppStore（payment_enabled 开关）和 useAuthRedirect
// （依赖 useRouter + useAuthStore），所以 mount 时必须挂上 Pinia 与 vue-router
// 两个全局插件，否则会抛 "no active Pinia"。
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

function mountCards(
  cards: PlazaPlanCard[],
  loading = false,
  extraProps: Partial<{ maxItems: number; viewAllHref: string }> = {},
) {
  return mount(PlanPlazaCards, {
    props: { cards, loading, ...extraProps },
    global: {
      plugins: [createPinia(), makeRouter()],
    },
  })
}

beforeEach(() => {
  setActivePinia(createPinia())
})

describe('PlanPlazaCards', () => {
  it('显示折扣徽章并划掉 original_price，当 original_price > price', () => {
    const wrapper = mountCards([
      makeCard({ price: 80, original_price: 100 }),
    ])
    const text = wrapper.text()
    expect(text).toContain('-20%')
    // 内置 formatter 使用 `min:0, max:2`，整数金额不补 .00
    expect(text).toContain(formatCny(80))
    expect(text).toContain(formatCny(100))
    expect(wrapper.html()).toContain('line-through')
  })

  it('original_price <= price 时既不显示折扣徽章，也不显示划掉价', () => {
    const wrapper = mountCards([
      makeCard({ price: 100, original_price: 100 }),
    ])
    // 折扣徽章使用 "-N%" 模式；用更精确的正则避免 model 名里的 "-" 误判
    expect(wrapper.text()).not.toMatch(/-\d+%/)
    expect(wrapper.html()).not.toContain('line-through')
  })

  it('original_price 缺失时按原价展示，无划掉线', () => {
    const wrapper = mountCards([makeCard({ price: 50 })])
    expect(wrapper.text()).toContain(formatCny(50))
    expect(wrapper.html()).not.toContain('line-through')
  })

  it('小数金额最多保留两位小数', () => {
    const wrapper = mountCards([makeCard({ price: 19.9 })])
    expect(wrapper.text()).toContain('¥19.9')
  })

  it('models 超过 10 个时折叠为 +N more 芯片', () => {
    const models = Array.from({ length: 13 }, (_, i) => `m-${i}`)
    const wrapper = mountCards([makeCard({ models })])
    expect(wrapper.text()).toContain('+3 ')
    expect(wrapper.text()).toContain('m-0')
    expect(wrapper.text()).toContain('m-9')
    expect(wrapper.text()).not.toContain('m-10')
  })

  it('models_overflow 与本地超出叠加', () => {
    const models = Array.from({ length: 12 }, (_, i) => `m-${i}`)
    const wrapper = mountCards([makeCard({ models, models_overflow: 5 })])
    // 本地 12-10=2 + 服务端 5 = 7
    expect(wrapper.text()).toContain('+7 ')
  })

  it('cards 为空时显示空态文案 key', () => {
    const wrapper = mountCards([])
    expect(wrapper.text()).toContain('plaza.plans.empty')
  })

  it('loading 时显示 loading 文案 key', () => {
    const wrapper = mountCards([], true)
    expect(wrapper.text()).toContain('common.loading')
  })

  it('features 按行拆分为列表', () => {
    const wrapper = mountCards([
      makeCard({ features: 'Unlimited usage\nPriority support\n  ' }),
    ])
    const text = wrapper.text()
    expect(text).toContain('Unlimited usage')
    expect(text).toContain('Priority support')
  })

  // 套餐列表强制使用 CNY，不再渲染 CurrencyToggle；曾经的 `emits currency-change` 用例已删除。
  it('不渲染 CurrencyToggle（套餐价格固定 CNY）', () => {
    const wrapper = mountCards([makeCard()])
    const usdBtn = wrapper.findAll('button').find((b) => b.text() === 'USD')
    expect(usdBtn).toBeUndefined()
  })

  // ----- 首页 showcase 用到的两个可选 prop：maxItems / viewAllHref -----
  // 默认行为（不传新 prop）必须与 PlazaPlansView 当前一致：渲染全部卡片、不渲染“查看全部”链接。
  describe('homepage showcase props', () => {
    function makeManyCards(n: number): PlazaPlanCard[] {
      return Array.from({ length: n }, (_, i) =>
        makeCard({ id: i + 1, name: `Plan ${i + 1}` }),
      )
    }

    it('maxItems=3 时只渲染前 3 张卡', () => {
      const wrapper = mountCards(makeManyCards(5), false, { maxItems: 3 })
      const articles = wrapper.findAll('article')
      expect(articles.length).toBe(3)
      expect(wrapper.text()).toContain('Plan 1')
      expect(wrapper.text()).toContain('Plan 3')
      expect(wrapper.text()).not.toContain('Plan 4')
      expect(wrapper.text()).not.toContain('Plan 5')
    })

    it('未传 maxItems 时维持现有行为：渲染全部卡片', () => {
      const wrapper = mountCards(makeManyCards(5))
      expect(wrapper.findAll('article').length).toBe(5)
    })

    it('cards 数 > maxItems 时（首页有截断）渲染"查看全部"链接', () => {
      const wrapper = mountCards(makeManyCards(5), false, {
        maxItems: 3,
        viewAllHref: '/plaza/plans',
      })
      // i18n key 直接被回显（见顶部 mock）
      expect(wrapper.text()).toContain('home.plans.view_all')
      const link = wrapper.find('a[href="/plaza/plans"]')
      expect(link.exists()).toBe(true)
    })

    it('cards 数 <= maxItems 时（首页已展示全部）不渲染"查看全部"链接', () => {
      // 边界 1：cards 数 === maxItems —— 全部已经在屏，再给 view-all 链接
      // 会指向相同卡片集合，纯多余。
      const wrapperEq = mountCards(makeManyCards(3), false, {
        maxItems: 3,
        viewAllHref: '/plaza/plans',
      })
      expect(wrapperEq.text()).not.toContain('home.plans.view_all')

      // 边界 2：cards 数 < maxItems —— 同上，也不该出现链接。
      const wrapperLt = mountCards(makeManyCards(2), false, {
        maxItems: 3,
        viewAllHref: '/plaza/plans',
      })
      expect(wrapperLt.text()).not.toContain('home.plans.view_all')
    })

    it('未传 maxItems（全量展示）时即使有 viewAllHref 也不渲染链接', () => {
      // PlazaPlansView 场景：不传 maxItems，本身就是全量管理页。理论上
      // 也不会传 viewAllHref，但这里加防御性断言：即使误传，也不会渲出
      // 一个"查看全部"按钮指回当前页。
      const wrapper = mountCards(makeManyCards(5), false, {
        viewAllHref: '/plaza/plans',
      })
      expect(wrapper.text()).not.toContain('home.plans.view_all')
    })

    it('未传 viewAllHref 时不渲染"查看全部"链接（PlazaPlansView 现状）', () => {
      const wrapper = mountCards(makeManyCards(3))
      expect(wrapper.text()).not.toContain('home.plans.view_all')
    })

    it('cards 为空时即使传了 viewAllHref 也不渲染链接（避免空态指向无意义页）', () => {
      const wrapper = mountCards([], false, { viewAllHref: '/plaza/plans' })
      expect(wrapper.text()).not.toContain('home.plans.view_all')
    })

    it('loading 时不渲染"查看全部"链接', () => {
      const wrapper = mountCards(makeManyCards(5), true, {
        maxItems: 3,
        viewAllHref: '/plaza/plans',
      })
      expect(wrapper.text()).not.toContain('home.plans.view_all')
    })
  })
})
