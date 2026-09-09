/**
 * Admin CPA (CLIProxyAPI) read-only panel API.
 *
 * 全部数据来自 CPA 的 /v0/management/* 接口，由 Sub2API 后端代理；
 * 前端不做任何写操作，只展示额度、用量与日志。
 */

import { apiClient } from './client'

export interface CpaConfigView {
  enabled: boolean
  base_url: string
  management_key_configured: boolean
  management_key_hint?: string
}

export interface CpaVersionInfo {
  reachable: boolean
  health_ok: boolean
  version?: string
  commit?: string
  build_date?: string
  support_plugin?: string
  key_accepted: boolean
  message?: string
}

export interface CpaOverview {
  config: CpaConfigView
  version: CpaVersionInfo
}

export interface CpaRecentRequestBucket {
  time: string
  success: number
  failed: number
}

export interface CpaQuotaObservation {
  observed_at?: string
  signals?: Record<string, string>
}

export interface CpaAuthFile {
  id: string
  auth_index: string
  name: string
  type?: string
  provider?: string
  label?: string
  status?: string
  status_message?: string
  disabled?: boolean
  unavailable?: boolean
  runtime_only?: boolean
  source?: string
  size?: number
  email?: string
  account_type?: string
  account?: string
  success?: number
  failed?: number
  recent_requests?: CpaRecentRequestBucket[]
  quota?: CpaQuotaObservation
  model_quotas?: Record<string, CpaQuotaObservation>
  created_at?: string
  updated_at?: string
  last_refresh?: string
  next_retry_after?: string
  path?: string
  priority?: number
  note?: string
  websockets?: boolean
}

export interface CpaAuthFileList {
  files: CpaAuthFile[]
}

export interface CpaCodexWindow {
  used_percent?: number | string
  usedPercent?: number | string
  limit_window_seconds?: number | string
  limitWindowSeconds?: number | string
  reset_after_seconds?: number | string
  resetAfterSeconds?: number | string
  reset_at?: number | string
  resetAt?: number | string
}

export interface CpaCodexRateLimit {
  allowed?: boolean
  limit_reached?: boolean
  limitReached?: boolean
  primary_window?: CpaCodexWindow | null
  primaryWindow?: CpaCodexWindow | null
  secondary_window?: CpaCodexWindow | null
  secondaryWindow?: CpaCodexWindow | null
}

export interface CpaCodexAdditionalRateLimit {
  limit_name?: string
  limitName?: string
  metered_feature?: string
  meteredFeature?: string
  rate_limit?: CpaCodexRateLimit | null
  rateLimit?: CpaCodexRateLimit | null
}

export interface CpaCodexResetCredits {
  available_count?: number | string
  availableCount?: number | string
  applicable_available_count?: number | string
  applicableAvailableCount?: number | string
}

export interface CpaCodexUsagePayload {
  plan_type?: string
  planType?: string
  rate_limit?: CpaCodexRateLimit | null
  rateLimit?: CpaCodexRateLimit | null
  code_review_rate_limit?: CpaCodexRateLimit | null
  codeReviewRateLimit?: CpaCodexRateLimit | null
  additional_rate_limits?: CpaCodexAdditionalRateLimit[] | null
  additionalRateLimits?: CpaCodexAdditionalRateLimit[] | null
  rate_limit_reset_credits?: CpaCodexResetCredits | null
  rateLimitResetCredits?: CpaCodexResetCredits | null
}

export interface CpaApiKeyUsageEntry {
  success: number
  failed: number
  recent_requests?: CpaRecentRequestBucket[]
}

/** provider -> "base_url|api_key" -> usage */
export type CpaApiKeyUsage = Record<string, Record<string, CpaApiKeyUsageEntry>>

export interface CpaErrorLogFile {
  name: string
  size: number
  modified: number
}

export interface CpaErrorLogList {
  files: CpaErrorLogFile[]
}

export interface CpaLogEntry {
  time?: string
  timestamp?: string
  level?: string
  message?: string
  [key: string]: unknown
}

export interface CpaLogsResponse {
  lines?: string[]
  logs?: CpaLogEntry[]
  total?: number
  latest?: number
  cursor?: string
  [key: string]: unknown
}

// apiClient 的响应拦截器已经把 { code, message, data } 解包成 data，
// 所以这里直接取 axios 的 response.data 即为业务负载（与 src/api/admin/ops.ts 一致）。
export const cpaApi = {
  async getConfig(): Promise<CpaConfigView> {
    const { data } = await apiClient.get<CpaConfigView>('/admin/cpa/config')
    return data
  },

  async updateConfig(payload: {
    enabled: boolean
    base_url: string
    management_key?: string | null
  }): Promise<CpaConfigView> {
    const { data } = await apiClient.put<CpaConfigView>('/admin/cpa/config', payload)
    return data
  },

  async testConnection(): Promise<CpaVersionInfo> {
    const { data } = await apiClient.post<CpaVersionInfo>('/admin/cpa/test')
    return data
  },

  async getOverview(): Promise<CpaOverview> {
    const { data } = await apiClient.get<CpaOverview>('/admin/cpa/overview')
    return data
  },

  async listAuthFiles(): Promise<CpaAuthFileList> {
    const { data } = await apiClient.get<CpaAuthFileList>('/admin/cpa/auth-files', { timeout: 30000 })
    return data
  },

  async getAuthFileQuota(authIndex: string): Promise<CpaCodexUsagePayload> {
    const { data } = await apiClient.get<CpaCodexUsagePayload>('/admin/cpa/auth-files/quota', {
      params: { auth_index: authIndex },
      timeout: 30000
    })
    return data
  },

  async getApiKeyUsage(): Promise<CpaApiKeyUsage> {
    const { data } = await apiClient.get<CpaApiKeyUsage>('/admin/cpa/api-key-usage')
    return data
  },

  async getLogs(params: { limit?: number; cursor?: string; after?: string } = {}): Promise<CpaLogsResponse> {
    const { data } = await apiClient.get<CpaLogsResponse>('/admin/cpa/logs', { params })
    return data
  },

  async listErrorLogs(): Promise<CpaErrorLogList> {
    const { data } = await apiClient.get<CpaErrorLogList>('/admin/cpa/error-logs')
    return data
  },

  async getErrorLogContent(name: string): Promise<{ name: string; content: string }> {
    const { data } = await apiClient.get<{ name: string; content: string }>(
      `/admin/cpa/error-logs/${encodeURIComponent(name)}`
    )
    return data
  }
}
