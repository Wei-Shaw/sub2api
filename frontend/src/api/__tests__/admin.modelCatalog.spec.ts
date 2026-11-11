import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put, del } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  del: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    get,
    post,
    put,
    delete: del,
  },
}))

import modelCatalogAPI, { type ModelCatalogEntry, type ModelCatalogRequest } from '@/api/admin/modelCatalog'

const request: ModelCatalogRequest = {
  model_id: 'claude-opus-5-5',
  platform: 'anthropic',
  vendor: 'anthropic',
  display_name: 'Claude Opus 5.5',
  description: '',
  input_price: 0.000005,
  output_price: 0.000025,
  cache_write_price: null,
  cache_read_price: null,
  context_window: 200000,
  max_output_tokens: 64000,
  released_at: '2026-11-01',
  enabled: true,
  sort_order: 0,
}

const entry: ModelCatalogEntry = {
  id: 7,
  ...request,
  created_at: '2026-11-01T00:00:00Z',
  updated_at: '2026-11-01T00:00:00Z',
}

describe('admin model catalog api', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
    del.mockReset()
  })

  it('lists with only the provided filters', async () => {
    get.mockResolvedValue({ data: { items: [entry] } })

    const result = await modelCatalogAPI.list({ platform: '', vendor: 'anthropic', search: '', enabled: false })

    expect(get).toHaveBeenCalledWith('/admin/model-catalog', {
      params: { vendor: 'anthropic', enabled: false },
    })
    expect(result.items).toEqual([entry])
  })

  it('gets, creates, updates and deletes by id', async () => {
    get.mockResolvedValue({ data: entry })
    post.mockResolvedValue({ data: entry })
    put.mockResolvedValue({ data: entry })
    del.mockResolvedValue({ data: null })

    await expect(modelCatalogAPI.get(7)).resolves.toEqual(entry)
    expect(get).toHaveBeenCalledWith('/admin/model-catalog/7')

    await expect(modelCatalogAPI.create(request)).resolves.toEqual(entry)
    expect(post).toHaveBeenCalledWith('/admin/model-catalog', request)

    await expect(modelCatalogAPI.update(7, request)).resolves.toEqual(entry)
    expect(put).toHaveBeenCalledWith('/admin/model-catalog/7', request)

    await modelCatalogAPI.remove(7)
    expect(del).toHaveBeenCalledWith('/admin/model-catalog/7')
  })
})
