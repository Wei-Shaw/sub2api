import { beforeEach, describe, expect, it, vi } from 'vitest'
import { subscriptionsAPI } from '../admin/subscriptions'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))
vi.mock('../client', () => ({ apiClient: { post } }))

describe('admin subscription batch APIs', () => {
  beforeEach(() => vi.clearAllMocks())

  it('sends the explicit operation key and returns partial results', async () => {
    const request = { subscription_ids: [3, 5], action: 'extend' as const, days: 7 }
    const result = { success_count: 1, failed_count: 1, results: [
      { subscription_id: 3, success: true }, { subscription_id: 5, success: false, error: 'Not found' }
    ] }
    post.mockResolvedValue({ data: result })
    expect(await subscriptionsAPI.bulkAction(request, 'operation-123')).toEqual(result)
    expect(post).toHaveBeenCalledWith('/admin/subscriptions/bulk-action', request, {
      headers: { 'Idempotency-Key': 'operation-123' }
    })
  })

  it('returns the assignment summary and per-user failures', async () => {
    const request = { user_ids: [11, 12], group_id: 3, validity_days: 30 }
    const result = { success_count: 1, failed_count: 1, subscriptions: [], errors: ['User 12: conflict'], statuses: { 11: 'created', 12: 'failed' } }
    post.mockResolvedValue({ data: result })
    expect(await subscriptionsAPI.bulkAssign(request)).toEqual(result)
    expect(post).toHaveBeenCalledWith('/admin/subscriptions/bulk-assign', request)
  })
})
