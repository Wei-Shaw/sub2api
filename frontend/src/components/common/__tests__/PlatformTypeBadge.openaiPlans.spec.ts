import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import type { AccountPlatform } from '@/types'
import PlatformTypeBadge from '../PlatformTypeBadge.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

function mountPlan(platform: AccountPlatform, planType: string) {
  return mount(PlatformTypeBadge, {
    props: { platform, type: 'oauth', planType },
  })
}

describe('PlatformTypeBadge ChatGPT plan tiers', () => {
  it('labels pro / chatgptpro as Pro 20x with the Pro color', () => {
    for (const planType of ['pro', 'chatgptpro', 'PRO']) {
      const wrapper = mountPlan('openai', planType)

      expect(wrapper.text()).toContain('Pro 20x')
      expect(wrapper.html()).toContain('bg-violet-100')
    }
  })

  it('labels prolite as Pro 5x sharing the Pro color', () => {
    for (const planType of ['prolite', 'PROLITE', 'pro_lite']) {
      const wrapper = mountPlan('openai', planType)

      expect(wrapper.text()).toContain('Pro 5x')
      expect(wrapper.html()).toContain('bg-violet-100')
      expect(wrapper.text()).not.toContain('Pro 20x')
    }
  })

  it('labels team as Business Standard with the Team color', () => {
    const wrapper = mountPlan('openai', 'team')

    expect(wrapper.text()).toContain('Business Standard')
    expect(wrapper.html()).toContain('bg-indigo-100')
  })

  it('labels self_serve_business_prolite as Business Premium sharing the Team color', () => {
    for (const planType of ['self_serve_business_prolite', 'selfservebusinessprolite']) {
      const wrapper = mountPlan('openai', planType)

      expect(wrapper.text()).toContain('Business Premium')
      expect(wrapper.html()).toContain('bg-indigo-100')
      expect(wrapper.text()).not.toContain('self_serve_business_prolite')
    }
  })

  it('keeps plus, free and abnormal labels unchanged', () => {
    const plus = mountPlan('openai', 'plus')
    expect(plus.text()).toContain('Plus')
    expect(plus.html()).toContain('bg-sky-100')

    const free = mountPlan('openai', 'free')
    expect(free.text()).toContain('Free')
    expect(free.html()).toContain('bg-gray-100')

    const abnormal = mountPlan('openai', 'abnormal')
    expect(abnormal.text()).toContain('admin.accounts.subscriptionAbnormal')
    expect(abnormal.html()).toContain('bg-red-100')
  })

  it('falls back to the raw value for unknown plans', () => {
    const wrapper = mountPlan('openai', 'enterprise')

    expect(wrapper.text()).toContain('enterprise')
    expect(wrapper.html()).not.toContain('bg-violet-100')
    expect(wrapper.html()).not.toContain('bg-indigo-100')
  })

  it('does not apply the ChatGPT tier naming to other platforms', () => {
    // Antigravity 的 Pro 与 Grok 的 pro 是各自产品线的档位，不能显示成 Pro 20x。
    for (const platform of ['antigravity', 'grok'] as AccountPlatform[]) {
      const wrapper = mountPlan(platform, 'pro')

      expect(wrapper.text()).toContain('Pro')
      expect(wrapper.text()).not.toContain('Pro 20x')
    }

    const antigravityTeam = mountPlan('antigravity', 'team')
    expect(antigravityTeam.text()).toContain('Team')
    expect(antigravityTeam.text()).not.toContain('Business Standard')
  })
})
