import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, del } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  del: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post, put: vi.fn(), delete: del }
}))

import {
  cancelOpenDesignDeviceAuth,
  createOpenDesignAccountFromSession,
  createOpenDesignDeviceAuth,
  getOpenDesignDeviceAuth
} from '@/api/admin/accounts'

// 这些路径必须与后端 routes/admin.go 的 accounts 分组逐字一致：前缀写错会整体 404，
// 而组件测试全 mock adminAPI，只有这里能发现漂移。
describe('admin OpenDesign device-auth API paths', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    del.mockReset()
  })

  it('posts the initiate request under /admin/accounts/opendesign', async () => {
    post.mockResolvedValueOnce({ data: { sessionId: 's1' } })
    await createOpenDesignDeviceAuth('host')
    expect(post).toHaveBeenCalledWith('/admin/accounts/opendesign/device-auth', { host: 'host' })
  })

  it('polls and cancels the session under /admin/accounts/opendesign', async () => {
    get.mockResolvedValueOnce({ data: { status: 'pending' } })
    await getOpenDesignDeviceAuth('s/1')
    expect(get).toHaveBeenCalledWith('/admin/accounts/opendesign/device-auth/s%2F1')

    del.mockResolvedValueOnce({ data: {} })
    await cancelOpenDesignDeviceAuth('s/1')
    expect(del).toHaveBeenCalledWith('/admin/accounts/opendesign/device-auth/s%2F1')
  })

  it('creates the account under /admin/accounts/opendesign', async () => {
    post.mockResolvedValueOnce({ data: { action: 'created' } })
    await createOpenDesignAccountFromSession({ session_id: 's1' })
    expect(post).toHaveBeenCalledWith('/admin/accounts/opendesign/accounts', { session_id: 's1' })
  })
})
