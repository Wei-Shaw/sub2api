/**
 * User API endpoints
 * Handles user profile management and password changes
 */

import { apiClient REDACTED from './client'
import type { User, ChangePasswordRequest REDACTED from '@/types'

/**
 * Get current user profile
 * @returns User profile data
 */
export async function getProfile(): Promise<User> {
  const { data REDACTED = await apiClient.get<User>('/user/profile')
  return data
REDACTED

/**
 * Update current user profile
 * @param profile - Profile data to update
 * @returns Updated user profile data
 */
export async function updateProfile(profile: {
  username?: string
  balance_notify_enabled?: boolean
  balance_notify_threshold?: number | null
  balance_notify_extra_emails?: string[]
REDACTED): Promise<User> {
  const { data REDACTED = await apiClient.put<User>('/user', profile)
  return data
REDACTED

/**
 * Change current user password
 * @param passwords - Old and new password
 * @returns Success message
 */
export async function changePassword(
  oldPassword: string,
  newPassword: string
): Promise<{ message: string REDACTED> {
  const payload: ChangePasswordRequest = {
    old_password: oldPassword,
    new_password: newPassword
  REDACTED

  const { data REDACTED = await apiClient.put<{ message: string REDACTED>('/user/password', payload)
  return data
REDACTED

/**
 * Send verification code for adding a notify email
 * @param email - Email address to verify
 */
export async function sendNotifyEmailCode(email: string): Promise<void> {
  await apiClient.post('/user/notify-email/send-code', { email REDACTED)
REDACTED

/**
 * Verify and add a notify email
 * @param email - Email address to add
 * @param code - Verification code
 */
export async function verifyNotifyEmail(email: string, code: string): Promise<void> {
  await apiClient.post('/user/notify-email/verify', { email, code REDACTED)
REDACTED

/**
 * Remove a notify email
 * @param email - Email address to remove
 */
export async function removeNotifyEmail(email: string): Promise<void> {
  await apiClient.delete('/user/notify-email', { data: { email REDACTED REDACTED)
REDACTED

export const userAPI = {
  getProfile,
  updateProfile,
  changePassword,
  sendNotifyEmailCode,
  verifyNotifyEmail,
  removeNotifyEmail
REDACTED

export default userAPI
