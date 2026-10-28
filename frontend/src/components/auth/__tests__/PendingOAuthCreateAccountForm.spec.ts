import { defineComponent, h } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import PendingOAuthCreateAccountForm from '../PendingOAuthCreateAccountForm.vue'

const sendVerifyCode = vi.fn()
const sendPendingOAuthVerifyCode = vi.fn()
const getPublicSettings = vi.fn()
const showError = vi.fn()
const turnstileReset = vi.fn()
const verifyAction = vi.fn()

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

vi.mock('@/api/auth', async () => {
  const actual = await vi.importActual<typeof import('@/api/auth')>('@/api/auth')
  return {
    ...actual,
    sendVerifyCode: (...args: any[]) => sendVerifyCode(...args),
    sendPendingOAuthVerifyCode: (...args: any[]) => sendPendingOAuthVerifyCode(...args),
    getPublicSettings: (...args: any[]) => getPublicSettings(...args)
  }
})

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError
  })
}))

describe('PendingOAuthCreateAccountForm', () => {
  beforeEach(() => {
    sendVerifyCode.mockReset()
    sendPendingOAuthVerifyCode.mockReset()
    getPublicSettings.mockReset()
    showError.mockReset()
    turnstileReset.mockReset()
    verifyAction.mockReset()
    getPublicSettings.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: ''
    })
  })

  it('requires a trimmed reason only when signup review is enabled', async () => {
    getPublicSettings.mockResolvedValue({ registration_review_enabled: true, email_verify_enabled: false })
    const wrapper = mount(PendingOAuthCreateAccountForm, {
      props: { testIdPrefix: 'oidc', initialEmail: 'user@example.com', isSubmitting: false },
      global: { stubs: { TurnstileWidget: true } }
    })
    await flushPromises()
    await wrapper.get('input[type="password"]').setValue('secret-123')
    await wrapper.get('[data-testid="oidc-create-account-verify-code"]').setValue('246810')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.emitted('submit')).toBeUndefined()
    await wrapper.get('textarea').setValue(' Research project ')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.emitted('submit')?.[0]?.[0]).toMatchObject({ registrationReason: 'Research project' })
    wrapper.unmount()
  })

  it('prevents reasons longer than 500 characters from implicit form submission', async () => {
    getPublicSettings.mockResolvedValue({ registration_review_enabled: true, email_verify_enabled: false })
    const wrapper = mount(PendingOAuthCreateAccountForm, {
      props: { testIdPrefix: 'oidc', initialEmail: 'user@example.com', isSubmitting: false },
      global: { stubs: { TurnstileWidget: true } }
    })
    await flushPromises()
    await wrapper.get('input[type="password"]').setValue('secret-123')
    await wrapper.get('textarea').setValue('x'.repeat(501))
    await wrapper.get('form').trigger('submit')
    expect(wrapper.emitted('submit')).toBeUndefined()
    expect(showError).toHaveBeenCalledWith('auth.registrationReasonTooLong')
    wrapper.unmount()
  })

  it('uses configured requirements and a Unicode character limit', async () => {
    getPublicSettings.mockResolvedValue({ registration_review_enabled: true, registration_review_instructions: 'Describe your team\nExplain the intended use', registration_reason_max_length: 3 })
    const wrapper = mount(PendingOAuthCreateAccountForm, {
      props: { testIdPrefix: 'oidc', initialEmail: 'user@example.com', isSubmitting: false },
      global: { stubs: { TurnstileWidget: true } }
    })
    await flushPromises()
    expect(wrapper.text()).toContain('Describe your team')
    await wrapper.get('input[type="password"]').setValue('secret-123')
    await wrapper.get('[data-testid="oidc-create-account-verify-code"]').setValue('246810')
    await wrapper.get('textarea').setValue('abcd')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.emitted('submit')).toBeUndefined()
    await wrapper.get('textarea').setValue('😀😀😀')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.emitted('submit')?.[0]?.[0]).toMatchObject({ registrationReason: '😀😀😀' })
    wrapper.unmount()
  })

  it('does not restart the code cooldown after switching away during a send', async () => {
    vi.useFakeTimers()
    let finish!: (value: { countdown: number }) => void
    sendPendingOAuthVerifyCode.mockImplementation(() => new Promise(resolve => { finish = resolve }))
    const wrapper = mount(PendingOAuthCreateAccountForm, {
      props: { testIdPrefix: 'oidc', initialEmail: 'user@example.com', isSubmitting: false },
      global: { stubs: { TurnstileWidget: true } },
    })
    try {
      await flushPromises()
      await wrapper.get('[data-testid="oidc-create-account-send-code"]').trigger('click')
      await flushPromises()
      expect(sendPendingOAuthVerifyCode).toHaveBeenCalledTimes(1)
      wrapper.unmount()
      finish({ countdown: 60 })
      await flushPromises()
      expect(vi.getTimerCount()).toBe(0)
    } finally {
      wrapper.unmount()
      vi.clearAllTimers()
      vi.useRealTimers()
    }
  })

  it('acquires separate proofs for pending OAuth send-code and create-account', async () => {
    getPublicSettings.mockResolvedValue({
      email_verify_enabled: true,
      turnstile_enabled: false,
      turnstile_site_key: '',
      tencent_captcha_enabled: true,
      tencent_captcha_app_id: 'tencent-app-id'
    })
    sendPendingOAuthVerifyCode.mockResolvedValue({ countdown: 0 })
    verifyAction
      .mockResolvedValueOnce({ token: 'ticket-1', randstr: '@rand-1' })
      .mockResolvedValueOnce({ token: 'ticket-2', randstr: '@rand-2' })
    const CaptchaChallengeStub = defineComponent({
      setup(_, { expose }) {
        expose({ verifyAction, reset: turnstileReset })
        return () => h('div')
      }
    })

    const wrapper = mount(PendingOAuthCreateAccountForm, {
      props: {
        testIdPrefix: 'oidc',
        initialEmail: 'user@example.com',
        isSubmitting: false
      },
      global: {
        stubs: { TurnstileWidget: CaptchaChallengeStub }
      }
    })

    await flushPromises()
    await wrapper.get('[data-testid="oidc-create-account-password"]').setValue('secret-123')
    await wrapper.get('[data-testid="oidc-create-account-verify-code"]').setValue('246810')
    await wrapper.get('[data-testid="oidc-create-account-send-code"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="oidc-create-account-submit"]').trigger('click')
    await flushPromises()

    expect(verifyAction).toHaveBeenCalledTimes(2)
    expect(sendPendingOAuthVerifyCode).toHaveBeenCalledWith({
      email: 'user@example.com',
      tencent_captcha_ticket: 'ticket-1',
      tencent_captcha_randstr: '@rand-1'
    })
    expect(wrapper.emitted('submit')).toEqual([
      [
        expect.objectContaining({
          tencentCaptchaTicket: 'ticket-2',
          tencentCaptchaRandstr: '@rand-2'
        })
      ]
    ])
    expect(turnstileReset).toHaveBeenCalledTimes(2)
  })

  it('emits trimmed email, password, and verify code on submit', async () => {
    const wrapper = mount(PendingOAuthCreateAccountForm, {
      props: {
        providerName: 'LinuxDo',
        testIdPrefix: 'linuxdo',
        initialEmail: 'prefill@example.com',
        isSubmitting: false
      }
    })

    await wrapper.get('[data-testid="linuxdo-create-account-email"]').setValue('  user@example.com  ')
    await wrapper.get('[data-testid="linuxdo-create-account-password"]').setValue('secret-123')
    await wrapper.get('[data-testid="linuxdo-create-account-verify-code"]').setValue(' 246810 ')
    await wrapper.get('form').trigger('submit.prevent')

    expect(wrapper.emitted('submit')).toEqual([
      [
        {
          email: 'user@example.com',
          password: 'secret-123',
          verifyCode: '246810'
        }
      ]
    ])
  })

  it('renders action labels through i18n keys', () => {
    const wrapper = mount(PendingOAuthCreateAccountForm, {
      props: {
        testIdPrefix: 'linuxdo',
        initialEmail: '',
        isSubmitting: false
      }
    })

    expect(wrapper.text()).toContain('auth.createAccount')
    expect(wrapper.text()).toContain('auth.alreadyHaveAccount')
  })

  it.each([false, true])('verifies the supplied OAuth email when ordinary email verification is disabled and review=%s', async (review) => {
    getPublicSettings.mockResolvedValue({
      email_verify_enabled: false,
      registration_review_enabled: review,
      turnstile_enabled: false,
      turnstile_site_key: ''
    })

    const wrapper = mount(PendingOAuthCreateAccountForm, {
      props: {
        testIdPrefix: 'linuxdo',
        initialEmail: 'prefill@example.com',
        isSubmitting: false
      }
    })

    await flushPromises()
    await wrapper.get('[data-testid="linuxdo-create-account-password"]').setValue('secret-123')
    if (review) await wrapper.get('textarea').setValue('Research access')
    expect(wrapper.find('[data-testid="linuxdo-create-account-verify-code"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="linuxdo-create-account-send-code"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="linuxdo-create-account-submit"]').attributes('disabled')).toBeDefined()
    await wrapper.get('form').trigger('submit.prevent')
    expect(wrapper.emitted('submit')).toBeUndefined()

    sendPendingOAuthVerifyCode.mockResolvedValue({ countdown: 0 })
    await wrapper.get('[data-testid="linuxdo-create-account-send-code"]').trigger('click')
    await flushPromises()
    expect(sendPendingOAuthVerifyCode).toHaveBeenCalledWith(expect.objectContaining({ email: 'prefill@example.com' }))
    await wrapper.get('[data-testid="linuxdo-create-account-verify-code"]').setValue('246810')
    await wrapper.get('form').trigger('submit.prevent')
    expect(wrapper.emitted('submit')).toEqual([
      [
        {
          email: 'prefill@example.com',
          password: 'secret-123',
          verifyCode: '246810',
          ...(review ? { registrationReason: 'Research access' } : {})
        }
      ]
    ])
    wrapper.unmount()
  })

  it('shows and emits invitation code when invitation-only signup is enabled', async () => {
    getPublicSettings.mockResolvedValue({
      invitation_code_enabled: true,
      email_verify_enabled: true,
      turnstile_enabled: false,
      turnstile_site_key: ''
    })

    const wrapper = mount(PendingOAuthCreateAccountForm, {
      props: {
        providerName: 'LinuxDo',
        testIdPrefix: 'linuxdo',
        initialEmail: 'prefill@example.com',
        isSubmitting: false
      }
    })

    await flushPromises()
    await wrapper.get('[data-testid="linuxdo-create-account-password"]').setValue('secret-123')
    await wrapper.get('[data-testid="linuxdo-create-account-verify-code"]').setValue('246810')
    await wrapper.get('[data-testid="linuxdo-create-account-invitation-code"]').setValue(' INVITE123 ')
    await wrapper.get('form').trigger('submit.prevent')

    expect(wrapper.emitted('submit')).toEqual([
      [
        {
          email: 'prefill@example.com',
          password: 'secret-123',
          verifyCode: '246810',
          invitationCode: 'INVITE123'
        }
      ]
    ])
  })

  it('sends a verify code for the trimmed email value', async () => {
    sendPendingOAuthVerifyCode.mockResolvedValue({
      message: 'sent',
      countdown: 60
    })

    const wrapper = mount(PendingOAuthCreateAccountForm, {
      props: {
        providerName: 'LinuxDo',
        testIdPrefix: 'linuxdo',
        initialEmail: '',
        isSubmitting: false
      }
    })

    await wrapper.get('[data-testid="linuxdo-create-account-email"]').setValue('  user@example.com  ')
    await wrapper.get('[data-testid="linuxdo-create-account-send-code"]').trigger('click')
    await flushPromises()

    expect(sendPendingOAuthVerifyCode).toHaveBeenCalledWith({
      email: 'user@example.com'
    })
  })

  it('shows send-code failures via toast without rendering inline error text', async () => {
    sendPendingOAuthVerifyCode.mockRejectedValue(new Error('send failed'))

    const wrapper = mount(PendingOAuthCreateAccountForm, {
      props: {
        testIdPrefix: 'linuxdo',
        initialEmail: '',
        isSubmitting: false
      }
    })

    await wrapper.get('[data-testid="linuxdo-create-account-email"]').setValue('user@example.com')
    await wrapper.get('[data-testid="linuxdo-create-account-send-code"]').trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('send failed')
    expect(wrapper.text()).not.toContain('send failed')
  })

  it('consumes the captcha proof when sending a verify code fails', async () => {
    getPublicSettings.mockResolvedValue({
      turnstile_enabled: true,
      turnstile_site_key: 'site-key'
    })
    sendPendingOAuthVerifyCode.mockRejectedValue(new Error('send failed'))

    const wrapper = mount(PendingOAuthCreateAccountForm, {
      props: {
        testIdPrefix: 'oidc',
        initialEmail: 'user@example.com',
        isSubmitting: false
      },
      global: {
        stubs: {
          TurnstileWidget: {
            template: '<button data-testid="turnstile-verify" @click="$emit(\'verify\', \'proof-token\')">verify</button>',
            methods: { reset: turnstileReset }
          }
        }
      }
    })

    await flushPromises()
    await wrapper.get('[data-testid="turnstile-verify"]').trigger('click')
    await wrapper.get('[data-testid="oidc-create-account-send-code"]').trigger('click')
    await flushPromises()

    expect(turnstileReset).toHaveBeenCalledOnce()
    expect(wrapper.get('[data-testid="oidc-create-account-send-code"]').attributes('disabled')).toBeDefined()
  })

  it('requires a turnstile token before sending a verify code when turnstile is enabled', async () => {
    getPublicSettings.mockResolvedValue({
      turnstile_enabled: true,
      turnstile_site_key: 'site-key'
    })
    sendPendingOAuthVerifyCode.mockResolvedValue({
      message: 'sent',
      countdown: 60
    })

    const wrapper = mount(PendingOAuthCreateAccountForm, {
      props: {
        providerName: 'LinuxDo',
        testIdPrefix: 'linuxdo',
        initialEmail: '',
        isSubmitting: false
      },
      global: {
        stubs: {
          TurnstileWidget: {
            template: '<button data-testid="turnstile-verify" @click="$emit(\'verify\', \'turnstile-token\')">verify</button>',
            methods: { reset: vi.fn() }
          }
        }
      }
    })

    await flushPromises()
    await wrapper.get('[data-testid="linuxdo-create-account-email"]').setValue('  user@example.com  ')

    expect(wrapper.get('[data-testid="linuxdo-create-account-send-code"]').attributes('disabled')).toBeDefined()

    await wrapper.get('[data-testid="turnstile-verify"]').trigger('click')
    await wrapper.get('[data-testid="linuxdo-create-account-send-code"]').trigger('click')
    await flushPromises()

    expect(sendPendingOAuthVerifyCode).toHaveBeenCalledWith({
      email: 'user@example.com',
      turnstile_token: 'turnstile-token'
    })
  })

  it('requires a turnstile token before submitting when turnstile is enabled', async () => {
    getPublicSettings.mockResolvedValue({
      turnstile_enabled: true,
      turnstile_site_key: 'site-key'
    })

    const wrapper = mount(PendingOAuthCreateAccountForm, {
      props: {
        testIdPrefix: 'linuxdo',
        initialEmail: 'user@example.com',
        isSubmitting: false
      },
      global: {
        stubs: {
          TurnstileWidget: {
            template: '<button data-testid="turnstile-verify" @click="$emit(\'verify\', \'turnstile-token\')">verify</button>',
            methods: { reset: turnstileReset }
          }
        }
      }
    })

    await flushPromises()
    await wrapper.get('[data-testid="linuxdo-create-account-password"]').setValue('secret-123')
    await wrapper.get('[data-testid="linuxdo-create-account-verify-code"]').setValue('246810')

    expect(wrapper.get('[data-testid="linuxdo-create-account-submit"]').attributes('disabled')).toBeDefined()

    // 隐式提交（输入框回车）绕过按钮 disabled，仍不能带着空票据发出请求
    await wrapper.get('form').trigger('submit.prevent')
    expect(wrapper.emitted('submit')).toBeUndefined()

    await wrapper.get('[data-testid="turnstile-verify"]').trigger('click')

    expect(wrapper.get('[data-testid="linuxdo-create-account-submit"]').attributes('disabled')).toBeUndefined()

    await wrapper.get('[data-testid="linuxdo-create-account-submit"]').trigger('click')

    expect(wrapper.emitted('submit')).toEqual([
      [
        {
          email: 'user@example.com',
          password: 'secret-123',
          verifyCode: '246810',
          turnstileToken: 'turnstile-token',
          invitationCode: undefined
        }
      ]
    ])
  })

  it('blocks submit again after sending a verify code consumes the turnstile proof', async () => {
    getPublicSettings.mockResolvedValue({
      turnstile_enabled: true,
      turnstile_site_key: 'site-key'
    })
    sendPendingOAuthVerifyCode.mockResolvedValue({ message: 'sent', countdown: 60 })

    const wrapper = mount(PendingOAuthCreateAccountForm, {
      props: {
        testIdPrefix: 'linuxdo',
        initialEmail: 'user@example.com',
        isSubmitting: false
      },
      global: {
        stubs: {
          TurnstileWidget: {
            template: '<button data-testid="turnstile-verify" @click="$emit(\'verify\', \'turnstile-token\')">verify</button>',
            methods: { reset: turnstileReset }
          }
        }
      }
    })

    await flushPromises()
    await wrapper.get('[data-testid="linuxdo-create-account-password"]').setValue('secret-123')
    await wrapper.get('[data-testid="linuxdo-create-account-verify-code"]').setValue('246810')
    await wrapper.get('[data-testid="turnstile-verify"]').trigger('click')
    await wrapper.get('[data-testid="linuxdo-create-account-send-code"]').trigger('click')
    await flushPromises()

    // 发码消耗掉票据并 reset 组件，新票据回调前提交必须保持关闭
    expect(turnstileReset).toHaveBeenCalled()
    expect(wrapper.get('[data-testid="linuxdo-create-account-submit"]').attributes('disabled')).toBeDefined()

    await wrapper.get('form').trigger('submit.prevent')
    expect(wrapper.emitted('submit')).toBeUndefined()

    // 新票据回调后恢复可提交，且带上新票据
    await wrapper.get('[data-testid="turnstile-verify"]').trigger('click')
    await wrapper.get('[data-testid="linuxdo-create-account-submit"]').trigger('click')

    expect(wrapper.emitted('submit')?.[0]?.[0]).toMatchObject({
      turnstileToken: 'turnstile-token'
    })
  })
})
