import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))

vi.mock('@/api/client', () => ({
  apiClient: { get }
}))

import { getGroupsStatus } from '@/api/groupsStatus'

describe('public groups status API', () => {
  beforeEach(() => {
    get.mockReset()
  })

  it('loads the anonymous aggregate endpoint and forwards cancellation', async () => {
    const payload = {
      groups: [],
      summary: {
        group_count: 0,
        available_group_count: 0,
        account_count: 0,
        available_account_count: 0,
        rate_limited_account_count: 0
      }
    }
    const controller = new AbortController()
    get.mockResolvedValue({ data: payload })

    await expect(getGroupsStatus({ signal: controller.signal })).resolves.toEqual(payload)
    expect(get).toHaveBeenCalledWith('/groups-status', { signal: controller.signal })
  })
})
