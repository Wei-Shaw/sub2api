import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))

vi.mock('@/api/client', () => ({
  apiClient: { post }
}))

import { getWindowUsage } from '@/api/admin/accounts'
import type { AccountWindowUsageRequest, AccountWindowUsageResponse } from '@/types'

describe('admin account window usage API', () => {
  beforeEach(() => {
    post.mockReset()
  })

  it('posts all quota-window targets in a single request', async () => {
    const payload: AccountWindowUsageRequest = {
      windows: [
        {
          window_key: 'five_hour',
          period: 'current',
          start_time: '2026-08-11T08:00:00Z',
          end_time: '2026-08-11T10:30:00Z'
        }
      ]
    }
    const response: AccountWindowUsageResponse = {
      generated_at: '2026-08-11T10:30:00Z',
      items: []
    }
    post.mockResolvedValueOnce({ data: response })

    await expect(getWindowUsage(7, payload)).resolves.toEqual(response)
    expect(post).toHaveBeenCalledWith('/admin/accounts/7/window-usage', payload)
  })
})
