import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({
  post: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { post }
}))

import { bindProxyByGroup } from '@/api/admin/accounts'

describe('admin account group proxy binding API', () => {
  beforeEach(() => {
    post.mockReset()
    post.mockResolvedValue({
      data: { success: 38, failed: 0, success_ids: [], failed_ids: [], results: [] }
    })
  })

  it('targets every account in the group and allows a long-running update', async () => {
    const result = await bindProxyByGroup(12, 7)

    expect(post).toHaveBeenCalledWith(
      '/admin/accounts/bulk-update',
      {
        filters: { group: '12' },
        proxy_id: 7
      },
      { timeout: 600000 }
    )
    expect(result.success).toBe(38)
  })
})
