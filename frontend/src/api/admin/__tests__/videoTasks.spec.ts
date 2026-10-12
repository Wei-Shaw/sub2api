import { beforeEach, describe, expect, it, vi } from 'vitest'
import { apiClient } from '@/api/client'
import { list } from '../videoTasks'

vi.mock('@/api/client', () => ({ apiClient: { get: vi.fn() } }))

describe('admin video task API', () => {
  beforeEach(() => vi.clearAllMocks())

  it('reads the shared client unwrapped envelope with the default query', async () => {
    const payload = { items: [], total: 0, page: 1, page_size: 20, pages: 0 }
    vi.mocked(apiClient.get).mockResolvedValue({ data: payload })
    expect(await list()).toBe(payload)
    expect(apiClient.get).toHaveBeenCalledWith('/admin/video-tasks', {
      params: { page: 1, page_size: 20, search: undefined, days: 7 }, signal: undefined
    })
  })

  it('preserves literal search text, filters and cancellation; caps page size at 100', async () => {
    vi.mocked(apiClient.get).mockResolvedValue({ data: { items: [] } })
    const controller = new AbortController()
    await list({ page: 3, page_size: 1000, search: ' %_\\ task ', status: 'query_retry', days: 30 }, controller.signal)
    expect(apiClient.get).toHaveBeenCalledWith('/admin/video-tasks', {
      params: { page: 3, page_size: 100, search: ' %_\\ task ', status: 'query_retry', days: 30 }, signal: controller.signal
    })
  })

  it('limits search to 100 Unicode characters without splitting characters', async () => {
    vi.mocked(apiClient.get).mockResolvedValue({ data: {} })
    await list({ search: '视🎬'.repeat(60) })
    expect(vi.mocked(apiClient.get).mock.calls[0][1]?.params.search).toBe('视🎬'.repeat(50))
  })
})
