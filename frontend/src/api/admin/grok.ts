/**
 * Admin Grok/xAI API endpoints
 * Handles xAI OAuth flows for administrators.
 */

import { apiClient } from '../client'
import type { GrokBillingSummary, GrokQuotaWindow, WindowStats } from '@/types'

export type { GrokBillingSummary, GrokQuotaWindow } from '@/types'

export interface GrokAuthUrlResponse {
  auth_url: string
  session_id: string
  state: string
}

export interface GrokAuthUrlRequest {
  proxy_id?: number
  redirect_uri?: string
}

export interface GrokOAuthCapabilities {
  password_auth_enabled: boolean
}

const GROK_AUTHORIZATION_TIMEOUT_MS = 120_000

export async function getCapabilities(): Promise<GrokOAuthCapabilities> {
  const { data } = await apiClient.get<GrokOAuthCapabilities>('/admin/grok/oauth/capabilities')
  return data
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
  sub?: string
  team_id?: string
  subscription_tier?: string
  entitlement_status?: string
  /** xAI access-token claim; 1 means risk-control marking that degrades media. */
  bot_flag_source?: number
  has_bfs?: boolean
  bfs?: unknown
  [key: string]: unknown
}

export interface GrokSSOToOAuthRequest {
  sso_tokens: string[]
  name?: string
  notes?: string | null
  proxy_id?: number | null
  group_ids?: number[]
  credentials?: Record<string, unknown>
  extra?: Record<string, unknown>
  concurrency?: number
  load_factor?: number
  priority?: number
  rate_multiplier?: number
  expires_at?: number | null
  auto_pause_on_expired?: boolean
  skip_risk_flagged?: boolean
}

export interface GrokSSOToOAuthItemResult {
  index: number
  name?: string
  email?: string
  account?: unknown
  error?: string
}

export interface GrokSSOToOAuthResponse {
  created: GrokSSOToOAuthItemResult[]
  failed: GrokSSOToOAuthItemResult[]
}

const GROK_SSO_IMPORT_CONCURRENCY = 3
const GROK_SSO_IMPORT_TIMEOUT_PER_BATCH_MS = 90_000
const GROK_SSO_IMPORT_TIMEOUT_BUFFER_MS = 90_000
export const GROK_RISK_MAX_BATCH_IDS = 200
const GROK_RISK_CHECK_TIMEOUT_PER_ID_MS = 500
const GROK_RISK_CHECK_TIMEOUT_BUFFER_MS = 30_000

export function getGrokSSOImportTimeout(keyCount: number): number {
  const batches = Math.ceil(Math.max(1, keyCount) / GROK_SSO_IMPORT_CONCURRENCY)
  return batches * GROK_SSO_IMPORT_TIMEOUT_PER_BATCH_MS + GROK_SSO_IMPORT_TIMEOUT_BUFFER_MS
}

