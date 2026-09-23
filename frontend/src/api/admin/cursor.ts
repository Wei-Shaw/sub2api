/**
 * Admin Cursor API endpoints
 * Handles the Cursor deep-control browser OAuth flow for administrators
 */

import { apiClient } from '../client'

export interface CursorOAuthStartResponse {
  url: string
  uuid: string
  verifier: string
}

export interface CursorOAuthPollResult {
  done: boolean
  credentials?: {
    access_token?: string
    refresh_token?: string
    token_kind?: string
    expires_at?: string
  }
}

export async function startCursorOAuth(): Promise<CursorOAuthStartResponse> {
  const res = await apiClient.post<{ data: CursorOAuthStartResponse }>('/admin/cursor/oauth/start')
  return res.data.data
}

export async function pollCursorOAuth(
  uuid: string,
  verifier: string,
  proxyId?: number | null
): Promise<CursorOAuthPollResult> {
  const res = await apiClient.post<{ data: CursorOAuthPollResult }>('/admin/cursor/oauth/poll', {
    uuid,
    verifier,
    proxy_id: proxyId ?? undefined
  })
  return res.data.data
}
