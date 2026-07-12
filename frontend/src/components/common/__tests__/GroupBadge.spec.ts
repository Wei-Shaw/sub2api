import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import GroupBadge from '../GroupBadge.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

describe('GroupBadge', () => {
  it('Kiro 分组徽章使用 Kiro 紫色主题', () => {
    const wrapper = mount(GroupBadge, {
      props: {
        name: 'kiro free',
        platform: 'kiro',
        rateMultiplier: 1
      }
    })

    expect(wrapper.html()).toContain('bg-violet-50')
    expect(wrapper.html()).toContain('text-violet-700')
    expect(wrapper.html()).not.toContain('text-amber-700')
  })
})
