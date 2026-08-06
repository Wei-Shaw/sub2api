import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))

vi.mock('../client', () => ({ apiClient: { get } }))

import { getByDateRange } from '../usage'

describe('usage API date ranges', () => {
  beforeEach(() => {
    get.mockReset().mockResolvedValue({ data: { items: [] } })
  })

  it('includes the caller IANA timezone in recent usage requests', async () => {
    await getByDateRange('2026-03-03', '2026-03-09', undefined, 'America/New_York')

    expect(get).toHaveBeenCalledWith('/usage', {
      params: {
        start_date: '2026-03-03',
        end_date: '2026-03-09',
        page: 1,
        page_size: 100,
        timezone: 'America/New_York',
      },
    })
  })
})
