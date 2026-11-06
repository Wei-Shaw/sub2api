import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import PlatformTypeBadge from '../PlatformTypeBadge.vue'

describe('Antigravity plan badge', () => {
  it.each([
    ['g1-ultra-lite-tier', 'Ultra 5x', 'text-purple-700'],
    ['g1-ultra-tier', 'Ultra 20x', 'text-amber-800']
  ])('distinguishes %s using the reported tier ID', (planType, label, color) => {
    const wrapper = mount(PlatformTypeBadge, {
      props: { platform: 'antigravity', type: 'oauth', planType },
      global: { plugins: [createI18n({ legacy: false, locale: 'en', messages: { en: {} } })] }
    })
    expect(wrapper.text()).toContain(label)
    expect(wrapper.find(`.${color}`).exists()).toBe(true)
    wrapper.unmount()
  })

  it('does not guess 20x from a legacy undifferentiated Ultra plan', () => {
    const wrapper = mount(PlatformTypeBadge, {
      props: { platform: 'antigravity', type: 'oauth', planType: 'Ultra' },
      global: { plugins: [createI18n({ legacy: false, locale: 'en', messages: { en: {} } })] }
    })
    expect(wrapper.text()).toContain('Ultra')
    expect(wrapper.text()).not.toContain('20x')
    expect(wrapper.find('.text-amber-800').exists()).toBe(false)
    wrapper.unmount()
  })
})
