import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import DingTalkCallbackView from '../DingTalkCallbackView.vue'

const replace = vi.fn()
const showSuccess = vi.fn()
const showError = vi.fn()
const setToken = vi.fn()
const setPendingAuthSession = vi.fn()
const clearPendingAuthSession = vi.fn()
const exchangePendingOAuthCompletion = vi.fn()
const apiClientPost = vi.fn()
let sessionVersion = 0
let currentToken: string | null = null

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ replace })
}))

vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key, te: () => false })
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => ({ setToken, setPendingAuthSession, clearPendingAuthSession, get authSessionVersion() { return sessionVersion }, get token() { return currentToken } }),
  useAppStore: () => ({ showSuccess, showError })
}))

vi.mock('@/api/client', () => ({
  apiClient: { post: (...args: unknown[]) => apiClientPost(...args) }
}))

vi.mock('@/api/auth', async () => ({
  ...await vi.importActual<typeof import('@/api/auth')>('@/api/auth'),
  exchangePendingOAuthCompletion: (...args: unknown[]) => exchangePendingOAuthCompletion(...args)
}))

const pendingSignup = {
  redirect: '/dashboard',
  synthetic_email: 'dingtalk-test@dingtalk.local',
  adoption_required: false,
  suggested_display_name: ''
}

const tokens = {
  access_token: 'test-access-token',
  refresh_token: 'test-refresh-token',
  expires_in: 3600,
  token_type: 'Bearer'
}

function mountCallback() {
  return mount(DingTalkCallbackView, {
    global: {
      stubs: {
        AuthLayout: { template: '<div><slot /></div>' },
        Icon: true,
        RouterLink: { template: '<a><slot /></a>' },
        transition: false
      }
    }
  })
}

