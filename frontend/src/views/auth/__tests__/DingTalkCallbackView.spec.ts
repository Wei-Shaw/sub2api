import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import DingTalkCallbackView from '../DingTalkCallbackView.vue'

const replace = vi.fn()
const showSuccess = vi.fn()
const showError = vi.fn()
const showInfo = vi.fn()
const setToken = vi.fn()
const setPendingAuthSession = vi.fn()
const clearPendingAuthSession = vi.fn()
const exchangePendingOAuthCompletion = vi.fn()
const getPublicSettings = vi.fn()
const login2FA = vi.fn()
const apiClientPost = vi.fn()
const sendPendingOAuthVerifyCode = vi.fn()

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
      t: (key: string, params?: Record<string, string>) => {
        if (key === 'auth.oauthFlow.totpHint') {
          return `verify ${params?.account ?? ''}`.trim()
        }
        return key
      },
      // 视图用 te() 判断 auth.dingtalk.error.<code> 是否有译文；这里一律视为缺失，
      // 让错误分支回落到 error_description / error 原文。
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
    exchangePendingOAuthCompletion: (...args: any[]) => exchangePendingOAuthCompletion(...args),
    getPublicSettings: (...args: any[]) => getPublicSettings(...args),
    login2FA: (...args: any[]) => login2FA(...args),
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

