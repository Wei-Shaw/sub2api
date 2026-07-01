import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import GroupOptionItem from '../GroupOptionItem.vue'

const localeMessages = {
  en: {
    'usage.rate': 'Rate'
  },
  zh: {
    'usage.rate': '倍率'
  }
} as const

let currentLocale: keyof typeof localeMessages = 'en'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: keyof (typeof localeMessages)[typeof currentLocale]) => localeMessages[currentLocale][key] ?? key
  })
}))

const createWrapper = () => mount(GroupOptionItem, {
  props: {
    name: 'Test Group',
    platform: 'openai',
    rateMultiplier: 1
  },
  global: {
    stubs: {
      GroupBadge: {
        template: '<div class="group-badge-stub" />'
      }
    }
  }
})

describe('GroupOptionItem', () => {
  it('renders the rate label in English without hardcoded Chinese text', () => {
    currentLocale = 'en'

    const wrapper = createWrapper()

    expect(wrapper.text()).toContain('1x Rate')
    expect(wrapper.text()).not.toContain('倍率')
  })

  it('renders the rate label in Chinese for zh locale', () => {
    currentLocale = 'zh'

    const wrapper = createWrapper()

    expect(wrapper.text()).toContain('1x 倍率')
  })
})
