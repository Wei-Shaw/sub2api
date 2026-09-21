import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import ModelPlazaContent from '../ModelPlazaContent.vue'
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

function model(name: string, launchDate: string, categories: PlazaModel['categories']): PlazaModel {
  return {
    name,
    platform: 'openai',
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

function group(id: number, models: PlazaModel[]): ModelPlazaGroup {
  return {
    id,
    name: `group-${id}`,
    description: '',
    platform: 'openai',
    subscription_type: 'standard',
    rate_multiplier: 1,
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

  it('uses OR semantics when multiple categories are selected', async () => {
    const wrapper = mountContent({
      description: '',
      groups: [
        group(1, [
          model('text-only', '2026-01-01', ['文本']),
          model('code-only', '2026-01-02', ['代码']),
          model('voice-only', '2026-01-03', ['语音'])
        ])
      ]
    })
    const buttons = wrapper.findAll('button')
    await buttons.find((button) => button.text() === '文本')!.trigger('click')
    await buttons.find((button) => button.text() === '代码')!.trigger('click')
    expect(wrapper.findAll('.model-card').map((card) => card.attributes('data-name'))).toEqual([
      'code-only',
      'text-only'
    ])
    expect(wrapper.text()).toContain('modelPlaza.filters.orHint')
  })
})
