import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import RegisterView from '@/views/auth/RegisterView.vue'
import viCommon from '@/i18n/locales/vi/common'

const {
  getPublicSettingsMock,
  registerMock,
  showErrorMock,
  pushMock,
  validateInvitationCodeMock,
  validatePromoCodeMock
} = vi.hoisted(() => ({
  getPublicSettingsMock: vi.fn(),
  registerMock: vi.fn(),
  showErrorMock: vi.fn(),
  pushMock: vi.fn(),
  validateInvitationCodeMock: vi.fn(),
  validatePromoCodeMock: vi.fn()
}))

vi.mock('vue-i18n', async () => {
  const { viT } = await import('./viTranslate')
  return {
    createI18n: () => ({ global: { t: viT } }),
    useI18n: () => ({ t: viT, locale: { value: 'vi' } })
  }
})

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: pushMock }),
  useRoute: () => ({ query: {} })
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => ({ register: (...args: unknown[]) => registerMock(...args) }),
  useAppStore: () => ({
    cachedPublicSettings: null,
    showError: (...args: unknown[]) => showErrorMock(...args),
    showSuccess: vi.fn(),
    showWarning: vi.fn()
  })
}))

vi.mock('@/api/auth', async () => {
  const actual = await vi.importActual<typeof import('@/api/auth')>('@/api/auth')
  return {
    ...actual,
    getPublicSettings: (...args: unknown[]) => getPublicSettingsMock(...args),
    validateInvitationCode: (...args: unknown[]) => validateInvitationCodeMock(...args),
    validatePromoCode: (...args: unknown[]) => validatePromoCodeMock(...args)
  }
})

const publicSettings = {
  registration_enabled: true,
  email_verify_enabled: false,
  promo_code_enabled: false,
  invitation_code_enabled: false,
  affiliate_enabled: false,
  turnstile_enabled: false,
  site_name: 'Sub2API',
  registration_email_suffix_whitelist: [],
  linuxdo_oauth_enabled: false,
  oidc_oauth_enabled: false,
  github_oauth_enabled: false,
  google_oauth_enabled: false
}

function mountRegister() {
  return mount(RegisterView, {
    global: {
      stubs: {
        AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
        Icon: true,
        TurnstileWidget: true,
        LoginAgreementPrompt: true,
        EmailOAuthButtons: true,
        LinuxDoOAuthSection: true,
        WechatOAuthSection: true,
        OidcOAuthSection: true,
        RouterLink: true,
        transition: false
      }
    }
  })
}

async function fillValidForm(wrapper: ReturnType<typeof mountRegister>) {
  await wrapper.get('#email').setValue('user@example.com')
  await wrapper.get('#password').setValue('secret-123')
  await wrapper.get('#confirmPassword').setValue('secret-123')
}

describe('RegisterView errors', () => {
  beforeEach(() => {
    getPublicSettingsMock.mockReset()
    registerMock.mockReset()
    showErrorMock.mockReset()
    pushMock.mockReset()
    validateInvitationCodeMock.mockReset()
    validatePromoCodeMock.mockReset()
    validatePromoCodeMock.mockResolvedValue({ valid: true, bonus_amount: 1 })
    sessionStorage.clear()
    getPublicSettingsMock.mockResolvedValue(publicSettings)
  })

  it('shows the Vietnamese message for a backend EMAIL_EXISTS reason', async () => {
    registerMock.mockRejectedValue({
      status: 409,
      code: 409,
      reason: 'EMAIL_EXISTS',
      message: 'email already exists'
    })
    const wrapper = mountRegister()
    await flushPromises()
    await fillValidForm(wrapper)
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(showErrorMock).toHaveBeenCalledWith(viCommon.auth.errors.EMAIL_EXISTS)
    expect(showErrorMock).not.toHaveBeenCalledWith('email already exists')
  })

  it('renders the blocking submit error instead of failing silently', async () => {
    getPublicSettingsMock.mockResolvedValue({ ...publicSettings, invitation_code_enabled: true })
    validateInvitationCodeMock.mockResolvedValue({ valid: false, error_code: 'INVITATION_CODE_INVALID' })
    const wrapper = mountRegister()
    await flushPromises()
    await fillValidForm(wrapper)
    await wrapper.get('#invitation_code').setValue('bad-code')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    const alert = wrapper.get('[data-testid="auth-form-error"]')
    expect(alert.attributes('role')).toBe('alert')
    expect(alert.text()).toBe(viCommon.auth.invitationCodeInvalidCannotRegister)
    expect(registerMock).not.toHaveBeenCalled()
  })

  it('links each invalid field to its inline error message', async () => {
    const wrapper = mountRegister()
    await flushPromises()
    await wrapper.get('#email').setValue('user@example.com')
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue('different')

    for (let attempt = 0; attempt < 2; attempt++) {
      await wrapper.get('form').trigger('submit.prevent')
      await flushPromises()
      const input = wrapper.get('#confirmPassword')
      expect(input.attributes('aria-invalid')).toBe('true')
      const message = wrapper.get(`#${input.attributes('aria-describedby')}`)
      expect(message.attributes('role')).toBe('alert')
      expect(message.text()).toBe(viCommon.auth.passwordsDoNotMatch)
    }
    expect(wrapper.get('#password').attributes('aria-invalid')).toBe('false')
    expect(registerMock).not.toHaveBeenCalled()
  })

  it('restores the fields saved by the email verification back button', async () => {
    getPublicSettingsMock.mockResolvedValue({
      ...publicSettings,
      invitation_code_enabled: true,
      promo_code_enabled: true
    })
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'user@example.com',
        promo_code: 'PROMO10',
        invitation_code: 'INVITE1',
        aff_code: 'AFF42'
      })
    )
    const wrapper = mountRegister()
    await flushPromises()

    expect((wrapper.get('#email').element as HTMLInputElement).value).toBe('user@example.com')
    expect((wrapper.get('#invitation_code').element as HTMLInputElement).value).toBe('INVITE1')
    expect((wrapper.get('#promo_code').element as HTMLInputElement).value).toBe('PROMO10')
    expect((wrapper.get('#password').element as HTMLInputElement).value).toBe('')
    expect(validatePromoCodeMock).toHaveBeenCalledWith('PROMO10')
    expect(sessionStorage.getItem('register_data')).toBeNull()
  })

  it('labels the optional affiliate field as a referral code, not an invitation code', async () => {
    getPublicSettingsMock.mockResolvedValue({ ...publicSettings, affiliate_enabled: true })
    const wrapper = mountRegister()
    await flushPromises()

    const field = wrapper.get('[data-testid="affiliate-invitation-field"]')
    expect(field.get('label').text()).toContain('Mã giới thiệu')
    expect(field.get('label').text()).not.toContain(viCommon.auth.invitationCodeLabel)
    expect(field.get('input').attributes('placeholder')).toBe('Nhập mã giới thiệu')
  })

  it('names the show/hide password buttons', async () => {
    const wrapper = mountRegister()
    await flushPromises()

    for (const id of ['#password', '#confirmPassword']) {
      const toggle = wrapper.get(id).element.parentElement!.querySelector('button')!
      expect(toggle.getAttribute('aria-label')).toBe(viCommon.auth.showPassword)
      toggle.click()
      await flushPromises()
      expect(toggle.getAttribute('aria-label')).toBe(viCommon.auth.hidePassword)
    }
  })
})
