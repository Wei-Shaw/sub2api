import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({
  post: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { post }
}))

import { createMissingForAllAccounts } from '@/api/admin/scheduledTests'

describe('admin scheduled tests API', () => {
  beforeEach(() => {
    post.mockReset()
    post.mockResolvedValue({ data: { created: 4 } })
  })

  it('creates plans only for accounts without schedules', async () => {
    await expect(createMissingForAllAccounts()).resolves.toEqual({ created: 4 })
    expect(post).toHaveBeenCalledWith('/admin/scheduled-test-plans/bulk-create-missing')
  })
})
