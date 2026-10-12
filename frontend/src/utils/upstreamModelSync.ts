import type { UpstreamModelSyncWarning } from '@/api/admin/accounts'

export function formatUpstreamModelSyncWarning(
  warnings: UpstreamModelSyncWarning[],
  t: (key: string, params?: Record<string, unknown>) => string
): string {
  const warning = warnings.find(item => item.code === 'upstream_model_metadata_incomplete')
    ?? warnings.find(item => item.code === 'upstream_model_metadata_partial')
  if (!warning) return ''
  if (warning.model_ids?.length && warning.total_count !== undefined) {
    const key = warning.reason === 'registry_unavailable'
      ? 'admin.accounts.syncUpstreamModelsRegistryUnavailable'
      : 'admin.accounts.syncUpstreamModelsMetadataDetails'
    return t(key, {
      count: warning.complete_count ?? 0,
      total: warning.total_count,
      models: warning.model_ids.slice(0, 8).join(', ') + (warning.model_ids.length > 8 ? '…' : '')
    })
  }
  return t(warning.code === 'upstream_model_metadata_partial'
    ? 'admin.accounts.syncUpstreamModelsMetadataPartial'
    : 'admin.accounts.syncUpstreamModelsMetadataIncomplete')
}
