/**
 * User API endpoints
 * Handles user profile management and password changes
 */

import { apiClient REDACTED from './client';
import type { User, ChangePasswordRequest REDACTED from '@/types';

/**
 * Get current user profile
 * @returns User profile data
 */
export async function getProfile(): Promise<User> {
  const { data REDACTED = await apiClient.get<User>('/users/me');
  return data;
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
    new_password: newPassword,
  REDACTED;

  const { data REDACTED = await apiClient.post<{ message: string REDACTED>('/users/me/password', payload);
  return data;
REDACTED

export const userAPI = {
  getProfile,
  changePassword,
REDACTED;

export default userAPI;
