import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import ModelPlazaContent from '../ModelPlazaContent.vue'
import PlazaFilterBar from '../PlazaFilterBar.vue'
import type { ModelPlazaGroup, ModelPlazaResponse, PlazaModel } from '@/api/modelPlaza'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
  createI18n: () => ({ global: { t: (key: string) => key } })
}))

const CardStub = defineComponent({
  props: { model: { type: Object, required: true } },
  setup(props) {
    return () => h('div', {
      class: 'model-card',
      'data-name': (props.model as PlazaModel).name,
      'data-platform': (props.model as PlazaModel).platform
    })
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
    expect(filter.props('platforms')).toEqual(['anthropic', 'openai'])
    expect(filter.props('rates')).toEqual([0.1, 0.5])

    await filter.vm.$emit('update:platform', 'anthropic')
    expect(wrapper.findAll('.model-card').map((card) => card.attributes('data-name'))).toEqual([
      'anthropic-model'
    ])

    await filter.vm.$emit('update:groupName', 'group-2')
    await filter.vm.$emit('update:rate', 0.5)
    expect(wrapper.findAll('.model-card').map((card) => card.attributes('data-name'))).toEqual([
      'anthropic-model'
    ])

    await filter.vm.$emit('update:search', 'openai')
    expect(wrapper.findAll('.model-card')).toHaveLength(0)
  })

  it('derives platform filters from every platform returned by the API', () => {
    const wrapper = mountContent({
      description: '',
      groups: [group(1, [model('custom-model', '2026-01-01', ['文本'], 'new-platform')], 'new-platform')]
    })

    expect(wrapper.findComponent(PlazaFilterBar).props('platforms')).toEqual(['new-platform'])
  })

  it('shows one shared group filter across platforms and preserves same-name models by platform', async () => {
    const wrapper = mountContent({
      description: '',
      groups: [
        { ...group(1, [model('shared-model', '2026-01-01', ['多模态'], 'openai')]), name: '多模态' },
        { ...group(2, [model('shared-model', '2026-01-01', ['多模态'], 'deepseek')], 'deepseek'), name: '多模态' }
      ]
    })

    const filter = wrapper.findComponent(PlazaFilterBar)
    expect(filter.findAll('button[data-filter="group"]').filter((button) => button.text() === '多模态')).toHaveLength(1)
    await filter.vm.$emit('update:groupName', '多模态')

    expect(wrapper.findAll('.model-card').map((card) => card.attributes('data-platform'))).toEqual(['openai', 'deepseek'])

    await filter.vm.$emit('update:platform', 'deepseek')
    expect(wrapper.findAll('.model-card').map((card) => card.attributes('data-platform'))).toEqual(['deepseek'])
  })

  it('paginates the models after applying a category filter', async () => {
    const models = Array.from({ length: 8 }, (_, index) =>
      model(`openai-${index + 1}`, `2026-01-${String(index + 1).padStart(2, '0')}`, ['文本'])
    )
    const wrapper = mountContent({
      description: '',
      groups: [group(1, models, 'openai')]
    })

    const filter = wrapper.findComponent(PlazaFilterBar)
    await filter.vm.$emit('update:platform', 'openai')
    await filter.vm.$emit('update:groupName', 'group-1')

    expect(wrapper.findAll('.model-card')).toHaveLength(6)
    expect(wrapper.find('nav[aria-label="Pagination"]').exists()).toBe(true)

    const pageTwo = wrapper
      .find('nav[aria-label="Pagination"]')
      .findAll('button')
      .find((button) => button.text() === '2')
    expect(pageTwo).toBeDefined()
    await pageTwo!.trigger('click')

    expect(wrapper.findAll('.model-card').map((card) => card.attributes('data-name'))).toEqual([
      'openai-2',
      'openai-1'
    ])
  })

  it('filters models by metadata modality', async () => {
    const wrapper = mountContent({
      description: '',
      groups: [group(1, [
        model('text-model', '2026-01-01', ['文本']),
        model('vision-model', '2026-01-02', ['多模态'])
      ])]
    })

    const filter = wrapper.findComponent(PlazaFilterBar)
    await filter.vm.$emit('update:category', '多模态')

    expect(wrapper.findAll('.model-card').map((card) => card.attributes('data-name'))).toEqual([
      'vision-model'
    ])
  })
})
