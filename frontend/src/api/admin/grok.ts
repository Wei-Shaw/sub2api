/**
 * Admin Grok/xAI API endpoints
 * Handles xAI OAuth flows for administrators.
 */

import { apiClient REDACTED from '../client'
import type { GrokBillingSummary, GrokQuotaWindow, WindowStats REDACTED from '@/types'

export type { GrokBillingSummary, GrokQuotaWindow REDACTED from '@/types'

export interface GrokAuthUrlResponse {
  auth_url: string
  session_id: string
  state: string
REDACTED

export interface GrokAuthUrlRequest {
  proxy_id?: number
  redirect_uri?: string
REDACTED

export interface GrokExchangeCodeRequest {
  session_id: string
  state: string
  code: string
  proxy_id?: number
  redirect_uri?: string
REDACTED

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
REDACTED

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
REDACTED

export interface GrokQuotaProbeResult {
  source: 'active_probe' | 'billing_probe' | 'hybrid_probe'
  model?: string
  billing?: GrokBillingSummary | null
  snapshot?: GrokQuotaSnapshot | null
  local_usage_7d?: WindowStats | null
  local_usage_monthly?: WindowStats | null
  status_code?: number
  headers_observed: boolean
  reset_supported: boolean
  fetched_at: number
  persisted?: boolean
  probe_error?: string
REDACTED

export interface GrokQuotaResetResult {
  supported: boolean
  code: string
  message: string
REDACTED

export async function generateAuthUrl(
  payload: GrokAuthUrlRequest
): Promise<GrokAuthUrlResponse> {
  const { data REDACTED = await apiClient.post<GrokAuthUrlResponse>(
    '/admin/grok/oauth/auth-url',
    payload
  )
  return data
REDACTED

export async function exchangeCode(payload: GrokExchangeCodeRequest): Promise<GrokTokenInfo> {
  const { data REDACTED = await apiClient.post<GrokTokenInfo>(
    '/admin/grok/oauth/exchange-code',
    payload
  )
  return data
REDACTED

export async function refreshGrokToken(
  refreshToken: string,
  proxyId?: number | null
): Promise<GrokTokenInfo> {
  const payload: Record<string, unknown> = { refresh_token: refreshToken REDACTED
  if (proxyId) payload.proxy_id = proxyId

  const { data REDACTED = await apiClient.post<GrokTokenInfo>(
    '/admin/grok/oauth/refresh-token',
    payload
  )
  return data
REDACTED

export async function queryQuota(id: number): Promise<GrokQuotaProbeResult> {
  const { data REDACTED = await apiClient.get<GrokQuotaProbeResult>(`/admin/grok/accounts/${idREDACTED/quota`)
  return data
REDACTED

export async function resetQuota(id: number): Promise<GrokQuotaResetResult> {
  const { data REDACTED = await apiClient.post<GrokQuotaResetResult>(`/admin/grok/accounts/${idREDACTED/reset-quota`)
  return data
REDACTED

export default { generateAuthUrl, exchangeCode, refreshGrokToken, queryQuota, resetQuota REDACTED
