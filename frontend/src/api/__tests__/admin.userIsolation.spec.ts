import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))

vi.mock('@/api/client', () => ({
  apiClient: { post }
}))

import { lookup, type UserIsolationLookupRequest, type UserIsolationLookupResult } from '@/api/admin/userIsolation'

describe('admin user isolation API', () => {
  beforeEach(() => post.mockReset())

  it('posts the selected account and upstream isolation ID', async () => {
    const request: UserIsolationLookupRequest = {
      account_id: 7,
      isolation_id: `u1_${'A'.repeat(43)}`
    }
    const response: UserIsolationLookupResult = {
      account: { id: 7, name: 'risk-account', platform: 'openai', type: 'apikey' },
      user: { id: 42, email: 'risk@example.com', username: 'risk', status: 'active' }
    }
    post.mockResolvedValue({ data: response })

    await expect(lookup(request)).resolves.toEqual(response)
    expect(post).toHaveBeenCalledWith('/admin/user-isolation/lookup', request)
  })
})
