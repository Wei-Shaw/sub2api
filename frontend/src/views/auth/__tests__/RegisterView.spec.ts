import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import RegisterView from '@/views/auth/RegisterView.vue'

const {
  pushMock,
  registerMock,
  showSuccessMock,
  showErrorMock,
  showWarningMock,
  getPublicSettingsMock,
  validatePromoCodeMock,
  validateInvitationCodeMock,
  routeState
} = vi.hoisted(() => ({
  pushMock: vi.fn(),
  registerMock: vi.fn(),
  showSuccessMock: vi.fn(),
  showErrorMock: vi.fn(),
  showWarningMock: vi.fn(),
  getPublicSettingsMock: vi.fn(),
  validatePromoCodeMock: vi.fn(),
  validateInvitationCodeMock: vi.fn(),
  routeState: {
    query: {} as Record<string, unknown>
  }
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: pushMock
  }),
  useRoute: () => routeState
}))

vi.mock('vue-i18n', () => ({
  createI18n: () => ({
    global: {
      t: (key: string) => key
    }
  }),
  useI18n: () => ({
    t: (key: string, params?: Record<string, string | number>) => {
      if (key === 'auth.accountCreatedSuccess') {
        return `Account created for ${params?.siteName ?? 'Sub2API'}`
      }
      return key
    },
    locale: { value: 'en' }
  })
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => ({
    register: (...args: any[]) => registerMock(...args)
  }),
  useAppStore: () => ({
    showSuccess: (...args: any[]) => showSuccessMock(...args),
    showError: (...args: any[]) => showErrorMock(...args),
    showWarning: (...args: any[]) => showWarningMock(...args)
  })
}))

vi.mock('@/api/auth', async () => {
  const actual = await vi.importActual<typeof import('@/api/auth')>('@/api/auth')
  return {
    ...actual,
    getPublicSettings: (...args: any[]) => getPublicSettingsMock(...args),
    validatePromoCode: (...args: any[]) => validatePromoCodeMock(...args),
    validateInvitationCode: (...args: any[]) => validateInvitationCodeMock(...args)
  }
})

function createSettings(overrides: Record<string, unknown> = {}) {
  return {
    registration_enabled: true,
    email_verify_enabled: false,
    promo_code_enabled: false,
    invitation_code_enabled: false,
    turnstile_enabled: false,
    turnstile_site_key: '',
    site_name: 'Sub2API',
    linuxdo_oauth_enabled: false,
    wechat_oauth_enabled: false,
    oidc_oauth_enabled: false,
    oidc_oauth_provider_name: 'OIDC',
    github_oauth_enabled: false,
    google_oauth_enabled: false,
    registration_email_suffix_whitelist: [],
    ...overrides
  }
}

function mountView() {
  return mount(RegisterView, {
    global: {
      stubs: {
        AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
        Icon: true,
        TurnstileWidget: true,
        LinuxDoOAuthSection: true,
        OidcOAuthSection: true,
        WechatOAuthSection: true,
        EmailOAuthButtons: true,
        LoginAgreementPrompt: true,
        RouterLink: { template: '<a><slot /></a>' },
        transition: false
      }
    }
  })
}

describe('RegisterView', () => {
  beforeEach(() => {
    pushMock.mockReset()
    pushMock.mockResolvedValue(undefined)
    registerMock.mockReset()
    registerMock.mockResolvedValue(undefined)
    showSuccessMock.mockReset()
    showErrorMock.mockReset()
    showWarningMock.mockReset()
    getPublicSettingsMock.mockReset()
    validatePromoCodeMock.mockReset()
    validateInvitationCodeMock.mockReset()
    routeState.query = {}
    sessionStorage.clear()
    localStorage.clear()
    getPublicSettingsMock.mockResolvedValue(createSettings())
  })

  it('submits the selected whitelisted email domain as part of the email', async () => {
    getPublicSettingsMock.mockResolvedValue(
      createSettings({
        registration_email_suffix_whitelist: ['@example.com', '@gmail.com']
      })
    )

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="register-email-local-part"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="register-email-suffix-select"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="register-email-input"]').exists()).toBe(false)

    await wrapper.get('[data-testid="register-email-local-part"]').setValue('member')
    await wrapper.get('[data-testid="register-email-suffix-select"]').setValue('@gmail.com')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')

    expect(registerMock).toHaveBeenCalledWith(
      expect.objectContaining({
        email: 'member@gmail.com',
        password: 'secret-123'
      })
    )
    expect(pushMock).toHaveBeenCalledWith('/dashboard')
  })

  it('renders a fixed suffix when only one exact whitelist domain is available', async () => {
    getPublicSettingsMock.mockResolvedValue(
      createSettings({
        registration_email_suffix_whitelist: ['@company.com']
      })
    )

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="register-email-fixed-suffix"]').text()).toBe('@company.com')
    expect(wrapper.find('[data-testid="register-email-suffix-select"]').exists()).toBe(false)

    await wrapper.get('[data-testid="register-email-local-part"]').setValue('ops')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')

    expect(registerMock).toHaveBeenCalledWith(
      expect.objectContaining({
        email: 'ops@company.com'
      })
    )
  })

  it('allows switching to a full email input when wildcard domains are also allowed', async () => {
    getPublicSettingsMock.mockResolvedValue(
      createSettings({
        registration_email_suffix_whitelist: ['@example.com', '*.edu.cn']
      })
    )

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="register-email-custom-toggle"]').exists()).toBe(true)

    await wrapper.get('[data-testid="register-email-custom-toggle"]').trigger('click')

    expect(wrapper.find('[data-testid="register-email-input"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="register-email-listed-toggle"]').exists()).toBe(true)

    await wrapper.get('[data-testid="register-email-input"]').setValue('student@cs.edu.cn')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')

    expect(registerMock).toHaveBeenCalledWith(
      expect.objectContaining({
        email: 'student@cs.edu.cn'
      })
    )
  })
})
