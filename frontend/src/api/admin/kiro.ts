/**
 * Admin Kiro API endpoints
 * Handles Kiro (CodeWhisperer-backed) OAuth flows for administrators.
 * Phase 2 ships Social refresh-token paste only; IdC and Builder ID flows
 * arrive in phase 3.
 */

import { apiClient } from '../client'

export type KiroAuthMethod = 'social' | 'idc' | 'builderid'

export interface KiroTokenInfo {
  access_token: string
  refresh_token: string
  expires_at: number
  profile_arn?: string
  auth_method: KiroAuthMethod
  email?: string
  user_id?: string
}

export interface KiroValidateSocialPayload {
  refresh_token: string
  proxy_id?: number | null
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

export default { validateSocialRefreshToken }
