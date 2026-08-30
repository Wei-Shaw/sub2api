import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, put, post } = vi.hoisted(() => ({
  get: vi.fn(),
  put: vi.fn(),
  post: vi.fn(),
}))

vi.mock('@/api/client', () => ({ apiClient: { get, put, post } }))

import upstreamPriceMonitorAPI from '@/api/admin/upstreamPriceMonitor'

describe('upstream price monitor API', () => {
  beforeEach(() => {
    get.mockReset()
    put.mockReset()
    post.mockReset()
  })

  it('uses the isolated admin monitor endpoints', async () => {
    const config = {
      enabled: true,
      mode: 'observe' as const,
      interval_minutes: 15,
      markup: 1.2,
      display_multiplier_decimals: 3,
      account_ids: [7],
      channel_ids: [3],
      domestic_models: ['deepseek-v4-flash-0731'],
      passive_sample_max_age_minutes: 60,
      active_probe_enabled: true,
    }
    get.mockResolvedValueOnce({ data: config })
    put.mockResolvedValueOnce({ data: config })

    await expect(upstreamPriceMonitorAPI.getConfig()).resolves.toEqual(config)
    await expect(upstreamPriceMonitorAPI.updateConfig(config)).resolves.toEqual(config)

    expect(get).toHaveBeenCalledWith('/admin/upstream-price-monitor/config', { signal: undefined })
    expect(put).toHaveBeenCalledWith('/admin/upstream-price-monitor/config', config)
  })

  it('always creates a manual dry run and applies by snapshot hash', async () => {
    const run = {
      id: 1,
      trigger: 'manual',
      status: 'completed',
      mode: 'observe',
      dry_run: true,
      started_at: '2026-08-30T00:00:00Z',
      matched_models: 18,
      mismatched_models: 0,
      probe_cost: 0,
      snapshot_hash: 'abc123',
    }
    post.mockResolvedValue({ data: run })

    await upstreamPriceMonitorAPI.createRun()
    await upstreamPriceMonitorAPI.applyRun(1, { snapshot_hash: 'abc123' })
    await upstreamPriceMonitorAPI.rollbackRun(1, { snapshot_hash: 'abc123' })

    expect(post).toHaveBeenNthCalledWith(1, '/admin/upstream-price-monitor/runs', { dry_run: true })
    expect(post).toHaveBeenNthCalledWith(2, '/admin/upstream-price-monitor/runs/1/apply', { snapshot_hash: 'abc123' })
    expect(post).toHaveBeenNthCalledWith(3, '/admin/upstream-price-monitor/runs/1/rollback', { snapshot_hash: 'abc123' })
  })

  it('discovers and manages model scope without a paid run', async () => {
    const item = {
      model: 'deepseek-new-v1', status: 'discovered', domestic_candidate: true,
      seen_account_count: 1, expected_account_count: 1, missing_runs: 0,
      discovery_complete: true, updated_at: '2026-08-30T00:00:00Z',
    }
    get.mockResolvedValueOnce({ data: { items: [item] } })
    post.mockResolvedValueOnce({ data: { items: [item] } })
    post.mockResolvedValueOnce({ data: { ...item, status: 'managed' } })

    await upstreamPriceMonitorAPI.listModels()
    await upstreamPriceMonitorAPI.discoverModels()
    await upstreamPriceMonitorAPI.updateModelStatus(item.model, 'managed')

    expect(get).toHaveBeenCalledWith('/admin/upstream-price-monitor/models', { signal: undefined })
    expect(post).toHaveBeenNthCalledWith(1, '/admin/upstream-price-monitor/models/discover')
    expect(post).toHaveBeenNthCalledWith(2, '/admin/upstream-price-monitor/models/status', { model: item.model, status: 'managed' })
  })
})
