import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LoginView from '@/views/auth/LoginView.vue'

const {
  getPublicSettingsMock,
  showWarningMock,
  showErrorMock,
  showSuccessMock,
  loginMock,
  login2FAMock,
  pushMock,
  currentRoute,
} = vi.hoisted(() => ({
  getPublicSettingsMock: vi.fn(),
  showWarningMock: vi.fn(),
  showErrorMock: vi.fn(),
  showSuccessMock: vi.fn(),
  loginMock: vi.fn(),
  login2FAMock: vi.fn(),
  pushMock: vi.fn(),
  currentRoute: {
    value: {
      query: {},
    },
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showWarning: (...args: any[]) => showWarningMock(...args),
    showError: (...args: any[]) => showErrorMock(...args),
    showSuccess: (...args: any[]) => showSuccessMock(...args),
  }),
  useAuthStore: () => ({
    login: (...args: any[]) => loginMock(...args),
    login2FA: (...args: any[]) => login2FAMock(...args),
  }),
}))

vi.mock('@/components/layout', () => ({
  AuthLayout: { template: '<section class="auth-layout-stub"><slot /><footer><slot name="footer" /></footer></section>' },
}))

vi.mock('@/api/auth', () => ({
  getPublicSettings: (...args: any[]) => getPublicSettingsMock(...args),
  isTotp2FARequired: (response: any) => response?.requires_2fa === true,
  isWeChatWebOAuthEnabled: (settings: any) => settings.wechat_oauth_enabled === true,
}))

vi.mock('@/utils/apiError', () => ({
  extractI18nErrorMessage: () => '登录失败',
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    currentRoute,
    push: (...args: any[]) => pushMock(...args),
  }),
}))

vi.mock('vue-i18n', () => ({
  createI18n: () => ({
    global: {
      t: (key: string) => key,
    },
  }),
  useI18n: () => ({
    t: (key: string) => {
      const messages: Record<string, string> = {
        'auth.welcomeBack': '欢迎回来',
        'auth.signInToAccount': '登录您的账户以继续',
        'auth.emailLabel': '邮箱',
        'auth.emailPlaceholder': 'you@example.com',
        'auth.passwordLabel': '密码',
        'auth.passwordPlaceholder': '请输入密码',
        'auth.signIn': '登录',
        'auth.signingIn': '登录中',
        'auth.oauthOrContinue': '或继续使用',
        'auth.forgotPassword': '忘记密码',
        'auth.dontHaveAccount': '还没有账号？',
        'auth.signUp': '注册',
      }
      return messages[key] ?? key
    },
  }),
}))

const localStorageMemory = new Map<string, string>()
const sessionStorageMemory = new Map<string, string>()

Object.defineProperty(window, 'localStorage', {
  configurable: true,
  value: {
    getItem: (key: string) => localStorageMemory.get(key) ?? null,
    setItem: (key: string, value: string) => localStorageMemory.set(key, value),
    removeItem: (key: string) => localStorageMemory.delete(key),
    clear: () => localStorageMemory.clear(),
  },
})

Object.defineProperty(window, 'sessionStorage', {
  configurable: true,
  value: {
    getItem: (key: string) => sessionStorageMemory.get(key) ?? null,
    setItem: (key: string, value: string) => sessionStorageMemory.set(key, value),
    removeItem: (key: string) => sessionStorageMemory.delete(key),
    clear: () => sessionStorageMemory.clear(),
  },
})

function publicSettings(overrides: Record<string, unknown> = {}) {
  return {
    turnstile_enabled: false,
    turnstile_site_key: '',
    linuxdo_oauth_enabled: false,
    wechat_oauth_enabled: false,
    backend_mode_enabled: false,
    oidc_oauth_enabled: false,
    oidc_oauth_provider_name: 'OIDC',
    github_oauth_enabled: true,
    google_oauth_enabled: true,
    password_reset_enabled: true,
    login_agreement_enabled: false,
    login_agreement_documents: [],
    ...overrides,
  }
}

