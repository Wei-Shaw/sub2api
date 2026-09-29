import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LoginView from '@/views/auth/LoginView.vue'
import viCommon from '@/i18n/locales/vi/common'
import enCommon from '@/i18n/locales/en/common'
import zhCommon from '@/i18n/locales/zh/common'

const {
  f208GetPublicSettingsMock,
  f208LoginMock,
  f208Login2FAMock,
  f208IsTotp2FARequiredMock,
  f208ShowErrorMock,
  f208SetErrorMock,
  f208SetVerifyingMock
} = vi.hoisted(() => ({
  f208GetPublicSettingsMock: vi.fn(),
  f208LoginMock: vi.fn(),
  f208Login2FAMock: vi.fn(),
  f208IsTotp2FARequiredMock: vi.fn(() => false),
  f208ShowErrorMock: vi.fn(),
  f208SetErrorMock: vi.fn(),
  f208SetVerifyingMock: vi.fn()
}))

vi.mock('vue-i18n', async () => {
  const { viT } = await import('./viTranslate')
  return {
    createI18n: () => ({ global: { t: viT } }),
    useI18n: () => ({ t: viT, locale: { value: 'vi' } })
  }
})

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn(), currentRoute: { value: { query: {} } } })
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => ({
    login: (...args: unknown[]) => f208LoginMock(...args),
    loginWithPasskey: vi.fn(),
    login2FA: (...args: unknown[]) => f208Login2FAMock(...args)
  }),
  useAppStore: () => ({
    showError: (...args: unknown[]) => f208ShowErrorMock(...args),
    showSuccess: vi.fn(),
    showWarning: vi.fn()
  })
}))

vi.mock('@/api/auth', () => ({
  buildOAuthLoginStartURL: vi.fn(),
  getPublicSettings: (...args: unknown[]) => f208GetPublicSettingsMock(...args),
  isTotp2FARequired: (...args: unknown[]) => f208IsTotp2FARequiredMock(...(args as [])),
  isWeChatWebOAuthEnabled: vi.fn(() => false),
  startOAuthLogin: vi.fn()
}))

const f208BackendLoginReasons = [
  'LOGIN_THROTTLED',
  'BACKEND_MODE_ADMIN_ONLY',
  'PASSKEY_VERIFICATION_FAILED',
  'TOTP_INVALID_CODE',
  'TOTP_TOO_MANY_ATTEMPTS'
] as const

function f208MountLogin() {
  return mount(LoginView, {
    global: {
      stubs: {
        AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
        DingTalkOAuthSection: true,
        EmailOAuthButtons: true,
        Icon: true,
        LinuxDoOAuthSection: true,
        LoginAgreementPrompt: true,
        OidcOAuthSection: true,
        RouterLink: { template: '<a><slot /></a>' },
        TotpLoginModal: {
          name: 'TotpLoginModal',
          template: '<div />',
          methods: {
            setError: (...args: unknown[]) => f208SetErrorMock(...args),
            setVerifying: (...args: unknown[]) => f208SetVerifyingMock(...args)
          }
        },
        TurnstileWidget: true,
        WechatOAuthSection: true,
        transition: false
      }
    }
  })
}

async function f208Submit(wrapper: ReturnType<typeof f208MountLogin>) {
  await wrapper.get('#email').setValue('user@example.com')
  await wrapper.get('#password').setValue('secret-123')
  await wrapper.get('form').trigger('submit.prevent')
  await flushPromises()
}

describe('LoginView localized backend login errors (F2-08)', () => {
  beforeEach(() => {
    f208GetPublicSettingsMock.mockReset()
    f208LoginMock.mockReset()
    f208Login2FAMock.mockReset()
    f208IsTotp2FARequiredMock.mockReset()
    f208IsTotp2FARequiredMock.mockReturnValue(false)
    f208ShowErrorMock.mockReset()
    f208SetErrorMock.mockReset()
    f208SetVerifyingMock.mockReset()
    sessionStorage.clear()
    f208GetPublicSettingsMock.mockResolvedValue({ registration_enabled: true })
  })

  it('defines every backend login reason under auth.errors in en/zh/vi', () => {
    for (const locale of [enCommon, zhCommon, viCommon]) {
      const errors = locale.auth.errors as Record<string, string | undefined>
      for (const reason of f208BackendLoginReasons) {
        expect(errors[reason], reason).toBeTruthy()
      }
    }
  })

  it('shows the localized throttle message instead of the English backend text', async () => {
    f208LoginMock.mockRejectedValue({
      status: 429,
      code: 429,
      reason: 'LOGIN_THROTTLED',
      message: 'too many failed login attempts, please try again later'
    })
    const wrapper = f208MountLogin()
    await flushPromises()
    await f208Submit(wrapper)

    const text = wrapper.get('[data-testid="auth-form-error"]').text()
    expect(text).not.toBe('too many failed login attempts, please try again later')
    expect(text).toBe((viCommon.auth.errors as Record<string, string>).LOGIN_THROTTLED)
  })

  it('localizes the 2FA verification error through auth.errors', async () => {
    f208IsTotp2FARequiredMock.mockReturnValue(true)
    f208LoginMock.mockResolvedValue({
      requires_2fa: true,
      temp_token: 'temp-token',
      user_email_masked: 'u***@example.com'
    })
    f208Login2FAMock.mockRejectedValue({
      status: 400,
      code: 400,
      reason: 'TOTP_INVALID_CODE',
      message: 'invalid totp code',
      response: { data: { message: 'invalid totp code' } }
    })
    const wrapper = f208MountLogin()
    await flushPromises()
    await f208Submit(wrapper)

    const modal = wrapper.findComponent({ name: 'TotpLoginModal' })
    expect(modal.exists()).toBe(true)
    modal.vm.$emit('verify', '123456')
    await flushPromises()

    expect(f208Login2FAMock).toHaveBeenCalledWith('temp-token', '123456')
    expect(f208SetErrorMock).toHaveBeenCalledWith(
      (viCommon.auth.errors as Record<string, string>).TOTP_INVALID_CODE
    )
    expect(f208SetErrorMock).not.toHaveBeenCalledWith('invalid totp code')
  })
})
