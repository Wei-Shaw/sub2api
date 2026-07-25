import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import SupportedModelChip from '../SupportedModelChip.vue'
import { BILLING_MODE_TOKEN } from '@/constants/channel'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

const PricingRowStub = {
  props: ['label', 'value'],
  template: '<div data-test="pricing-row">{{ label }}:{{ value }}</div>',
}

const PlatformIconStub = {
  props: ['platform'],
  template: '<span data-test="platform-icon">{{ platform }}</span>',
}

describe('SupportedModelChip auto candidates', () => {
  it('shows Auto candidate group multipliers in the model popover', async () => {
    const wrapper = mount(SupportedModelChip, {
      props: {
        model: {
          name: 'claude-fable-5',
          platform: 'auto',
          auto_candidate_rates: [
            { group_id: 31, rate_multiplier: 8 },
            { group_id: 35, rate_multiplier: 12.5 },
          ],
          pricing: {
            billing_mode: BILLING_MODE_TOKEN,
            input_price: 0.00001,
            output_price: 0.00002,
            cache_write_price: null,
            cache_read_price: null,
            image_input_price: null,
            image_output_price: null,
            per_request_price: null,
            intervals: [],
          },
        },
      },
      global: {
        stubs: {
          PricingRow: PricingRowStub,
          PlatformIcon: PlatformIconStub,
          Teleport: true,
        },
      },
      attachTo: document.body,
    })

    await wrapper.get('[tabindex="0"]').trigger('mouseenter')

    expect(document.body.textContent).toContain('31')
    expect(document.body.textContent).toContain('8x')
    expect(document.body.textContent).toContain('35')
    expect(document.body.textContent).toContain('12.5x')

    wrapper.unmount()
  })
})
