import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post }
}))

import { getOpenAIAutoProvisionStatus, resetOpenAIAutoProvisionStatus } from '@/api/admin/settings'

describe('admin OpenAI auto-provision status API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
  })

  it('fetches the runtime status endpoint', async () => {
    const status = {
      phase: 'waiting_for_provision_callback',
      pending_provision_count: 2,
      last_callback_status: 'completed'
    }
    get.mockResolvedValueOnce({ data: status })

    await expect(getOpenAIAutoProvisionStatus()).resolves.toEqual(status)
    expect(get).toHaveBeenCalledWith('/admin/settings/openai-auto-provision/status')
  })

  it('resets pending provisioning through the admin endpoint', async () => {
    const status = { phase: 'idle', pending_provision_count: 0 }
    post.mockResolvedValueOnce({ data: status })

    await expect(resetOpenAIAutoProvisionStatus()).resolves.toEqual(status)
    expect(post).toHaveBeenCalledWith('/admin/settings/openai-auto-provision/status/reset')
  })
})
