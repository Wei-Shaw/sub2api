import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, put, post, del } = vi.hoisted(() => ({
  get: vi.fn(),
  put: vi.fn(),
  post: vi.fn(),
  del: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, put, post, delete: del }
}))

import { getModelPrices } from '@/api/modelPrices'
import displayPricingAPI from '@/api/admin/displayPricing'

describe('display pricing APIs', () => {
  beforeEach(() => {
    get.mockReset()
    put.mockReset()
    post.mockReset()
    del.mockReset()
  })

  it('loads the signed-in customer catalogue from the isolated endpoint', async () => {
    const response = { global_multiplier: 1.25, updated_at: '', providers: [] }
    get.mockResolvedValue({ data: response })

    await expect(getModelPrices()).resolves.toEqual(response)
    expect(get).toHaveBeenCalledWith('/model-prices', { signal: undefined })
  })

  it('updates a provider without touching channel or group endpoints', async () => {
    const payload = {
      display_name: 'Kimi',
      provider_note: '工作日高峰期价格说明',
      per_request_note: '按次价格说明',
      image_note: '生图价格说明',
      currency: 'CNY' as const,
      multiplier: 1.2,
      sort_order: 2,
      logo_key: 'kimi',
      logo_url: ''
    }
    get.mockResolvedValue({ data: { items: [] } })
    put.mockResolvedValue({ data: { provider: 'moonshot', ...payload } })

    await displayPricingAPI.updateProvider('moonshot', payload)

    expect(put).toHaveBeenCalledWith('/admin/display-pricing/providers/moonshot', payload)
    expect(get).not.toHaveBeenCalled()
  })

  it('creates and deletes provider display settings through dedicated CRUD endpoints', async () => {
    const payload = {
      provider: 'custom-ai',
      display_name: 'Custom AI',
      provider_note: '',
      per_request_note: '',
      image_note: '',
      currency: 'USD' as const,
      multiplier: null,
      sort_order: 50,
      logo_key: 'openai',
      logo_url: 'https://cdn.example.com/custom.svg'
    }
    post.mockResolvedValue({ data: { ...payload, updated_at: '' } })
    del.mockResolvedValue({ data: { provider: payload.provider, deleted_models: 3 } })

    await displayPricingAPI.createProvider(payload)
    await expect(displayPricingAPI.deleteProvider(payload.provider)).resolves.toEqual({
      provider: payload.provider,
      deleted_models: 3
    })

    expect(post).toHaveBeenCalledWith('/admin/display-pricing/providers', payload)
    expect(del).toHaveBeenCalledWith('/admin/display-pricing/providers/custom-ai')
  })

  it('uses the explicit per-request override fields in model upserts', async () => {
    const payload = {
      platform: 'openai',
      model_name: 'deepseek-test',
      model_note: '新模型上线',
      provider: 'deepseek',
      billing_mode: 'per_request' as const,
      currency: 'CNY' as const,
      enabled: true,
      sort_order: 0,
      official_input_per_million: null,
      official_output_per_million: null,
      official_cache_write_per_million: null,
      official_cache_read_per_million: null,
      model_multiplier: null,
      per_request_lte_256k: 0.01,
      per_request_256k_512k_override: null,
      per_request_gt_512k_override: 0.025,
      image_prices: []
    }
    post.mockResolvedValue({ data: { id: 1, ...payload } })

    await displayPricingAPI.createModel(payload)

    expect(post).toHaveBeenCalledWith('/admin/display-pricing/models', payload)
  })
})