describe('DingTalkCallbackView', () => {
  beforeEach(() => {
    replace.mockReset()
    showSuccess.mockReset()
    showError.mockReset()
    showInfo.mockReset()
    setToken.mockReset()
    setPendingAuthSession.mockReset()
    clearPendingAuthSession.mockReset()
    exchangePendingOAuthCompletion.mockReset()
    getPublicSettings.mockReset()
    login2FA.mockReset()
    apiClientPost.mockReset()
    sendPendingOAuthVerifyCode.mockReset()
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

  it('accepts the legacy fragment token success callback and redirects to the sanitized path', async () => {
    window.location.hash =
      '#access_token=legacy-access-token&refresh_token=legacy-refresh-token&expires_in=3600&token_type=Bearer&redirect=%2Flegacy-dashboard'

    mount(DingTalkCallbackView, mountOptions)
    await flushPromises()

    expect(exchangePendingOAuthCompletion).not.toHaveBeenCalled()
    expect(setToken).toHaveBeenCalledWith('legacy-access-token')
    expect(localStorage.getItem('refresh_token')).toBe('legacy-refresh-token')
    expect(localStorage.getItem('token_expires_at')).not.toBeNull()
    expect(showSuccess).toHaveBeenCalledWith('auth.loginSuccess')
    expect(replace).toHaveBeenCalledWith('/legacy-dashboard')
  })

  it('ignores a protocol-relative redirect in the legacy fragment and falls back to the default path', async () => {
    window.location.hash = '#access_token=legacy-access-token&redirect=%2F%2Fevil.com'

    mount(DingTalkCallbackView, mountOptions)
    await flushPromises()

    expect(setToken).toHaveBeenCalledWith('legacy-access-token')
    expect(replace).toHaveBeenCalledWith('/dashboard')
    expect(replace).not.toHaveBeenCalledWith('//evil.com')
  })

  it('completes the pending oauth exchange and honours a relative redirect query', async () => {
    routeState.query = { redirect: '/usage' }
    exchangePendingOAuthCompletion.mockResolvedValue({
      access_token: 'access-token',
      refresh_token: 'refresh-token',
      expires_in: 3600
    })

    mount(DingTalkCallbackView, mountOptions)
    await flushPromises()

    expect(exchangePendingOAuthCompletion).toHaveBeenCalledTimes(1)
    expect(exchangePendingOAuthCompletion).toHaveBeenCalledWith()
    expect(setToken).toHaveBeenCalledWith('access-token')
    expect(showSuccess).toHaveBeenCalledWith('auth.loginSuccess')
    expect(replace).toHaveBeenCalledWith('/usage')
  })

  it('ignores a protocol-relative redirect query and falls back to the default path', async () => {
    routeState.query = { redirect: '//evil.com' }
    exchangePendingOAuthCompletion.mockResolvedValue({
      access_token: 'access-token'
    })

    mount(DingTalkCallbackView, mountOptions)
    await flushPromises()

    expect(setToken).toHaveBeenCalledWith('access-token')
    expect(replace).toHaveBeenCalledWith('/dashboard')
    expect(replace).not.toHaveBeenCalledWith('//evil.com')
  })

  it('treats a completion without token as bind success and returns to profile', async () => {
    exchangePendingOAuthCompletion.mockResolvedValue({})

    mount(DingTalkCallbackView, mountOptions)
    await flushPromises()

    expect(setToken).not.toHaveBeenCalled()
    expect(clearPendingAuthSession).toHaveBeenCalledTimes(1)
    expect(showSuccess).toHaveBeenCalledWith('profile.authBindings.bindSuccess')
    expect(replace).toHaveBeenCalledWith('/profile')
  })

  it('falls back to the global default instead of /profile when the bind redirect is unsafe', async () => {
    exchangePendingOAuthCompletion.mockResolvedValue({
      redirect: '//evil.com'
    })

    mount(DingTalkCallbackView, mountOptions)
    await flushPromises()

    expect(setToken).not.toHaveBeenCalled()
    expect(showSuccess).toHaveBeenCalledWith('profile.authBindings.bindSuccess')
    // sanitizeRedirectPath 的兜底值是 /dashboard，因此不安全的绑定 redirect 不会退回 /profile。
    expect(replace).toHaveBeenCalledWith('/dashboard')
  })

  it('shows the provider error from the fragment and stays on the page', async () => {
    window.location.hash = '#error=access_denied&error_description=User+denied+the+request'

    mount(DingTalkCallbackView, mountOptions)
    await flushPromises()

    expect(exchangePendingOAuthCompletion).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('User denied the request')
    expect(setToken).not.toHaveBeenCalled()
    expect(replace).not.toHaveBeenCalled()
  })

  it('reports a failed pending oauth exchange and clears the pending auth session', async () => {
    exchangePendingOAuthCompletion.mockRejectedValue(new Error('exchange failed'))

    mount(DingTalkCallbackView, mountOptions)
    await flushPromises()

    expect(clearPendingAuthSession).toHaveBeenCalledTimes(1)
    expect(showError).toHaveBeenCalledWith('exchange failed')
    expect(setToken).not.toHaveBeenCalled()
    expect(replace).not.toHaveBeenCalled()
  })

  it('renders the invitation form and persists the pending auth session', async () => {
    routeState.query = { redirect: '/usage' }
    exchangePendingOAuthCompletion.mockResolvedValue({
      error: 'invitation_required'
    })

    const wrapper = mount(DingTalkCallbackView, mountOptions)
    await flushPromises()

    expect(wrapper.find('input[type="text"]').exists()).toBe(true)
    expect(setPendingAuthSession).toHaveBeenCalledWith({
      token: '',
      token_field: 'pending_oauth_token',
      provider: 'dingtalk',
      redirect: '/usage'
    })
    expect(replace).not.toHaveBeenCalled()
  })

  it('submits the invitation code and finishes the login', async () => {
    routeState.query = { redirect: '/usage' }
    exchangePendingOAuthCompletion.mockResolvedValue({
      error: 'invitation_required'
    })
    apiClientPost.mockResolvedValue({
      data: {
        access_token: 'invite-access-token',
        refresh_token: 'invite-refresh-token',
        expires_in: 3600,
        token_type: 'Bearer'
      }
    })

    const wrapper = mount(DingTalkCallbackView, mountOptions)
    await flushPromises()

    await wrapper.find('input[type="text"]').setValue('invite-code')
    await wrapper.find('button').trigger('click')
    await flushPromises()

    expect(apiClientPost).toHaveBeenCalledWith('/auth/oauth/dingtalk/complete-registration', {
      pending_oauth_token: undefined,
      invitation_code: 'invite-code',
      adopt_display_name: false,
      adopt_avatar: false
    })
    expect(setToken).toHaveBeenCalledWith('invite-access-token')
    expect(replace).toHaveBeenCalledWith('/usage')
  })

  it('redirects to the email completion page with the sanitized redirect', async () => {
    routeState.query = { redirect: '/usage' }
    exchangePendingOAuthCompletion.mockResolvedValue({
      step: 'email_completion'
    })

    mount(DingTalkCallbackView, mountOptions)
    await flushPromises()

    expect(replace).toHaveBeenCalledWith('/auth/dingtalk/email-completion?redirect=%2Fusage')
    expect(setToken).not.toHaveBeenCalled()
  })

  it('ignores a protocol-relative redirect when routing to the email completion page', async () => {
    routeState.query = { redirect: '//evil.com' }
    exchangePendingOAuthCompletion.mockResolvedValue({
      requires_email_completion: true
    })

    mount(DingTalkCallbackView, mountOptions)
    await flushPromises()

    expect(replace).toHaveBeenCalledWith('/auth/dingtalk/email-completion?redirect=%2Fdashboard')
  })

  it('renders the bind-login form when returning from email completion with bind=1', async () => {
    routeState.query = { bind: '1', email: 'existing@example.com', redirect: '/usage' }
    exchangePendingOAuthCompletion.mockResolvedValue({
      step: 'email_completion'
    })

    const wrapper = mount(DingTalkCallbackView, mountOptions)
    await flushPromises()

    expect(replace).not.toHaveBeenCalled()
    expect((wrapper.get('[data-testid="dingtalk-bind-login-email"]').element as HTMLInputElement).value).toBe(
      'existing@example.com'
    )
    expect(setPendingAuthSession).toHaveBeenCalledWith({
      token: '',
      token_field: 'pending_oauth_token',
      provider: 'dingtalk',
      redirect: '/usage'
    })
  })

  it('submits the bind-login credentials and honours the sanitized completion redirect', async () => {
    routeState.query = { bind: '1', email: 'existing@example.com', redirect: '/usage' }
    exchangePendingOAuthCompletion.mockResolvedValue({
      step: 'email_completion'
    })
    apiClientPost.mockResolvedValue({
      data: {
        access_token: 'bind-access-token',
        refresh_token: 'bind-refresh-token',
        expires_in: 3600,
        token_type: 'Bearer',
        redirect: '//evil.com'
      }
    })

    const wrapper = mount(DingTalkCallbackView, mountOptions)
    await flushPromises()

    await wrapper.get('[data-testid="dingtalk-bind-login-password"]').setValue('secret-password')
    await wrapper.get('[data-testid="dingtalk-bind-login-submit"]').trigger('click')
    await flushPromises()

    expect(apiClientPost).toHaveBeenCalledWith('/auth/oauth/pending/bind-login', {
      email: 'existing@example.com',
      password: 'secret-password',
      adopt_display_name: false,
      adopt_avatar: false
    })
    expect(setToken).toHaveBeenCalledWith('bind-access-token')
    expect(replace).toHaveBeenCalledWith('/dashboard')
    expect(replace).not.toHaveBeenCalledWith('//evil.com')
  })
})
