/**
 * User invitation API endpoints
 * Lets registered users generate one-time invitation codes (invitation-only registration)
 */

import { apiClient } from './client'

export type UserInvitationStatus = 'unused' | 'used' | 'expired'

export interface UserInvitationCode {
  code: string
  status: UserInvitationStatus
  created_at: string
  expires_at: string | null
  used_at: string | null
  used_by_email_mask?: string
}

export interface UserInvitationOverview {
  enabled: boolean
  max_codes_per_user: number // 0 = unlimited
  used_count: number
  remaining: number // -1 = unlimited
  code_validity_days: number // 0 = never expires
  codes: UserInvitationCode[]
}

/**
 * Get the current user's invitation quota and codes
 */
export async function getOverview(): Promise<UserInvitationOverview> {
  const { data } = await apiClient.get<UserInvitationOverview>('/invitations')
  return data
}

/**
 * Generate a new one-time invitation code
 */
export async function create(): Promise<UserInvitationCode> {
  const { data } = await apiClient.post<UserInvitationCode>('/invitations')
  return data
}

export const invitationsAPI = {
  getOverview,
  create
}

export default invitationsAPI
