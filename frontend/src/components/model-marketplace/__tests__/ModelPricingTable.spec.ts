import { mount } from '@vue/test-utils'
import { defineComponent, h, type PropType } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import ModelPricingTable from '../ModelPricingTable.vue'
import type { ModelMarketplaceItem, ModelMarketplacePricePart } from '@/api/modelMarketplace'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const labels: Record<string, string> = {
    'modelMarketplace.price.input': 'Input',
    'modelMarketplace.price.output': 'Output',
    'modelMarketplace.price.cacheRead': 'Cache read',
    'modelMarketplace.price.cacheWrite': 'Cache write',
    'modelMarketplace.price.cacheWrite5m': 'Cache write 5m',
    'modelMarketplace.price.cacheWrite1h': 'Cache write 1h',
    'modelMarketplace.price.imageOutput': 'Image output',
    'modelMarketplace.price.perRequest': 'Per request',
    'modelMarketplace.units.request': 'request',
  }
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => labels[key] ?? key,
    }),
  }
})

const DataTableStub = defineComponent({
  name: 'DataTable',
  props: {
    data: {
      type: Array as PropType<ModelMarketplaceItem[]>,
      required: true,
    },
  },
  setup(props, { slots }) {
    return () => h('div', { 'data-testid': 'data-table-stub' },
      props.data.map((row) => h('div', { key: row.model, 'data-testid': 'pricing-cell' },
        slots['cell-pricing']?.({ row }) ?? []
      ))
    )
  },
})

function price(displayUsd = 0): ModelMarketplacePricePart {
  return {
    per_token: displayUsd > 0 ? displayUsd / 1_000_000 : 0,
    per_1k: displayUsd > 0 ? displayUsd / 1_000 : 0,
    per_1m: displayUsd,
    display_usd: displayUsd,
  }
}

function makeItem(): ModelMarketplaceItem {
  return {
    model: 'claude-cache-test',
    provider: 'anthropic',
    provider_label: 'Anthropic / Claude',
    provider_icon: 'anthropic',
    family: 'Claude',
    mode: 'chat',
    billing_mode: 'token',
    supported_endpoints: ['anthropic'],
    features: {
      prompt_caching: true,
      service_tier: false,
      vision: false,
      reasoning: false,
      web_search: false,
      audio_output: false,
    },
    context: {
      max_input_tokens: 200_000,
      max_output_tokens: 8_192,
    },
    pricing: {
      source: 'litellm_catalog',
      catalog_unit: 'per_token_usd',
      display_unit: '1M_tokens',
      rate_multiplier: 1,
      service_tier: 'standard',
      input: price(3),
      output: price(15),
      cache_read: price(0.3),
      cache_write: price(0),
      cache_write_5m: price(3.75),
      cache_write_1h: price(4.5),
      image_output: price(0),
      per_request: null,
    },
    calculation_note: 'ActualCost = TotalCost * rate_multiplier',
    tags: ['cache'],
  }
}

describe('ModelPricingTable', () => {
  it('renders cache write duration price rows from the marketplace API contract', () => {
    const wrapper = mount(ModelPricingTable, {
      props: {
        items: [makeItem()],
        loading: false,
        unitLabel: '1M tokens',
      },
      global: {
        stubs: {
          DataTable: DataTableStub,
        },
      },
    })

    expect(wrapper.text()).toContain('Cache write 5m')
    expect(wrapper.text()).toContain('$3.7500 / 1M tokens')
    expect(wrapper.text()).toContain('Cache write 1h')
    expect(wrapper.text()).toContain('$4.5000 / 1M tokens')
  })
})
