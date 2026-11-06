import { beforeEach, describe, expect, it, vi } from 'vitest'
import { getDashboardOverview, getDashboardSnapshotV2, type OpsTrafficSummary } from '@/api/admin/ops'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { get }, buildGatewayUrl: vi.fn() }))

const traffic: OpsTrafficSummary = {
  source: 'http_requests',
  request_count: 2,
  upstream: {
    original_bytes: 100,
    actual_bytes: 120,
    original_sample_count: 2,
    actual_sample_count: 3,
    paired_sample_count: 2,
    compression_percent: -20,
    request: {
      original_bytes: 40,
      actual_bytes: 50,
      original_sample_count: 1,
      actual_sample_count: 2
    },
    response: {
      original_bytes: 60,
      actual_bytes: 70,
      original_sample_count: 1,
      actual_sample_count: 1
    }
  },
  downstream: {
    original_bytes: null,
    actual_bytes: 0,
    original_sample_count: 0,
    actual_sample_count: 1,
    paired_sample_count: 0,
    compression_percent: null
  }
}

beforeEach(() => get.mockReset())

describe('admin ops traffic overview transport', () => {
  it.each(['auto', 'raw', 'preagg'] as const)('keeps traffic on the existing filtered overview in %s mode', async (mode) => {
    const params = { time_range: '6h' as const, platform: 'openai', group_id: 42, mode }
    const signal = new AbortController().signal
    const data = { traffic }
    get.mockResolvedValue({ data })

    await expect(getDashboardOverview(params, { signal })).resolves.toBe(data)
    expect(get).toHaveBeenCalledTimes(1)
    expect(get).toHaveBeenCalledWith('/admin/ops/dashboard/overview', { params, signal })
  })

  it('preserves traffic, unknowns and measured zero in snapshot-v2 with custom time filters', async () => {
    const params = {
      start_time: '2026-10-01T12:00:00Z',
      end_time: '2026-10-01T18:00:00Z',
      platform: 'anthropic', group_id: 7, mode: 'preagg' as const
    }
    const signal = new AbortController().signal
    const data = { generated_at: params.end_time, overview: { traffic } }
    get.mockResolvedValue({ data })

    const result = await getDashboardSnapshotV2(params, { signal })
    expect(result).toBe(data)
    expect(result.overview.traffic?.upstream.compression_percent).toBe(-20)
    expect(result.overview.traffic?.upstream.request?.actual_bytes).toBe(50)
    expect(result.overview.traffic?.upstream.response?.original_bytes).toBe(60)
    expect(result.overview.traffic?.downstream.request).toBeUndefined()
    expect(result.overview.traffic?.downstream.response).toBeUndefined()
    expect(result.overview.traffic?.downstream.original_bytes).toBeNull()
    expect(result.overview.traffic?.downstream.actual_bytes).toBe(0)
    expect(get).toHaveBeenCalledTimes(1)
    expect(get).toHaveBeenCalledWith('/admin/ops/dashboard/snapshot-v2', { params, signal })
  })

  it.each([undefined, null])('does not invent zero traffic for an older response: %s', async (traffic) => {
    const data = traffic === undefined ? {} : { traffic }
    get.mockResolvedValue({ data })
    const result = await getDashboardOverview({ time_range: '1h' })
    expect(result.traffic).toBe(traffic)
    expect(get).toHaveBeenCalledTimes(1)
  })

  it.each([undefined, null, { scope: 'instance_lifetime' as const, pending: 2, failed: 3, flushed: 100 }])(
    'preserves instance-scoped ingestion health without substituting a healthy zero: %s', async (ingestion) => {
      const overview = { traffic: { ...traffic, ingestion } }
      get.mockResolvedValueOnce({ data: overview }).mockResolvedValueOnce({ data: { overview } })
      const params = { time_range: '5m' as const, platform: 'openai', group_id: 7, mode: 'raw' as const }
      const direct = await getDashboardOverview(params)
      const snapshot = await getDashboardSnapshotV2(params)
      expect(direct.traffic?.ingestion).toEqual(ingestion)
      expect(snapshot.overview.traffic?.ingestion).toEqual(ingestion)
      expect(get).toHaveBeenCalledTimes(2)
    }
  )
})
