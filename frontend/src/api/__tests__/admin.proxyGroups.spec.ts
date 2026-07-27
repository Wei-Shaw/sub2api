import { beforeEach, describe, expect, it, vi } from 'vitest'

const apiClientMock = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  delete: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: apiClientMock,
}))

import proxyGroupsAPI from '@/api/admin/proxyGroups'

describe('admin proxyGroups API', () => {
  beforeEach(() => {
    apiClientMock.get.mockReset()
    apiClientMock.post.mockReset()
    apiClientMock.put.mockReset()
    apiClientMock.delete.mockReset()
  })

  it('lists proxy groups', async () => {
    apiClientMock.get.mockResolvedValue({ data: { items: [], total: 0, page: 1, page_size: 20 } })
    await proxyGroupsAPI.list(1, 20)
    expect(apiClientMock.get).toHaveBeenCalledTimes(1)
    expect(apiClientMock.get.mock.calls[0]?.[0]).toBe('/admin/proxy-groups')
    expect(apiClientMock.get.mock.calls[0]?.[1]?.params).toMatchObject({ page: 1, page_size: 20 })
  })

  it('creates a proxy group with members', async () => {
    apiClientMock.post.mockResolvedValue({ data: { id: 1, name: 'pool' } })
    await proxyGroupsAPI.create({
      name: 'pool',
      strategy: 'sticky',
      sticky_by_account: true,
      proxy_ids: [1, 2],
    })
    expect(apiClientMock.post).toHaveBeenCalledWith(
      '/admin/proxy-groups',
      expect.objectContaining({
        name: 'pool',
        strategy: 'sticky',
        sticky_by_account: true,
        proxy_ids: [1, 2],
      }),
    )
  })

  it('sets members via dedicated endpoint', async () => {
    apiClientMock.put.mockResolvedValue({ data: { id: 1 } })
    await proxyGroupsAPI.setMembers(1, [3, 4])
    expect(apiClientMock.put).toHaveBeenCalledWith('/admin/proxy-groups/1/members', {
      proxy_ids: [3, 4],
    })
  })
})
