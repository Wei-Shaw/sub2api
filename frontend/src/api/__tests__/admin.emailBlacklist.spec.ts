import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post, put }
}))

import {
  getRegistrationEmailBlacklist,
  updateRegistrationEmailBlacklist
} from '@/api/admin/settings'
import { batchDisable, findBlacklistedEmailUsers } from '@/api/admin/users'

describe('admin email blacklist api', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
  })

  it('reads and replaces the exact email blacklist', async () => {
    get.mockResolvedValue({ data: { emails: ['a@gmail.com'], max: 20000 } })
    put.mockResolvedValue({ data: { emails: [], added: [], max: 20000 } })

    await expect(getRegistrationEmailBlacklist()).resolves.toEqual({ emails: ['a@gmail.com'], max: 20000 })
    expect(get).toHaveBeenCalledWith('/admin/settings/registration-email-blacklist')

    await expect(updateRegistrationEmailBlacklist([])).resolves.toEqual({ emails: [], added: [], max: 20000 })
    expect(put).toHaveBeenCalledWith('/admin/settings/registration-email-blacklist', { emails: [] })
  })

  it('finds blacklisted users and disables them in batch', async () => {
    post.mockResolvedValueOnce({ data: { items: [{ id: 3, email: 'a.b@gmail.com', username: '' }], total: 1 } })
    post.mockResolvedValueOnce({ data: { affected: 1 } })

    await expect(findBlacklistedEmailUsers(['ab@gmail.com'])).resolves.toEqual({
      items: [{ id: 3, email: 'a.b@gmail.com', username: '' }],
      total: 1
    })
    expect(post).toHaveBeenNthCalledWith(1, '/admin/users/blacklist-matches', { emails: ['ab@gmail.com'] })

    await expect(batchDisable([3])).resolves.toEqual({ affected: 1 })
    expect(post).toHaveBeenNthCalledWith(2, '/admin/users/batch-disable', { user_ids: [3] })
  })
})
