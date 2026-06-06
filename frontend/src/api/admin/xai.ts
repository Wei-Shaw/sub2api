/**
 * Admin xAI API endpoints
 * Handles xAI/Grok OAuth flows for administrators.
 */

import { apiClient } from '../client'

export interface XAIAuthUrlResponse {
  auth_url: string
  session_id: string
  state: string
}

export interface XAIAuthUrlRequest {
  proxy_id?: number
  redirect_uri?: string
}

export interface XAIExchangeCodeRequest {
  session_id: string
  state: string
  code: string
  proxy_id?: number
}

export interface XAITokenInfo {
  access_token?: string
  refresh_token?: string
  id_token?: string
  token_type?: string
  expires_at?: number | string
  expires_in?: number
  email?: string
  sub?: string
  base_url?: string
  redirect_uri?: string
  token_endpoint?: string
  auth_kind?: string
  type?: string
  [key: string]: unknown
}

export async function generateAuthUrl(payload: XAIAuthUrlRequest): Promise<XAIAuthUrlResponse> {
  const { data } = await apiClient.post<XAIAuthUrlResponse>('/admin/xai/oauth/auth-url', payload)
  return data
}

export async function exchangeCode(payload: XAIExchangeCodeRequest): Promise<XAITokenInfo> {
  const { data } = await apiClient.post<XAITokenInfo>('/admin/xai/oauth/exchange-code', payload)
  return data
}

export async function refreshXAIToken(
  refreshToken: string,
  proxyId?: number | null
): Promise<XAITokenInfo> {
  const payload: Record<string, unknown> = { refresh_token: refreshToken }
  if (proxyId) payload.proxy_id = proxyId

  const { data } = await apiClient.post<XAITokenInfo>('/admin/xai/oauth/refresh-token', payload)
  return data
}

export default { generateAuthUrl, exchangeCode, refreshXAIToken }