describe('DingTalkCallbackView no-email registration', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    window.location.hash = ''
    localStorage.clear()
    sessionStorage.clear()
    sessionVersion = 0
    currentToken = null
    exchangePendingOAuthCompletion.mockResolvedValue({ ...pendingSignup })
    apiClientPost.mockResolvedValue({ data: { ...tokens } })
    setToken.mockImplementation(async (_token, context) => {
      currentToken = _token
      localStorage.setItem('auth_token', _token)
      if (context) {
        localStorage.setItem('refresh_token', context.refreshToken)
        localStorage.setItem('token_expires_at', String(Date.now() + context.expiresIn * 1000))
        sessionVersion++
      }
      return {}
    })
  })

  it('completes a new cookie-bound no-email signup and enters the dashboard without email/password input', async () => {
    const wrapper = mountCallback()
    await flushPromises()

    expect(exchangePendingOAuthCompletion).toHaveBeenCalledTimes(1)
    expect(exchangePendingOAuthCompletion).toHaveBeenCalledWith()
    expect(apiClientPost).toHaveBeenCalledTimes(1)
    expect(apiClientPost).toHaveBeenCalledWith('/auth/oauth/dingtalk/complete-registration', {
      adopt_display_name: false,
      adopt_avatar: false
    })
    expect(setToken).toHaveBeenCalledTimes(1)
    expect(setToken).toHaveBeenCalledWith(tokens.access_token, { refreshToken: tokens.refresh_token, expiresIn: tokens.expires_in, expectedSessionVersion: 0 })
    expect(localStorage.getItem('refresh_token')).toBe(tokens.refresh_token)
    expect(Number(localStorage.getItem('token_expires_at'))).toBeGreaterThan(Date.now())
    expect(replace).toHaveBeenCalledTimes(1)
    expect(replace).toHaveBeenCalledWith('/dashboard')
    expect(showSuccess).toHaveBeenCalledTimes(1)
    expect(showSuccess).toHaveBeenCalledWith('auth.loginSuccess')
    expect(wrapper.find('input[type="email"]').exists()).toBe(false)
    expect(wrapper.find('input[type="password"]').exists()).toBe(false)
  })

  it('does not register again when an existing account already has login tokens', async () => {
    exchangePendingOAuthCompletion.mockResolvedValue({ ...pendingSignup, ...tokens, adoption_required: false })
    mountCallback()
    await flushPromises()

    expect(apiClientPost).not.toHaveBeenCalled()
    expect(setToken).toHaveBeenCalledWith(tokens.access_token)
    expect(replace).toHaveBeenCalledWith('/dashboard')
  })

  it.each([undefined, '', '   ', 42])('does not initiate signup for an invalid synthetic marker: %s', async marker => {
    exchangePendingOAuthCompletion.mockResolvedValue({ ...pendingSignup, synthetic_email: marker })
    mountCallback()
    await flushPromises()

    expect(apiClientPost).not.toHaveBeenCalled()
    expect(setToken).not.toHaveBeenCalled()
    expect(showSuccess).not.toHaveBeenCalledWith('auth.loginSuccess')
    expect(replace).not.toHaveBeenCalledWith('/dashboard')
  })

  it.each(['bind_login', 'choose_account_action', 'create_account'])('preserves explicit %s account actions instead of auto-registering', async step => {
    exchangePendingOAuthCompletion.mockResolvedValue({ ...pendingSignup, step })
    mountCallback()
    await flushPromises()

    expect(apiClientPost).not.toHaveBeenCalled()
    expect(setToken).not.toHaveBeenCalled()
    expect(setPendingAuthSession).toHaveBeenCalled()
    expect(replace).not.toHaveBeenCalled()
  })

  it('preserves the email-completion flow', async () => {
    exchangePendingOAuthCompletion.mockResolvedValue({ ...pendingSignup, step: 'email_completion' })
    mountCallback()
    await flushPromises()

    expect(apiClientPost).not.toHaveBeenCalled()
    expect(setToken).not.toHaveBeenCalled()
    expect(replace).toHaveBeenCalledTimes(1)
    expect(replace).toHaveBeenCalledWith('/auth/dingtalk/email-completion?redirect=%2Fdashboard')
  })

  it('does not bypass invitation requirements', async () => {
    exchangePendingOAuthCompletion.mockResolvedValue({ ...pendingSignup, error: 'invitation_required' })
    const wrapper = mountCallback()
    await flushPromises()

    expect(apiClientPost).not.toHaveBeenCalled()
    expect(setToken).not.toHaveBeenCalled()
    expect(replace).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('auth.dingtalk.invitationRequired')
  })

  it('keeps the pending session and lets the user submit an invitation required by the registration endpoint', async () => {
    apiClientPost.mockRejectedValueOnce({
      status: 403,
      reason: 'OAUTH_INVITATION_REQUIRED',
      message: 'invitation code required to complete oauth registration'
    })
    const wrapper = mountCallback()
    await flushPromises()

    expect(wrapper.text()).toContain('auth.dingtalk.invitationRequired')
    expect(clearPendingAuthSession).not.toHaveBeenCalled()
    expect(setPendingAuthSession).toHaveBeenCalledWith(expect.objectContaining({ provider: 'dingtalk' }))
    expect(setToken).not.toHaveBeenCalled()
    expect(replace).not.toHaveBeenCalled()
    expect(showError).not.toHaveBeenCalled()

    await wrapper.find('input[type="text"]').setValue(' test-invitation ')
    await wrapper.find('button').trigger('click')
    await flushPromises()

    expect(apiClientPost).toHaveBeenCalledTimes(2)
    expect(apiClientPost).toHaveBeenLastCalledWith('/auth/oauth/dingtalk/complete-registration', {
      pending_oauth_token: undefined,
      invitation_code: 'test-invitation',
      adopt_display_name: false,
      adopt_avatar: false
    })
    expect(setToken).toHaveBeenCalledWith(tokens.access_token, { refreshToken: tokens.refresh_token, expiresIn: tokens.expires_in, expectedSessionVersion: 0 })
    expect(replace).toHaveBeenCalledWith('/dashboard')
  })

  it('preserves the two-factor challenge without registering or logging in', async () => {
    exchangePendingOAuthCompletion.mockResolvedValue({ ...pendingSignup, requires_2fa: true, temp_token: 'test-challenge' })
    mountCallback()
    await flushPromises()

    expect(apiClientPost).not.toHaveBeenCalled()
    expect(setToken).not.toHaveBeenCalled()
    expect(setPendingAuthSession).toHaveBeenCalled()
    expect(replace).not.toHaveBeenCalled()
  })

  it('shows a registration failure and never issues a login success or navigation', async () => {
    apiClientPost.mockRejectedValue({ response: { data: { message: 'Registration rejected' } } })
    mountCallback()
    await flushPromises()

    expect(apiClientPost).toHaveBeenCalledTimes(1)
    expect(setToken).not.toHaveBeenCalled()
    expect(localStorage.getItem('refresh_token')).toBeNull()
    expect(showSuccess).not.toHaveBeenCalled()
    expect(replace).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('Registration rejected')
  })

  it('handles another pending step from the server without repeating signup', async () => {
    apiClientPost.mockResolvedValue({ data: { ...pendingSignup, step: 'bind_login' } })
    const wrapper = mountCallback()
    await flushPromises()

    expect(apiClientPost).toHaveBeenCalledTimes(1)
    expect(setToken).not.toHaveBeenCalled()
    expect(showSuccess).not.toHaveBeenCalled()
    expect(replace).not.toHaveBeenCalled()
    expect(wrapper.find('input[type="password"]').exists()).toBe(true)
    expect(setPendingAuthSession).toHaveBeenCalled()
  })

  it('sanitizes an external redirect after successful registration', async () => {
    exchangePendingOAuthCompletion.mockResolvedValue({ ...pendingSignup, redirect: '//outside.example' })
    apiClientPost.mockResolvedValue({ data: { ...tokens, redirect: 'https://outside.example' } })
    mountCallback()
    await flushPromises()

    expect(setToken).toHaveBeenCalledWith(tokens.access_token, { refreshToken: tokens.refresh_token, expiresIn: tokens.expires_in, expectedSessionVersion: 0 })
    expect(replace).toHaveBeenCalledTimes(1)
    expect(replace).toHaveBeenCalledWith('/dashboard')
  })

  it.each([true, false])('requires profile confirmation and preserves adoption choice=%s', async adopt => {
    exchangePendingOAuthCompletion.mockResolvedValue({ ...pendingSignup, adoption_required: true, suggested_display_name: 'Test User', suggested_avatar_url: 'https://example.test/avatar.png' })
    const wrapper = mountCallback()
    await flushPromises()
    expect(apiClientPost).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('auth.oauthFlow.reviewProfileBeforeContinue')
    for (const box of wrapper.findAll('input[type="checkbox"]')) await box.setValue(adopt)
    await wrapper.find('button').trigger('click')
    await flushPromises()
    expect(exchangePendingOAuthCompletion).toHaveBeenCalledTimes(1)
    expect(apiClientPost).toHaveBeenCalledWith('/auth/oauth/dingtalk/complete-registration', { adopt_display_name: adopt, adopt_avatar: adopt })
    expect(replace).toHaveBeenCalledWith('/dashboard')
  })

  it('keeps a rejected profile choice on an invitation retry', async () => {
    exchangePendingOAuthCompletion.mockResolvedValue({ ...pendingSignup, adoption_required: true, suggested_display_name: 'Test User' })
    apiClientPost.mockRejectedValueOnce({ reason: 'OAUTH_INVITATION_REQUIRED' })
    const wrapper = mountCallback()
    await flushPromises()
    await wrapper.find('input[type="checkbox"]').setValue(false)
    await wrapper.find('button').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('auth.dingtalk.invitationRequired')
    await wrapper.find('input[type="text"]').setValue('test-invite')
    await wrapper.find('button').trigger('click')
    await flushPromises()
    expect(apiClientPost).toHaveBeenLastCalledWith('/auth/oauth/dingtalk/complete-registration', expect.objectContaining({ invitation_code: 'test-invite', adopt_display_name: false, adopt_avatar: false }))
    expect(replace).toHaveBeenCalledWith('/dashboard')
  })

  it.each([{}, { redirect: '/dashboard' }, { auth_result: 'bind_success' }, { requires_2fa: true }])('does not report login or bind success for a registration response without tokens: %j', async result => {
    apiClientPost.mockResolvedValue({ data: result })
    mountCallback()
    await flushPromises()
    expect(setToken).not.toHaveBeenCalled()
    expect(showSuccess).not.toHaveBeenCalled()
    expect(replace).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('auth.dingtalk.callbackMissingToken')
  })

  it.each(['automatic', 'confirmation', 'invitation'])('does not overwrite a later session while %s registration is pending', async flow => {
    if (flow === 'confirmation') exchangePendingOAuthCompletion.mockResolvedValue({ ...pendingSignup, adoption_required: true, suggested_display_name: 'Test User' })
    if (flow === 'invitation') exchangePendingOAuthCompletion.mockResolvedValue({ ...pendingSignup, error: 'invitation_required' })
    let finish!: (v: unknown) => void
    apiClientPost.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const wrapper = mountCallback()
    await flushPromises()
    if (flow !== 'automatic') {
      if (flow === 'invitation') await wrapper.find('input[type="text"]').setValue('test-invite')
      await wrapper.find('button').trigger('click')
      await flushPromises()
    }
    expect(apiClientPost).toHaveBeenCalledTimes(1)
    sessionVersion++
    localStorage.setItem('auth_token', 'later-access')
    localStorage.setItem('refresh_token', 'later-refresh')
    localStorage.setItem('token_expires_at', '9999999999999')
    localStorage.setItem('pending_auth_session', 'later-pending')
    localStorage.setItem('oauth_aff_code', 'later-referral')
    const saved = { ...localStorage }
    clearPendingAuthSession.mockClear()
    finish({ data: tokens })
    await flushPromises()
    expect(setToken).not.toHaveBeenCalled()
    expect({ ...localStorage }).toEqual(saved)
    expect(clearPendingAuthSession).not.toHaveBeenCalled()
    expect(showSuccess).not.toHaveBeenCalled()
    expect(replace).not.toHaveBeenCalled()
  })

  it('ignores an obsolete initial exchange without clearing a later pending session', async () => {
    let finish!: (v: unknown) => void
    exchangePendingOAuthCompletion.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    mountCallback()
    sessionVersion++
    finish(pendingSignup)
    await flushPromises()
    expect(apiClientPost).not.toHaveBeenCalled()
    expect(clearPendingAuthSession).not.toHaveBeenCalled()
    expect(setToken).not.toHaveBeenCalled()
    expect(replace).not.toHaveBeenCalled()
  })

  it('preserves later pending state when an obsolete initial exchange fails', async () => {
    let fail!: (v: unknown) => void
    exchangePendingOAuthCompletion.mockImplementationOnce(() => new Promise((_, reject) => { fail = reject }))
    mountCallback()
    sessionVersion++
    localStorage.setItem('pending_auth_session', 'later-pending')
    fail(new Error('Network failure'))
    await flushPromises()
    expect(clearPendingAuthSession).not.toHaveBeenCalled()
    expect(localStorage.getItem('pending_auth_session')).toBe('later-pending')
  })

  it('shows a genuine token publication failure instead of leaving processing indefinitely', async () => {
    setToken.mockRejectedValue({ status: 401, code: 'INVALID_TOKEN', message: 'Invalid token' })
    const wrapper = mountCallback()
    await flushPromises()
    expect(showError).toHaveBeenCalledWith('Invalid token')
    expect(wrapper.text()).not.toContain('auth.dingtalk.callbackProcessing')
    expect(showSuccess).not.toHaveBeenCalled()
    expect(replace).not.toHaveBeenCalled()
  })

  it('preserves a new pending session when guarded token publication rejects it as obsolete', async () => {
    setToken.mockRejectedValue({ code: 'AUTH_SESSION_CHANGED' })
    mountCallback()
    await flushPromises()
    expect(clearPendingAuthSession).not.toHaveBeenCalled()
    expect(showSuccess).not.toHaveBeenCalled()
    expect(replace).not.toHaveBeenCalled()
  })
})
