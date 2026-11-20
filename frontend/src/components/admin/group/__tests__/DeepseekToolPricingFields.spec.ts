import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { describe, expect, it } from 'vitest'
import type { ChannelModelPricing } from '@/api/admin/channels'
import en from '@/i18n/locales/en/admin/overview'
import zh from '@/i18n/locales/zh/admin/overview'
import DeepseekToolPricingFields from '../DeepseekToolPricingFields.vue'
import { splitDeepseekToolPrices } from '../deepseekToolPricing'

describe('DeepseekToolPricingFields', () => {
  it.each(['en', 'zh'] as const)('shows both official price lists and identifies existing pricing rules (%s)', locale => {
    const wildcard: ChannelModelPricing = {
      platform: 'deepseek', models: ['DEEPSEEK-TOOLS-T*'], billing_mode: 'per_request', per_request_price: 0.25,
      input_price: null, output_price: null, image_input_price: null, image_output_price: null,
      cache_write_price: null, cache_read_price: null, intervals: [], time_pricing: null,
    }
    const { tools, generic } = splitDeepseekToolPrices('deepseek', [wildcard])
    expect(tools).toEqual({})
    expect(generic).toEqual([wildcard])
    const wrapper = mount(DeepseekToolPricingFields, {
      props: { modelValue: tools, genericPricing: generic },
      global: { plugins: [createI18n({ legacy: false, locale, messages: { en: { admin: en }, zh: { admin: zh } } })] },
    })
    const refs = ['USD 0.75 / CNY 5', 'USD 0.75 / CNY 5', 'USD 0.225 / CNY 1.5', 'USD 0.45 / CNY 3', 'USD 0.116 / CNY 0.78']
    const inputs = wrapper.findAll<HTMLInputElement>('input')
    expect(inputs).toHaveLength(5)
    for (const [i, input] of inputs.entries()) {
      const hint = wrapper.get(`[id="${input.attributes('aria-describedby')}"]`).text()
      expect(hint).toContain(refs[i])
      expect(input.element.disabled).toBe(i === 3)
      if (i === 3) expect(hint).toContain(wildcard.models[0])
    }
    expect(wrapper.get('h4').text()).toBe(locale === 'en' ? 'DeepSeek tool prices' : 'DeepSeek 工具价格')
    wrapper.unmount()
  })
})
