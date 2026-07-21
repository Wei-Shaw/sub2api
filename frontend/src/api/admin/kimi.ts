/**
 * Admin Kimi API endpoints
 * Handles Kimi (kimi.com subscription) OAuth device-flow for administrators.
 *
 * Flow: POST /admin/kimi/oauth/device-code → poll /admin/kimi/oauth/poll
 * with the returned session_id until the user confirms in the browser.
 * Poll errors carry business reasons (KIMI_OAUTH_AUTHORIZATION_PENDING /
 * KIMI_OAUTH_SLOW_DOWN / KIMI_OAUTH_DEVICE_CODE_EXPIRED / ...) plus metadata.
 */

import { apiClient } from '../client'
import type { Account } from '@/types'

export interface KimiDeviceAuthRequest {
  proxy_id?: number
}

export interface KimiDeviceAuthResponse {
  session_id: string
  user_code: string
  verification_uri: string
  verification_uri_complete: string
  interval: number
  expires_in: number
}

export interface KimiPollDeviceTokenRequest {
  session_id: string
  proxy_id?: number
}

export interface KimiTokenInfo {
  access_token?: string
  refresh_token?: string
  token_type?: string
  expires_in?: number
  expires_at?: number | string
  client_id?: string
  scope?: string
  device_id?: string
  [key: string]: unknown
}

export interface KimiRefreshTokenRequest {
  refresh_token: string
  device_id?: string
  proxy_id?: number
}

export interface KimiCreateAccountFromOAuthRequest {
  session_id: string
  proxy_id?: number
  name?: string
  concurrency?: number
  priority?: number
  group_ids?: number[]
}

export async function startDeviceAuth(
  payload: KimiDeviceAuthRequest
): Promise<KimiDeviceAuthResponse> {
  const { data } = await apiClient.post<KimiDeviceAuthResponse>(
    '/admin/kimi/oauth/device-code',
    payload
  )
  return data
}

export async function pollDeviceToken(
  payload: KimiPollDeviceTokenRequest
): Promise<KimiTokenInfo> {
  const { data } = await apiClient.post<KimiTokenInfo>('/admin/kimi/oauth/poll', payload)
  return data
}

export async function refreshKimiToken(
  refreshToken: string,
  proxyId?: number | null,
  deviceId?: string
): Promise<KimiTokenInfo> {
  const payload: KimiRefreshTokenRequest = { refresh_token: refreshToken }
  if (proxyId) payload.proxy_id = proxyId
  if (deviceId) payload.device_id = deviceId

  const { data } = await apiClient.post<KimiTokenInfo>('/admin/kimi/oauth/refresh-token', payload)
  return data
}

export async function refreshAccountToken(id: number): Promise<Account> {
  const { data } = await apiClient.post<Account>(`/admin/kimi/accounts/${id}/refresh`)
  return data
}

export async function createAccountFromOAuth(
  payload: KimiCreateAccountFromOAuthRequest
): Promise<Account> {
  const { data } = await apiClient.post<Account>('/admin/kimi/oauth/create-from-oauth', payload)
  return data
}

export default {
  startDeviceAuth,
  pollDeviceToken,
  refreshKimiToken,
  refreshAccountToken,
  createAccountFromOAuth
}
