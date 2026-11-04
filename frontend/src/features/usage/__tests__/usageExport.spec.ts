import { afterEach, describe, expect, it, vi } from 'vitest'
import { queryUsageExportPage } from '../presentation/usageExport'

const { query } = vi.hoisted(() => ({ query: vi.fn() }))
vi.mock('../data/datasources/usageDatasource', () => ({ query }))

afterEach(() => {
  vi.useRealTimers()
  query.mockReset()
})

describe('usage export rate limits', () => {
  it('waits for Retry-After and retries the same frozen page', async () => {
    vi.useFakeTimers()
    const controller = new AbortController()
    const notify = vi.fn()
    const response = { items: [], total: 1001, pages: 2 }
    query.mockRejectedValueOnce({ status: 429, retry_after_seconds: 2 }).mockResolvedValue(response)
    const params = { page: 2, page_size: 1000, model: 'gpt-6.1-sol' }
    const pending = queryUsageExportPage(params, controller.signal, notify)
    await vi.advanceTimersByTimeAsync(1999)
    expect(query).toHaveBeenCalledTimes(1)
    expect(notify).toHaveBeenCalledWith(2)
    await vi.advanceTimersByTimeAsync(1)
    expect(await pending).toBe(response)
    expect(query.mock.calls[1]).toEqual(query.mock.calls[0])
  })

  it('stops after three retries', async () => {
    vi.useFakeTimers()
    query.mockRejectedValue({ status: 429, retry_after_seconds: 1 })
    const rejected = expect(queryUsageExportPage({ page: 1 }, new AbortController().signal, vi.fn()))
      .rejects.toMatchObject({ status: 429 })
    await vi.advanceTimersByTimeAsync(3000)
    await rejected
    expect(query).toHaveBeenCalledTimes(4)
  })

  it('cancels a pending retry without leaving a timer or making another request', async () => {
    vi.useFakeTimers()
    query.mockRejectedValue({ status: 429 })
    const controller = new AbortController()
    const rejected = expect(queryUsageExportPage({ page: 1 }, controller.signal, vi.fn()))
      .rejects.toMatchObject({ name: 'AbortError' })
    await vi.advanceTimersByTimeAsync(0)
    expect(vi.getTimerCount()).toBe(1)
    controller.abort()
    await rejected
    expect(vi.getTimerCount()).toBe(0)
    expect(query).toHaveBeenCalledTimes(1)
  })

  it('does not retry ordinary server errors', async () => {
    query.mockRejectedValue({ status: 503 })
    await expect(queryUsageExportPage({ page: 1 }, new AbortController().signal, vi.fn()))
      .rejects.toMatchObject({ status: 503 })
    expect(query).toHaveBeenCalledTimes(1)
  })
})
