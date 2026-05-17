import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import HomeView from '@/views/HomeView.vue'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const homeViewSource = readFileSync(
  resolve(dirname(fileURLToPath(import.meta.url)), '../HomeView.vue'),
  'utf8',
)

const authState = vi.hoisted(() => ({
  isAuthenticated: false,
  isAdmin: false,
  user: null as null | { email: string },
  checkAuth: vi.fn(),
}))

const appState = vi.hoisted(() => ({
  cachedPublicSettings: {
    site_name: 'DevRouter',
    site_logo: '',
    site_subtitle: 'AI API Gateway Platform',
    doc_url: 'https://docs.example.com',
    home_content: '',
  },
  siteName: 'DevRouter',
  siteLogo: '',
  docUrl: 'https://docs.example.com',
  publicSettingsLoaded: true,
  fetchPublicSettings: vi.fn(),
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => authState,
  useAppStore: () => appState,
}))

vi.mock('@/i18n', () => ({
  i18n: {
    global: {
      locale: { value: 'zh' },
      t: (key: string) => key,
      setLocaleMessage: vi.fn(),
    },
  },
}))

vi.mock('vue-i18n', () => ({
  createI18n: () => ({
    global: {
      locale: { value: 'zh' },
      t: (key: string) => key,
      setLocaleMessage: vi.fn(),
    },
  }),
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

const storage = new Map<string, string>()
Object.defineProperty(window, 'localStorage', {
  value: {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => storage.set(key, value),
    removeItem: (key: string) => storage.delete(key),
    clear: () => storage.clear(),
  },
  configurable: true,
})

Object.defineProperty(window, 'matchMedia', {
  value: vi.fn().mockImplementation((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
  configurable: true,
})

describe('HomeView', () => {
  beforeEach(() => {
    authState.isAuthenticated = false
    authState.isAdmin = false
    authState.user = null
    authState.checkAuth.mockReset()
    appState.publicSettingsLoaded = true
    appState.fetchPublicSettings.mockReset()
    appState.cachedPublicSettings = {
      site_name: 'DevRouter',
      site_logo: '',
      site_subtitle: 'AI API Gateway Platform',
      doc_url: 'https://docs.example.com',
      home_content: '',
    }
    storage.clear()
    document.documentElement.classList.remove('dark')
  })

  it('does not include a dark-mode toggle in the landing page source', () => {
    expect(homeViewSource).not.toContain('@click="toggleTheme"')
    expect(homeViewSource).not.toContain('useThemeTransition')
    expect(homeViewSource).not.toContain('home.switchToDark')
    expect(homeViewSource).not.toContain('home.switchToLight')
  })

  it('renders the mist-style centered landing page', async () => {
    const wrapper = mount(HomeView, {
      global: {
        stubs: {
          RouterLink: {
            props: ['to'],
            template: '<a :href="typeof to === `string` ? to : to.path"><slot /></a>',
          },
          LocaleSwitcher: { template: '<button>Locale</button>' },
          Icon: { template: '<span />' },
        },
      },
    })
    await flushPromises()

    expect(wrapper.find('.home-glass-nav').exists()).toBe(true)
    expect(wrapper.find('.home-glass-nav nav').classes()).toEqual(expect.arrayContaining(['max-w-6xl', 'gap-8']))
    expect(wrapper.find('.home-brand').exists()).toBe(true)
    expect(wrapper.find('.home-brand-mark').text()).toBe('DR')
    expect(wrapper.find('.home-brand-name').classes()).toEqual(expect.arrayContaining(['font-extrabold', 'font-[Inter,Geist,system-ui,sans-serif]']))
    expect(wrapper.find('.home-primary-nav').text()).toContain('首页')
    expect(wrapper.find('.home-primary-nav').text()).toContain('文档')
    expect(wrapper.find('.home-primary-nav').text()).not.toContain('价格')
    expect(wrapper.find('.home-primary-nav').text()).toContain('服务状态')
    expect(wrapper.find('.home-primary-nav a').classes()).toEqual(expect.arrayContaining(['rounded-md', 'px-2.5', 'py-1.5', 'hover:bg-slate-100/70']))
    const navLinks = wrapper.findAll('.home-primary-nav a')
    expect(navLinks.map((link) => link.text())).toEqual(['首页', '文档', '服务状态'])
    expect(navLinks[0].attributes('href')).toBe('/home')
    expect(navLinks[0].attributes('target')).toBeUndefined()
    expect(navLinks[1].attributes('target')).toBe('_blank')
    expect(navLinks[1].attributes('rel')).toContain('noopener')
    expect(navLinks[2].attributes('href')).toBe('/monitor')
    expect(navLinks[2].attributes('target')).toBe('_blank')
    expect(navLinks[2].attributes('rel')).toContain('noopener')
    expect(wrapper.find('.home-nav-divider').exists()).toBe(true)
    expect(wrapper.find('.home-nav-actions').classes()).toEqual(expect.arrayContaining(['gap-2.5']))
    expect(wrapper.find('.home-theme-toggle').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('home.switchToDark')
    expect(wrapper.text()).not.toContain('home.switchToLight')
    expect(wrapper.find('.home-hero').classes()).toContain('text-center')
    expect(wrapper.find('h1').text()).toBe('LLM 的统一接口')
    expect(wrapper.find('h1').classes()).toEqual(expect.arrayContaining(['whitespace-nowrap', 'font-black', 'font-[Inter,Geist,system-ui,sans-serif]']))
    expect(wrapper.find('h1').classes()).toContain('tracking-[-0.04em]')
    expect(wrapper.text()).toContain('让每个人都用得起顶尖大模型')
    expect(wrapper.find('.home-hero-subtitle').classes()).toEqual(expect.arrayContaining(['bg-gradient-to-r', 'from-violet-700', 'to-cyan-600', 'bg-clip-text', 'text-transparent']))
    expect(wrapper.find('.home-hero-subtitle').classes().join(' ')).not.toContain('bg-green')
    expect(wrapper.find('.home-stage').exists()).toBe(true)
    expect(wrapper.find('.home-stage').classes()).toEqual(expect.arrayContaining(['mt-12', 'max-w-3xl', 'p-1.5', 'backdrop-blur-2xl', 'shadow-[0_18px_54px_rgba(15,23,42,0.08)]']))
    expect(wrapper.find('.home-shell').exists()).toBe(true)
    expect(wrapper.find('.home-shell').classes()).toEqual(expect.arrayContaining(['rounded-xl', 'border', 'border-white/10']))
    expect(wrapper.find('.home-shell').classes().join(' ')).toContain('rgba(139,92,246,0.22)')
    expect(wrapper.find('.home-shell').classes().join(' ')).toContain('rgba(0,0,0,0.42)')
    expect(wrapper.findAll('.home-shell-dot').every((dot) => dot.classes().includes('opacity-80'))).toBe(true)
    expect(wrapper.find('.home-shell pre').classes()).toEqual(expect.arrayContaining(['px-3.5', 'py-3', 'text-[11px]', 'leading-5', 'whitespace-normal']))
    expect(wrapper.findAll('.home-shell-line')).toHaveLength(16)
    expect(wrapper.find('.home-shell-cursor').exists()).toBe(true)
    expect(wrapper.find('.home-shell-status').classes()).toContain('home-shell-line')
    expect(wrapper.find('.home-shell code').text()).not.toMatch(/\n\s*\n/)
    expect(wrapper.find('.home-shell').text()).toContain('curl http://localhost:3000/v1/chat/completions')
    expect(wrapper.find('.home-shell').text()).not.toContain('https://api.devrouter.ai')
    expect(wrapper.find('.home-shell').text()).toContain('"model": "gpt-4.1"')
    expect(wrapper.find('.home-shell').text()).toContain('"content": "Hello from DevRouter."')
    expect(wrapper.find('.home-shell').text()).toContain('[DevRouter] Request routed to: openai:gpt-4.1')
    expect(wrapper.find('.home-shell').text()).toContain('"id": "chatcmpl-123"')
    expect(wrapper.find('.home-shell-footer').exists()).toBe(false)
    expect(wrapper.find('.home-shell').text()).not.toContain('OpenAI-compatible')
    expect(wrapper.find('.home-shell').text()).not.toContain('pay-as-you-go')
    expect(wrapper.find('.home-shell').text()).not.toContain('not required')
    expect(wrapper.find('.home-chart').exists()).toBe(false)
    expect(wrapper.find('.home-secondary-cta').classes()).toEqual(expect.arrayContaining(['border', 'border-slate-200/80', 'hover:bg-white/70']))
    expect(wrapper.findAll('.home-feature-item')).toHaveLength(4)
    expect(wrapper.find('.home-feature-icon-glow').exists()).toBe(true)
    expect(wrapper.find('.home-feature-icon').classes()).not.toContain('bg-cyan-500/[0.06]')
    expect(wrapper.find('.home-feature-item p').classes()).toContain('text-slate-600')
    expect(wrapper.find('.home-provider-wall').text()).toContain('OpenAI')
    expect(wrapper.findAll('.home-provider-logo')).toHaveLength(4)
    expect(wrapper.find('.home-footer-cta').classes()).toContain('home-cta-grid')
    expect(wrapper.find('.home-footer-cta').classes()).toContain('[mask-image:radial-gradient(ellipse_at_center,black_40%,transparent_72%)]')
    expect(wrapper.find('.home-mist').classes()).toContain('home-mist-breathe')
    expect(wrapper.find('.home-sla-badge').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('99.99% API 连通率')
    expect(wrapper.text()).not.toContain('GitHub')
    expect(wrapper.find('.home-footer-cta').text()).toContain('准备好构建你的 AI 应用了吗？')
  })

  it('keeps configured custom home content mode', async () => {
    appState.cachedPublicSettings = {
      ...appState.cachedPublicSettings,
      home_content: '<main class="custom-home">Custom</main>',
    }

    const wrapper = mount(HomeView, {
      global: {
        stubs: {
          RouterLink: true,
          LocaleSwitcher: true,
          Icon: true,
        },
      },
    })
    await flushPromises()

    expect(wrapper.find('.custom-home').exists()).toBe(true)
    expect(wrapper.text()).toContain('Custom')
    expect(wrapper.text()).not.toContain('One API, All Intelligence.')
  })
})
