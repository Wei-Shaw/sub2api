import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LoginView from '@/views/auth/LoginView.vue'
import viCommon from '@/i18n/locales/vi/common'

const { getPublicSettingsMock, loginMock, showErrorMock } = vi.hoisted(() => ({
  getPublicSettingsMock: vi.fn(),
  loginMock: vi.fn(),
  showErrorMock: vi.fn()
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
    login: (...args: unknown[]) => loginMock(...args),
    loginWithPasskey: vi.fn(),
    login2FA: vi.fn()
  }),
  useAppStore: () => ({
    showError: (...args: unknown[]) => showErrorMock(...args),
    showSuccess: vi.fn(),
    showWarning: vi.fn()
  })
}))

vi.mock('@/api/auth', () => ({
  buildOAuthLoginStartURL: vi.fn(),
  getPublicSettings: (...args: unknown[]) => getPublicSettingsMock(...args),
  isTotp2FARequired: vi.fn(() => false),
  isWeChatWebOAuthEnabled: vi.fn(() => false),
  startOAuthLogin: vi.fn()
}))

function mountLogin() {
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
        TotpLoginModal: true,
        TurnstileWidget: true,
        WechatOAuthSection: true,
        transition: false
      }
    }
  })
}

async function submit(wrapper: ReturnType<typeof mountLogin>) {
  await wrapper.get('form').trigger('submit.prevent')
  await flushPromises()
}

describe('LoginView errors', () => {
  beforeEach(() => {
    getPublicSettingsMock.mockReset()
    loginMock.mockReset()
    showErrorMock.mockReset()
    sessionStorage.clear()
    getPublicSettingsMock.mockResolvedValue({ registration_enabled: true })
  })

  it('keeps a localized inline error after the same wrong password is submitted twice', async () => {
    loginMock.mockRejectedValue({
      status: 401,
      code: 401,
      reason: 'INVALID_CREDENTIALS',
      message: 'invalid email or password'
    })
    const wrapper = mountLogin()
    await flushPromises()
    await wrapper.get('#email').setValue('user@example.com')
    await wrapper.get('#password').setValue('wrong-password')

    for (let attempt = 0; attempt < 2; attempt++) {
      await submit(wrapper)
      const alert = wrapper.get('[data-testid="auth-form-error"]')
      expect(alert.attributes('role')).toBe('alert')
      expect(alert.text()).toBe(viCommon.auth.errors.INVALID_CREDENTIALS)
    }
    expect(loginMock).toHaveBeenCalledTimes(2)
  })

  it('shows the field error inline and linked to the input on every failed validation', async () => {
    const wrapper = mountLogin()
    await flushPromises()
    await wrapper.get('#email').setValue('user@example.com')
    await wrapper.get('#password').setValue('123')

    for (let attempt = 0; attempt < 2; attempt++) {
      await submit(wrapper)
      const input = wrapper.get('#password')
      expect(input.attributes('aria-invalid')).toBe('true')
      const describedBy = input.attributes('aria-describedby')
      expect(describedBy).toBeTruthy()
      const message = wrapper.get(`#${describedBy}`)
      expect(message.attributes('role')).toBe('alert')
      expect(message.text()).toBe(viCommon.auth.passwordMinLength)
    }
    // The toast only fires when the message changes, so the inline text is the
    // only feedback on the second attempt.
    expect(showErrorMock).toHaveBeenCalledTimes(1)
    expect(loginMock).not.toHaveBeenCalled()

    await wrapper.get('#password').setValue('secret-123')
    loginMock.mockResolvedValue({})
    await submit(wrapper)
    expect(wrapper.get('#password').attributes('aria-invalid')).toBe('false')
    expect(wrapper.find('#password-error').exists()).toBe(false)
  })
})
