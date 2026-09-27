import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ResetPasswordView from '@/views/auth/ResetPasswordView.vue'
import viCommon from '@/i18n/locales/vi/common'

const { resetPasswordMock, showErrorMock } = vi.hoisted(() => ({
  resetPasswordMock: vi.fn(),
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
  useRoute: () => ({ query: { email: 'user@example.com', token: 'reset-token' } })
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: (...args: unknown[]) => showErrorMock(...args),
    showSuccess: vi.fn()
  })
}))

vi.mock('@/api/auth', () => ({
  resetPassword: (...args: unknown[]) => resetPasswordMock(...args)
}))

function mountReset() {
  return mount(ResetPasswordView, {
    global: {
      stubs: {
        AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
        Icon: true,
        RouterLink: true
      }
    }
  })
}

describe('ResetPasswordView errors', () => {
  beforeEach(() => {
    resetPasswordMock.mockReset()
    showErrorMock.mockReset()
  })

  it('localizes an expired reset token instead of showing the backend English message', async () => {
    resetPasswordMock.mockRejectedValue({
      status: 400,
      code: 400,
      reason: 'INVALID_RESET_TOKEN',
      message: 'invalid or expired password reset token'
    })
    const wrapper = mountReset()
    await flushPromises()
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue('secret-123')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(showErrorMock).toHaveBeenCalledWith(viCommon.auth.errors.INVALID_RESET_TOKEN)
    const alert = wrapper.get('[data-testid="auth-form-error"]')
    expect(alert.attributes('role')).toBe('alert')
    expect(alert.text()).toBe(viCommon.auth.errors.INVALID_RESET_TOKEN)
  })

  it('links a mismatched confirmation to its inline error message', async () => {
    const wrapper = mountReset()
    await flushPromises()
    await wrapper.get('#password').setValue('secret-123')
    await wrapper.get('#confirmPassword').setValue('different')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    const input = wrapper.get('#confirmPassword')
    expect(input.attributes('aria-invalid')).toBe('true')
    const message = wrapper.get(`#${input.attributes('aria-describedby')}`)
    expect(message.attributes('role')).toBe('alert')
    expect(message.text()).toBe(viCommon.auth.passwordsDoNotMatch)
    expect(resetPasswordMock).not.toHaveBeenCalled()
  })

  it('names the show/hide password buttons', async () => {
    const wrapper = mountReset()
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
