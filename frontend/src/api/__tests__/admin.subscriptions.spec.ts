import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({
  post: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { post }
}))

import { resetQuota } from '@/api/admin/subscriptions'

describe('admin subscription quota reset API', () => {
  beforeEach(() => {
    post.mockReset()
    post.mockResolvedValue({ data: { id: 42 } })
  })

  it('sends only the selected quota windows', async () => {
    await expect(resetQuota(42, { daily: false, weekly: true, monthly: true }))
      .resolves.toEqual({ id: 42 })

    expect(post).toHaveBeenCalledWith(
      '/admin/subscriptions/42/reset-quota',
      { daily: false, weekly: true, monthly: true }
    )
  })
})
