import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ProfileProxyOverrides from '../ProfileProxyOverrides.vue'
import type { CodexOSProfilePolicy } from '@/types/codexIdentity'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    te: () => true,
    t: (key: string) => key,
  }),
}))

const profile: CodexOSProfilePolicy = {
  os_class: 'windows',
  canonical_surface: 'desktop',
  architecture: 'x86_64',
  slot_count: 1,
  proxy_mode: 'inherit',
}

describe('ProfileProxyOverrides', () => {
  it('describes account inheritance generically while editing a reusable template', () => {
    const wrapper = mount(ProfileProxyOverrides, {
      props: {
        modelValue: profile,
        templateContext: true,
        idPrefix: 'template-proxy',
      },
      global: {
        stubs: { Select: true, Icon: true },
      },
    })

    expect(wrapper.text()).toContain('admin.accounts.codexIdentity.templateProxyInherit')
    expect(wrapper.text()).not.toContain('admin.accounts.codexIdentity.accountProxyDirect')
  })
})
