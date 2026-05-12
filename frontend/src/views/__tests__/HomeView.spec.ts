import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import HomeView from '../HomeView.vue'

const authStore = {
  isAuthenticated: false,
  isAdmin: false,
  user: null as { email?: string } | null,
  checkAuth: vi.fn(),
}

const appStore = {
  cachedPublicSettings: {
    site_name: 'MyCustomSite',
    site_logo: '/logo.png',
    site_subtitle: 'Readable public home page',
    doc_url: '',
    seo_home_title: '公开首页',
    home_content: '<h2>欢迎使用</h2><p>公开正文</p>',
  },
  siteName: 'MyCustomSite',
  siteLogo: '/logo.png',
  docUrl: '',
  publicSettingsLoaded: true,
  fetchPublicSettings: vi.fn(),
}

vi.mock('@/stores', () => ({
  useAuthStore: () => authStore,
  useAppStore: () => appStore,
}))

vi.mock('@/components/common/LocaleSwitcher.vue', () => ({
  default: {
    name: 'LocaleSwitcher',
    template: '<div />',
  },
}))

vi.mock('@/components/icons/Icon.vue', () => ({
  default: {
    name: 'Icon',
    template: '<span />',
  },
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => {
        if (key === 'common.login') return '登录'
        return key
      },
    }),
  }
})

describe('HomeView', () => {
  beforeEach(() => {
    authStore.isAuthenticated = false
    authStore.isAdmin = false
    authStore.user = null
    authStore.checkAuth.mockReset()
    appStore.fetchPublicSettings.mockReset()
    appStore.publicSettingsLoaded = true
    appStore.cachedPublicSettings = {
      site_name: 'MyCustomSite',
      site_logo: '/logo.png',
      site_subtitle: 'Readable public home page',
      doc_url: '',
      seo_home_title: '公开首页',
      home_content: '<h2>欢迎使用</h2><p>公开正文</p>',
    }
    vi.stubGlobal('matchMedia', vi.fn().mockImplementation(() => ({
      matches: false,
      media: '',
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })))
  })

  it('renders custom home content inside the public content shell', () => {
    const wrapper = mount(HomeView, {
      global: {
        stubs: {
          RouterLink: {
            props: ['to'],
            template: '<a :href="to"><slot /></a>',
          },
        },
      },
    })

    expect(wrapper.html()).toContain('MyCustomSite')
    expect(wrapper.html()).toContain('公开首页')
    expect(wrapper.html()).toContain('<h2>欢迎使用</h2>')
    expect(wrapper.html()).toContain('<p>公开正文</p>')
    expect(wrapper.find('.public-home-content').exists()).toBe(true)
    expect(wrapper.find('header').exists()).toBe(true)
  })

  it('falls back to the default home page when sanitized home content is empty', () => {
    appStore.cachedPublicSettings = {
      site_name: 'MyCustomSite',
      site_logo: '/logo.png',
      site_subtitle: 'Readable public home page',
      doc_url: '',
      seo_home_title: '公开首页',
      home_content: '<script>alert(1)</script><iframe src=\"https://evil.example\"></iframe>',
    }

    const wrapper = mount(HomeView, {
      global: {
        stubs: {
          RouterLink: {
            props: ['to'],
            template: '<a :href="to"><slot /></a>',
          },
        },
      },
    })

    expect(wrapper.find('.public-home-content').exists()).toBe(false)
    expect(wrapper.text()).toContain('Readable public home page')
  })
})
