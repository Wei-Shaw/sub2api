import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

// 关键 mock：HomeView 会在 onMounted 调 authStore.checkAuth()（纯 localStorage，
// 已被全局 setup 的 in-memory storage 覆盖）和 appStore.fetchPublicSettings()。
// 后者会调 `@/api/auth` 的 getPublicSettings，所以这里把它打成可控的 stub。
vi.mock('@/api/auth', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/auth')>()
  return {
    ...actual,
    getPublicSettings: vi.fn().mockResolvedValue({}),
  }
})

vi.mock('@/api/plaza', () => ({
  plazaAPI: {
    getRechargePromo: vi.fn().mockResolvedValue({ promo: null }),
    listPlans: vi.fn().mockResolvedValue({
      cards: [],
      currency_meta: { balance_recharge_multiplier: 1, model_native: 'USD', plan_native: 'CNY' },
    }),
  },
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

import HomeView from '../HomeView.vue'

function makeRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: HomeView },
      { path: '/login', component: { template: '<div />' } },
      { path: '/dashboard', component: { template: '<div />' } },
      { path: '/admin/dashboard', component: { template: '<div />' } },
      { path: '/plaza/models', component: { template: '<div />' } },
      { path: '/plaza/plans', component: { template: '<div />' } },
    ],
  })
}

beforeEach(() => {
  setActivePinia(createPinia())
  // jsdom 默认不实现 matchMedia；HomeView.initTheme 会无条件调用。
  if (typeof window.matchMedia !== 'function') {
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: vi.fn().mockReturnValue({
        matches: false,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
        media: '',
        onchange: null,
      }),
    })
  }
})

describe('HomeView (smoke)', () => {
  it('不再渲染已下线的 Supported Providers 段（providers i18n key 不出现在 DOM）', async () => {
    const router = makeRouter()
    router.push('/')
    await router.isReady()

    const wrapper = mount(HomeView, {
      global: {
        plugins: [createPinia(), router],
        stubs: {
          // 隔离首页转化区与切换器组件，避免它们的内部异步打扰本 smoke test。
          HomeShowcaseSection: { template: '<section data-test="showcase-stub" />' },
          LocaleSwitcher: { template: '<div />' },
          Icon: { template: '<i />' },
        },
      },
    })
    await flushPromises()

    const html = wrapper.html()
    // 静态 Providers 段已删除；i18n key 也已从 locale 文件移除。
    expect(html).not.toContain('home.providers.title')
    expect(html).not.toContain('home.providers.description')
    // 二级 "View model pricing" CTA 仍保留，链接到公开 plaza 模型价格页。
    expect(html).toContain('home.cta_view_pricing')
    expect(html).toContain('href="/plaza/models"')
    // 新的转化区 stub 必须挂在 DOM 上。
    expect(wrapper.find('[data-test="showcase-stub"]').exists()).toBe(true)
  })
})
