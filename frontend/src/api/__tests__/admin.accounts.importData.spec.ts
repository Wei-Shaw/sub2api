import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({
  post: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { post }
}))

import { importData } from '@/api/admin/accounts'

describe('admin account importData API', () => {
  beforeEach(() => {
    post.mockReset()
    post.mockResolvedValue({ data: { account_created: 1 } })
  })

  it('uses a long timeout, forwards group_ids and sends the idempotency key', async () => {
    const data = { exported_at: '2026-09-01T00:00:00Z', proxies: [], accounts: [] }
    await importData({ data, skip_default_group_bind: true, group_ids: [3] }, { idempotencyKey: 'account-import-k1' })

    expect(post).toHaveBeenCalledWith(
      '/admin/accounts/data',
      { data, skip_default_group_bind: true, group_ids: [3] },
      { timeout: 300000, headers: { 'Idempotency-Key': 'account-import-k1' } }
    )
  })
})
