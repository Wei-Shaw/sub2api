import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const api = vi.hoisted(() => ({
  listModels: vi.fn(),
  listProviders: vi.fn(),
  updateModel: vi.fn(),
  previewOfficialPriceSync: vi.fn(),
  applyOfficialPriceSync: vi.fn(),
}))
const showSuccess = vi.hoisted(() => vi.fn())
const showError = vi.hoisted(() => vi.fn())
const notifyDisplayPricingUpdated = vi.hoisted(() => vi.fn())

vi.mock('@/api/admin/displayPricing', () => ({ default: api }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess, showError }) }))
vi.mock('@/utils/displayPricingSync', () => ({ notifyDisplayPricingUpdated }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key,
    }),
  }
})

import OfficialPricingView from '../OfficialPricingView.vue'

const tokenModel = {
  id: 1,
  platform: 'openai',
  model_name: 'deepseek-v4-flash-0731',
  provider: 'deepseek',
  billing_mode: 'token' as const,
  currency: 'CNY' as const,
  enabled: true,
  sort_order: 1,
  model_note: 'peak note',
  official_input_per_million: 1.6,
  official_output_per_million: 4.7,
  official_cache_write_per_million: null,
  official_cache_read_per_million: 0.1,
  official_price_source: 'provider_official',
  official_price_source_url: 'https://api-docs.deepseek.com/quick_start/pricing',
  official_price_synced_at: '2026-08-30T00:00:00Z',
  model_multiplier: 0.2,
  per_request_lte_256k: null,
  per_request_256k_512k_override: null,
  per_request_gt_512k_override: null,
  image_prices: [],
  created_at: '2026-08-29T00:00:00Z',
  updated_at: '2026-08-30T00:00:00Z',
}

const perRequestModel = {
  ...tokenModel,
  id: 2,
  model_name: 'deepseek-per-request',
  billing_mode: 'per_request' as const,
  per_request_lte_256k: 0.01,
}

const provider = {
  provider: 'deepseek',
  display_name: 'DeepSeek',
  provider_note: '',
  per_request_note: '',
  image_note: '',
  currency: 'CNY' as const,
  multiplier: 1,
  sort_order: 1,
  logo_key: 'deepseek',
  logo_url: '',
  updated_at: '2026-08-30T00:00:00Z',
}

const candidates = {
  fetched_at: '2026-08-30T01:00:00Z',
  items: [
    {
      model_id: 1,
      model_name: tokenModel.model_name,
      provider: 'deepseek',
      currency: 'CNY' as const,
      current: { input_per_million: 1.6, output_per_million: 4.7, cache_write_per_million: null, cache_read_per_million: 0.1 },
      proposed: { input_per_million: 2, output_per_million: 5, cache_write_per_million: null, cache_read_per_million: 0.2 },
      changed: true,
      applicable: true,
      reason: '',
      source: 'herohao_aggregate',
      confidence: 'unverified',
      source_updated_at: '2026-08-30T00:30:00Z',
      official_reference_url: 'https://api-docs.deepseek.com/quick_start/pricing',
      expected_updated_at: tokenModel.updated_at,
      proposal_hash: 'proposal-hash-1',
    },
    {
      model_id: 9,
      model_name: 'unknown-model',
      provider: 'deepseek',
      currency: 'CNY' as const,
      current: { input_per_million: null, output_per_million: null, cache_write_per_million: null, cache_read_per_million: null },
      proposed: null,
      changed: false,
      applicable: false,
      reason: 'model version does not match',
      source: 'herohao_aggregate',
      confidence: 'unverified',
      source_updated_at: null,
      official_reference_url: '',
      expected_updated_at: '2026-08-30T00:00:00Z',
      proposal_hash: '',
    },
  ],
}

function mountView() {
  return mount(OfficialPricingView, {
    global: {
      stubs: {
        Icon: true,
      },
    },
  })
}

describe('OfficialPricingView', () => {
  beforeEach(() => {
    Object.values(api).forEach(mock => mock.mockReset())
    showSuccess.mockReset()
    showError.mockReset()
    notifyDisplayPricingUpdated.mockReset()

    api.listModels.mockResolvedValue([tokenModel, perRequestModel])
    api.listProviders.mockResolvedValue([provider])
    api.updateModel.mockImplementation(async (_id, payload) => ({ ...tokenModel, ...payload }))
    api.previewOfficialPriceSync.mockResolvedValue(candidates)
    api.applyOfficialPriceSync.mockResolvedValue({ applied_count: 1 })
  })

  it('renders as an embedded panel without its own application layout', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="official-pricing-panel"]').exists()).toBe(true)
    expect(wrapper.findComponent({ name: 'AppLayout' }).exists()).toBe(false)
  })

  it('lists token models only and preserves all model/source metadata on manual save', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain(tokenModel.model_name)
    expect(wrapper.text()).not.toContain(perRequestModel.model_name)

    await wrapper.get('[data-testid="official-input-1"]').setValue('2.4')
    await wrapper.get('[data-testid="save-official-model-1"]').trigger('click')
    await flushPromises()

    expect(api.updateModel).toHaveBeenCalledWith(1, expect.objectContaining({
      official_input_per_million: 2.4,
      official_output_per_million: 4.7,
      model_multiplier: 0.2,
      model_note: 'peak note',
      official_price_source: 'manual',
      official_price_source_url: '',
      official_price_synced_at: null,
    }))
    expect(notifyDisplayPricingUpdated).toHaveBeenCalled()
  })

  it('labels aggregate candidates as unverified and explains non-applicable rows', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="official-sync-preview"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="aggregate-warning"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('admin.officialPricing.source.herohao_aggregate')
    expect(wrapper.text()).toContain('model version does not match')
    expect(wrapper.get('[data-testid="official-sync-select-9"]').attributes('disabled')).toBeDefined()
  })

  it('applies only selected applicable snapshots', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-testid="official-sync-preview"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="apply-official-sync"]').trigger('click')
    await flushPromises()

    expect(api.applyOfficialPriceSync).toHaveBeenCalledWith({
      models: [{ model_id: 1, expected_updated_at: tokenModel.updated_at, proposal_hash: 'proposal-hash-1' }],
    })
  })
})
