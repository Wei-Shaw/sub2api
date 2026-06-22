import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'

import GroupOptionItem from '../GroupOptionItem.vue'
import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'

const currentLocale = ref<'en' | 'zh'>('en')

function getMessage(messages: Record<string, any>, key: string): string {
  return key.split('.').reduce<any>((value, segment) => value?.[segment], messages) ?? key
}

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => getMessage(currentLocale.value === 'zh' ? zh : en, key),
    locale: currentLocale
  })
}))

describe('GroupOptionItem', () => {
  it('renders the rate pill in English without hardcoded Chinese text', () => {
    currentLocale.value = 'en'

    const wrapper = mount(GroupOptionItem, {
      props: {
        name: 'OpenAI',
        platform: 'openai',
        rateMultiplier: 1
      },
      global: {
        stubs: {
          GroupBadge: {
            template: '<span class="group-badge-stub">{{ $attrs.name }}</span>'
          }
        }
      }
    })

    expect(wrapper.text()).toContain('1x Rate')
    expect(wrapper.text()).not.toContain('倍率')
  })

  it('renders the localized Chinese multiplier label', () => {
    currentLocale.value = 'zh'

    const wrapper = mount(GroupOptionItem, {
      props: {
        name: 'OpenAI',
        platform: 'openai',
        rateMultiplier: 1
      },
      global: {
        stubs: {
          GroupBadge: {
            template: '<span class="group-badge-stub">{{ $attrs.name }}</span>'
          }
        }
      }
    })

    expect(wrapper.text()).toContain('1x 倍率')
  })
})