export function getGrokRiskCheckTimeout(idCount: number): number {
  return Math.max(1, idCount) * GROK_RISK_CHECK_TIMEOUT_PER_ID_MS + GROK_RISK_CHECK_TIMEOUT_BUFFER_MS
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

export interface GrokQuotaProbeResult {
  source: 'active_probe' | 'billing_probe' | 'hybrid_probe'
  model?: string
  billing?: GrokBillingSummary | null
  snapshot?: GrokQuotaSnapshot | null
  local_usage_24h?: WindowStats | null
  local_usage_7d?: WindowStats | null
  local_usage_monthly?: WindowStats | null
  status_code?: number
  headers_observed: boolean
  reset_supported: boolean
  fetched_at: number
  persisted?: boolean
  probe_error?: string
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

export interface GrokRiskReport {
  verdict: string
  kind?: string
  bot_flag_source?: number
  has_bfs?: boolean
  bfs?: unknown
  policy?: string
  risk?: number
  event?: string
  details?: string
  denied?: boolean
  source?: string
  error?: string
  email?: string
  checked_at?: string
}

export interface GrokSSOCheckStateItem {
  index: number
  email?: string
  verdict: string
  kind?: string
  bot_flag_source?: number
  details?: string
  policy?: string
  risk?: number
  event?: string
  denied?: boolean
  status_code?: number
  error?: string
}

export interface GrokSSOCheckStateResponse {
  total: number
  flagged: number
  ip: number
  account: number
  clean: number
  unknown: number
  error: number
  items: GrokSSOCheckStateItem[]
}

export interface GrokCheckRiskItem {
  account_id: number
  name?: string
  email?: string
  verdict: string
  kind?: string
  report?: GrokRiskReport
  error?: string
}

export interface GrokCheckRiskResponse {
  total: number
  flagged: number
  clean: number
  error: number
  skipped: number
  items: GrokCheckRiskItem[]
}

export async function createFromSSO(payload: GrokSSOToOAuthRequest): Promise<GrokSSOToOAuthResponse> {
  const { data } = await apiClient.post<GrokSSOToOAuthResponse>(
    '/admin/grok/sso-to-oauth',
    payload,
    { timeout: getGrokSSOImportTimeout(payload.sso_tokens.length) }
  )
  return data
}

/** Validate a browser SSO cookie and convert to Build OAuth tokens (no raw SSO stored). */
export async function validateSSOToken(
  ssoToken: string,
  proxyId?: number | null
): Promise<GrokTokenInfo> {
  const payload: Record<string, unknown> = { sso_token: ssoToken }
  if (proxyId) payload.proxy_id = proxyId
  const { data } = await apiClient.post<GrokTokenInfo>('/admin/grok/oauth/sso-token', payload, {
    timeout: GROK_AUTHORIZATION_TIMEOUT_MS
  })
  return data
}

/**
 * Password login → ephemeral SSO → Build OAuth.
 * Password is only sent over the wire for this call; never persist it in credentials.
 */
export async function authorizePassword(
  emailAndPassword: string,
  proxyId?: number | null
): Promise<GrokTokenInfo> {
  // Format: email----password (password may contain dashes).
  const sep = '----'
  const idx = emailAndPassword.indexOf(sep)
  const email = (idx >= 0 ? emailAndPassword.slice(0, idx) : emailAndPassword).trim()
  const password = idx >= 0 ? emailAndPassword.slice(idx + sep.length) : ''
  const payload: Record<string, unknown> = { email, password }
  if (proxyId) payload.proxy_id = proxyId
  const { data } = await apiClient.post<GrokTokenInfo>('/admin/grok/oauth/password', payload, {
    timeout: GROK_AUTHORIZATION_TIMEOUT_MS
  })
  return data
}

export async function checkSSOState(payload: {
  sso_tokens: string[]
  proxy_id?: number | null
}): Promise<GrokSSOCheckStateResponse> {
  const { data } = await apiClient.post<GrokSSOCheckStateResponse>(
    '/admin/grok/sso-check-state',
    payload,
    { timeout: getGrokSSOImportTimeout(payload.sso_tokens.length) }
  )
  return data
}

function emptyGrokCheckRiskResponse(): GrokCheckRiskResponse {
  return { total: 0, flagged: 0, clean: 0, error: 0, skipped: 0, items: [] }
}

function mergeGrokCheckRiskResponses(parts: GrokCheckRiskResponse[]): GrokCheckRiskResponse {
  const out = emptyGrokCheckRiskResponse()
  for (const part of parts) {
    out.flagged += part.flagged
    out.clean += part.clean
    out.error += part.error
    out.skipped += part.skipped
    out.items.push(...(part.items ?? []))
  }
  out.total = out.items.length
  return out
}

export async function checkAccountsRisk(accountIds: number[]): Promise<GrokCheckRiskResponse> {
  const ids = [...new Set(accountIds.filter((id) => Number.isFinite(id) && id > 0))]
  if (ids.length === 0) {
    return emptyGrokCheckRiskResponse()
  }
  const parts: GrokCheckRiskResponse[] = []
  for (let offset = 0; offset < ids.length; offset += GROK_RISK_MAX_BATCH_IDS) {
    const chunk = ids.slice(offset, offset + GROK_RISK_MAX_BATCH_IDS)
    const { data } = await apiClient.post<GrokCheckRiskResponse>(
      '/admin/grok/accounts/check-risk',
      { account_ids: chunk },
      { timeout: getGrokRiskCheckTimeout(chunk.length) }
    )
    parts.push(data)
  }
  return mergeGrokCheckRiskResponses(parts)
}

export async function checkAccountRisk(id: number): Promise<GrokCheckRiskItem> {
  const { data } = await apiClient.post<GrokCheckRiskItem>(`/admin/grok/accounts/${id}/check-risk`)
  return data
}

export default {
  generateAuthUrl,
  getCapabilities,
  exchangeCode,
  refreshGrokToken,
  queryQuota,
  resetQuota,
  createFromSSO,
  checkSSOState,
  checkAccountsRisk,
  checkAccountRisk,
  validateSSOToken,
  authorizePassword,
}
