import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, del } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  del: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post, delete: del },
}))

import { developerKeysAPI } from '@/api/developerKeys'

describe('developer keys API', () => {
  beforeEach(() => vi.clearAllMocks())

  it('lists the current user developer keys', async () => {
    get.mockResolvedValue({ data: { items: [{ id: 1, name: 'Uploader' }] } })

    await expect(developerKeysAPI.list()).resolves.toEqual([{ id: 1, name: 'Uploader' }])
    expect(get).toHaveBeenCalledWith('/user/developer-keys')
  })

  it('creates and deletes developer keys through user-scoped routes', async () => {
    const created = {
      key: { id: 2, name: 'CLI' },
      secret: 'dev_secret',
      display_once: true,
    }
    post.mockResolvedValue({ data: created })
    del.mockResolvedValue({ data: { deleted: 2 } })

    await expect(developerKeysAPI.create('CLI')).resolves.toEqual(created)
    await developerKeysAPI.remove(2)

    expect(post).toHaveBeenCalledWith('/user/developer-keys', { name: 'CLI' })
    expect(del).toHaveBeenCalledWith('/user/developer-keys/2')
  })
})
