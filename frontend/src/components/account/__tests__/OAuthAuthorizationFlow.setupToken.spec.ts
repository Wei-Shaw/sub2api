import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn() })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    grok: {
      getCapabilities: vi.fn().mockResolvedValue({ password_auth_enabled: false })
    }
  }
}))

import OAuthAuthorizationFlow from '../OAuthAuthorizationFlow.vue'

describe('OAuthAuthorizationFlow direct Claude setup-token mode', () => {
  it('replaces manual and Cookie authorization with direct token import', async () => {
    const wrapper = mount(OAuthAuthorizationFlow, {
      props: {
        addMethod: 'setup-token',
        platform: 'anthropic',
        showCookieOption: true,
        showManualOption: true,
        allowMultiple: false
      },
      global: {
        stubs: { Icon: true }
      }
    })

    expect(wrapper.text()).toContain('admin.accounts.oauth.setupTokenDirectDesc')
    expect(wrapper.text()).not.toContain('admin.accounts.oauth.cookieAutoAuthDesc')
    expect(wrapper.text()).not.toContain('admin.accounts.oauth.followSteps')

    await wrapper.get('textarea').setValue('sk-ant-oat01-test-token')
    await wrapper.get('button.btn-primary').trigger('click')

    expect(wrapper.emitted('import-setup-token')).toEqual([['sk-ant-oat01-test-token']])
    expect(wrapper.emitted('cookie-auth')).toBeUndefined()
  })
})
