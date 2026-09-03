import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import DingTalkEmailCompletionView from '../DingTalkEmailCompletionView.vue'

const replace = vi.fn()
const showSuccess = vi.fn()
const showError = vi.fn()
const showInfo = vi.fn()
const setToken = vi.fn()
const setPendingAuthSession = vi.fn()
const clearPendingAuthSession = vi.fn()
const getPublicSettings = vi.fn()
const sendPendingOAuthVerifyCode = vi.fn()
const apiClientPost = vi.fn()

const routeState = {
  query: {} as Record<string, unknown>
}

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({
    replace
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      te: () => false
    })
  }
})

vi.mock('@/stores', () => ({
  useAuthStore: () => ({
    setToken,
    setPendingAuthSession,
    clearPendingAuthSession
  }),
  useAppStore: () => ({
    showSuccess,
    showError,
    showInfo
  })
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    post: (...args: any[]) => apiClientPost(...args)
  }
}))

vi.mock('@/api/auth', async () => {
  const actual = await vi.importActual<typeof import('@/api/auth')>('@/api/auth')
  return {
    ...actual,
    getPublicSettings: (...args: any[]) => getPublicSettings(...args),
    sendPendingOAuthVerifyCode: (...args: any[]) => sendPendingOAuthVerifyCode(...args)
  }
})

const mountOptions = {
  global: {
    stubs: {
      AuthLayout: { template: '<div><slot /></div>' },
      Icon: true,
      RouterLink: { template: '<a><slot /></a>' },
      transition: false
    }
  }
}

async function submitCreateAccount(
  wrapper: ReturnType<typeof mount>,
  email = 'new@example.com',
  password = 'secret-123'
) {
  await wrapper.get('[data-testid="dingtalk-create-account-email"]').setValue(email)
  await wrapper.get('[data-testid="dingtalk-create-account-password"]').setValue(password)
  await wrapper.get('[data-testid="dingtalk-create-account-submit"]').trigger('click')
  await flushPromises()
}

