/**
 * Admin Kiro (Amazon Q Developer / CodeWhisperer) OAuth API endpoints.
 *
 * 对应后端 `internal/handler/admin/kiro_oauth_handler.go` 的四个管理端点：
 * authorize-url（IdC 授权码，生成授权链接）、idc/complete（IdC 授权码，
 * 用管理员手动粘贴回来的回调 URL 完成兑换）、device/start + device/poll
 * （Builder ID 设备码）。
 *
 * IdC 流程没有服务端回调页——真实账号联调验证过，AWS SSO-OIDC 的
 * client/register 对 public client 强制要求 redirect_uri 是裸 loopback
 * 地址，服务端自建回调页会被直接拒绝（invalid_redirect_uri）。redirect_uri
 * 现在固定在后端（`http://127.0.0.1/oauth/callback`，没有任何服务监听），
 * 授权完成后浏览器会跳到这个打不开的地址，管理员手动复制地址栏完整 URL
 * 粘贴回来，交给 idc/complete 解析并换取 token。
 */

import { apiClient } from '../client'

export interface KiroAuthorizeUrlRequest {
  proxy_id?: number | null
  issuer_url: string
  region?: string
}

export interface KiroAuthorizeUrlResponse {
  session_id: string
  authorize_url: string
  expires_in: number
}

export interface KiroIdCCompleteRequest {
  session_id: string
  /** 管理员从浏览器地址栏复制粘贴回来的完整回调 URL（含 code+state，或 error）。 */
  callback_url: string
  proxy_id?: number | null
}

export interface KiroDeviceStartRequest {
  proxy_id?: number | null
  region?: string
}

export interface KiroDeviceStartResponse {
  session_id: string
  user_code: string
  verification_uri: string
  verification_uri_complete: string
  expires_in: number
  interval: number
}

export interface KiroDevicePollRequest {
  session_id: string
  proxy_id?: number | null
}

export type KiroOAuthPollStatus = 'pending' | 'ok'

/** 与后端 KiroOAuthService.BuildAccountCredentials 的输出一一对应（snake_case）。 */
export interface KiroRawCredentials {
  auth_method?: string
  access_token?: string
  refresh_token?: string
  profile_arn?: string
  expires_at?: string
  region?: string
  issuer_url?: string
  client_id?: string
  client_secret?: string
  machine_id?: string
  [key: string]: unknown
}

export interface KiroDevicePollResponse {
  status: KiroOAuthPollStatus
  interval?: number
  credentials?: KiroRawCredentials
}

export interface KiroIdCCompleteResponse {
  status: KiroOAuthPollStatus
  credentials?: KiroRawCredentials
}

export async function authorizeUrl(
  payload: KiroAuthorizeUrlRequest
): Promise<KiroAuthorizeUrlResponse> {
  const { data } = await apiClient.post<KiroAuthorizeUrlResponse>(
    '/admin/kiro/oauth/authorize-url',
    payload
  )
  return data
}

export async function completeIdC(
  payload: KiroIdCCompleteRequest
): Promise<KiroIdCCompleteResponse> {
  const { data } = await apiClient.post<KiroIdCCompleteResponse>(
    '/admin/kiro/oauth/idc/complete',
    payload
  )
  return data
}

export async function deviceStart(
  payload: KiroDeviceStartRequest
): Promise<KiroDeviceStartResponse> {
  const { data } = await apiClient.post<KiroDeviceStartResponse>(
    '/admin/kiro/oauth/device/start',
    payload
  )
  return data
}

export async function devicePoll(
  payload: KiroDevicePollRequest
): Promise<KiroDevicePollResponse> {
  const { data } = await apiClient.post<KiroDevicePollResponse>(
    '/admin/kiro/oauth/device/poll',
    payload
  )
  return data
}

export default {
  authorizeUrl,
  completeIdC,
  deviceStart,
  devicePoll
}
