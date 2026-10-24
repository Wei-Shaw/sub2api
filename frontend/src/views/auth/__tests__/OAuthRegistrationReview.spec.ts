import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LinuxDoCallbackView from '../LinuxDoCallbackView.vue'
import OidcCallbackView from '../OidcCallbackView.vue'
import WechatCallbackView from '../WechatCallbackView.vue'
import DingTalkCallbackView from '../DingTalkCallbackView.vue'
import DingTalkEmailCompletionView from '../DingTalkEmailCompletionView.vue'

const state = vi.hoisted(() => ({
  exchange: vi.fn(), post: vi.fn(), replace: vi.fn(), setToken: vi.fn(), clear: vi.fn(),
  showSuccess: vi.fn(), showError: vi.fn(), settings: vi.fn()
}))
vi.mock('vue-router', () => ({ useRoute: () => ({ query: {} }), useRouter: () => ({ replace: state.replace }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))
vi.mock('@/api/client', () => ({ apiClient: { post: state.post } }))
vi.mock('@/api/auth', async () => ({
  ...await vi.importActual<typeof import('@/api/auth')>('@/api/auth'),
  exchangePendingOAuthCompletion: state.exchange,
  getPublicSettings: state.settings
}))
vi.mock('@/stores', () => ({
  useAuthStore: () => ({ setToken: state.setToken, setPendingAuthSession: vi.fn(), clearPendingAuthSession: state.clear }),
  useAppStore: () => ({ showSuccess: state.showSuccess, showError: state.showError, cachedPublicSettings: { registration_reason_max_length: 500, registration_review_instructions: "Old requirements" } })
}))
const options = { global: { stubs: {
  AuthLayout: { template: '<div><slot /></div>' }, Icon: true, RouterLink: true, transition: false
} } }
const providers = [
  ['linuxdo', LinuxDoCallbackView], ['oidc', OidcCallbackView],
  ['wechat', WechatCallbackView], ['dingtalk', DingTalkCallbackView]
] as const

describe('OAuth signup review', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    sessionStorage.clear()
    window.location.hash = ''
    state.settings.mockResolvedValue({ email_verify_enabled: false, registration_review_enabled: true })
    state.post.mockResolvedValue({ data: { pending_review: true } })
  })

  it.each(providers)('%s forwards a reason and sends pending accounts to login without authenticating', async (provider, component) => {
    state.exchange.mockResolvedValue({ error: 'create_account_required', registration_review_enabled: true })
    const wrapper = mount(component, options)
    await flushPromises()
    await wrapper.get(`[data-testid="${provider}-create-account-email"]`).setValue('applicant@example.com')
    await wrapper.get(`[data-testid="${provider}-create-account-password"]`).setValue('secret-123')
    await wrapper.get('textarea').setValue(' Team research ')
    expect(wrapper.get(`[data-testid="${provider}-create-account-submit"]`).attributes('disabled')).toBeDefined()
    await wrapper.get(`[data-testid="${provider}-create-account-verify-code"]`).setValue('246810')
    await wrapper.get(`[data-testid="${provider}-create-account-submit"]`).trigger('click')
    await flushPromises()
    expect(state.post).toHaveBeenCalledWith('/auth/oauth/pending/create-account', expect.objectContaining({ registration_reason: 'Team research', verify_code: '246810' }))
    expect(state.setToken).not.toHaveBeenCalled()
    expect(localStorage.getItem('refresh_token')).toBeNull()
    expect(state.showSuccess).toHaveBeenCalledWith('auth.registrationPendingReviewSuccess')
    expect(state.replace).toHaveBeenCalledWith('/login')
    expect(state.clear).toHaveBeenCalled()
    wrapper.unmount()
  })

  it.each(providers)('%s invitation completion accepts a reason and handles pending review', async (provider, component) => {
    state.exchange.mockResolvedValue({ error: 'invitation_required', registration_review_enabled: true })
    const wrapper = mount(component, options)
    await flushPromises()
    await wrapper.get('input[type="text"]').setValue('INVITE')
    await wrapper.get('textarea').setValue('Research')
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(state.post).toHaveBeenCalledWith(`/auth/oauth/${provider}/complete-registration`, expect.objectContaining({ registration_reason: 'Research' }))
    expect(state.setToken).not.toHaveBeenCalled()
    expect(state.showSuccess).toHaveBeenCalledWith('auth.registrationPendingReviewSuccess')
    expect(state.replace).toHaveBeenCalledWith('/login')
    wrapper.unmount()
  })

  it.each(providers)('%s invitation completion uses custom requirements and limits', async (provider, component) => {
    state.settings.mockResolvedValue({ registration_review_enabled: true, registration_review_instructions: 'Include your use case', registration_reason_max_length: 800 })
    state.exchange.mockResolvedValue({ error: 'invitation_required', registration_review_enabled: true })
    const wrapper = mount(component, options)
    await flushPromises()
    expect(wrapper.text()).toContain('Include your use case')
    await wrapper.get('input[type="text"]').setValue('INVITE')
    await wrapper.get('textarea').setValue('申'.repeat(600))
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(state.post).toHaveBeenCalledWith(`/auth/oauth/${provider}/complete-registration`, expect.objectContaining({ registration_reason: '申'.repeat(600) }))
    wrapper.unmount()
  })

  it('DingTalk email completion forwards reason and handles pending without a token', async () => {
    const wrapper = mount(DingTalkEmailCompletionView, options)
    await flushPromises()
    await wrapper.get('input[type="email"]').setValue('applicant@example.com')
    await wrapper.get('input[type="password"]').setValue('secret-123')
    await wrapper.get('textarea').setValue('Research')
    await wrapper.get('[data-testid="dingtalk-create-account-verify-code"]').setValue('246810')
    await wrapper.get('[data-testid="dingtalk-create-account-submit"]').trigger('click')
    await flushPromises()
    expect(state.post).toHaveBeenCalledWith('/auth/oauth/pending/create-account', expect.objectContaining({ registration_reason: 'Research', verify_code: '246810' }))
    expect(state.setToken).not.toHaveBeenCalled()
    expect(state.showSuccess).toHaveBeenCalledWith('auth.registrationPendingReviewSuccess')
    expect(state.replace).toHaveBeenCalledWith('/login')
    wrapper.unmount()
  })
})
