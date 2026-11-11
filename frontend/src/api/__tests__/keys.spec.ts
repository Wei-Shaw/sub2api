import { beforeEach, describe, expect, it, vi } from 'vitest'
import { create, getConcurrency, update } from '../keys'
import { apiClient } from '../client'

vi.mock('../client', () => ({ apiClient: { post: vi.fn(), put: vi.fn(), get: vi.fn() } }))

describe('API key concurrency payloads', () => {
  beforeEach(() => vi.resetAllMocks())

  it.each([0, 8])('preserves concurrency_limit=%s in create and update requests and responses', async (limit) => {
    const key = { id: 1, name: 'test-key', concurrency_limit: limit }
    vi.mocked(apiClient.post).mockResolvedValue({ data: key })
    vi.mocked(apiClient.put).mockResolvedValue({ data: key })

    expect(await create('test-key', 42, undefined, undefined, undefined, undefined, undefined, undefined, limit)).toEqual(key)
    expect(apiClient.post).toHaveBeenCalledWith('/keys', {
      name: 'test-key', group_id: 42, concurrency_limit: limit,
    })
    expect(await update(1, { concurrency_limit: limit })).toEqual(key)
    expect(apiClient.put).toHaveBeenCalledWith('/keys/1', { concurrency_limit: limit })
  })

  it('sends an explicit zero for the default create limit', async () => {
    vi.mocked(apiClient.post).mockResolvedValue({ data: {} })
    await create('test-key')
    expect(apiClient.post).toHaveBeenCalledWith('/keys', { name: 'test-key', concurrency_limit: 0 })
  })

  it('preserves server validation errors', async () => {
    const error = { response: { data: { detail: 'Concurrency limit rejected' } } }
    vi.mocked(apiClient.put).mockRejectedValue(error)
    await expect(update(1, { concurrency_limit: 8 })).rejects.toBe(error)
  })

  it.each([{ ids: [] }, { ids: [11, 12] }])('reads actual policy and counts for $ids with cancellation', async ({ ids }) => {
    const snapshot = { queue_policy: { max_waiting: 7, timeout_seconds: 12 }, items: [] }
    vi.mocked(apiClient.get).mockResolvedValue({ data: snapshot })
    const controller = new AbortController()
    expect(await getConcurrency(ids, { signal: controller.signal })).toEqual(snapshot)
    expect(apiClient.get).toHaveBeenCalledWith('/keys/concurrency', {
      params: ids.length ? { ids: '11,12' } : {}, signal: controller.signal,
    })
  })

  it('propagates statistics failure without substituting zero counts or a default policy', async () => {
    const error = { response: { status: 503 } }
    vi.mocked(apiClient.get).mockRejectedValue(error)
    await expect(getConcurrency([1])).rejects.toBe(error)
  })
})
