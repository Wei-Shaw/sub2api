import { beforeEach, describe, expect, it, vi } from 'vitest'
import { subscriptionsAPI } from '../admin/subscriptions'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))
vi.mock('../client', () => ({ apiClient: { post } }))

describe('admin subscription quota window APIs', () => {
  beforeEach(() => vi.clearAllMocks())

  it('sets quota windows for one subscription without extra fields', async () => {
    const request = { weekly_window_start: '2026-09-12T08:32:57.000Z', monthly_window_start: '2026-09-12T08:32:57.000Z' }
    const result = { id: 1 }
    post.mockResolvedValue({ data: result })
    expect(await subscriptionsAPI.setQuotaWindows(1, request)).toEqual(result)
    expect(post).toHaveBeenCalledWith('/admin/subscriptions/1/quota-windows', request)
  })

  it('sets quota windows for every active subscription in a group', async () => {
    const request = { weekly_window_start: '2026-09-12T08:32:57.000Z' }
    const result = { total: 2, success: 2, failed: 0, failed_subscription_ids: [], errors: [] }
    post.mockResolvedValue({ data: result })
    expect(await subscriptionsAPI.setGroupQuotaWindows(3, request)).toEqual(result)
    expect(post).toHaveBeenCalledWith('/admin/groups/3/subscriptions/quota-windows', request)
  })

  it('resets quota for every active subscription in a group', async () => {
    const request = { daily: true, weekly: true, monthly: true }
    const result = { total: 2, success: 2, failed: 0, failed_subscription_ids: [], errors: [] }
    post.mockResolvedValue({ data: result })
    expect(await subscriptionsAPI.resetGroupQuota(3, request)).toEqual(result)
    expect(post).toHaveBeenCalledWith('/admin/groups/3/subscriptions/reset-quota', request)
  })
})
