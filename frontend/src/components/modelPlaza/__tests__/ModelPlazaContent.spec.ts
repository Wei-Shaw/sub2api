import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ModelPlazaContent from '../ModelPlazaContent.vue'
import type { DisplayPriceModel, ModelPricesResponse } from '@/api/modelPrices'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      locale: { value: 'zh-CN' },
      t: (key: string) => key
    })
  }
})

function model(name: string, billingMode: DisplayPriceModel['billing_mode']): DisplayPriceModel {
  return {
    id: name.length,
    platform: 'openai',
    model_name: name,
    model_note: '',
    billing_mode: billingMode,
    provider: 'openai',
    currency: 'USD',
    configured: true,
    enabled: true,
    official_prices: null,
    model_multiplier: null,
    effective_multiplier: billingMode === 'token' ? 1.2 : null,
    display_prices: null,
    per_request: null,
    image_base_prices: [],
    image_prices: []
  }
}

const response: ModelPricesResponse = {
  global_multiplier: 1.2,
  updated_at: '2026-08-29T00:00:00Z',
  providers: [
    {
      provider: 'openai',
      display_name: 'OpenAI',
      provider_note: '按量备注',
      per_request_note: '按次备注',
      image_note: '生图备注',
      currency: 'USD',
      logo_key: 'openai',
      logo_url: '',
      configured_multiplier: null,
      effective_multiplier: 1.2,
      // Deliberately shuffled to verify the customer page enforces billing section order.
      models: [model('image-test', 'image'), model('request-test', 'per_request'), model('token-test', 'token')]
    }
  ]
}

describe('ModelPlazaContent billing sections', () => {
  it('separates pricing into token, per-request, and image regions in that order', () => {
    const wrapper = mount(ModelPlazaContent, {
      props: { response, loading: false },
      global: {
        stubs: {
          Icon: true,
          PlazaFilterBar: true,
          PlazaGroupSection: true
        }
      }
    })

    expect(wrapper.findAll('[data-billing-mode]').map((node) => node.attributes('data-billing-mode'))).toEqual([
      'token',
      'per_request',
      'image'
    ])
    expect(wrapper.text()).toContain('modelPlaza.sections.token.title')
    expect(wrapper.text()).toContain('modelPlaza.sections.perRequest.title')
    expect(wrapper.text()).toContain('modelPlaza.sections.image.title')
    expect(wrapper.get('[data-testid="catalog-hero"]').classes()).toEqual(
      expect.arrayContaining(['bg-[#121a35]', 'border-[#202b51]'])
    )
    expect(wrapper.findAll('[data-testid="billing-region-header"]')).toHaveLength(3)
    expect(
      wrapper.findAll('[data-testid="billing-region-header"]').every((header) =>
        header.classes().includes('bg-[#f7f9ff]')
      )
    ).toBe(true)
  })

  it('splits a provider by model currency so the price symbol cannot be mislabeled', () => {
    const mixedCurrencyResponse: ModelPricesResponse = {
      ...response,
      providers: [
        {
          ...response.providers[0],
          models: [
            model('usd-model', 'token'),
            { ...model('cny-model', 'token'), currency: 'CNY' }
          ]
        }
      ]
    }
    const wrapper = mount(ModelPlazaContent, {
      props: { response: mixedCurrencyResponse, loading: false },
      global: {
        stubs: {
          Icon: true,
          PlazaFilterBar: true,
          PlazaGroupSection: true
        }
      }
    })

    expect(wrapper.findAll('plaza-group-section-stub').map((node) => node.attributes('currency'))).toEqual([
      'USD',
      'CNY'
    ])
  })

  it('uses independent provider notes for every billing mode', () => {
    const wrapper = mount(ModelPlazaContent, {
      props: { response, loading: false },
      global: {
        stubs: {
          Icon: true,
          PlazaFilterBar: true,
          PlazaGroupSection: true
        }
      }
    })

    const sections = wrapper.findAllComponents({ name: 'PlazaGroupSection' })
    expect(sections.map((section) => section.props('providerNote'))).toEqual([
      '按量备注',
      '按次备注',
      '生图备注'
    ])
  })
})
