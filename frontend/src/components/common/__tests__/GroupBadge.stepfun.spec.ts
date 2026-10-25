import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import GroupBadge from '../GroupBadge.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ cachedPublicSettings: undefined }),
}))

describe('GroupBadge StepFun', () => {
  it('renders the StepFun emerald badge and subscription label colors', () => {
    const wrapper = mount(GroupBadge, {
      props: {
        name: 'StepFun Group',
        platform: 'stepfun',
        subscriptionType: 'subscription',
      },
      global: {
        stubs: { PlatformIcon: true },
      },
    })

    expect(wrapper.classes()).toContain('bg-emerald-100')
    expect(wrapper.html()).toContain('bg-emerald-200/60')
  })
})
