import { afterEach, describe, expect, it, vi } from 'vitest'
import { apiClient } from '../client'
import { accountsAPI as userAccountsAPI } from '../accounts'
import { listVisibleUsers, updateVisibleUsers } from '../admin/accounts'

afterEach(() => vi.restoreAllMocks())

describe('account visibility APIs', () => {
  it('queries the user account list with visibility filters', async () => {
    const get = vi.spyOn(apiClient, 'get').mockResolvedValue({
      data: { items: [], total: 0, page: 1, page_size: 20, pages: 1 }
    })

    await userAccountsAPI.list(1, 20, { platform: 'openai', group: '7' })

    expect(get).toHaveBeenCalledWith('/accounts', expect.objectContaining({
      params: expect.objectContaining({ page: 1, page_size: 20, platform: 'openai', group: '7' })
    }))
  })

  it('reads and replaces the visible users for an admin account', async () => {
    const get = vi.spyOn(apiClient, 'get').mockResolvedValue({ data: [] })
    const put = vi.spyOn(apiClient, 'put').mockResolvedValue({ data: { user_ids: [3, 9] } })

    await listVisibleUsers(21)
    await updateVisibleUsers(21, [3, 9])

    expect(get).toHaveBeenCalledWith('/admin/accounts/21/visible-users')
    expect(put).toHaveBeenCalledWith('/admin/accounts/21/visible-users', { user_ids: [3, 9] })
  })
})
