/**
 * Admin CodeBuddy API endpoints
 * Handles CodeBuddy (Tencent copilot) OAuth flows for administrators.
 */

import { apiClient } from '../client'

export interface CodeBuddyAuthUrlResponse {
  auth_url: string
  session_id: string
  state: string
}

export interface CodeBuddyAuthUrlRequest {
  proxy_id?: number
  redirect_uri?: string
}

export interface CodeBuddyExchangeStateRequest {
  session_id?: string
  state: string
  proxy_id?: number
}

export interface CodeBuddyTokenInfo {
  access_token?: string
  refresh_token?: string
  token_type?: string
  expires_at?: number | string
  expires_in?: number
  uid?: string
  nickname?: string
  uin?: string
  phone_number?: string
  domain?: string
  scope?: string
  enabled_models?: string[]
  [key: string]: unknown
}

export async function generateAuthUrl(
  payload: CodeBuddyAuthUrlRequest
): Promise<CodeBuddyAuthUrlResponse> {
  const { data } = await apiClient.post<CodeBuddyAuthUrlResponse>(
    '/admin/codebuddy/oauth/auth-url',
    payload
  )
  return data
}

export async function exchangeState(payload: CodeBuddyExchangeStateRequest): Promise<CodeBuddyTokenInfo> {
  const { data } = await apiClient.post<CodeBuddyTokenInfo>(
    '/admin/codebuddy/oauth/exchange-state',
    payload
  )
  return data
}

export async function refreshCodeBuddyToken(
  refreshToken: string,
  proxyId?: number | null
): Promise<CodeBuddyTokenInfo> {
  const payload: Record<string, unknown> = { refresh_token: refreshToken }
  if (proxyId) payload.proxy_id = proxyId

  const { data } = await apiClient.post<CodeBuddyTokenInfo>(
    '/admin/codebuddy/oauth/refresh-token',
    payload
  )
  return data
}

export async function createAccountFromOAuth(payload: {
  session_id?: string
  state: string
  proxy_id?: number
  name?: string
  concurrency?: number
  priority?: number
  group_ids?: number[]
}): Promise<unknown> {
  const { data } = await apiClient.post<unknown>(
    '/admin/codebuddy/oauth/create-from-oauth',
    payload
  )
  return data
}

export async function refreshAccountToken(id: number): Promise<unknown> {
  const { data } = await apiClient.post<unknown>(
    `/admin/codebuddy/accounts/${id}/refresh`,
    {}
  )
  return data
}

export default {
  generateAuthUrl,
  exchangeState,
  refreshCodeBuddyToken,
  createAccountFromOAuth,
  refreshAccountToken
}
