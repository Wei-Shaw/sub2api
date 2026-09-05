import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post, deleteRequest } = vi.hoisted(() => ({
  post: vi.fn(),
  deleteRequest: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    post,
    delete: deleteRequest
  }
}))

import { batchAction, permanentDelete } from '@/api/admin/subscriptions'

describe('admin subscriptions API', () => {
  beforeEach(() => {
    post.mockReset()
    deleteRequest.mockReset()
  })

  it('sends a batch action with its idempotency key', async () => {
    const result = {
      total_count: 2,
      succeeded_count: 1,
      skipped_count: 1,
      failed_count: 0,
      items: [
        { subscription_id: 11, status: 'succeeded' as const },
        { subscription_id: 12, status: 'skipped' as const, reason: 'ineligible' }
      ]
    }
    post.mockResolvedValue({ data: result })

    await expect(batchAction({
      subscription_ids: [11, 12],
      action: 'reset_quota',
      reset_quota: { daily: true, weekly: false, monthly: true }
    }, 'subscription-batch-key')).resolves.toEqual(result)

    expect(post).toHaveBeenCalledWith(
      '/admin/subscriptions/batch-action',
      {
        subscription_ids: [11, 12],
        action: 'reset_quota',
        reset_quota: { daily: true, weekly: false, monthly: true }
      },
      { headers: { 'Idempotency-Key': 'subscription-batch-key' } }
    )
  })

  it('uses the permanent-delete endpoint and idempotency key', async () => {
    deleteRequest.mockResolvedValue({ data: { message: 'deleted' } })

    await expect(permanentDelete(17, 'permanent-delete-key')).resolves.toEqual({
      message: 'deleted'
    })

    expect(deleteRequest).toHaveBeenCalledWith(
      '/admin/subscriptions/17/permanent',
      { headers: { 'Idempotency-Key': 'permanent-delete-key' } }
    )
  })
})
