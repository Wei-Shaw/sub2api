import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ModelManagementView from '../ModelManagementView.vue'
import { ref } from 'vue'

const channels = vi.hoisted(() => [
  {
    id: 1, name: 'OpenAI primary', status: 'active', restrict_models: false,
    model_pricing: [{ platform: 'openai', models: ['gpt-5.6-sol'], billing_mode: 'token', input_price: null, output_price: null, cache_write_price: null, cache_read_price: null, image_input_price: null, image_output_price: null, per_request_price: null, intervals: [], time_pricing: null }],
    model_mapping: {}, group_ids: [], account_stats_pricing_rules: []
  },
  {
    id: 2, name: 'OpenAI empty', status: 'active', restrict_models: false,
    model_pricing: [], model_mapping: {}, group_ids: [], account_stats_pricing_rules: []
  }
])
const api = vi.hoisted(() => ({
  channels: {
    list: vi.fn(),
    update: vi.fn()
  }
}))
vi.mock('@/api/admin', () => ({ adminAPI: api }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn() }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ locale: ref('en') }) }))

describe('ModelManagementView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.channels.list.mockResolvedValue({ items: structuredClone(channels), total: 2 })
    api.channels.update.mockResolvedValue({})
  })

  it('adds a model to selected channels already configured for the platform', async () => {
    const wrapper = mount(ModelManagementView)
    await flushPromises()
    await wrapper.get('input[placeholder="e.g. gpt-6-astra"]').setValue('gpt-6-astra')
    await wrapper.findAll('button').find(button => button.text() === 'Apply bulk change')!.trigger('click')
    await flushPromises()

    expect(api.channels.update).toHaveBeenCalledTimes(1)
    expect(api.channels.update.mock.calls[0][1].model_pricing[0].models).toContain('gpt-6-astra')
  })

  it('routes a source model and removes a model with restriction enabled', async () => {
    const routedChannels = structuredClone(channels)
    routedChannels[0].model_mapping = { openai: { 'gpt-6-astra': 'gpt-5.6-sol' } }
    api.channels.list.mockReset()
    api.channels.list.mockResolvedValueOnce({ items: structuredClone(channels), total: 2 })
      .mockResolvedValue({ items: routedChannels, total: 2 })
    const wrapper = mount(ModelManagementView)
    await flushPromises()
    await wrapper.get('input[placeholder="e.g. gpt-6-astra"]').setValue('gpt-6-astra')
    await wrapper.findAll('button').find(button => button.text() === 'Route model')!.trigger('click')
    await wrapper.get('input[placeholder="e.g. gpt-5.6-sol"]').setValue('gpt-5.6-sol')
    await wrapper.findAll('button').find(button => button.text() === 'Apply bulk change')!.trigger('click')
    await flushPromises()
    expect(api.channels.update.mock.calls[0][1].model_mapping.openai).toEqual({ 'gpt-6-astra': 'gpt-5.6-sol' })

    await wrapper.findAll('button').find(button => button.text() === 'Remove model')!.trigger('click')
    await wrapper.findAll('button').find(button => button.text() === 'Apply bulk change')!.trigger('click')
    await flushPromises()
    expect(api.channels.update.mock.calls.at(-1)?.[1]).toMatchObject({ restrict_models: true })
  })
})
