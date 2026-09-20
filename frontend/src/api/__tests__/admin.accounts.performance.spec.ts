import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { post } }))

import { getBatchPerformance, type BatchAccountPerformanceResponse } from '@/api/admin/accounts'

describe('admin passive account performance API', () => {
  beforeEach(() => {
    post.mockReset()
  })

  it('queries only the passive batch endpoint and forwards cancellation', async () => {
    const response: BatchAccountPerformanceResponse = {
      stats: {},
      window_start: '2026-09-16T02:00:00Z',
      window_end: '2026-09-16T03:00:00Z'
    }
    post.mockResolvedValue({ data: response })
    const controller = new AbortController()

    expect(await getBatchPerformance([42, 43], { signal: controller.signal })).toEqual(response)
    expect(post).toHaveBeenCalledTimes(1)
    expect(post).toHaveBeenCalledWith(
      '/admin/accounts/performance/batch',
      { account_ids: [42, 43] },
      { signal: controller.signal }
    )
  })

  it('propagates query errors rather than returning zero statistics', async () => {
    post.mockRejectedValue(new Error('statistics unavailable'))
    await expect(getBatchPerformance([42])).rejects.toThrow('statistics unavailable')
  })
})
