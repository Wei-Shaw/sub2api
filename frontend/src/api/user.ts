/**
 * User API endpoints
 * Handles user profile management and password changes
 */

import { apiClient } from './client';
import type { User, ChangePasswordRequest } from '@/types';

/**
 * Get current user profile
 * @returns User profile data
 */
export async function getProfile(): Promise<User> {
  const { data } = await apiClient.get<User>('/user/profile');
  return data;
}

/**
 * Update current user profile
 * @param profile - Profile data to update
 * @returns Updated user profile data
 */
export async function updateProfile(profile: {
  username?: string;
  wechat?: string;
}): Promise<User> {
  const { data } = await apiClient.put<User>('/user', profile);
  return data;
}

/**
 * Change current user password
 * @param passwords - Old and new password
 * @returns Success message
 */
export async function changePassword(
  oldPassword: string,
  newPassword: string
): Promise<{ message: string }> {
  const payload: ChangePasswordRequest = {
    old_password: oldPassword,
    new_password: newPassword,
  };

  const { data } = await apiClient.put<{ message: string }>('/user/password', payload);
  return data;
}

export const userAPI = {
  getProfile,
  updateProfile,
  changePassword,
};

export default userAPI;
