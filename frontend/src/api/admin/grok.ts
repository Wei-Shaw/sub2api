/**
 * Admin Grok/xAI API endpoints
 * Handles xAI OAuth flows for administrators.
 */

import { apiClient } from '../client'

export interface GrokAuthUrlResponse {
  auth_url: string
  session_id: string
  state: string
}

export interface GrokAuthUrlRequest {
  proxy_id?: number
  redirect_uri?: string
}

export interface GrokExchangeCodeRequest {
  session_id: string
  state: string
  code: string
  proxy_id?: number
  redirect_uri?: string
}

export interface GrokTokenInfo {
  access_token?: string
  refresh_token?: string
  token_type?: string
  id_token?: string
  expires_at?: number | string
  expires_in?: number
  scope?: string
  client_id?: string
  email?: string
  subscription_tier?: string
  entitlement_status?: string
  [key: string]: unknown
}

export interface GrokQuotaWindow {
  limit?: number | null
  remaining?: number | null
  reset_unix?: number | null
  reset_at?: string | null
}

export interface GrokQuotaSnapshot {
  requests?: GrokQuotaWindow | null
  tokens?: GrokQuotaWindow | null
  retry_after_seconds?: number | null
  subscription_tier?: string
  entitlement_status?: string
  status_code?: number
  headers?: Record<string, string>
  headers_observed: boolean
  observation_source?: string
  last_probe_at?: string
  last_headers_seen_at?: string
  updated_at: string
}

export interface GrokProductUsage {
  product: string
  usage_percent?: number | null
}

export interface GrokBillingSummary {
  period_type?: string
  usage_percent?: number | null
  period_start?: string
  period_end?: string
  product_usage?: GrokProductUsage[]
  monthly_limit_cents?: number | null
  used_cents?: number | null
  included_used_cents?: number | null
  on_demand_cap_cents?: number | null
  on_demand_used_cents?: number | null
  on_demand_used_percent?: number | null
  billing_period_start?: string
  billing_period_end?: string
  used_percent?: number | null
  plan_label?: string
  updated_at?: string
  source?: string
}

export interface GrokQuotaProbeResult {
  source: 'billing_probe' | 'billing_and_ratelimit_probe' | 'active_probe' | string
  model?: string
  billing?: GrokBillingSummary | null
  snapshot?: GrokQuotaSnapshot | null
  status_code?: number
  headers_observed: boolean
  billing_observed?: boolean
  reset_supported: boolean
  fetched_at: number
}

export interface GrokQuotaResetResult {
  supported: boolean
  code: string
  message: string
}

export async function generateAuthUrl(
  payload: GrokAuthUrlRequest
): Promise<GrokAuthUrlResponse> {
  const { data } = await apiClient.post<GrokAuthUrlResponse>(
    '/admin/grok/oauth/auth-url',
    payload
  )
  return data
}

export async function exchangeCode(payload: GrokExchangeCodeRequest): Promise<GrokTokenInfo> {
  const { data } = await apiClient.post<GrokTokenInfo>(
    '/admin/grok/oauth/exchange-code',
    payload
  )
  return data
}

export async function refreshGrokToken(
  refreshToken: string,
  proxyId?: number | null
): Promise<GrokTokenInfo> {
  const payload: Record<string, unknown> = { refresh_token: refreshToken }
  if (proxyId) payload.proxy_id = proxyId

  const { data } = await apiClient.post<GrokTokenInfo>(
    '/admin/grok/oauth/refresh-token',
    payload
  )
  return data
}

export async function queryQuota(id: number): Promise<GrokQuotaProbeResult> {
  const { data } = await apiClient.get<GrokQuotaProbeResult>(`/admin/grok/accounts/${id}/quota`)
  return data
}

export async function resetQuota(id: number): Promise<GrokQuotaResetResult> {
  const { data } = await apiClient.post<GrokQuotaResetResult>(`/admin/grok/accounts/${id}/reset-quota`)
  return data
}

export default { generateAuthUrl, exchangeCode, refreshGrokToken, queryQuota, resetQuota }
