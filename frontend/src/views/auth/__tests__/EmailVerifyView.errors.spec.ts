import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import EmailVerifyView from '@/views/auth/EmailVerifyView.vue'
import viCommon from '@/i18n/locales/vi/common'

const { pushMock, showErrorMock, registerMock, getPublicSettingsMock, sendVerifyCodeMock } =
  vi.hoisted(() => ({
    pushMock: vi.fn(),
    showErrorMock: vi.fn(),
    registerMock: vi.fn(),
    getPublicSettingsMock: vi.fn(),
    sendVerifyCodeMock: vi.fn()
  }))

vi.mock('vue-i18n', async () => {
  const { viT } = await import('./viTranslate')
  return {
    createI18n: () => ({ global: { t: viT } }),
    useI18n: () => ({ t: viT, locale: { value: 'vi' } })
  }
})

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: pushMock })
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => ({
    pendingAuthSession: null,
    register: (...args: unknown[]) => registerMock(...args),
    setToken: vi.fn(),
    setPendingAuthSession: vi.fn(),
    clearPendingAuthSession: vi.fn()
  }),
  useAppStore: () => ({
    showSuccess: vi.fn(),
    showError: (...args: unknown[]) => showErrorMock(...args)
  })
}))

vi.mock('@/api/auth', async () => {
  const actual = await vi.importActual<typeof import('@/api/auth')>('@/api/auth')
  return {
    ...actual,
    getPublicSettings: (...args: unknown[]) => getPublicSettingsMock(...args),
    sendVerifyCode: (...args: unknown[]) => sendVerifyCodeMock(...args)
  }
})

vi.mock('@/api/client', () => ({ apiClient: { post: vi.fn() } }))

function mountVerify() {
  return mount(EmailVerifyView, {
    global: {
      stubs: {
        AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
        Icon: true,
        TurnstileWidget: true
      }
    }
  })
}

describe('EmailVerifyView errors', () => {
  beforeEach(() => {
    pushMock.mockReset()
    showErrorMock.mockReset()
    registerMock.mockReset()
    getPublicSettingsMock.mockReset()
    sendVerifyCodeMock.mockReset()
    sessionStorage.clear()
    localStorage.clear()
    getPublicSettingsMock.mockResolvedValue({
      turnstile_enabled: false,
      site_name: 'Sub2API',
      registration_email_suffix_whitelist: []
    })
    sendVerifyCodeMock.mockResolvedValue({ countdown: 60 })
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({ email: 'user@example.com', password: 'secret-123' })
    )
  })

  it('shows a localized inline error when the verification code is rejected', async () => {
    registerMock.mockRejectedValue({
      status: 400,
      code: 400,
      reason: 'INVALID_VERIFY_CODE',
      message: 'invalid or expired verification code'
    })
    const wrapper = mountVerify()
    await flushPromises()
    await wrapper.get('#code').setValue('123456')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    const alert = wrapper.get('[data-testid="auth-form-error"]')
    expect(alert.attributes('role')).toBe('alert')
    expect(alert.text()).toBe(viCommon.auth.errors.INVALID_VERIFY_CODE)
  })

  it('links a malformed code to its inline error message', async () => {
    const wrapper = mountVerify()
    await flushPromises()
    await wrapper.get('#code').setValue('12ab')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    const input = wrapper.get('#code')
    expect(input.attributes('aria-invalid')).toBe('true')
    const message = wrapper.get(`#${input.attributes('aria-describedby')}`)
    expect(message.attributes('role')).toBe('alert')
    expect(message.text()).toBe(viCommon.auth.invalidCode)
    expect(registerMock).not.toHaveBeenCalled()
  })
})
