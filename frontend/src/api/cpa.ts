/**
 * Admin CPA (CLIProxyAPI) read-only panel API.
 *
 * 全部数据来自 CPA 的 /v0/management/* 接口，由 Sub2API 后端代理；
 * 前端不做任何写操作，只展示额度、用量与日志。
 */

import { apiClient } from './client'
import type { ApiResponse } from '@/types'

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

const unwrap = <T>(response: { data: ApiResponse<T> }): T => response.data.data

export const cpaApi = {
  async getConfig(): Promise<CpaConfigView> {
    return unwrap(await apiClient.get<ApiResponse<CpaConfigView>>('/admin/cpa/config'))
  },

  async updateConfig(payload: {
    enabled: boolean
    base_url: string
    management_key?: string | null
  }): Promise<CpaConfigView> {
    return unwrap(await apiClient.put<ApiResponse<CpaConfigView>>('/admin/cpa/config', payload))
  },

  async testConnection(): Promise<CpaVersionInfo> {
    return unwrap(await apiClient.post<ApiResponse<CpaVersionInfo>>('/admin/cpa/test'))
  },

  async getOverview(): Promise<CpaOverview> {
    return unwrap(await apiClient.get<ApiResponse<CpaOverview>>('/admin/cpa/overview'))
  },

  async listAuthFiles(): Promise<CpaAuthFileList> {
    return unwrap(
      await apiClient.get<ApiResponse<CpaAuthFileList>>('/admin/cpa/auth-files', {
        timeout: 30000
      })
    )
  },

  async getAuthFileQuota(authIndex: string): Promise<CpaCodexUsagePayload> {
    return unwrap(
      await apiClient.get<ApiResponse<CpaCodexUsagePayload>>('/admin/cpa/auth-files/quota', {
        params: { auth_index: authIndex },
        timeout: 30000
      })
    )
  },

  async getApiKeyUsage(): Promise<CpaApiKeyUsage> {
    return unwrap(await apiClient.get<ApiResponse<CpaApiKeyUsage>>('/admin/cpa/api-key-usage'))
  },

  async getLogs(params: { limit?: number; cursor?: string; after?: string } = {}): Promise<CpaLogsResponse> {
    return unwrap(await apiClient.get<ApiResponse<CpaLogsResponse>>('/admin/cpa/logs', { params }))
  },

  async listErrorLogs(): Promise<CpaErrorLogList> {
    return unwrap(await apiClient.get<ApiResponse<CpaErrorLogList>>('/admin/cpa/error-logs'))
  },

  async getErrorLogContent(name: string): Promise<{ name: string; content: string }> {
    return unwrap(
      await apiClient.get<ApiResponse<{ name: string; content: string }>>(
        `/admin/cpa/error-logs/${encodeURIComponent(name)}`
      )
    )
  }
}
