import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PlazaGroupSection from '../PlazaGroupSection.vue'
import PlazaModelPricingTable from '../PlazaModelPricingTable.vue'
import ProviderLogo from '../ProviderLogo.vue'
import type { DisplayPriceModel } from '@/api/modelPrices'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: { count?: number; currency?: string }) =>
        `${key}:${params?.count ?? params?.currency ?? ''}`
    })
  }
})

function model(name = 'gpt-test'): DisplayPriceModel {
  return {
    id: 1,
    platform: 'openai',
    model_name: name,
    model_note: '',
    billing_mode: 'token',
    provider: 'openai',
    currency: 'USD',
    configured: true,
    enabled: true,
    official_prices: null,
    model_multiplier: null,
    effective_multiplier: 1.2,
    display_prices: null,
    per_request: null,
    image_base_prices: [],
    image_prices: []
  }
}

function mountSection(models = [model()]) {
  return mount(PlazaGroupSection, {
    props: {
      provider: 'openai',
      providerName: 'OpenAI',
      providerNote: '高峰时段价格按平常价格 ×2 计算。',
      logoKey: 'openai',
      logoUrl: 'https://cdn.example.com/openai.svg',
      currency: 'USD',
      billingMode: 'token',
      models,
      sectionId: 'openai-token'
    },
    global: {
      stubs: {
        ProviderLogo: true,
        PlazaModelPricingTable: true
      }
    }
  })
}

describe('PlazaGroupSection', () => {
  it('renders the provider heading and the model count once', () => {
    const wrapper = mountSection([model('gpt-a'), model('gpt-b')])

    expect(wrapper.text()).toContain('OpenAI')
    expect(wrapper.text()).toContain('modelPlaza.detail.modelCount:2')
    expect(wrapper.attributes('id')).toBe('openai-token')
    expect(wrapper.get('[data-testid="provider-note"]').text()).toContain('高峰时段价格')
    expect(wrapper.get('[data-testid="price-legend"]').text()).toContain('modelPlaza.table.officialPrice')
    expect(wrapper.get('[data-testid="price-legend"]').text()).toContain('modelPlaza.table.sitePrice')
    expect(wrapper.get('[data-testid="provider-card"]').classes()).toEqual(
      expect.arrayContaining(['border-[#dce3f7]', 'shadow-none'])
    )
    expect(wrapper.get('[data-testid="price-legend"]').html()).toContain('text-[#315bd6]')
  })

  it('passes catalogue models and billing metadata to the shared pricing table', () => {
    const models = [model()]
    const table = mountSection(models).findComponent(PlazaModelPricingTable)

    expect(table.props('models')).toEqual(models)
    expect(table.props('platform')).toBe('openai')
    expect(table.props('billingMode')).toBe('token')
    expect(table.props('currency')).toBe('USD')
  })

  it('passes custom and fallback logo settings to ProviderLogo', () => {
    const logo = mountSection().findComponent(ProviderLogo)

    expect(logo.props('provider')).toBe('openai')
    expect(logo.props('logoKey')).toBe('openai')
    expect(logo.props('logoUrl')).toBe('https://cdn.example.com/openai.svg')
  })
})
