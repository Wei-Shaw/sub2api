/**
 * Admin Kiro API endpoints
 * Handles Kiro (CodeWhisperer-backed) OAuth flows for administrators.
 * Three auth methods:
 *  - Social: paste a refresh token captured from the Kiro desktop app.
 *  - IdC: AWS Identity Center PKCE auth-code flow.
 *  - Builder ID: AWS Builder ID device-code flow.
 */

import { apiClient } from '../client'

export type KiroAuthMethod = 'social' | 'idc' | 'builderid'

export interface KiroTokenInfo {
  access_token: string
  refresh_token: string
  expires_at: number
  profile_arn?: string
  auth_method: KiroAuthMethod
  client_id?: string
  client_secret?: string
  region?: string
  start_url?: string
  email?: string
  user_id?: string
}

export interface KiroValidateSocialPayload {
  refresh_token: string
  proxy_id?: number | null
}

export interface KiroStartIdCPayload {
  start_url: string
  region?: string
  proxy_id?: number | null
}

export interface KiroIdCAuthURLResult {
  auth_url: string
  session_id: string
  expires_in: number
}

export interface KiroCompleteIdCPayload {
  session_id: string
  callback_url: string
}

export interface KiroStartBuilderIDPayload {
  region?: string
  proxy_id?: number | null
}

export interface KiroBuilderIDLoginResult {
  session_id: string
  user_code: string
  verification_uri: string
  interval: number
  expires_at: number
}

export interface KiroPollBuilderIDPayload {
  session_id: string
}

export type KiroBuilderIDPollStatus = 'pending' | 'slow_down' | 'completed'

export interface KiroBuilderIDPollResult {
  status: KiroBuilderIDPollStatus
  token_info?: KiroTokenInfo
}

export async function validateSocialRefreshToken(
  payload: KiroValidateSocialPayload
): Promise<KiroTokenInfo> {
  const { data } = await apiClient.post<KiroTokenInfo>(
    '/admin/kiro/oauth/validate-social',
    payload
  )
  return data
}

export async function startIdCLogin(
  payload: KiroStartIdCPayload
): Promise<KiroIdCAuthURLResult> {
  const { data } = await apiClient.post<KiroIdCAuthURLResult>(
    '/admin/kiro/oauth/idc/start',
    payload
  )
  return data
}

export async function completeIdCLogin(
  payload: KiroCompleteIdCPayload
): Promise<KiroTokenInfo> {
  const { data } = await apiClient.post<KiroTokenInfo>(
    '/admin/kiro/oauth/idc/complete',
    payload
  )
  return data
}

export async function startBuilderIDLogin(
  payload: KiroStartBuilderIDPayload
): Promise<KiroBuilderIDLoginResult> {
  const { data } = await apiClient.post<KiroBuilderIDLoginResult>(
    '/admin/kiro/oauth/builderid/start',
    payload
  )
  return data
}

export async function pollBuilderIDLogin(
  payload: KiroPollBuilderIDPayload
): Promise<KiroBuilderIDPollResult> {
  const { data } = await apiClient.post<KiroBuilderIDPollResult>(
    '/admin/kiro/oauth/builderid/poll',
    payload
  )
  return data
}

export default {
  validateSocialRefreshToken,
  startIdCLogin,
  completeIdCLogin,
  startBuilderIDLogin,
  pollBuilderIDLogin
}
