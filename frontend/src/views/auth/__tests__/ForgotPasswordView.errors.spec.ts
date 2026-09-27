import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ForgotPasswordView from '@/views/auth/ForgotPasswordView.vue'
import viCommon from '@/i18n/locales/vi/common'

const { forgotPasswordMock, showErrorMock } = vi.hoisted(() => ({
  forgotPasswordMock: vi.fn(),
  showErrorMock: vi.fn()
}))

vi.mock('vue-i18n', async () => {
  const { viT } = await import('./viTranslate')
  return {
    createI18n: () => ({ global: { t: viT } }),
    useI18n: () => ({ t: viT, locale: { value: 'vi' } })
  }
})

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: (...args: unknown[]) => showErrorMock(...args),
    showSuccess: vi.fn()
  })
}))

vi.mock('@/api/auth', () => ({
  getPublicSettings: vi.fn().mockResolvedValue({ turnstile_enabled: false }),
  forgotPassword: (...args: unknown[]) => forgotPasswordMock(...args)
}))

function mountForgot() {
  return mount(ForgotPasswordView, {
    global: {
      stubs: {
        AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
        Icon: true,
        RouterLink: true,
        TurnstileWidget: true
      }
    }
  })
}

describe('ForgotPasswordView errors', () => {
  beforeEach(() => {
    forgotPasswordMock.mockReset()
    showErrorMock.mockReset()
  })

  it('links an invalid email to its inline alert and clears it on input', async () => {
    const wrapper = mountForgot()
    await flushPromises()
    await wrapper.get('#email').setValue('not-an-email')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    const input = wrapper.get('#email')
    expect(input.attributes('aria-invalid')).toBe('true')
    const message = wrapper.get(`#${input.attributes('aria-describedby')}`)
    expect(message.attributes('role')).toBe('alert')
    expect(message.text()).toBe(viCommon.auth.invalidEmail)
    expect(forgotPasswordMock).not.toHaveBeenCalled()

    await input.setValue('user@example.com')
    expect(input.attributes('aria-invalid')).toBe('false')
    expect(input.attributes('aria-describedby')).toBeUndefined()
    expect(wrapper.find('#email-error').exists()).toBe(false)
  })

  it('renders a failed request as a form-level alert', async () => {
    forgotPasswordMock.mockRejectedValue({ message: 'mail server down' })
    const wrapper = mountForgot()
    await flushPromises()
    await wrapper.get('#email').setValue('user@example.com')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    const alert = wrapper.get('[data-testid="auth-form-error"]')
    expect(alert.attributes('role')).toBe('alert')
    expect(alert.text()).toBe('mail server down')
    expect(showErrorMock).toHaveBeenCalledWith('mail server down')
  })
})
