import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LoginView from '@/views/auth/LoginView.vue'

const getPublicSettingsMock = vi.fn()
const locationState = { href: 'http://localhost/login' }
const routeQuery: Record<string, string> = {}

vi.mock('vue-router', () => ({
  useRouter: () => ({
    currentRoute: { value: { query: routeQuery } },
    push: vi.fn()
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

vi.mock('@/stores', () => ({
  useAuthStore: () => ({ login: vi.fn(), loginWithPasskey: vi.fn() }),
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn(), showWarning: vi.fn() })
}))

vi.mock('@/api/auth', async () => {
  const actual = await vi.importActual<typeof import('@/api/auth')>('@/api/auth')
  return {
    ...actual,
    getPublicSettings: (...args: unknown[]) => getPublicSettingsMock(...args),
    isTotp2FARequired: () => false,
    isWeChatWebOAuthEnabled: () => false
  }
})

function mountLogin() {
  return mount(LoginView, {
    global: {
      stubs: {
        AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
        RouterLink: true,
        TurnstileWidget: true,
        Icon: true,
        LoginAgreementPrompt: true,
        TotpLoginModal: true,
        EmailOAuthButtons: true,
        LinuxDoOAuthSection: true,
        DingTalkOAuthSection: true,
        OidcOAuthSection: defineComponent({
          setup() {
            return () => h('button', { 'data-testid': 'oidc-login' }, 'OIDC')
          }
        }),
        WechatOAuthSection: true
      }
    }
  })
}

describe('OIDC exclusive login', () => {
  beforeEach(() => {
    getPublicSettingsMock.mockReset()
    Object.keys(routeQuery).forEach((key) => delete routeQuery[key])
    locationState.href = 'http://localhost/login'
    Object.defineProperty(window, 'location', { configurable: true, value: locationState })
    getPublicSettingsMock.mockResolvedValue({
      oidc_oauth_enabled: true,
      oidc_oauth_exclusive: true,
      oidc_oauth_provider_name: 'Company SSO',
      backend_mode_enabled: false,
      registration_enabled: true,
      password_reset_enabled: false,
      passkey_enabled: false,
      turnstile_enabled: false,
      tencent_captcha_enabled: false,
      github_oauth_enabled: false,
      google_oauth_enabled: false,
      linuxdo_oauth_enabled: false,
      dingtalk_oauth_enabled: false,
      wechat_oauth_enabled: false
    })
  })

  it('redirects unauthenticated visitors directly to OIDC and hides password login', async () => {
    routeQuery.redirect = '/keys'
    const wrapper = mountLogin()
    await flushPromises()

    expect(locationState.href).toBe('/api/v1/auth/oauth/oidc/start?redirect=%2Fkeys')
    expect(wrapper.find('#email').exists()).toBe(false)
    expect(wrapper.find('#password').exists()).toBe(false)
  })

  it('stays logged out when returning from the identity provider logout', async () => {
    routeQuery.logged_out = '1'
    const wrapper = mountLogin()
    await flushPromises()

    expect(locationState.href).toBe('http://localhost/login')
    expect(wrapper.find('#email').exists()).toBe(false)
    expect(wrapper.find('[data-testid="oidc-login"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('auth.dontHaveAccount')
  })

  it('keeps local login hidden when public settings cannot be loaded', async () => {
    getPublicSettingsMock.mockRejectedValue(new Error('settings unavailable'))
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined)

    try {
      const wrapper = mountLogin()
      await flushPromises()

      expect(wrapper.find('#email').exists()).toBe(false)
      expect(wrapper.text()).toContain('auth.settingsLoadFailed')
      expect(wrapper.find('button').text()).toContain('common.tryAgain')
    } finally {
      consoleError.mockRestore()
    }
  })
})