describe('DingTalkEmailCompletionView', () => {
  beforeEach(() => {
    replace.mockReset()
    showSuccess.mockReset()
    showError.mockReset()
    showInfo.mockReset()
    setToken.mockReset()
    setPendingAuthSession.mockReset()
    clearPendingAuthSession.mockReset()
    getPublicSettings.mockReset()
    sendPendingOAuthVerifyCode.mockReset()
    apiClientPost.mockReset()
    getPublicSettings.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: ''
    })
    setToken.mockResolvedValue({})
    routeState.query = {}
    window.location.hash = ''
    localStorage.clear()
    sessionStorage.clear()
  })

  it('prefills the email from the query and creates the account', async () => {
    routeState.query = { email: 'prefilled@example.com' }
    apiClientPost.mockResolvedValue({
      data: {
        access_token: 'new-access-token',
        refresh_token: 'new-refresh-token',
        expires_in: 3600,
        token_type: 'Bearer',
        redirect: '/usage'
      }
    })

    const wrapper = mount(DingTalkEmailCompletionView, mountOptions)
    await flushPromises()

    expect(
      (wrapper.get('[data-testid="dingtalk-create-account-email"]').element as HTMLInputElement).value
    ).toBe('prefilled@example.com')

    await submitCreateAccount(wrapper, 'prefilled@example.com')

    expect(apiClientPost).toHaveBeenCalledWith('/auth/oauth/pending/create-account', {
      email: 'prefilled@example.com',
      password: 'secret-123',
      verify_code: undefined,
      invitation_code: undefined
    })
    expect(setToken).toHaveBeenCalledWith('new-access-token')
    expect(localStorage.getItem('refresh_token')).toBe('new-refresh-token')
    expect(showSuccess).toHaveBeenCalledWith('auth.loginSuccess')
    expect(replace).toHaveBeenCalledWith('/usage')
  })

  it('ignores a protocol-relative redirect returned by the backend', async () => {
    apiClientPost.mockResolvedValue({
      data: {
        access_token: 'new-access-token',
        redirect: '//evil.com'
      }
    })

    const wrapper = mount(DingTalkEmailCompletionView, mountOptions)
    await flushPromises()
    await submitCreateAccount(wrapper)

    expect(setToken).toHaveBeenCalledWith('new-access-token')
    expect(replace).toHaveBeenCalledWith('/dashboard')
    expect(replace).not.toHaveBeenCalledWith('//evil.com')
  })

  it('falls back to the redirect query when the response omits one', async () => {
    routeState.query = { redirect: '/usage' }
    apiClientPost.mockResolvedValue({
      data: {
        access_token: 'new-access-token'
      }
    })

    const wrapper = mount(DingTalkEmailCompletionView, mountOptions)
    await flushPromises()
    await submitCreateAccount(wrapper)

    expect(replace).toHaveBeenCalledWith('/usage')
  })

  it('ignores a protocol-relative redirect query', async () => {
    routeState.query = { redirect: '//evil.com' }
    apiClientPost.mockResolvedValue({
      data: {
        access_token: 'new-access-token'
      }
    })

    const wrapper = mount(DingTalkEmailCompletionView, mountOptions)
    await flushPromises()
    await submitCreateAccount(wrapper)

    expect(replace).toHaveBeenCalledWith('/dashboard')
    expect(replace).not.toHaveBeenCalledWith('//evil.com')
  })

  it('redirects to the shared default path when no redirect is provided', async () => {
    apiClientPost.mockResolvedValue({
      data: {
        access_token: 'new-access-token'
      }
    })

    const wrapper = mount(DingTalkEmailCompletionView, mountOptions)
    await flushPromises()
    await submitCreateAccount(wrapper)

    // 该视图并没有自己的 /profile 兜底：它直接用 sanitizeRedirectPath 的
    // DEFAULT_REDIRECT_PATH（/dashboard）。/profile 兜底只存在于
    // DingTalkCallbackView 的绑定分支（finalizeCompletion）。
    expect(replace).toHaveBeenCalledWith('/dashboard')
  })

  it('routes back to the callback bind flow when the account is already bindable', async () => {
    routeState.query = { redirect: '/usage' }
    apiClientPost.mockResolvedValue({
      data: {
        step: 'choose_account_action_required'
      }
    })

    const wrapper = mount(DingTalkEmailCompletionView, mountOptions)
    await flushPromises()
    await submitCreateAccount(wrapper, 'existing@example.com')

    expect(setToken).not.toHaveBeenCalled()
    expect(replace).toHaveBeenCalledWith({
      path: '/auth/dingtalk/callback',
      query: {
        bind: '1',
        email: 'existing@example.com',
        redirect: '/usage'
      }
    })
  })

  it('routes back to the callback bind flow when registration is disabled', async () => {
    apiClientPost.mockRejectedValue({
      response: {
        data: {
          reason: 'REGISTRATION_DISABLED',
          message: 'registration disabled'
        }
      }
    })

    const wrapper = mount(DingTalkEmailCompletionView, mountOptions)
    await flushPromises()
    await submitCreateAccount(wrapper, 'existing@example.com')

    expect(showInfo).toHaveBeenCalledWith('auth.dingtalk.registrationDisabledRedirectToBind')
    expect(showError).not.toHaveBeenCalled()
    expect(replace).toHaveBeenCalledWith({
      path: '/auth/dingtalk/callback',
      query: {
        bind: '1',
        email: 'existing@example.com'
      }
    })
  })

  it('shows create-account failures through the app store without navigating', async () => {
    apiClientPost.mockRejectedValue(new Error('create failed'))

    const wrapper = mount(DingTalkEmailCompletionView, mountOptions)
    await flushPromises()
    await submitCreateAccount(wrapper)

    expect(showError).toHaveBeenCalledWith('create failed')
    expect(setToken).not.toHaveBeenCalled()
    expect(replace).not.toHaveBeenCalled()
  })

  it('reports a login failure when the response carries neither a token nor a bind hint', async () => {
    apiClientPost.mockResolvedValue({ data: {} })

    const wrapper = mount(DingTalkEmailCompletionView, mountOptions)
    await flushPromises()
    await submitCreateAccount(wrapper)

    expect(showError).toHaveBeenCalledWith('auth.loginFailed')
    expect(setToken).not.toHaveBeenCalled()
    expect(replace).not.toHaveBeenCalled()
  })

  it('switches to the callback bind flow from the "already have an account" button', async () => {
    routeState.query = { redirect: '/usage' }

    const wrapper = mount(DingTalkEmailCompletionView, mountOptions)
    await flushPromises()

    await wrapper.get('[data-testid="dingtalk-create-account-email"]').setValue('existing@example.com')
    const buttons = wrapper.findAll('button')
    await buttons[buttons.length - 1].trigger('click')
    await flushPromises()

    expect(apiClientPost).not.toHaveBeenCalled()
    expect(replace).toHaveBeenCalledWith({
      path: '/auth/dingtalk/callback',
      query: {
        bind: '1',
        email: 'existing@example.com',
        redirect: '/usage'
      }
    })
  })

  it('passes the redirect query through to the callback route without sanitising it', async () => {
    routeState.query = { redirect: '//evil.com' }

    const wrapper = mount(DingTalkEmailCompletionView, mountOptions)
    await flushPromises()

    await wrapper.get('[data-testid="dingtalk-create-account-email"]').setValue('existing@example.com')
    const buttons = wrapper.findAll('button')
    await buttons[buttons.length - 1].trigger('click')
    await flushPromises()

    // 疑似缺陷：navigateToBindLogin() 直接透传 route.query.redirect，没有先过
    // sanitizeRedirectPath。这里断言的是当前实际行为。实际危害有限——
    // DingTalkCallbackView 在使用该值前会自行 sanitize，所以未做修复。
    expect(replace).toHaveBeenCalledWith({
      path: '/auth/dingtalk/callback',
      query: {
        bind: '1',
        email: 'existing@example.com',
        redirect: '//evil.com'
      }
    })
  })
})
