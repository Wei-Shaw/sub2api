import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import PlazaModelPricingTable from '../PlazaModelPricingTable.vue'
import type { DisplayPriceModel } from '@/api/modelPrices'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

function model(overrides: Partial<DisplayPriceModel>): DisplayPriceModel {
  return {
    id: 1,
    platform: 'openai',
    model_name: 'gpt-test',
    model_note: '',
    billing_mode: 'token',
    provider: 'openai',
    currency: 'USD',
    configured: true,
    enabled: true,
    official_prices: {
      input_per_million: 1,
      output_per_million: 4,
      cache_write_per_million: null,
      cache_read_per_million: 0.1
    },
    model_multiplier: null,
    effective_multiplier: 1.25,
    display_prices: {
      input_per_million: 1.25,
      output_per_million: 5,
      cache_write_per_million: null,
      cache_read_per_million: 0.125
    },
    per_request: null,
    image_base_prices: [],
    image_prices: [],
    ...overrides
  }
}

describe('PlazaModelPricingTable', () => {
  it('shows official and site token prices together with the display multiplier', () => {
    const wrapper = mount(PlazaModelPricingTable, {
      props: {
        models: [model({})],
        platform: 'openai',
        billingMode: 'token',
        currency: 'USD'
      },
      global: { stubs: { PlatformIcon: true } }
    })

    expect(wrapper.text()).toContain('$1')
    expect(wrapper.text()).toContain('$1.25')
    expect(wrapper.text()).toContain('$4')
    expect(wrapper.text()).toContain('$5')
    expect(wrapper.text()).toContain('×1.25')
    expect(wrapper.findAll('[data-testid="official-price"]')).toHaveLength(4)
    expect(wrapper.findAll('[data-testid="site-price"]')).toHaveLength(4)
    expect(wrapper.text()).toContain('modelPlaza.table.displayMultiplier')
    expect(wrapper.get('table').classes()).toContain('table-fixed')
    expect(wrapper.get('[data-testid="token-columns"]').findAll('col')).toHaveLength(6)
    expect(wrapper.get('[data-testid="multiplier-badge"]').classes()).toEqual(
      expect.arrayContaining(['bg-[#effbf8]', 'text-[#07866f]'])
    )
    expect(wrapper.get('[data-testid="multiplier-badge"]').classes().join(' ')).not.toContain('orange')
  })

  it('renders a highlighted presentation-only model note below the model name', () => {
    const wrapper = mount(PlazaModelPricingTable, {
      props: {
        models: [model({ model_note: '资源紧张，价格将适时下调。' })],
        platform: 'deepseek',
        billingMode: 'token',
        currency: 'USD'
      },
      global: { stubs: { PlatformIcon: true } }
    })

    expect(wrapper.get('[data-testid="model-note"]').text()).toBe('资源紧张，价格将适时下调。')
  })

  it('renders the three per-request tiers without any multiplier', () => {
    const wrapper = mount(PlazaModelPricingTable, {
      props: {
        models: [
          model({
            billing_mode: 'per_request',
            currency: 'CNY',
            effective_multiplier: null,
            official_prices: null,
            display_prices: null,
            per_request: {
              lte_256k: 0.01,
              from_256k_to_512k: 0.015,
              gt_512k: 0.02
            }
          })
        ],
        platform: 'deepseek',
        billingMode: 'per_request',
        currency: 'CNY'
      },
      global: { stubs: { PlatformIcon: true } }
    })

    expect(wrapper.text()).toContain('¥0.01')
    expect(wrapper.text()).toContain('¥0.015')
    expect(wrapper.text()).toContain('¥0.02')
    expect(wrapper.text()).not.toContain('×')
    expect(wrapper.text()).not.toContain('modelPlaza.table.displayMultiplier')
    expect(wrapper.get('[data-testid="per-request-columns"]').findAll('col')).toHaveLength(4)
  })

  it('uses image specification labels as dynamic columns', () => {
    const wrapper = mount(PlazaModelPricingTable, {
      props: {
        models: [
          model({
            billing_mode: 'image',
            currency: 'CNY',
            effective_multiplier: null,
            official_prices: null,
            display_prices: null,
            image_base_prices: [
              { label: '1024×1024', price: 0.08 },
              { label: '2048×2048', price: 0.2 }
            ],
            image_prices: [
              { label: '1024×1024', price: 0.12 },
              { label: '2048×2048', price: 0.28 }
            ]
          })
        ],
        platform: 'qwen',
        billingMode: 'image',
        currency: 'CNY'
      },
      global: { stubs: { PlatformIcon: true } }
    })

    expect(wrapper.text()).toContain('1024×1024')
    expect(wrapper.text()).toContain('2048×2048')
    expect(wrapper.text()).toContain('¥0.12')
    expect(wrapper.text()).toContain('¥0.28')
    expect(wrapper.text()).toContain('¥0.08')
    expect(wrapper.text()).toContain('¥0.2')
    expect(wrapper.findAll('[data-testid="image-base-price"]')).toHaveLength(2)
    expect(wrapper.findAll('[data-testid="image-site-price"]')).toHaveLength(2)
  })

  it('preserves the administrator-defined API model order', () => {
    const wrapper = mount(PlazaModelPricingTable, {
      props: {
        models: [
          model({ id: 1, model_name: 'z-first' }),
          model({ id: 2, model_name: 'a-second' })
        ],
        platform: 'openai',
        billingMode: 'token',
        currency: 'USD'
      },
      global: { stubs: { PlatformIcon: true } }
    })

    expect(wrapper.findAll('tbody tr').map((row) => row.text())).toEqual([
      expect.stringContaining('z-first'),
      expect.stringContaining('a-second')
    ])
  })

  it('does not render video models as image pricing', () => {
    const wrapper = mount(PlazaModelPricingTable, {
      props: {
        models: [
          model({
            billing_mode: 'video',
            image_prices: [{ label: 'should-not-render', price: 10 }]
          })
        ],
        platform: 'openai',
        billingMode: 'video',
        currency: 'USD'
      },
      global: { stubs: { PlatformIcon: true } }
    })

    expect(wrapper.text()).toContain('modelPlaza.table.unavailable')
    expect(wrapper.text()).not.toContain('should-not-render')
    expect(wrapper.text()).not.toContain('$10')
  })
})
