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
import { useAppStore } from '@/stores'

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

  it('点击其他产品会展开已配置的首页产品菜单', async () => {
    const router = makeRouter()
    router.push('/')
    await router.isReady()

    const pinia = createPinia()
    setActivePinia(pinia)
    const appStore = useAppStore()
    appStore.cachedPublicSettings = {
      site_name: 'Sub2API',
      site_logo: '',
      site_subtitle: '',
      contact_info: '',
      doc_url: '',
      home_content: '',
      home_product_menu_items: [
        {
          id: 'one',
          label: 'First Product',
          icon_svg: '',
          url: 'https://example.com/one',
          action: 'same_tab',
          visibility: 'user',
          sort_order: 0,
        },
        {
          id: 'two',
          label: 'Second Product',
          icon_svg: '',
          url: 'https://example.com/two',
          action: 'new_tab',
          visibility: 'user',
          sort_order: 1,
        },
      ],
    } as any
    appStore.publicSettingsLoaded = true

    const wrapper = mount(HomeView, {
      attachTo: document.body,
      global: {
        plugins: [pinia, router],
        stubs: {
          HomeShowcaseSection: { template: '<section data-test="showcase-stub" />' },
          LocaleSwitcher: { template: '<div />' },
          Icon: { template: '<i />' },
        },
      },
    })
    await flushPromises()

    await wrapper.get('button[aria-haspopup="menu"]').trigger('click')
    await flushPromises()

    expect(document.body.textContent).toContain('First Product')
    expect(document.body.textContent).toContain('Second Product')
    expect(document.body.innerHTML).toContain('https://example.com/one')
    expect(document.body.innerHTML).toContain('https://example.com/two')

    wrapper.unmount()
  })

  it('鼠标 hover 其他产品会展开已配置的首页产品菜单', async () => {
    const router = makeRouter()
    router.push('/')
    await router.isReady()

    const pinia = createPinia()
    setActivePinia(pinia)
    const appStore = useAppStore()
    appStore.cachedPublicSettings = {
      site_name: 'Sub2API',
      site_logo: '',
      site_subtitle: '',
      contact_info: '',
      doc_url: '',
      home_content: '',
      home_product_menu_items: [
        {
          id: 'hover-one',
          label: 'Hover Product',
          icon_svg: '',
          url: 'https://example.com/hover',
          action: 'same_tab',
          visibility: 'user',
          sort_order: 0,
        },
      ],
    } as any
    appStore.publicSettingsLoaded = true

    const wrapper = mount(HomeView, {
      attachTo: document.body,
      global: {
        plugins: [pinia, router],
        stubs: {
          HomeShowcaseSection: { template: '<section data-test="showcase-stub" />' },
          LocaleSwitcher: { template: '<div />' },
          Icon: { template: '<i />' },
        },
      },
    })
    await flushPromises()

    await wrapper.get('[data-test="home-products-menu"]').trigger('mouseenter')
    await flushPromises()

    expect(document.body.textContent).toContain('Hover Product')
    expect(document.body.innerHTML).toContain('https://example.com/hover')

    wrapper.unmount()
  })

  it('开始使用区域下方也展示其他产品按钮并可展开菜单', async () => {
    const router = makeRouter()
    router.push('/')
    await router.isReady()

    const pinia = createPinia()
    setActivePinia(pinia)
    const appStore = useAppStore()
    appStore.cachedPublicSettings = {
      site_name: 'Sub2API',
      site_logo: '',
      site_subtitle: '',
      contact_info: '',
      doc_url: '',
      home_content: '',
      home_product_menu_items: [
        {
          id: 'hero-product',
          label: 'Hero Product',
          icon_svg: '',
          url: 'https://example.com/hero',
          action: 'same_tab',
          visibility: 'user',
          sort_order: 0,
        },
      ],
    } as any
    appStore.publicSettingsLoaded = true

    const wrapper = mount(HomeView, {
      attachTo: document.body,
      global: {
        plugins: [pinia, router],
        stubs: {
          HomeShowcaseSection: { template: '<section data-test="showcase-stub" />' },
          LocaleSwitcher: { template: '<div />' },
          Icon: { template: '<i />' },
        },
      },
    })
    await flushPromises()

    const heroButton = wrapper.get('[data-test="home-products-hero-button"]')
    expect(heroButton.text()).toContain('home.products')

    await heroButton.trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="home-products-hero-button"] i:last-child').classes()).toContain(
      'rotate-180',
    )
    expect(wrapper.get('[data-test="home-products-menu"] button i:last-child').classes()).not.toContain(
      'rotate-180',
    )
    expect(document.body.textContent).toContain('Hero Product')
    expect(document.body.innerHTML).toContain('https://example.com/hero')

    wrapper.unmount()
  })
})
