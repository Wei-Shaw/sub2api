import { nextTick } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import SupportedModelChip from '../SupportedModelChip.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

describe('SupportedModelChip', () => {
  it('仅配置区间倍率时按基础价展示 token 档位', async () => {
    const wrapper = mount(SupportedModelChip, {
      attachTo: document.body,
      props: {
        model: {
          name: 'gpt-test',
          platform: '',
          pricing: {
            billing_mode: 'token',
            input_price: 10e-6,
            output_price: 50e-6,
            cache_write_price: null,
            cache_read_price: null,
            image_input_price: null,
            image_output_price: null,
            per_request_price: null,
            intervals: [{
              min_tokens: 272000,
              max_tokens: null,
              input_price: null,
              output_price: null,
              cache_write_price: null,
              cache_read_price: null,
              input_multiplier: 2,
              output_multiplier: 1.5,
              per_request_price: null
            }]
          }
        },
        showPlatform: false
      }
    })

    await wrapper.find('[tabindex="0"]').trigger('mouseenter')
    await nextTick()

    expect(document.body.textContent).toContain('$20 / $75')
    wrapper.unmount()
  })
})
