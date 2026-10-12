import { describe, expect, it, vi } from 'vitest'
import { formatUpstreamModelSyncWarning } from '../upstreamModelSync'

describe('model capability sync diagnostics', () => {
  it('names the missing model and distinguishes it from a registry outage', () => {
    const t = vi.fn((key: string) => key)
    const warning = { code: 'upstream_model_metadata_partial', message: '', model_ids: ['hy4'], complete_count: 25, total_count: 26, reason: 'metadata_missing' }
    expect(formatUpstreamModelSyncWarning([warning], t)).toBe('admin.accounts.syncUpstreamModelsMetadataDetails')
    expect(t).toHaveBeenLastCalledWith('admin.accounts.syncUpstreamModelsMetadataDetails', { count: 25, total: 26, models: 'hy4' })
    expect(formatUpstreamModelSyncWarning([{ ...warning, reason: 'registry_unavailable' }], t)).toBe('admin.accounts.syncUpstreamModelsRegistryUnavailable')
  })
})
