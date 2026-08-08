import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, patch } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  patch: vi.fn(),
}))

vi.mock('@/api/client', () => ({ default: { get, post, patch }, apiClient: { get, post, patch } }))

import costCenterAPI from '@/api/admin/costCenter'

describe('admin cost center API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    patch.mockReset()
  })

  it('loads summary and events with the inclusive/exclusive date filters', async () => {
    const filters = { start: '2026-01-01T00:00:00Z', end: '2026-02-01T00:00:00Z', source_type: 'subscription' }
    const summary = { cash_income: 20, operating_profit: 8 }
    const page = { items: [], total: 0, page: 1, page_size: 50, pages: 0 }
    get.mockResolvedValueOnce({ data: summary }).mockResolvedValueOnce({ data: page })

    await expect(costCenterAPI.getSummary(filters)).resolves.toEqual({ data: summary })
    await expect(costCenterAPI.getEvents({ ...filters, page: 2, page_size: 25 })).resolves.toEqual({ data: page })
    expect(get).toHaveBeenNthCalledWith(1, '/admin/cost-center/summary', { params: filters })
    expect(get).toHaveBeenNthCalledWith(2, '/admin/cost-center/events', { params: { ...filters, page: 2, page_size: 25 } })
  })

  it('creates, confirms, reverses and reconciles expense events', async () => {
    const event = { id: 4, event_type: 'expense', amount_usd: 10 }
    const filters = { start: '2026-01-01T00:00:00Z', end: '2026-02-01T00:00:00Z' }
    post.mockResolvedValueOnce({ data: event }).mockResolvedValueOnce({ data: { ...event, event_type: 'reversal' } })
    patch.mockResolvedValueOnce({ data: { ...event, status: 'settled' } })
    get.mockResolvedValueOnce({ data: { unknown_events: 0, pending_events: 1, duplicate_keys: 0 } })

    await expect(costCenterAPI.createExpense({ amount_usd: 10, category: 'proxy', account_id: 7 })).resolves.toEqual({ data: event })
    await expect(costCenterAPI.updateEventStatus(4, 'settled', 'paid')).resolves.toMatchObject({ data: { status: 'settled' } })
    await expect(costCenterAPI.reverseEvent(4, 'duplicate')).resolves.toMatchObject({ data: { event_type: 'reversal' } })
    await expect(costCenterAPI.reconcile(filters)).resolves.toMatchObject({ data: { pending_events: 1 } })

    expect(post).toHaveBeenNthCalledWith(1, '/admin/cost-center/expenses', { amount_usd: 10, category: 'proxy', account_id: 7 })
    expect(patch).toHaveBeenCalledWith('/admin/cost-center/events/4/status', { status: 'settled', reason: 'paid' })
    expect(post).toHaveBeenNthCalledWith(2, '/admin/cost-center/events/4/reverse', { reason: 'duplicate' })
    expect(get).toHaveBeenCalledWith('/admin/cost-center/reconciliation', { params: filters })
  })
})
