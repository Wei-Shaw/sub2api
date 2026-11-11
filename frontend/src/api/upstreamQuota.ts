import { apiClient } from './client'

export type QuotaPlatform = 'openai' | 'antigravity' | 'gemini'
export interface UpstreamQuotaSettings { enabled: boolean; account_ids: number[] }
export interface SharedQuotaWindow { key: string; variant?: number; used_percent: number; resets_at: string | null }
export interface SharedQuotaAccount {
  id: number
  alias: string
  status: 'available' | 'rate_limited' | 'disabled' | 'error'
  quota_state: 'known' | 'unknown' | 'unavailable'
  current_concurrency: number | null
  max_concurrency: number
  updated_at: string | null
  windows: SharedQuotaWindow[]
}
export interface UpstreamQuotaSnapshot { accounts: SharedQuotaAccount[]; cache_seconds: number }
export const upstreamQuotaAPI = {
  async config(): Promise<{ enabled: boolean }> {
    return (await apiClient.get('/upstream-quota/config')).data
  },
  async snapshot(platform: QuotaPlatform): Promise<UpstreamQuotaSnapshot> {
    return (await apiClient.get('/upstream-quota', { params: { platform } })).data
  },
  async settings(): Promise<UpstreamQuotaSettings> {
    return (await apiClient.get('/admin/upstream-quota/settings')).data
  },
  async save(settings: UpstreamQuotaSettings): Promise<void> {
    await apiClient.put('/admin/upstream-quota/settings', settings)
  }
}
