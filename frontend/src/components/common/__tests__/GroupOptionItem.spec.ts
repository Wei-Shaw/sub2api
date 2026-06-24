import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import GroupOptionItem from '../GroupOptionItem.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => (key === 'common.rateMultiplier' ? 'Hệ số giá' : key),
    }),
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ cachedPublicSettings: null }),
}))

describe('GroupOptionItem', () => {
  it('applies multiline and overflow-safe text styles', () => {
    const description = 'First section\nvery-long-unbroken-description-value-that-must-not-overflow'
    const wrapper = mount(GroupOptionItem, {
      props: {
        name: 'Example group',
        platform: 'openai',
        description,
      },
      global: {
        stubs: {
          GroupBadge: true,
        },
      },
    })

    const descriptionElement = wrapper
      .findAll('span')
      .find((element) => element.text() === description)

    expect(descriptionElement).toBeDefined()
    expect(descriptionElement?.classes()).toContain('whitespace-pre-line')
    expect(descriptionElement?.classes()).toContain('[overflow-wrap:anywhere]')
    expect(descriptionElement?.classes()).toContain('line-clamp-3')
    expect(wrapper.find('[title]').attributes('title')).toBe(description)
  })

  it('renders the translated rate label together with the numeric multiplier', () => {
    const wrapper = mount(GroupOptionItem, {
      props: {
        name: 'GPT Group',
        platform: 'openai',
        rateMultiplier: 0.2,
      },
      global: {
        stubs: {
          PlatformIcon: true,
        },
      },
    })

    expect(wrapper.text()).toContain('Hệ số giá')
    expect(wrapper.text()).toContain('0.2x')
    expect(wrapper.text()).toContain('Hệ số giá: 0.2x')
  })
})
