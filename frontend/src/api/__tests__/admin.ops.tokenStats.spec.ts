import { afterEach, describe, expect, it, vi } from 'vitest'
import { apiClient } from '@/api/client'
import {
  getTokenStats,
  type OpsTokenStatsParams,
  type OpsTokenStatsResponse,
} from '@/api/admin/ops'

afterEach(() => vi.restoreAllMocks())

describe('admin ops token stats API', () => {
  it('uses the generic endpoint and preserves token-stat filters', async () => {
    const response: OpsTokenStatsResponse = {
      time_range: '1h',
      start_time: '2026-08-15T00:00:00Z',
      end_time: '2026-08-15T01:00:00Z',
      platform: 'deepseek',
      group_id: 7,
      items: [{
        platform: 'deepseek',
        model: 'deepseek-v4-pro',
        request_count: 2,
        avg_tokens_per_sec: 12.5,
        avg_first_token_ms: 80,
        total_output_tokens: 100,
        avg_duration_ms: 4000,
        requests_with_first_token: 2,
      }],
      total: 1,
      top_n: 20,
    }
    const get = vi.spyOn(apiClient, 'get').mockResolvedValue({ data: response })
    const controller = new AbortController()
    const params: OpsTokenStatsParams = {
      time_range: '1h',
      platform: 'deepseek',
      group_id: 7,
      top_n: 20,
    }

    await expect(getTokenStats(params, { signal: controller.signal })).resolves.toEqual(response)

    expect(get).toHaveBeenCalledWith('/admin/ops/dashboard/token-stats', {
      params,
      signal: controller.signal,
    })
  })
})
