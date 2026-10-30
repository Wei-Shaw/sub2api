import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { get, post } }))
import { exportData, importData, syncFromCrs } from '@/api/admin/accounts'
import { accountSchedulingPoolsAPI } from '@/api/admin/accountSchedulingPools'

describe('account pool import and export integration', () => {
  beforeEach(() => { get.mockReset(); post.mockReset(); get.mockResolvedValue({ data: {} }); post.mockResolvedValue({ data: {} }) })

  it.each(['0', '17'])('exports all matching accounts with server-side pool filter %s', async pool => {
    await exportData({ filters: { pool_id: pool, platform: 'openai' } })
    expect(get).toHaveBeenCalledWith('/admin/accounts/data', { params: { pool_id: pool, platform: 'openai' } })
  })

  it('forwards pool and naming options for a multi-account JSON import', async () => {
    const naming = { prefix: 'GPT-Free-', suffix: '', start_number: 1, padding: 2 }
    const data = { proxies: [], accounts: [] }
    await importData({ data, scheduling_pool_id: 7, naming, skip_default_group_bind: true })
    expect(post).toHaveBeenCalledWith('/admin/accounts/data', { data, scheduling_pool_id: 7, naming, skip_default_group_bind: true })
  })

  it('preserves the chosen CRS management and pool options', async () => {
    const input = { base_url: 'https://crs.example.test', username: 'admin', password: 'test', scheduling_pool_id: 7, preserve_local_management: true, selected_account_ids: ['a', 'b'] }
    await syncFromCrs(input)
    expect(post.mock.calls[0]?.[1]).toEqual(input)
  })

  it('reuses pool creation idempotency keys after an ambiguous failure', async () => {
    post.mockRejectedValueOnce(new Error('timeout'))
    const input = { name: 'Free accounts', enabled: false }
    await expect(accountSchedulingPoolsAPI.create(input)).rejects.toThrow('timeout')
    await accountSchedulingPoolsAPI.create(input)
    expect(post.mock.calls[0]?.[2].headers).toEqual(post.mock.calls[1]?.[2].headers)
  })
})
