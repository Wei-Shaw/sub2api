import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    grok: {
      getCapabilities: vi.fn().mockResolvedValue({})
    }
  }
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copied: false, copyToClipboard: vi.fn() })
}))

import OAuthAuthorizationFlow from '../OAuthAuthorizationFlow.vue'

const mountFlow = (reauth: boolean) =>
  mount(OAuthAuthorizationFlow, {
    props: {
      addMethod: 'oauth',
      platform: 'openai',
      showCookieOption: false,
      showCodexSessionImportOption: true,
      initialInputMethod: 'codex_session',
      reauth
    },
    global: {
      stubs: { Icon: true }
    }
  })

describe('OAuthAuthorizationFlow codex session import', () => {
  it('uses create-account copy by default', () => {
    const wrapper = mountFlow(false)
    expect(wrapper.text()).toContain('admin.accounts.oauth.openai.codexSessionDesc')
    expect(wrapper.text()).toContain('admin.accounts.oauth.openai.codexSessionImportAndCreate')
  })

  it('uses update-account copy in reauth mode and emits the pasted content', async () => {
    const wrapper = mountFlow(true)
    expect(wrapper.text()).toContain('admin.accounts.oauth.openai.codexSessionReauthDesc')
    expect(wrapper.text()).toContain('admin.accounts.oauth.openai.codexSessionReauthSubmit')
    expect(wrapper.text()).not.toContain('codexSessionImportAndCreate')

    await wrapper.get('textarea').setValue('  {"tokens":{"access_token":"at"}}  ')
    const submit = wrapper
      .findAll('button')
      .find((b) => b.text().includes('codexSessionReauthSubmit'))
    expect(submit).toBeTruthy()
    await submit!.trigger('click')
    expect(wrapper.emitted('import-codex-session')).toEqual([['{"tokens":{"access_token":"at"}}']])
  })
})
