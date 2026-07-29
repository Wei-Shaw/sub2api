import { describe, expect, it, vi, beforeEach } from 'vitest'

vi.mock('@/api/client', () => ({
  apiClient: {
    get: vi.fn(),
    put: vi.fn(),
    post: vi.fn(),
    delete: vi.fn(),
  },
}))

import { apiClient } from '@/api/client'
import * as api from '../api'

describe('connection-risk api', () => {
  beforeEach(() => {
    vi.mocked(apiClient.get).mockReset()
    vi.mocked(apiClient.put).mockReset()
    vi.mocked(apiClient.post).mockReset()
    vi.mocked(apiClient.delete).mockReset()
  })

  it('listEvents passes filters and pagination', async () => {
    vi.mocked(apiClient.get).mockResolvedValue({
      data: { items: [], total: 0, page: 1, page_size: 20 },
    })
    await api.listEvents(
      { status: 'open', severity: 'high', user_id: '3', api_key_id: '', rule: 'R1' },
      2,
      20,
    )
    expect(apiClient.get).toHaveBeenCalledWith('/admin/connection-risk/events', {
      params: { page: 2, page_size: 20, status: 'open', severity: 'high', user_id: '3', rule: 'R1' },
    })
  })

  it('updateConfig puts body', async () => {
    const body = { enabled: true } as any
    vi.mocked(apiClient.put).mockResolvedValue({ data: body })
    const res = await api.updateConfig(body)
    expect(apiClient.put).toHaveBeenCalledWith('/admin/connection-risk/config', body)
    expect(res).toEqual(body)
  })
})
