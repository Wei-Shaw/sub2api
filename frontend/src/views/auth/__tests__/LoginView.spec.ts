import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import LoginView from '../LoginView.vue'

const mocks = vi.hoisted(() => ({
  route: {
    query: {}
  },
  push: vi.fn(),
  login: vi.fn(),
  redeemLogin: vi.fn(),
  inviteLogin: vi.fn(),
  login2FA: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
  showWarning: vi.fn(),
  getPublicSettings: vi.fn(),
  clearAllAffiliateReferralCodes: vi.fn()
}))

vi.mock('vue-router', () => ({
  useRoute: () => mocks.route,
  useRouter: () => ({ push: mocks.push }),
  RouterLink: {
    name: 'RouterLink',
    props: ['to'],
    template: '<a><slot /></a>'
  }
}))

vi.mock('vue-i18n', () => ({
  createI18n: () => ({
    global: {
      locale: { value: 'en' },
      t: (key: string) => key
    }
  }),
  useI18n: () => ({
    t: (key: string) => key
  })
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => ({
    login: (...args: unknown[]) => mocks.login(...args),
    redeemLogin: (...args: unknown[]) => mocks.redeemLogin(...args),
    inviteLogin: (...args: unknown[]) => mocks.inviteLogin(...args),
    login2FA: (...args: unknown[]) => mocks.login2FA(...args)
  }),
  useAppStore: () => ({
    showSuccess: (...args: unknown[]) => mocks.showSuccess(...args),
    showError: (...args: unknown[]) => mocks.showError(...args),
    showWarning: (...args: unknown[]) => mocks.showWarning(...args)
  })
}))

vi.mock('@/api/auth', () => ({
  getPublicSettings: (...args: unknown[]) => mocks.getPublicSettings(...args),
  isTotp2FARequired: (response: { requires_2fa?: boolean }) => response?.requires_2fa === true,
  isWeChatWebOAuthEnabled: () => false
}))

vi.mock('@/utils/oauthAffiliate', () => ({
  clearAllAffiliateReferralCodes: () => mocks.clearAllAffiliateReferralCodes()
}))

const stubs = {
  AuthLayout: {
    template: '<main><slot /><footer><slot name="footer" /></footer></main>'
  },
  LinuxDoOAuthSection: true,
  OidcOAuthSection: true,
  WechatOAuthSection: true,
  TotpLoginModal: true,
  TurnstileWidget: {
    template: '<div />',
    methods: {
      reset: vi.fn()
    }
  },
  Icon: {
    props: ['name'],
    template: '<span />'
  },
  'router-link': {
    props: ['to'],
    template: '<a><slot /></a>'
  }
}

describe('LoginView redeem-code login', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    sessionStorage.clear()
    mocks.route.query = {}
    mocks.getPublicSettings.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      linuxdo_oauth_enabled: false,
      wechat_oauth_enabled: false,
      wechat_oauth_open_enabled: false,
      wechat_oauth_mp_enabled: false,
      oidc_oauth_enabled: false,
      oidc_oauth_provider_name: 'OIDC',
      backend_mode_enabled: false,
      password_reset_enabled: false
    })
    mocks.login.mockResolvedValue({
      access_token: 'access-token',
      token_type: 'Bearer',
      user: { id: 1 }
    })
    mocks.redeemLogin.mockResolvedValue({ id: 1 })
    mocks.inviteLogin.mockResolvedValue({ id: 1 })
  })

  it('renders the code-login mode and submits DLG codes through web invite-login wiring', async () => {
    const wrapper = mount(LoginView, {
      global: { stubs }
    })
    await flushPromises()

    expect(wrapper.text()).toContain('auth.loginWithRedeemCode')

    await wrapper.findAll('button').find((button) => button.text() === 'auth.loginWithRedeemCode')?.trigger('click')
    await wrapper.find('#invitation-code').setValue('dlg-7tty-sq2q-47te')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(mocks.redeemLogin).not.toHaveBeenCalled()
    expect(mocks.inviteLogin).toHaveBeenCalledWith({
      invitation_code: 'DLG-7TTY-SQ2Q-47TE',
      client_kind: 'web',
      turnstile_token: undefined
    })
    expect(mocks.clearAllAffiliateReferralCodes).toHaveBeenCalled()
    expect(mocks.showSuccess).toHaveBeenCalledWith('auth.redeemLoginSuccess')
    expect(mocks.push).toHaveBeenCalledWith({
      path: '/profile',
      query: { inviteBootstrap: '1' }
    })
  })
})
