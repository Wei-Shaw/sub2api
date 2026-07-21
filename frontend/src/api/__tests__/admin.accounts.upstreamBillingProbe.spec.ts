import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post, put }
}))

import {
  getUpstreamBillingProbeSettings,
  getUpstreamBillingRateHistoryWithEtag,
  getUpstreamBillingRatesWithEtag,
  probeUpstreamBilling,
  probeUpstreamBillingBatch,
  queryUpstreamQuota,
  setUpstreamBillingProbeEnabled,
  updateUpstreamBillingProbeSettings
} from '@/api/admin/accounts'

describe('admin account upstream billing probe API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
  })

  it('reads and updates global settings', async () => {
    const settings = { enabled: true, interval_minutes: 30 }
    get.mockResolvedValueOnce({ data: settings })
    put.mockResolvedValueOnce({ data: settings })

    await expect(getUpstreamBillingProbeSettings()).resolves.toEqual(settings)
    await expect(updateUpstreamBillingProbeSettings(settings)).resolves.toEqual(settings)
    expect(get).toHaveBeenCalledWith('/admin/accounts/upstream-billing-probe/settings')
    expect(put).toHaveBeenCalledWith('/admin/accounts/upstream-billing-probe/settings', settings)
  })

  it('uses dedicated account and batch endpoints', async () => {
    const result = { account_id: 7, snapshot: { status: 'unsupported' } }
    put.mockResolvedValueOnce({ data: {} })
    post.mockResolvedValueOnce({ data: result })
    post.mockResolvedValueOnce({ data: { results: [result] } })

    await setUpstreamBillingProbeEnabled(7, true)
    await expect(probeUpstreamBilling(7)).resolves.toEqual(result)
    await expect(probeUpstreamBillingBatch([7])).resolves.toEqual([result])

    expect(put).toHaveBeenCalledWith('/admin/accounts/7/upstream-billing-probe', { enabled: true })
    expect(post).toHaveBeenNthCalledWith(1, '/admin/accounts/7/upstream-billing-probe')
    expect(post).toHaveBeenNthCalledWith(
      2,
      '/admin/accounts/upstream-billing-probe/batch',
      { account_ids: [7] },
      { timeout: 120000 }
    )
  })

  it('queries transient upstream quota through its dedicated command', async () => {
    const result = {
      account_id: 7,
      observed_at: '2026-07-17T00:00:00Z',
      quota: { provider: 'sub2api', mode: 'balance', unit: 'USD', remaining: 80 }
    }
    post.mockResolvedValueOnce({ data: result })

    await expect(queryUpstreamQuota(7)).resolves.toEqual(result)
    expect(post).toHaveBeenCalledWith('/admin/accounts/7/upstream-quota/query')
  })

  it('reads only persisted rate snapshots and supports ETag revalidation', async () => {
    const data = { items: [{ account_id: 7, snapshot: { status: 'ok' } }], total: 1, page: 1, page_size: 20 }
    get.mockResolvedValueOnce({
      status: 200,
      headers: { etag: '"rate-v1"' },
      data
    })
    await expect(getUpstreamBillingRatesWithEtag(1, 20, { sort_by: 'name', sort_order: 'asc' })).resolves.toEqual({
      notModified: false,
      etag: '"rate-v1"',
      data
    })
    expect(get).toHaveBeenCalledWith('/admin/accounts/upstream-billing-rates', expect.objectContaining({
      params: expect.objectContaining({ page: 1, page_size: 20, sort_by: 'name', sort_order: 'asc' }),
      validateStatus: expect.any(Function)
    }))

    get.mockResolvedValueOnce({ status: 304, headers: { etag: '"rate-v1"' }, data: '' })
    await expect(getUpstreamBillingRatesWithEtag(1, 20, undefined, { etag: '"rate-v1"' })).resolves.toEqual({
      notModified: true,
      etag: '"rate-v1"',
      data: null
    })
    expect(get).toHaveBeenNthCalledWith(2, '/admin/accounts/upstream-billing-rates', expect.objectContaining({
      headers: { 'If-None-Match': '"rate-v1"' }
    }))
  })

  it('reads one account history range and preserves 304 semantics', async () => {
    const data = {
      account_id: 7,
      range_days: 90,
      truncated: false,
      events: []
    }
    get.mockResolvedValueOnce({
      status: 200,
      headers: { etag: '"history-v1"' },
      data
    })

    await expect(getUpstreamBillingRateHistoryWithEtag(7)).resolves.toEqual({
      notModified: false,
      etag: '"history-v1"',
      data
    })
    expect(get).toHaveBeenCalledWith('/admin/accounts/7/upstream-billing-rate-history', expect.objectContaining({
      params: { days: 90, limit: 500 },
      validateStatus: expect.any(Function)
    }))

    get.mockResolvedValueOnce({ status: 304, headers: { etag: '"history-v1"' }, data: '' })
    await expect(getUpstreamBillingRateHistoryWithEtag(7, 30, { etag: '"history-v1"' })).resolves.toEqual({
      notModified: true,
      etag: '"history-v1"',
      data: null
    })
    expect(get).toHaveBeenNthCalledWith(2, '/admin/accounts/7/upstream-billing-rate-history', expect.objectContaining({
      params: { days: 30, limit: 500 },
      headers: { 'If-None-Match': '"history-v1"' }
    }))
  })

})
