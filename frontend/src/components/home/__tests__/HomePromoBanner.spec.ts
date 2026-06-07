import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import HomePromoBanner from '../HomePromoBanner.vue'
import type { PublicRechargePromo } from '@/api/plaza'
import { useAuthStore } from '@/stores/auth'

/**
 * Mirrors the i18n mock pattern used by `PlanPlazaCards.spec.ts`: surface the
 * raw key plus interpolation params verbatim so we can assert against any of
 * them without depending on translated copy.
 */
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (!params) return key
        const parts = Object.entries(params).map(([k, v]) => `${k}=${String(v)}`)
        return `${key}|${parts.join(',')}`
      },
    }),
  }
})

function makePromo(over: Partial<PublicRechargePromo> = {}): PublicRechargePromo {
  return {
    name: '春节充值大放送',
    valid_from: '2026-01-01T00:00:00Z',
    valid_until: '2026-02-01T00:00:00Z',
    tiers: [
      { min_amount: 100, bonus_rate: 0.05 },
      { min_amount: 500, bonus_rate: 0.1 },
    ],
    version: '1:1700000000',
    ...over,
  }
}

function makeRouter(): Router {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/login', component: { template: '<div />' } },
      { path: '/purchase', component: { template: '<div />' } },
    ],
  })
  return router
}

async function mountBanner(promo: PublicRechargePromo, authed = false) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const router = makeRouter()
  router.push('/')
  await router.isReady()

  const auth = useAuthStore()
  if (authed) {
    // 直接写 setup-store 暴露的 ref state，跳过 setToken/login 的网络副作用。
    auth.token = 'tk'
    auth.user = { id: 1, email: 'a@b.c' } as never
  }

  const wrapper = mount(HomePromoBanner, {
    props: { promo },
    global: { plugins: [pinia, router] },
  })
  return { wrapper, router, auth }
}

beforeEach(() => {
  setActivePinia(createPinia())
})

describe('HomePromoBanner', () => {
  it('右上角渲染斜置 "限时" 角标（图形化 LIMITED-TIME 视觉锚点）', async () => {
    const { wrapper } = await mountBanner(makePromo())
    const ribbon = wrapper.find('[data-test="home-promo-ribbon"]')
    expect(ribbon.exists()).toBe(true)
    // 文案走 i18n key（mock 透传 key 本身）；并且必须用 rotate-45 实现"斜置"
    expect(ribbon.text()).toContain('home.promo.ribbon')
    expect(ribbon.classes()).toContain('rotate-45')
    // 装饰性元素，不应该截获点击
    expect(ribbon.classes()).toContain('pointer-events-none')
  })

  it('以纯文本渲染活动名（不走 v-html，挡 XSS）', async () => {
    const malicious = makePromo({ name: '<script>alert(1)</script>恶意活动' })
    const { wrapper } = await mountBanner(malicious)
    const nameEl = wrapper.find('[data-test="home-promo-name"]')
    // 文本里包含原始字符；HTML 里不能出现 <script> 标签
    expect(nameEl.text()).toContain('<script>')
    expect(nameEl.text()).toContain('恶意活动')
    expect(wrapper.html()).not.toContain('<script>alert(1)')
  })

  it('渲染每一档 tier，min_amount 透传 i18n 参数；rate% 由模板独立高亮渲染', async () => {
    const { wrapper } = await mountBanner(makePromo())
    const tiers = wrapper.find('[data-test="home-promo-tiers"]')
    expect(tiers.exists()).toBe(true)
    const items = tiers.findAll('li')
    expect(items.length).toBe(2)
    // 金额段走 i18n key `tier_amount_label`，参数 min 透传
    expect(items[0].text()).toContain('home.promo.tier_amount_label')
    expect(items[0].text()).toContain('min=100')
    // rate% 是模板侧的字面文本（formatBonusRate(0.05) → "5"，整数百分比保持纯整数）
    expect(items[0].text()).toContain('+5%')
    expect(items[1].text()).toContain('min=500')
    expect(items[1].text()).toContain('+10%')
  })

  it('hero "最高赠送 +X%" 行：从所有 tier 取最大 bonus_rate 渲染', async () => {
    const { wrapper } = await mountBanner(
      makePromo({
        tiers: [
          { min_amount: 100, bonus_rate: 0.05 },
          { min_amount: 500, bonus_rate: 0.12 }, // 最高档
          { min_amount: 300, bonus_rate: 0.08 },
        ],
      }),
    )
    // hero 行同时包含 prefix/suffix i18n key 占位 + 字面 "+12%"
    const html = wrapper.text()
    expect(html).toContain('home.promo.bonus_headline_prefix')
    expect(html).toContain('home.promo.bonus_headline_suffix')
    expect(html).toContain('+12%')
  })

  it('tiers 全部为 0 / 负值时 hero 行不渲染（避免 "+0%"）', async () => {
    const { wrapper } = await mountBanner(
      makePromo({ tiers: [{ min_amount: 100, bonus_rate: 0 }] }),
    )
    expect(wrapper.text()).not.toContain('home.promo.bonus_headline_prefix')
  })

  it('tiers 为空数组时不渲染 tier 列表（防止空 <ul>）', async () => {
    const { wrapper } = await mountBanner(makePromo({ tiers: [] }))
    expect(wrapper.find('[data-test="home-promo-tiers"]').exists()).toBe(false)
  })

  it('valid_until 缺失时整段 expires_at 不显示', async () => {
    const { wrapper } = await mountBanner(makePromo({ valid_until: undefined }))
    expect(wrapper.find('[data-test="home-promo-expires"]').exists()).toBe(false)
  })

  it('valid_until 存在时显示本地化日期', async () => {
    const { wrapper } = await mountBanner(makePromo())
    const exp = wrapper.find('[data-test="home-promo-expires"]')
    expect(exp.exists()).toBe(true)
    expect(exp.text()).toContain('home.promo.expires_at')
    // 我们不强行断言具体本地化日期串（依赖 runtime locale），只确认 date= 参数被填充。
    expect(exp.text()).toMatch(/date=.+/)
  })

  it('CTA 文案使用 home.promo.cta_recharge', async () => {
    const { wrapper } = await mountBanner(makePromo())
    const cta = wrapper.find('[data-test="home-promo-cta"]')
    expect(cta.text()).toContain('home.promo.cta_recharge')
  })

  it('匿名访客点击 CTA → /login?redirect=/purchase', async () => {
    const { wrapper, router } = await mountBanner(makePromo(), false)
    const spy = vi.spyOn(router, 'push')
    await wrapper.find('[data-test="home-promo-cta"]').trigger('click')
    // gotoOrLogin 在未登录时把目标 path 编码进 redirect query
    expect(spy).toHaveBeenCalledWith({
      path: '/login',
      query: { redirect: '/purchase' },
    })
  })

  it('已登录访客点击 CTA → /purchase（无 redirect 跳转）', async () => {
    const { wrapper, router } = await mountBanner(makePromo(), true)
    const spy = vi.spyOn(router, 'push')
    await wrapper.find('[data-test="home-promo-cta"]').trigger('click')
    expect(spy).toHaveBeenCalledWith({ path: '/purchase' })
  })
})
