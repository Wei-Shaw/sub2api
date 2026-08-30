import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))

vi.mock('@/api/client', () => ({ apiClient: { post } }))

import displayPricingAPI from '@/api/admin/displayPricing'

describe('official pricing sync API', () => {
  beforeEach(() => post.mockReset())

  it('previews official prices without applying any candidate', async () => {
    const response = { items: [], fetched_at: '2026-08-30T00:00:00Z' }
    post.mockResolvedValue({ data: response })

    await expect(displayPricingAPI.previewOfficialPriceSync()).resolves.toEqual(response)

    expect(post).toHaveBeenCalledWith('/admin/display-pricing/official-sync/preview')
  })

  it('applies only explicitly selected model snapshots', async () => {
    const payload = {
      models: [
        { model_id: 12, expected_updated_at: '2026-08-30T00:00:00Z' },
        { model_id: 18, expected_updated_at: '2026-08-30T00:01:00Z' },
      ],
    }
    post.mockResolvedValue({ data: { applied_count: 2 } })

    await expect(displayPricingAPI.applyOfficialPriceSync(payload)).resolves.toEqual({ applied_count: 2 })

    expect(post).toHaveBeenCalledWith('/admin/display-pricing/official-sync/apply', payload)
  })
})