describe('LoginView', () => {
  beforeEach(() => {
    getPublicSettingsMock.mockReset()
    showWarningMock.mockReset()
    showErrorMock.mockReset()
    showSuccessMock.mockReset()
    loginMock.mockReset()
    login2FAMock.mockReset()
    pushMock.mockReset()
    currentRoute.value = { query: {} }
    sessionStorage.clear()
    localStorage.clear()
    getPublicSettingsMock.mockResolvedValue(publicSettings())
  })

  it('uses the refined DevRouter login presentation without changing auth controls', async () => {
    const wrapper = mount(LoginView, {
      global: {
        stubs: {
          RouterLink: { props: ['to'], template: '<a :href="typeof to === `string` ? to : to.path"><slot /></a>' },
          EmailOAuthButtons: { props: ['githubEnabled', 'googleEnabled'], template: '<div class="email-oauth-stub">Email OAuth</div>' },
          LinuxDoOAuthSection: true,
          WechatOAuthSection: true,
          OidcOAuthSection: true,
          LoginAgreementPrompt: true,
          TotpLoginModal: true,
          TurnstileWidget: true,
          Icon: { props: ['name', 'size', 'strokeWidth'], template: '<span class="icon-stub" :data-name="name" :data-stroke-width="strokeWidth" />' },
        },
      },
    })
    await flushPromises()

    expect(wrapper.find('.auth-login-panel').exists()).toBe(true)
    expect(wrapper.find('h2').text()).toBe('欢迎回来')
    expect(wrapper.find('h2').classes()).toEqual(
      expect.arrayContaining(['text-3xl', 'font-bold', 'tracking-[-0.02em]', 'text-slate-950'])
    )
    expect(wrapper.find('.auth-login-subtitle').classes()).toContain('text-slate-500')
    expect(wrapper.find('.auth-email-oauth').exists()).toBe(true)
    expect(wrapper.find('.auth-email-oauth').element.compareDocumentPosition(wrapper.find('#email').element)).toBe(Node.DOCUMENT_POSITION_FOLLOWING)
    expect(wrapper.find('#email').classes()).toEqual(
      expect.arrayContaining(['auth-input', 'border-slate-200', 'bg-white', 'shadow-sm', 'focus:border-slate-950', 'focus:ring-slate-950/5'])
    )
    expect(wrapper.find('#email').classes()).toEqual(
      expect.arrayContaining(['dark:bg-slate-900/80', 'dark:placeholder:text-slate-500', 'dark:focus:bg-slate-900', 'dark:focus:ring-slate-400/10'])
    )
    expect(wrapper.find('#email').classes()).not.toContain('dark:focus:bg-white')
    expect(wrapper.find('#password').classes()).toContain('auth-input')
    expect(wrapper.findAll('.auth-field-icon')).toHaveLength(2)
    expect(wrapper.findAll('.auth-field-icon').every((icon) => icon.classes().includes('dark:text-slate-500'))).toBe(true)
    expect(wrapper.findAll('.auth-field-icon .icon-stub').every((icon) => icon.attributes('data-stroke-width') === '1.5')).toBe(true)
    expect(wrapper.find('.auth-submit-button').classes()).toEqual(
      expect.arrayContaining(['bg-slate-950', 'border', 'dark:bg-black', 'dark:text-white', 'dark:border-white/20', 'shadow-[0_16px_40px_rgba(139,92,246,0.18)]'])
    )
    expect(wrapper.find('.auth-submit-button').classes()).not.toContain('dark:bg-white')
    expect(wrapper.find('.auth-forgot-link').classes()).toEqual(
      expect.arrayContaining(['text-slate-600', 'hover:text-slate-950', 'hover:underline'])
    )
    expect(wrapper.find('.auth-signup-link').classes()).toEqual(
      expect.arrayContaining(['font-semibold', 'text-slate-950', 'underline-offset-4', 'hover:underline'])
    )
  })
})
