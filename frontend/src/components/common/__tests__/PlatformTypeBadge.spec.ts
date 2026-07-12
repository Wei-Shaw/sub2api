import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import PlatformTypeBadge from '../PlatformTypeBadge.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key === 'admin.accounts.status.overageActive' ? 'Overage' : key
    })
  }
})

describe('PlatformTypeBadge', () => {
  it('uses Kiro theme instead of Anthropic orange theme', () => {
    const wrapper = mount(PlatformTypeBadge, {
      props: {
        platform: 'kiro',
        type: 'oauth',
        planType: 'KIRO FREE'
      }
    })

    expect(wrapper.html()).toContain('bg-violet-100')
    expect(wrapper.html()).toContain('text-violet-700')
    expect(wrapper.html()).not.toContain('bg-orange-100')
  })

  it('shows Kiro overages tag next to the plan tag when enabled', () => {
    const wrapper = mount(PlatformTypeBadge, {
      props: {
        platform: 'kiro',
        type: 'oauth',
        planType: 'KIRO PRO+',
        overagesEnabled: true
      }
    })

    expect(wrapper.text()).toContain('KIRO PRO+')
    expect(wrapper.text()).toContain('Overage')
  })

  it('does not show overages tag for non-Kiro accounts', () => {
    const wrapper = mount(PlatformTypeBadge, {
      props: {
        platform: 'openai',
        type: 'oauth',
        planType: 'Pro',
        overagesEnabled: true
      }
    })

    expect(wrapper.text()).not.toContain('Overage')
  })
})
