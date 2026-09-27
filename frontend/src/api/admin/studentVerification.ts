/**
 * Admin API for student verification records
 */

import { apiClient } from '../client'
import type { PaginatedResponse } from '@/types'

export interface AdminStudentVerificationRecord {
  id: number
  /** 0 when the owning user was deleted (email claim is still held) */
  user_id: number
  /** Server-side masked school email */
  email: string
  status: 'active' | 'expired' | 'revoked'
  verified_at: string
  expires_at: string
  revoked_at?: string
  revoked_by?: number
  revoke_reason?: string
  granted_group_ids: number[]
  rebate_rate_applied?: number
  created_at: string
}

export interface ListStudentVerificationsParams {
  page?: number
  page_size?: number
  status?: 'active' | 'expired' | 'revoked' | ''
  keyword?: string
}

export async function listStudentVerifications(
  params: ListStudentVerificationsParams = {},
): Promise<PaginatedResponse<AdminStudentVerificationRecord>> {
  const { data } = await apiClient.get<PaginatedResponse<AdminStudentVerificationRecord>>(
    '/admin/student-verifications',
    {
      params: {
        page: params.page ?? 1,
        page_size: params.page_size ?? 20,
        status: params.status || undefined,
        keyword: params.keyword?.trim() || undefined,
      },
    },
  )
  return data
}

export async function revokeStudentVerification(id: number, reason?: string): Promise<void> {
  await apiClient.post(`/admin/student-verifications/${id}/revoke`, { reason: reason?.trim() || '' })
}

/**
 * Permanently delete a record and release the school email claim.
 */
export async function deleteStudentVerification(id: number): Promise<void> {
  await apiClient.delete(`/admin/student-verifications/${id}`)
}

export const adminStudentVerificationAPI = {
  listStudentVerifications,
  revokeStudentVerification,
  deleteStudentVerification,
}

export default adminStudentVerificationAPI
