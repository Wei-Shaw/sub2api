import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import WechatOAuthSection from '@/components/auth/WechatOAuthSection.vue'
import { useAppStore } from '@/stores'
import type { PublicSettings } from '@/types'

const routeState = vi.hoisted(() => ({
  query: {} as Record<string, unknown>,
}))

const locationState = vi.hoisted(() => ({
  current: { href: 'http://localhost/login' } as { href: string },
}))

let pinia: ReturnType<typeof createPinia>

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      locale: { value: 'en' },
      t: (key: string, params?: Record<string, string>) => {
        if (key === 'auth.wechatProviderName') {
          return 'Mock WeChat'
        }
        if (key === 'auth.oidc.signIn') {
          return `Continue with ${params?.providerName ?? ''}`.trim()
        }
        if (key === 'auth.oauthFlow.wechatSystemBrowserOnly') {
          return 'MOCK-SYSTEM-BROWSER-ONLY'
        }
        if (key === 'auth.oauthFlow.wechatBrowserOnly') {
          return 'MOCK-WECHAT-BROWSER-ONLY'
        }
        if (key === 'auth.oauthFlow.wechatNotConfigured') {
          return 'MOCK-NOT-CONFIGURED'
        }
        if (key === 'auth.oauthOrContinue') {
          return 'or continue'
        }
        return key
      },
    }),
  }
})

type WeChatPublicSettings = PublicSettings & {
  wechat_oauth_open_enabled?: boolean
  wechat_oauth_mp_enabled?: boolean
}

function buildPublicSettings(overrides: Partial<WeChatPublicSettings> = {}): WeChatPublicSettings {
  return {
    registration_enabled: true,
    email_verify_enabled: false,
    force_email_on_third_party_signup: false,
    registration_email_suffix_whitelist: [],
    promo_code_enabled: true,
    password_reset_enabled: false,
    invitation_code_enabled: false,
    turnstile_enabled: false,
    turnstile_site_key: '',
    site_name: 'Sub2API',
    site_logo: '',
    site_subtitle: '',
    api_base_url: '/api/v1',
    contact_info: '',
    doc_url: '',
    home_content: '',
    hide_ccs_import_button: false,
    payment_enabled: false,
    table_default_page_size: 20,
    table_page_size_options: [10, 20, 50, 100],
    custom_menu_items: [],
    custom_endpoints: [],
    linuxdo_oauth_enabled: false,
    wechat_oauth_enabled: true,
    oidc_oauth_enabled: false,
    oidc_oauth_provider_name: 'OIDC',
    backend_mode_enabled: false,
    version: 'test',
    balance_low_notify_enabled: false,
    account_quota_notify_enabled: false,
    balance_low_notify_threshold: 0,
    ...overrides,
  }
}

function seedPublicSettings(overrides: Partial<WeChatPublicSettings> = {}): void {
  const appStore = useAppStore()
  const settings = buildPublicSettings(overrides)
  appStore.cachedPublicSettings = settings
  appStore.publicSettingsLoaded = true
}

describe('WechatOAuthSection', () => {
  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    routeState.query = { redirect: '/billing?plan=pro' }
    locationState.current = { href: 'http://localhost/login' }
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: locationState.current,
    })
    Object.defineProperty(window.navigator, 'userAgent', {
      configurable: true,
      value: 'Mozilla/5.0',
    })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('starts the open WeChat OAuth flow with the current redirect target when open mode is configured', async () => {
    seedPublicSettings({
      wechat_oauth_open_enabled: true,
      wechat_oauth_mp_enabled: false,
    })
    const wrapper = mount(WechatOAuthSection, {
      global: {
        plugins: [pinia],
      },
    })

    expect(wrapper.text()).toContain('Mock WeChat')

    await wrapper.get('button').trigger('click')

    expect(locationState.current.href).toContain(
      '/api/v1/auth/oauth/wechat/start?mode=open&redirect=%2Fbilling%3Fplan%3Dpro'
    )
  })

  it('uses mp mode inside the WeChat browser when mp mode is configured', async () => {
    Object.defineProperty(window.navigator, 'userAgent', {
      configurable: true,
      value: 'Mozilla/5.0 MicroMessenger',
    })
    seedPublicSettings({
      wechat_oauth_open_enabled: false,
      wechat_oauth_mp_enabled: true,
    })
    const wrapper = mount(WechatOAuthSection, {
      global: {
        plugins: [pinia],
      },
    })

    await wrapper.get('button').trigger('click')

    expect(locationState.current.href).toContain(
      '/api/v1/auth/oauth/wechat/start?mode=mp&redirect=%2Fbilling%3Fplan%3Dpro'
    )
  })

  it('disables the button outside the WeChat browser when only mp mode is configured', async () => {
    seedPublicSettings({
      wechat_oauth_open_enabled: false,
      wechat_oauth_mp_enabled: true,
    })
    const wrapper = mount(WechatOAuthSection, {
      global: {
        plugins: [pinia],
      },
    })

    expect(wrapper.get('button').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('MOCK-WECHAT-BROWSER-ONLY')

    await wrapper.get('button').trigger('click')

    expect(locationState.current.href).toBe('http://localhost/login')
  })

  it('disables the button inside the WeChat browser when only open mode is configured', async () => {
    Object.defineProperty(window.navigator, 'userAgent', {
      configurable: true,
      value: 'Mozilla/5.0 MicroMessenger',
    })
    seedPublicSettings({
      wechat_oauth_open_enabled: true,
      wechat_oauth_mp_enabled: false,
    })
    const wrapper = mount(WechatOAuthSection, {
      global: {
        plugins: [pinia],
      },
    })

    expect(wrapper.get('button').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('MOCK-SYSTEM-BROWSER-ONLY')

    await wrapper.get('button').trigger('click')

    expect(locationState.current.href).toBe('http://localhost/login')
  })

  it('uses the legacy overall enabled flag when per-mode settings are not present', async () => {
    seedPublicSettings({
      wechat_oauth_enabled: true,
    })
    const wrapper = mount(WechatOAuthSection, {
      global: {
        plugins: [pinia],
      },
    })

    await wrapper.get('button').trigger('click')

    expect(locationState.current.href).toContain(
      '/api/v1/auth/oauth/wechat/start?mode=open&redirect=%2Fbilling%3Fplan%3Dpro'
    )
  })

  it('shows the localized not-configured hint when WeChat OAuth is unavailable', async () => {
    seedPublicSettings({
      wechat_oauth_enabled: false,
      wechat_oauth_open_enabled: false,
      wechat_oauth_mp_enabled: false,
    })

    const wrapper = mount(WechatOAuthSection, {
      global: {
        plugins: [pinia],
      },
    })

    expect(wrapper.text()).toContain('MOCK-NOT-CONFIGURED')
  })
})
