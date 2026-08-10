import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'

import HomeView from '../HomeView.vue'

const { appStore, authStore } = vi.hoisted(() => ({
  appStore: {
    cachedPublicSettings: {} as Record<string, unknown>,
    siteName: 'Fallback site',
    siteLogo: '',
    docUrl: '',
    publicSettingsLoaded: true,
    fetchPublicSettings: vi.fn(),
  },
  authStore: {
    isAuthenticated: false,
    isAdmin: false,
    user: null as { email?: string } | null,
    checkAuth: vi.fn(),
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => appStore,
  useAuthStore: () => authStore,
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

function mountHome(settings: Record<string, unknown> = {}) {
  appStore.cachedPublicSettings = {
    site_name: 'Test site',
    site_subtitle: 'Test subtitle',
    ...settings,
  }

  return mount(HomeView, {
    global: {
      stubs: {
        RouterLink: RouterLinkStub,
        LocaleSwitcher: { template: '<div data-testid="locale-switcher" />' },
        Icon: { template: '<span data-testid="icon" />' },
      },
    },
  })
}

function compactDestination(wrapper: ReturnType<typeof mountHome>) {
  return wrapper.get('[data-testid="compact-home"]').findComponent(RouterLinkStub).props('to')
}

const styleTestIds = {
  classic: 'classic-home',
  compact: 'compact-home',
  editorial: 'editorial-home',
  operations: 'operations-home',
  minimal: 'minimal-home',
  catalog: 'catalog-home',
} as const

describe('HomeView style resolver', () => {
  beforeEach(() => {
    authStore.isAuthenticated = false
    authStore.isAdmin = false
    authStore.user = null
    authStore.checkAuth.mockClear()
    appStore.fetchPublicSettings.mockClear()
    localStorage.clear()
    vi.spyOn(window, 'matchMedia').mockReturnValue({ matches: false } as MediaQueryList)
  })

  it('renders custom HTML ahead of compact mode', () => {
    const wrapper = mountHome({
      compact_home_enabled: true,
      home_content: '<section id="custom-home">Custom home</section>',
    })

    expect(wrapper.get('#custom-home').text()).toBe('Custom home')
    expect(wrapper.find('[data-testid="compact-home"]').exists()).toBe(false)
  })

  it('renders custom URL content ahead of compact mode', () => {
    const wrapper = mountHome({
      compact_home_enabled: true,
      home_content: ' https://example.com/home ',
    })

    expect(wrapper.get('iframe').attributes('src')).toBe('https://example.com/home')
    expect(wrapper.find('[data-testid="compact-home"]').exists()).toBe(false)
  })

  it('treats whitespace-only custom content as empty and selects compact mode', () => {
    const wrapper = mountHome({ compact_home_enabled: true, home_content: ' \n\t ' })

    expect(wrapper.get('[data-testid="compact-home"]').text()).toContain('Test site')
  })

  it.each(Object.entries(styleTestIds))('renders the explicit %s style', (style, testId) => {
    const wrapper = mountHome({ home_style: style, compact_home_enabled: style !== 'classic' })

    expect(wrapper.get(`[data-testid="${testId}"]`).exists()).toBe(true)
  })

  it('labels operations as a routing demo without simulated live health claims', () => {
    const wrapper = mountHome({ home_style: 'operations' })
    const operations = wrapper.get('[data-testid="operations-home"]')

    expect(operations.text()).toContain('home.styles.operations.demoNotice')
    expect(operations.text()).toContain('home.styles.operations.disclaimer')
    expect(operations.text()).not.toContain('99.99%')
    expect(operations.text()).not.toContain('ALL SYSTEMS NOMINAL')
  })

  it('gives shared header icon actions accessible names', () => {
    const wrapper = mountHome({ home_style: 'editorial', doc_url: 'https://docs.example.com' })
    const editorial = wrapper.get('[data-testid="editorial-home"]')

    expect(editorial.get('a[aria-label="home.viewDocs"]').attributes('href')).toBe('https://docs.example.com/')
    expect(editorial.get('button[aria-label="home.switchToDark"]').attributes('type')).toBe('button')
  })

  it('keeps the catalog to four static platform cards', () => {
    const wrapper = mountHome({ home_style: 'catalog' })
    const catalog = wrapper.get('[data-testid="catalog-home"]')

    expect(catalog.findAll('article')).toHaveLength(4)
    expect(catalog.text()).toContain('home.styles.catalog.staticNote')
  })

  it('lets an explicit classic style override the legacy compact flag', () => {
    const wrapper = mountHome({ home_style: 'classic', compact_home_enabled: true })

    expect(wrapper.get('[data-testid="classic-home"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="compact-home"]').exists()).toBe(false)
  })

  it.each(['unknown', 42, false])('falls back to classic for invalid style %s', (homeStyle) => {
    const wrapper = mountHome({ home_style: homeStyle, compact_home_enabled: true })

    expect(wrapper.get('[data-testid="classic-home"]').exists()).toBe(true)
  })

  it.each([undefined, false])('selects the default home when compact mode is %s', (enabled) => {
    const settings = enabled === undefined ? {} : { compact_home_enabled: enabled }
    const wrapper = mountHome(settings)

    expect(wrapper.find('[data-testid="compact-home"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="classic-home"]').exists()).toBe(true)
    expect(wrapper.find('.terminal-container').exists()).toBe(true)
  })

  it('omits the GitHub link from the default home footer', () => {
    const wrapper = mountHome({ doc_url: 'https://docs.example.com' })
    const links = wrapper.findAll('a')

    expect(links.some((link) => link.attributes('href')?.includes('github.com'))).toBe(false)
    expect(links.some((link) => link.attributes('href') === 'https://docs.example.com/')).toBe(true)
  })

  it('links unauthenticated visitors to login', () => {
    expect(compactDestination(mountHome({ compact_home_enabled: true }))).toBe('/login')
  })

  it('links authenticated users to their dashboard', () => {
    authStore.isAuthenticated = true

    expect(compactDestination(mountHome({ compact_home_enabled: true }))).toBe('/dashboard')
  })

  it('links administrators to the admin dashboard', () => {
    authStore.isAuthenticated = true
    authStore.isAdmin = true

    const wrapper = mountHome({ compact_home_enabled: true })
    expect(compactDestination(wrapper)).toBe('/admin/dashboard')
    expect(authStore.checkAuth).toHaveBeenCalledOnce()
    expect(appStore.fetchPublicSettings).not.toHaveBeenCalled()
  })
})
