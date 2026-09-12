import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { bulkUpdate } from '../admin/groups'

const { put } = vi.hoisted(() => ({ put: vi.fn() }))
vi.mock('../client', () => ({ apiClient: { put } }))

describe('admin group bulk updates', () => {
  beforeEach(() => {
    put.mockReset()
  })

  it('deduplicates IDs, sends sparse updates, and preserves partial failures', async () => {
    const failure = { status: 403, message: 'Permission denied' }
    put.mockImplementation((url: string) => url === '/admin/groups/2'
      ? Promise.reject(failure) : Promise.resolve({ data: {} }))
    const updates = { rate_multiplier: 0.5, is_exclusive: false, daily_limit_usd: null, weekly_limit_usd: 0 }
    const result = await bulkUpdate([1, 2, 1, 3], updates)

    expect(put.mock.calls).toEqual([
      ['/admin/groups/1', updates], ['/admin/groups/2', updates], ['/admin/groups/3', updates]
    ])
    expect(result).toEqual({ succeededIds: [1, 3], failures: [{ id: 2, error: failure }] })
  })

  it('limits in-flight requests to five and continues after failures', async () => {
    const settle: Array<() => void> = []
    let inFlight = 0
    let peak = 0
    put.mockImplementation(() => new Promise((resolve, reject) => {
      inFlight++
      peak = Math.max(peak, inFlight)
      const index = settle.length
      settle.push(() => {
        inFlight--
        if (index === 0) reject(new Error('network error'))
        else resolve({ data: {} })
      })
    }))
    const pending = bulkUpdate([1, 2, 3, 4, 5, 6, 7], { status: 'inactive' })
    expect(put).toHaveBeenCalledTimes(5)
    settle.slice(0, 4).forEach((finish) => finish())
    await flushPromises()
    expect(put).toHaveBeenCalledTimes(5)
    settle[4]()
    await flushPromises()
    expect(put).toHaveBeenCalledTimes(7)
    settle.slice(5).forEach((finish) => finish())
    const result = await pending
    expect(peak).toBe(5)
    expect(result.succeededIds).toEqual([2, 3, 4, 5, 6, 7])
    expect(result.failures.map(({ id }) => id)).toEqual([1])
  })

  it('does not send requests for an empty selection', async () => {
    expect(await bulkUpdate([], { rpm_limit: 0 })).toEqual({ succeededIds: [], failures: [] })
    expect(put).not.toHaveBeenCalled()
  })
})
