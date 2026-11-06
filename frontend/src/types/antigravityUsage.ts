import type { AccountUsageInfo } from './index'

export interface AntigravityQuotaBucket {
  id: string
  name?: string
  window: string
  utilization: number | null
  resets_at: string | null
  disabled?: boolean
}

export interface AntigravityQuotaGroup {
  name: string
  description?: string
  buckets: AntigravityQuotaBucket[]
}

export interface AntigravityUsageInfo extends AccountUsageInfo {
  antigravity_quota_groups?: AntigravityQuotaGroup[]
  antigravity_quota_source?: 'summary' | 'models' | 'unavailable'
  antigravity_quota_error?: string
  antigravity_quota_stale?: boolean
}
