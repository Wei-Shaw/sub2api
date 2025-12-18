/**
 * User Groups API endpoints (non-admin)
 * Handles group-related operations for regular users
 */

import { apiClient REDACTED from './client';
import type { Group REDACTED from '@/types';

/**
 * Get available groups that the current user can bind to API keys
 * This returns groups based on user's permissions:
 * - Standard groups: public (non-exclusive) or explicitly allowed
 * - Subscription groups: user has active subscription
 * @returns List of available groups
 */
export async function getAvailable(): Promise<Group[]> {
  const { data REDACTED = await apiClient.get<Group[]>('/groups/available');
  return data;
REDACTED

export const userGroupsAPI = {
  getAvailable,
REDACTED;

export default userGroupsAPI;
