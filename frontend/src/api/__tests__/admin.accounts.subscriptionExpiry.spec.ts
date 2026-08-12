import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))

vi.mock('@/api/client', () => ({
  apiClient: { post }
}))

import {
  queryOpenAISubscriptionExpiry,
  queryOpenAISubscriptionExpiryBatch
} from '@/api/admin/accounts'

describe('admin account OpenAI subscription expiry API', () => {
  beforeEach(() => post.mockReset())

  it('uses explicit mutating endpoints for single and batch cache refreshes', async () => {
    const result = {
      account_id: 41,
      snapshot: {
        status: 'available' as const,
        expires_at: '2026-08-08T07:23:45Z',
        checked_at: '2026-08-07T06:17:00Z',
        source: 'subscriptions'
      }
    }
    post.mockResolvedValueOnce({ data: result })
    post.mockResolvedValueOnce({ data: { results: [result] } })

    await expect(queryOpenAISubscriptionExpiry(41)).resolves.toEqual(result)
    await expect(queryOpenAISubscriptionExpiryBatch([41])).resolves.toEqual([result])

    expect(post).toHaveBeenNthCalledWith(
      1,
      '/admin/openai/accounts/41/subscription-expiry/query'
    )
    expect(post).toHaveBeenNthCalledWith(
      2,
      '/admin/openai/accounts/subscription-expiry/query',
      { account_ids: [41] },
      { timeout: 120_000 }
    )
  })
})
