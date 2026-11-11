import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { CompositeModelRouteInput } from '@/types'

const { get, post, put } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), put: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { get, post, put } }))
import { createCompositeRoute, listCompositeRoutes, previewCompositeRoute, updateCompositeRoute } from '@/api/admin/groups'

const input: CompositeModelRouteInput = {
  public_model: 'deepseek-pro', match_type: 'exact', target_platform: 'opencode_go',
  upstream_model: 'deepseek-v4-pro', endpoint: 'any', priority: 100,
  fallback_targets: [{ target_platform: 'deepseek', upstream_model: 'deepseek-v4-pro' }]
}

describe('composite route fallback API contract', () => {
  beforeEach(() => vi.resetAllMocks())

  it('preserves explicit fallback targets through create, update and list', async () => {
    const saved = { ...input, id: 7, group_id: 12 }
    post.mockResolvedValue({ data: saved })
    put.mockResolvedValue({ data: { ...saved, fallback_targets: [] } })
    get.mockResolvedValue({ data: [saved] })
    await expect(createCompositeRoute(12, input)).resolves.toEqual(saved)
    expect(post).toHaveBeenCalledWith('/admin/groups/12/composite-routes', input)
    await expect(updateCompositeRoute(12, 7, { ...input, fallback_targets: [] })).resolves.toMatchObject({ fallback_targets: [] })
    expect(put).toHaveBeenCalledWith('/admin/groups/12/composite-routes/7', { ...input, fallback_targets: [] })
    await expect(listCompositeRoutes(12)).resolves.toEqual([saved])
  })

  it.each([
    { publicModel: 'deepseek-pro', platform: 'opencode_go', upstreamModel: 'deepseek-v4-pro', fallbackPlatform: 'deepseek', fallbackModel: 'deepseek-v4-pro' },
    { publicModel: 'deepseek-flash', platform: 'opencode_go', upstreamModel: 'deepseek-v4.1-flash', fallbackPlatform: 'deepseek', fallbackModel: 'deepseek-flash' },
    { publicModel: 'minimax-m3', platform: 'minimax', upstreamModel: 'MiniMax-M3', fallbackPlatform: 'opencode_go', fallbackModel: 'minimax-m3' }
  ] as const)('keeps the $publicModel alias separate from real upstream IDs', async ({ publicModel, platform, upstreamModel, fallbackPlatform, fallbackModel }) => {
    const payload: CompositeModelRouteInput = { ...input, public_model: publicModel, target_platform: platform,
      upstream_model: upstreamModel, fallback_targets: [{ target_platform: fallbackPlatform, upstream_model: fallbackModel }] }
    post.mockResolvedValue({ data: payload })
    put.mockResolvedValue({ data: payload })
    await createCompositeRoute(12, payload)
    await updateCompositeRoute(12, 7, payload)
    expect(post).toHaveBeenCalledWith('/admin/groups/12/composite-routes', payload)
    expect(put).toHaveBeenCalledWith('/admin/groups/12/composite-routes/7', payload)
  })

  it('reads the chain from preview decision.route without changing the request', async () => {
    const decision = { matched: true, source: 'route', route: { ...input, id: 7, group_id: 12 } }
    post.mockResolvedValue({ data: decision })
    const request = { model: 'deepseek-pro', endpoint: 'any' as const }
    await expect(previewCompositeRoute(12, request)).resolves.toEqual(decision)
    expect(post).toHaveBeenCalledWith('/admin/groups/12/composite-routes/preview', request)
  })
})
