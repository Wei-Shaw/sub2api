import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({
  post: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { post }
}))

import { resetGroupQuota } from '@/api/admin/subscriptions'

describe('admin subscription group quota reset API', () => {
  beforeEach(() => {
    post.mockReset()
    post.mockResolvedValue({
      data: {
        total: 3,
        success: 3,
        failed: 0,
        failed_subscription_ids: [],
        errors: []
      }
    })
  })

  it('resets every quota window for the selected group', async () => {
    const result = await resetGroupQuota(42, {
      daily: true,
      weekly: true,
      monthly: true
    })

    expect(post).toHaveBeenCalledWith('/admin/groups/42/subscriptions/reset-quota', {
      daily: true,
      weekly: true,
      monthly: true
    })
    expect(result).toEqual({
      total: 3,
      success: 3,
      failed: 0,
      failed_subscription_ids: [],
      errors: []
    })
  })
})
