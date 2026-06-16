/**
 * Antigravity OAuth + default-mapping API calls.
 *
 * Uses the host axios instance (via setClient in install()) which
 * already includes auth headers, base URL, and error interceptors.
 */
import { getClient } from './client'

export interface AntigravityTokenInfo {
  access_token?: string
  refresh_token?: string
  token_type?: string
  expires_at?: number | string
  expires_in?: number
  project_id?: string
  email?: string
  [key: string]: unknown
}

interface AuthUrlResponse {
  auth_url: string
  session_id: string
  state: string
}

export async function generateAuthUrl(
  payload: Record<string, unknown>
): Promise<AuthUrlResponse> {
  const { data } = await getClient().post<AuthUrlResponse>(
    '/admin/antigravity/oauth/auth-url',
    payload
  )
  return data
}

export async function exchangeCode(
  payload: Record<string, unknown>
): Promise<AntigravityTokenInfo> {
  const { data } = await getClient().post<AntigravityTokenInfo>(
    '/admin/antigravity/oauth/exchange-code',
    payload
  )
  return data
}

export async function refreshAntigravityToken(
  refreshToken: string,
  proxyId?: number | null
): Promise<AntigravityTokenInfo> {
  const payload: Record<string, unknown> = { refresh_token: refreshToken }
  if (proxyId) payload.proxy_id = proxyId
  const { data } = await getClient().post<AntigravityTokenInfo>(
    '/admin/antigravity/oauth/refresh-token',
    payload
  )
  return data
}

export async function getDefaultModelMapping(): Promise<Record<string, string>> {
  const { data } = await getClient().get<Record<string, string>>(
    '/admin/accounts/antigravity/default-model-mapping'
  )
  return data
}
