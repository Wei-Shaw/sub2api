import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'
import type * as VueI18n from 'vue-i18n'
import type { Account, AccountPlatform } from '@/types'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  apply: vi.fn(), refresh: vi.fn(), importToken: vi.fn(),
  generateClaudeAuthUrl: vi.fn(), generateGeminiAuthUrl: vi.fn(),
  showError: vi.fn(), showSuccess: vi.fn()
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => mocks }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof VueI18n>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))
vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: { applyOAuthCredentials: mocks.apply, generateAuthUrl: mocks.generateClaudeAuthUrl },
    cursor: { refreshCursorToken: mocks.refresh },
    devin: { importToken: mocks.importToken },
    gemini: { generateAuthUrl: mocks.generateGeminiAuthUrl },
    grok: { getCapabilities: vi.fn().mockResolvedValue({ password_auth_enabled: false }) }
  }
}))

import ReAuthAccountModal from '../ReAuthAccountModal.vue'
import OAuthAuthorizationFlow from '@/components/account/OAuthAuthorizationFlow.vue'

const BaseDialog = defineComponent({
  props: { show: Boolean }, template: '<div v-if="show"><slot/><slot name="footer"/></div>'
})
const mounted: VueWrapper[] = []
function mountModal(platform: AccountPlatform | null) {
  const account = platform === null ? null : { id: 42, name: 'Existing', platform, type: 'oauth', proxy_id: null, credentials: { refresh_token: 'stored' } } as Account
  const wrapper = mount(ReAuthAccountModal, {
    props: { show: platform !== null, account },
    global: {
      stubs: { BaseDialog, Icon: true },
      mocks: { $t: (key: string) => key }
    }
  })
  mounted.push(wrapper)
  return wrapper
}
beforeEach(() => {
  vi.clearAllMocks()
  mocks.apply.mockReset()
  mocks.refresh.mockReset().mockImplementation(async (token: string) => ({ access_token: `access:${token}`, refresh_token: token }))
  mocks.importToken.mockReset().mockImplementation(async (token: string) => ({ access_token: `devin-session-token$${token}` }))
  mocks.generateClaudeAuthUrl.mockReset().mockResolvedValue({ auth_url: 'https://example.invalid/claude', session_id: 'claude-session' })
  mocks.generateGeminiAuthUrl.mockReset().mockResolvedValue({ auth_url: 'https://example.invalid/gemini', session_id: 'gemini-session', state: 'state' })
})
afterEach(() => { mounted.splice(0).forEach(wrapper => wrapper.unmount()) })

