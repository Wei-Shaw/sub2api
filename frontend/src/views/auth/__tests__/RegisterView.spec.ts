import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import RegisterView from '@/views/auth/RegisterView.vue'

const {
  getPublicSettingsMock,
  validatePromoCodeMock,
  validateInvitationCodeMock,
  showWarningMock,
  showErrorMock,
  showSuccessMock,
  registerMock,
  pushMock,
  routeState,
} = vi.hoisted(() => ({
  getPublicSettingsMock: vi.fn(),
  validatePromoCodeMock: vi.fn(),
  validateInvitationCodeMock: vi.fn(),
  showWarningMock: vi.fn(),
  showErrorMock: vi.fn(),
  showSuccessMock: vi.fn(),
  registerMock: vi.fn(),
  pushMock: vi.fn(),
  routeState: {
    query: {} as Record<string, unknown>,
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showWarning: (...args: any[]) => showWarningMock(...args),
    showError: (...args: any[]) => showErrorMock(...args),
    showSuccess: (...args: any[]) => showSuccessMock(...args),
  }),
  useAuthStore: () => ({
    register: (...args: any[]) => registerMock(...args),
  }),
}))

vi.mock('@/components/layout', () => ({
  AuthLayout: { template: '<section class="auth-layout-stub"><slot /><footer><slot name="footer" /></footer></section>' },
}))

vi.mock('@/api/auth', () => ({
  getPublicSettings: (...args: any[]) => getPublicSettingsMock(...args),
  validatePromoCode: (...args: any[]) => validatePromoCodeMock(...args),
  validateInvitationCode: (...args: any[]) => validateInvitationCodeMock(...args),
  isWeChatWebOAuthEnabled: (settings: any) => settings.wechat_oauth_enabled === true,
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: (...args: any[]) => pushMock(...args),
  }),
  useRoute: () => routeState,
}))

vi.mock('vue-i18n', () => ({
  createI18n: () => ({
    global: {
      t: (key: string) => key,
    },
  }),
  useI18n: () => ({
    locale: { value: 'zh' },
    t: (key: string, params?: Record<string, string>) => {
      const messages: Record<string, string> = {
        'auth.createAccount': '创建账户',
        'auth.signUpToStart': `注册并开始使用 ${params?.siteName ?? 'DevRouter'}`,
        'auth.emailLabel': '邮箱',
        'auth.emailPlaceholder': 'you@example.com',
        'auth.passwordLabel': '密码',
        'auth.createPasswordPlaceholder': '创建密码',
        'auth.passwordHint': '至少 6 个字符',
        'auth.promoCodeLabel': '优惠码',
        'auth.promoCodePlaceholder': '输入优惠码（可选）',
        'auth.oauthOrContinue': '或继续使用',
        'auth.processing': '处理中',
        'auth.continue': '继续',
        'auth.alreadyHaveAccount': '已有账户？',
        'auth.signIn': '登录',
        'common.optional': '可选',
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
    registration_enabled: true,
    email_verify_enabled: true,
    promo_code_enabled: true,
    invitation_code_enabled: false,
    turnstile_enabled: false,
    turnstile_site_key: '',
    site_name: 'DevRouter',
    linuxdo_oauth_enabled: false,
    wechat_oauth_enabled: false,
    oidc_oauth_enabled: false,
    oidc_oauth_provider_name: 'OIDC',
    github_oauth_enabled: true,
    google_oauth_enabled: true,
    registration_email_suffix_whitelist: [],
    login_agreement_enabled: false,
    login_agreement_documents: [],
    ...overrides,
  }
}

describe('RegisterView', () => {
  beforeEach(() => {
    getPublicSettingsMock.mockReset()
    validatePromoCodeMock.mockReset()
    validateInvitationCodeMock.mockReset()
    showWarningMock.mockReset()
    showErrorMock.mockReset()
    showSuccessMock.mockReset()
    registerMock.mockReset()
    pushMock.mockReset()
    routeState.query = {}
    localStorage.clear()
    sessionStorage.clear()
    getPublicSettingsMock.mockResolvedValue(publicSettings())
  })

  it('uses the refined auth presentation and visually de-emphasizes optional promo code', async () => {
    const wrapper = mount(RegisterView, {
      global: {
        stubs: {
          RouterLink: { props: ['to'], template: '<a :href="typeof to === `string` ? to : to.path"><slot /></a>' },
          EmailOAuthButtons: { template: '<div class="email-oauth-stub">Email OAuth</div>' },
          LinuxDoOAuthSection: true,
          WechatOAuthSection: true,
          OidcOAuthSection: true,
          LoginAgreementPrompt: true,
          TurnstileWidget: true,
          Icon: { props: ['name', 'size', 'strokeWidth'], template: '<span class="icon-stub" :data-name="name" :data-stroke-width="strokeWidth" />' },
        },
      },
    })
    await flushPromises()

    expect(wrapper.find('.auth-register-panel').exists()).toBe(true)
    expect(wrapper.find('h2').classes()).toEqual(
      expect.arrayContaining(['text-3xl', 'font-bold', 'tracking-[-0.02em]', 'text-slate-950'])
    )
    expect(wrapper.find('.auth-register-subtitle').classes()).toContain('text-slate-500')
    expect(wrapper.find('#email').classes()).toEqual(
      expect.arrayContaining(['auth-input', 'bg-white', 'border-slate-200', 'dark:bg-slate-900/80', 'dark:focus:bg-slate-900'])
    )
    expect(wrapper.find('#password').classes()).toContain('auth-input')
    expect(wrapper.find('.auth-password-hint').classes()).toEqual(
      expect.arrayContaining(['mt-2', 'text-slate-500', 'dark:text-slate-500'])
    )
    expect(wrapper.find('.auth-promo-field').exists()).toBe(true)
    expect(wrapper.find('#promo_code').classes()).toEqual(
      expect.arrayContaining(['auth-input', 'border-dashed', 'border-slate-200/70', 'dark:border-white/10'])
    )
    expect(wrapper.find('.auth-submit-button').classes()).toEqual(
      expect.arrayContaining(['border', 'bg-slate-950', 'dark:bg-black', 'dark:text-white', 'dark:border-white/20'])
    )
    expect(wrapper.find('.auth-submit-button').classes()).not.toContain('dark:bg-white')
    expect(wrapper.find('.auth-signin-link').classes()).toEqual(
      expect.arrayContaining(['font-semibold', 'text-slate-950', 'dark:text-white', 'hover:underline'])
    )
    expect(wrapper.find('.auth-signin-link').classes().join(' ')).not.toContain('primary')
  })
})
