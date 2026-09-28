import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import ModelPlazaContent from '../ModelPlazaContent.vue'
import PlazaFilterBar from '../PlazaFilterBar.vue'
import type { ModelPlazaGroup, ModelPlazaResponse, PlazaModel } from '@/api/modelPlaza'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

const CardStub = defineComponent({
  props: { model: { type: Object, required: true } },
  setup(props) {
    return () => h('div', { class: 'model-card', 'data-name': (props.model as PlazaModel).name })
  }
})

function model(
  name: string,
  launchDate: string,
  categories: PlazaModel['categories'],
  platform = 'openai'
): PlazaModel {
  return {
    name,
    platform,
    display_name: name.toUpperCase(),
    capability: 'capability',
    use_cases: 'use cases',
    categories,
    tier_condition: '<272K',
    input_price: '¥1',
    output_price: '¥2',
    cache_read_price: '¥0.1',
    cache_write_price: '¥0.2',
    glossary: 'terms',
    launch_date: launchDate,
    pricing: null,
    official_pricing: null
  }
}

function group(
  id: number,
  models: PlazaModel[],
  platform = 'openai',
  rateMultiplier = 1
): ModelPlazaGroup {
  return {
    id,
    name: `group-${id}`,
    description: '',
    platform,
    subscription_type: 'standard',
    rate_multiplier: rateMultiplier,
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 1,
    is_exclusive: false,
    image_rate_independent: false,
    image_rate_multiplier: 1,
    long_context_pricing_enabled: true,
    models
  }
}

function mountContent(response: ModelPlazaResponse) {
  return mount(ModelPlazaContent, {
    props: { response, loading: false, embedded: true, showRateDetails: false },
    global: { stubs: { ModelMetadataCard: CardStub } }
  })
}

describe('ModelPlazaContent R2', () => {
  it('deduplicates models and sorts by launch date descending', () => {
    const duplicate = model('shared', '2026-01-01', ['文本'])
    const wrapper = mountContent({
      description: '',
      groups: [
        group(1, [duplicate, model('newest', '2026-09-19', ['代码'])]),
        group(2, [duplicate])
      ]
    })
    expect(wrapper.findAll('.model-card').map((card) => card.attributes('data-name'))).toEqual([
      'newest',
      'shared'
    ])
  })

  it('keeps models whose metadata display name is missing', () => {
    const missingDisplayName = model('model-without-display-name', '2026-01-01', ['文本'])
    missingDisplayName.display_name = ''
    const wrapper = mountContent({
      description: '',
      groups: [group(1, [missingDisplayName])]
    })
    expect(wrapper.findAll('.model-card').map((card) => card.attributes('data-name'))).toEqual([
      'model-without-display-name'
    ])
  })

  it('uses platform, group, and rate filters from the new filter bar', async () => {
    const wrapper = mountContent({
      description: '',
      groups: [
        group(1, [
          model('openai-model', '2026-01-01', ['文本'])
        ], 'openai', 0.1),
        group(2, [
          model('anthropic-model', '2026-01-02', ['文本'], 'anthropic')
        ], 'anthropic', 0.5)
      ]
    })
    const filter = wrapper.findComponent(PlazaFilterBar)
    expect(filter.props('platforms')).toEqual(['anthropic', 'deepseek', 'zhipu', 'openai'])
    expect(filter.props('rates')).toEqual([0.1, 0.5])

    await filter.vm.$emit('update:platform', 'anthropic')
    expect(wrapper.findAll('.model-card').map((card) => card.attributes('data-name'))).toEqual([
      'anthropic-model'
    ])

    await filter.vm.$emit('update:groupId', 2)
    await filter.vm.$emit('update:rate', 0.5)
    expect(wrapper.findAll('.model-card').map((card) => card.attributes('data-name'))).toEqual([
      'anthropic-model'
    ])

    await filter.vm.$emit('update:search', 'openai')
    expect(wrapper.findAll('.model-card')).toHaveLength(0)
  })
})
