/**
 * Gemini OAuth API endpoints (plugin-local).
 * Mirrors the host's src/api/admin/gemini.ts using the plugin API client.
 */
import { getClient } from './client'

export interface GeminiAuthUrlResponse {
  auth_url: string
  session_id: string
  state: string
}

export interface GeminiOAuthCapabilities {
  ai_studio_oauth_enabled: boolean
  required_redirect_uris: string[]
}

export interface GeminiAuthUrlRequest {
  proxy_id?: number
  project_id?: string
  oauth_type?: 'code_assist' | 'google_one' | 'ai_studio'
  tier_id?: string
}

export interface GeminiExchangeCodeRequest {
  session_id: string
  state: string
  code: string
  proxy_id?: number
  oauth_type?: 'code_assist' | 'google_one' | 'ai_studio'
  tier_id?: string
}

export interface GeminiTokenInfo {
  access_token?: string
  refresh_token?: string
  token_type?: string
  scope?: string
  expires_in?: number
  expires_at?: number | string
  project_id?: string
  oauth_type?: string
  tier_id?: string
  extra?: Record<string, unknown>
  [key: string]: unknown
}

export async function generateAuthUrl(
  payload: GeminiAuthUrlRequest
): Promise<GeminiAuthUrlResponse> {
  const { data } = await getClient().post<GeminiAuthUrlResponse>(
    '/api/v1/admin/gemini/oauth/auth-url',
    payload
  )
  return data
}

export async function exchangeCode(
  payload: GeminiExchangeCodeRequest
): Promise<GeminiTokenInfo> {
  const { data } = await getClient().post<GeminiTokenInfo>(
    '/api/v1/admin/gemini/oauth/exchange-code',
    payload
  )
  return data
}

export async function getCapabilities(): Promise<GeminiOAuthCapabilities> {
  const { data } = await getClient().get<GeminiOAuthCapabilities>(
    '/api/v1/admin/gemini/oauth/capabilities'
  )
  return data
}
