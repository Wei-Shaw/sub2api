/**
 * Student verification API endpoints (user-facing)
 * School-email verification that grants student groups + exclusive rebate rate.
 */

import { apiClient } from './client'

export type StudentVerificationStatus = 'active' | 'expired' | 'revoked'

export interface StudentVerificationRecord {
  status: StudentVerificationStatus
  /** Server-side masked email (e.g. st***@stu.example.edu) */
  email: string
  verified_at: string
  expires_at: string
}

export interface StudentVerificationStatusResponse {
  enabled: boolean
  verification?: StudentVerificationRecord
  /** Configured exclusive rebate rate granted to verified students (percent) */
  rebate_rate: number
  /** Number of currently grantable student groups */
  group_count: number
}

export async function getStudentVerificationStatus(): Promise<StudentVerificationStatusResponse> {
  const { data } = await apiClient.get<StudentVerificationStatusResponse>('/user/student-verification')
  return data
}

export async function sendStudentVerificationCode(email: string): Promise<void> {
  await apiClient.post('/user/student-verification/send-code', { email })
}

export async function verifyStudentEmail(email: string, code: string): Promise<StudentVerificationStatusResponse> {
  const { data } = await apiClient.post<StudentVerificationStatusResponse>('/user/student-verification/verify', {
    email,
    code,
  })
  return data
}

export const studentVerificationAPI = {
  getStudentVerificationStatus,
  sendStudentVerificationCode,
  verifyStudentEmail,
}

export default studentVerificationAPI
