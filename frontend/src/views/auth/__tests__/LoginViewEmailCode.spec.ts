import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import LoginView from '@/views/auth/LoginView.vue'

const mocks = vi.hoisted(() => ({
  getPublicSettings: vi.fn(),
  push: vi.fn(),
  login: vi.fn(),
  login2FA: vi.fn(),
  loginEmailCode: vi.fn(),
  resendLoginEmailCode: vi.fn(),
  startResendCountdown: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: mocks.push,
    currentRoute: { value: { query: {} } }
  })
}))

vi.mock('vue-i18n', () => ({
  createI18n: () => ({ global: { t: (key: string) => key } }),
  useI18n: () => ({ t: (key: string) => key })
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => ({
    login: mocks.login,
    loginWithPasskey: vi.fn(),
    login2FA: mocks.login2FA,
    loginEmailCode: mocks.loginEmailCode
  }),
  useAppStore: () => ({
    showError: mocks.showError,
    showSuccess: mocks.showSuccess,
    showWarning: vi.fn()
  })
}))

vi.mock('@/api/auth', () => ({
  buildOAuthLoginStartURL: vi.fn(),
  getPublicSettings: (...args: unknown[]) => mocks.getPublicSettings(...args),
  isTotp2FARequired: (response: any) => response?.requires_2fa === true,
  isEmailCodeLoginRequired: (response: any) => response?.requires_email_code === true,
  resendLoginEmailCode: (...args: unknown[]) => mocks.resendLoginEmailCode(...args),
  isWeChatWebOAuthEnabled: vi.fn(() => false),
  startOAuthLogin: vi.fn()
}))

const TotpLoginModalStub = defineComponent({
  name: 'TotpLoginModal',
  props: {
    tempToken: { type: String, default: '' },
    userEmailMasked: { type: String, default: '' },
    mode: { type: String, default: 'totp' },
    allowSwitch: { type: Boolean, default: false }
  },
  emits: ['verify', 'cancel', 'resend', 'switch'],
  setup(_, { expose }) {
    expose({
      setVerifying: vi.fn(),
      setError: vi.fn(),
      startResendCountdown: mocks.startResendCountdown
    })
    return () => h('div', { 'data-test': 'login-challenge-modal' })
  }
})

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
        TotpLoginModal: TotpLoginModalStub,
        TurnstileWidget: true,
        WechatOAuthSection: true,
        transition: false
      }
    }
  })
}

async function submitLogin(wrapper: ReturnType<typeof mountLogin>) {
  await wrapper.find('#email').setValue('admin@example.com')
  await wrapper.find('#password').setValue('secret-123')
  await wrapper.find('form').trigger('submit')
  await flushPromises()
}

describe('LoginView admin email code login', () => {
  beforeEach(() => {
    Object.values(mocks).forEach((mock) => mock.mockReset())
    mocks.getPublicSettings.mockResolvedValue({
      registration_enabled: true,
      turnstile_enabled: false,
      tencent_captcha_enabled: false,
      aliyun_captcha_enabled: false,
      backend_mode_enabled: false,
      password_reset_enabled: false,
      passkey_enabled: false,
      login_agreement_enabled: false,
      login_agreement_documents: []
    })
  })

  it('opens the modal in email mode and completes login with the email code', async () => {
    mocks.login.mockResolvedValue({
      requires_email_code: true,
      temp_token: 'temp-email',
      user_email_masked: 'a***n@example.com'
    })
    mocks.loginEmailCode.mockResolvedValue({})

    const wrapper = mountLogin()
    await flushPromises()
    await submitLogin(wrapper)

    const modal = wrapper.findComponent(TotpLoginModalStub)
    expect(modal.exists()).toBe(true)
    expect(modal.props('mode')).toBe('email')
    expect(modal.props('allowSwitch')).toBe(false)
    expect(modal.props('userEmailMasked')).toBe('a***n@example.com')

    modal.vm.$emit('verify', '123456')
    await flushPromises()
    expect(mocks.loginEmailCode).toHaveBeenCalledWith('temp-email', '123456')
    expect(mocks.login2FA).not.toHaveBeenCalled()
    expect(mocks.push).toHaveBeenCalled()
  })

  it('resends the email code and restarts the countdown', async () => {
    mocks.login.mockResolvedValue({ requires_email_code: true, temp_token: 'temp-email' })
    mocks.resendLoginEmailCode.mockResolvedValue({ message: 'ok', countdown: 60 })

    const wrapper = mountLogin()
    await flushPromises()
    await submitLogin(wrapper)

    wrapper.findComponent(TotpLoginModalStub).vm.$emit('resend')
    await flushPromises()
    expect(mocks.resendLoginEmailCode).toHaveBeenCalledWith('temp-email')
    expect(mocks.startResendCountdown).toHaveBeenCalledWith(60)
    expect(mocks.showSuccess).toHaveBeenCalledWith('auth.emailCodeLogin.resent')
  })

  it('lets an admin with TOTP switch to the email code and back', async () => {
    mocks.login.mockResolvedValue({
      requires_2fa: true,
      temp_token: 'temp-both',
      email_code_available: true
    })
    mocks.resendLoginEmailCode.mockResolvedValue({ message: 'ok', countdown: 60 })
    mocks.loginEmailCode.mockResolvedValue({})

    const wrapper = mountLogin()
    await flushPromises()
    await submitLogin(wrapper)

    const modal = () => wrapper.findComponent(TotpLoginModalStub)
    expect(modal().props('mode')).toBe('totp')
    expect(modal().props('allowSwitch')).toBe(true)
    // No email is requested until the admin switches.
    expect(mocks.resendLoginEmailCode).not.toHaveBeenCalled()

    modal().vm.$emit('switch')
    await flushPromises()
    expect(mocks.resendLoginEmailCode).toHaveBeenCalledTimes(1)
    expect(modal().props('mode')).toBe('email')

    // Switching back and forth again reuses the code that was already sent.
    modal().vm.$emit('switch')
    await flushPromises()
    expect(modal().props('mode')).toBe('totp')
    modal().vm.$emit('switch')
    await flushPromises()
    expect(modal().props('mode')).toBe('email')
    expect(mocks.resendLoginEmailCode).toHaveBeenCalledTimes(1)

    modal().vm.$emit('verify', '654321')
    await flushPromises()
    expect(mocks.loginEmailCode).toHaveBeenCalledWith('temp-both', '654321')
  })

  it('keeps the TOTP flow when the email code is not offered', async () => {
    mocks.login.mockResolvedValue({ requires_2fa: true, temp_token: 'temp-totp' })
    mocks.login2FA.mockResolvedValue({})

    const wrapper = mountLogin()
    await flushPromises()
    await submitLogin(wrapper)

    const modal = wrapper.findComponent(TotpLoginModalStub)
    expect(modal.props('mode')).toBe('totp')
    expect(modal.props('allowSwitch')).toBe(false)

    modal.vm.$emit('verify', '111111')
    await flushPromises()
    expect(mocks.login2FA).toHaveBeenCalledWith('temp-totp', '111111')
    expect(mocks.loginEmailCode).not.toHaveBeenCalled()
  })
})
