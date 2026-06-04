import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({
  get: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    get,
  },
}))

import { usageAPI } from '@/api/usage'

describe('usage api', () => {
  beforeEach(() => {
    get.mockReset()
  })

  it('fetches the daily token leaderboard from the user usage endpoint', async () => {
    const response = {
      items: [{ rank: 1, display_name: 'ali***', total_tokens: 123456 }],
      start_date: '2026-05-27',
      end_date: '2026-05-27',
      limit: 5,
      generated_at: '2026-05-27T12:00:00+08:00',
    }
    get.mockResolvedValue({ data: response })

    await expect(usageAPI.getDailyTokenLeaderboard()).resolves.toEqual(response)

    expect(get).toHaveBeenCalledWith('/usage/leaderboard/daily')
  })
})
