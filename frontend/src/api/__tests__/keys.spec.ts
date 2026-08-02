import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post, put } = vi.hoisted(() => ({
  post: vi.fn(),
  put: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: { post, put },
}))

import { create, update } from '@/api/keys'

describe('API key fallback group payloads', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    post.mockResolvedValue({ data: {} })
    put.mockResolvedValue({ data: {} })
  })

  it('serializes ordered fallback groups for a personal key', async () => {
    await create('ordered', 1, undefined, [], [], 0, undefined, undefined, null, [3, 2])

    expect(post).toHaveBeenCalledWith('/keys', {
      name: 'ordered',
      group_id: 1,
      fallback_group_ids: [3, 2],
    })
  })

  it('does not serialize personal fallback groups for an enterprise key', async () => {
    await create('enterprise', 1, undefined, [], [], 0, undefined, undefined, 90, [3, 2])

    expect(post).toHaveBeenCalledWith('/keys', {
      name: 'enterprise',
      organization_subscription_id: 90,
    })
  })

  it('preserves ordered fallback groups on update', async () => {
    await update(7, { group_id: 1, fallback_group_ids: [2, 3] })

    expect(put).toHaveBeenCalledWith('/keys/7', {
      group_id: 1,
      fallback_group_ids: [2, 3],
    })
  })
})
