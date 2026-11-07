import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TotpLoginModal from '@/components/auth/TotpLoginModal.vue'

const { showErrorMock } = vi.hoisted(() => ({
  showErrorMock: vi.fn(),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}:${JSON.stringify(params)}` : key,
  }),
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: (...args: any[]) => showErrorMock(...args),
  }),
}))

describe('TotpLoginModal', () => {
  beforeEach(() => {
    showErrorMock.mockReset()
  })

  it('sends verification errors to toast and does not render inline red text', async () => {
    const wrapper = mount(TotpLoginModal, {
      props: {
        tempToken: 'temp-token',
        userEmailMasked: 'u***@example.com',
      },
    })

    ;(wrapper.vm as unknown as { setError: (message: string) => void }).setError('Invalid code')
    await wrapper.vm.$nextTick()

    expect(showErrorMock).toHaveBeenCalledWith('Invalid code')
    expect(wrapper.text()).not.toContain('Invalid code')
    expect(wrapper.find('.bg-red-50').exists()).toBe(false)
  })

  it('shows resend button with countdown only in email mode', async () => {
    vi.useFakeTimers()
    try {
      const totp = mount(TotpLoginModal, { props: { tempToken: 't' } })
      expect(totp.find('[data-test="email-code-resend"]').exists()).toBe(false)
      expect(totp.text()).toContain('profile.totp.loginTitle')

      const email = mount(TotpLoginModal, { props: { tempToken: 't', mode: 'email' } })
      expect(email.text()).toContain('auth.emailCodeLogin.title')
      await email.vm.$nextTick()
      const resend = () => email.find('[data-test="email-code-resend"]')
      expect(resend().attributes('disabled')).toBeDefined()
      expect(resend().text()).toContain('auth.emailCodeLogin.resendIn')

      vi.advanceTimersByTime(60_000)
      await email.vm.$nextTick()
      expect(resend().attributes('disabled')).toBeUndefined()
      await resend().trigger('click')
      expect(email.emitted('resend')).toHaveLength(1)
    } finally {
      vi.useRealTimers()
    }
  })

  it('offers switching between TOTP and email code only when allowed', async () => {
    const plain = mount(TotpLoginModal, { props: { tempToken: 't' } })
    expect(plain.find('[data-test="login-challenge-switch"]').exists()).toBe(false)

    const wrapper = mount(TotpLoginModal, { props: { tempToken: 't', allowSwitch: true } })
    const sw = () => wrapper.find('[data-test="login-challenge-switch"]')
    expect(sw().text()).toContain('auth.emailCodeLogin.useEmail')
    await sw().trigger('click')
    expect(wrapper.emitted('switch')).toHaveLength(1)

    await wrapper.setProps({ mode: 'email' })
    expect(sw().text()).toContain('auth.emailCodeLogin.useTotp')
    expect(wrapper.find('[data-test="email-code-resend"]').exists()).toBe(true)
  })

  it('ignores input events fired during IME composition so each digit lands in one cell', async () => {
    const wrapper = mount(TotpLoginModal, { props: { tempToken: 't' }, attachTo: document.body })
    const cells = () => wrapper.findAll('input[maxlength="1"]')
    const typeDigit = async (index: number, digit: string) => {
      const el = cells()[index].element as HTMLInputElement
      el.focus()
      el.value = digit
      // While composing, the browser fires input with isComposing=true; focus must not move yet.
      el.dispatchEvent(new InputEvent('input', { data: digit, isComposing: true }))
      await wrapper.vm.$nextTick()
      expect(document.activeElement).toBe(el)
      el.dispatchEvent(new CompositionEvent('compositionend', { data: digit }))
      await wrapper.vm.$nextTick()
    }

    for (const [i, d] of ['3', '3', '4', '4', '5', '5'].entries()) await typeDigit(i, d)

    expect(cells().map((c) => (c.element as HTMLInputElement).value)).toEqual(['3', '3', '4', '4', '5', '5'])
    expect(wrapper.emitted('verify')?.[0]).toEqual(['334455'])
    wrapper.unmount()
  })

  it('keeps only one digit per cell', async () => {
    const wrapper = mount(TotpLoginModal, { props: { tempToken: 't' } })
    const el = wrapper.findAll('input[maxlength="1"]')[0].element as HTMLInputElement
    el.value = '33'
    el.dispatchEvent(new InputEvent('input'))
    await wrapper.vm.$nextTick()
    expect(el.value).toBe('3')
  })
})
