import { apiClient } from '../client'

export interface CursorAuthUrlRequest {
  proxy_id?: number
}

export interface CursorAuthUrlResponse {
  auth_url: string
  session_id: string
}

export interface CursorPollRequest {
  session_id: string
  proxy_id?: number
}

export interface CursorTokenInfo {
  access_token?: string
  refresh_token?: string
  expires_at?: number | string
  user_id?: string
  email?: string
  pending?: boolean
}

export async function generateAuthUrl(payload: CursorAuthUrlRequest): Promise<CursorAuthUrlResponse> {
  const { data } = await apiClient.post<CursorAuthUrlResponse>('/admin/cursor/oauth/auth-url', payload)
  return data
}

export async function poll(payload: CursorPollRequest, signal?: AbortSignal): Promise<CursorTokenInfo> {
  const { data } = await apiClient.post<CursorTokenInfo>('/admin/cursor/oauth/poll', payload, { signal })
  return data
}

export async function refreshCursorToken(
  refreshToken: string,
  proxyId?: number | null
): Promise<CursorTokenInfo> {
  const payload: Record<string, unknown> = { refresh_token: refreshToken }
  if (proxyId) payload.proxy_id = proxyId
  const { data } = await apiClient.post<CursorTokenInfo>('/admin/cursor/oauth/refresh-token', payload)
  return data
}

export async function queryQuota(id: number): Promise<unknown> {
  const { data } = await apiClient.get(`/admin/cursor/accounts/${id}/quota`)
  return data
}

export default {
  generateAuthUrl,
  poll,
  refreshCursorToken,
  queryQuota
}