describe('Account reauthorization', () => {
  it('restores Claude setup-token when the account and visibility change together', async () => {
    const wrapper = mountModal(null)
    const account = { id: 43, name: 'Saved setup token', platform: 'anthropic', type: 'setup-token', credentials: {}, proxy_id: null } as Account
    await wrapper.setProps({ show: true, account })

    expect((wrapper.get('input[value="setup-token"]').element as HTMLInputElement).checked).toBe(true)
    expect((wrapper.get('input[value="oauth"]').element as HTMLInputElement).checked).toBe(false)
    wrapper.getComponent(OAuthAuthorizationFlow).vm.$emit('generate-url')
    await flushPromises()
    expect(mocks.generateClaudeAuthUrl).toHaveBeenCalledWith('/admin/accounts/generate-setup-token-url', {})
  })

  it.each(['google_one', 'ai_studio'] as const)('restores Gemini %s when opening an existing account', async (oauthType) => {
    const wrapper = mountModal(null)
    const account = { id: 44, name: 'Saved Gemini', platform: 'gemini', type: 'oauth', credentials: { oauth_type: oauthType }, proxy_id: null } as Account
    await wrapper.setProps({ show: true, account })

    expect(wrapper.text()).toContain(oauthType === 'google_one' ? 'Google One' : 'admin.accounts.gemini.oauthType.customTitle')
    wrapper.getComponent(OAuthAuthorizationFlow).vm.$emit('generate-url')
    await flushPromises()
    expect(mocks.generateGeminiAuthUrl).toHaveBeenCalledWith({ oauth_type: oauthType })
  })

  it('keeps Grok refresh-token and SSO methods without offering password auth', async () => {
    const wrapper = mountModal('grok')
    const flow = wrapper.getComponent(OAuthAuthorizationFlow)
    expect((flow.get('input[value="refresh_token"]').element as HTMLInputElement).checked).toBe(true)
    expect(flow.find('input[value="email_password"]').exists()).toBe(false)
    await flow.get('input[value="sso_cookie"]').setValue()
    expect(flow.find('textarea').exists()).toBe(true)
    expect(wrapper.findAll('button').some(button => button.text() === 'admin.accounts.oauth.completeAuth')).toBe(false)
  })

  it('preserves entered tokens and pending authorization during a same-ID metadata refresh', async () => {
    const pendingToken = Promise.withResolvers<{ access_token: string; refresh_token: string }>()
    mocks.refresh.mockReturnValueOnce(pendingToken.promise)
    mocks.apply.mockResolvedValueOnce({ id: 42, platform: 'cursor' })
    const wrapper = mountModal('cursor')
    const flow = wrapper.getComponent(OAuthAuthorizationFlow)
    await flow.get('input[value="refresh_token"]').setValue()
    await flow.get('textarea').setValue('entered-refresh-token')
    const submit = flow.get('button')
    await submit.trigger('click')
    await flushPromises()

    const account = wrapper.props('account')!
    await wrapper.setProps({ account: { ...account, name: 'Refreshed account metadata', credentials: { ...account.credentials } } })
    expect(wrapper.text()).toContain('Refreshed account metadata')
    expect((flow.get('input[value="refresh_token"]').element as HTMLInputElement).checked).toBe(true)
    expect((flow.get('textarea').element as HTMLTextAreaElement).value).toBe('entered-refresh-token')
    expect((submit.element as HTMLButtonElement).disabled).toBe(true)

    pendingToken.resolve({ access_token: 'accepted-access-token', refresh_token: 'rotated-refresh-token' })
    await flushPromises()
    expect(mocks.apply).toHaveBeenCalledTimes(1)
    expect(wrapper.emitted('reauthorized')).toHaveLength(1)
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('still invalidates pending authorization when a different account replaces the open account', async () => {
    const pendingToken = Promise.withResolvers<{ access_token: string }>()
    mocks.refresh.mockReturnValueOnce(pendingToken.promise)
    const wrapper = mountModal('cursor')
    const flow = wrapper.getComponent(OAuthAuthorizationFlow)
    await flow.get('input[value="refresh_token"]').setValue()
    await flow.get('textarea').setValue('old-account-token')
    await flow.get('button').trigger('click')
    await flushPromises()

    await wrapper.setProps({ account: { ...wrapper.props('account')!, id: 43, name: 'Different account' } })
    pendingToken.resolve({ access_token: 'obsolete-access-token' })
    await flushPromises()
    expect(wrapper.text()).toContain('Different account')
    expect((flow.get('input[value="manual"]').element as HTMLInputElement).checked).toBe(true)
    expect(mocks.apply).not.toHaveBeenCalled()
    expect(wrapper.emitted('reauthorized')).toBeUndefined()
    expect(wrapper.emitted('close')).toBeUndefined()
  })

  for (const platform of ['cursor', 'devin'] as const) {
    it(`shows ${platform} persistence errors and retains the input for retry`, async () => {
      const pendingSave = Promise.withResolvers<Account>()
      mocks.apply.mockReturnValueOnce(pendingSave.promise)
      const wrapper = mountModal(platform)
      const flow = wrapper.getComponent(OAuthAuthorizationFlow)
      await flow.get('input[value="refresh_token"]').setValue()
      await flow.get('textarea').setValue('first\nsecond')
      const submit = flow.get('button')
      await submit.trigger('click')
      await flushPromises()
      expect((submit.element as HTMLButtonElement).disabled).toBe(true)
      expect(mocks.apply.mock.calls[0][0]).toBe(42)
      expect(mocks.apply.mock.calls[0][1].credentials.access_token).toBe(platform === 'cursor' ? 'access:first' : 'devin-session-token$first')
      pendingSave.reject({ message: 'Credential storage unavailable' })
      await flushPromises()
      expect(flow.text()).toContain('Credential storage unavailable')
      expect((submit.element as HTMLButtonElement).disabled).toBe(false)
      expect((flow.get('textarea').element as HTMLTextAreaElement).value).toBe('first\nsecond')
      expect(wrapper.emitted('reauthorized')).toBeUndefined()
      expect(wrapper.emitted('close')).toBeUndefined()
      expect(mocks.showSuccess).not.toHaveBeenCalled()

      mocks.apply.mockResolvedValueOnce({ id: 42, platform })
      await submit.trigger('click')
      await flushPromises()
      expect(wrapper.emitted('reauthorized')).toHaveLength(1)
      expect(wrapper.emitted('close')).toHaveLength(1)
    })
  }
})
