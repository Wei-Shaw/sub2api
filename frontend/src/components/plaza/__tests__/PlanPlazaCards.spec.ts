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
    ],
  })
}

function mountCards(cards: PlazaPlanCard[], loading = false) {
  return mount(PlanPlazaCards, {
    props: { cards, loading },
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
})
