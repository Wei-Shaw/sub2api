/**
 * Admin Groups API endpoints
 * Handles API key group management for administrators
 */

import { apiClient } from '../client';
import type {
  Group,
  GroupPlatform,
  CreateGroupRequest,
  UpdateGroupRequest,
  PaginatedResponse,
  ApiKey,
} from '@/types';

/**
 * List all groups with pagination
 * @param page - Page number (default: 1)
 * @param pageSize - Items per page (default: 20)
 * @param filters - Optional filters (platform, status, is_exclusive)
 * @returns Paginated list of groups
 */
export async function list(
  page: number = 1,
  pageSize: number = 20,
  filters?: {
    platform?: GroupPlatform;
    status?: 'active' | 'inactive';
    is_exclusive?: boolean;
  }
): Promise<PaginatedResponse<Group>> {
  const { data } = await apiClient.get<PaginatedResponse<Group>>('/admin/groups', {
    params: {
      page,
      page_size: pageSize,
      ...filters,
    },
  });
  return data;
}

/**
 * Get all active groups (without pagination)
 * @param platform - Optional platform filter
 * @returns List of all active groups
 */
export async function getAll(platform?: GroupPlatform): Promise<Group[]> {
  const { data } = await apiClient.get<Group[]>('/admin/groups/all', {
    params: platform ? { platform } : undefined
  });
  return data;
}

/**
 * Get active groups by platform
 * @param platform - Platform to filter by
 * @returns List of groups for the specified platform
 */
export async function getByPlatform(platform: GroupPlatform): Promise<Group[]> {
  return getAll(platform);
}

/**
 * Get group by ID
 * @param id - Group ID
 * @returns Group details
 */
export async function getById(id: number): Promise<Group> {
  const { data } = await apiClient.get<Group>(`/admin/groups/${id}`);
  return data;
}

/**
 * Create new group
 * @param groupData - Group data
 * @returns Created group
 */
export async function create(groupData: CreateGroupRequest): Promise<Group> {
  const { data } = await apiClient.post<Group>('/admin/groups', groupData);
  return data;
}

/**
 * Update group
 * @param id - Group ID
 * @param updates - Fields to update
 * @returns Updated group
 */
export async function update(id: number, updates: UpdateGroupRequest): Promise<Group> {
  const { data } = await apiClient.put<Group>(`/admin/groups/${id}`, updates);
  return data;
}

/**
 * Delete group
 * @param id - Group ID
 * @returns Success confirmation
 */
export async function deleteGroup(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/groups/${id}`);
  return data;
}

/**
 * Toggle group status
 * @param id - Group ID
 * @param status - New status
 * @returns Updated group
 */
export async function toggleStatus(
  id: number,
  status: 'active' | 'inactive'
): Promise<Group> {
  return update(id, { status });
}

/**
 * Get group statistics
 * @param id - Group ID
 * @returns Group usage statistics
 */
export async function getStats(id: number): Promise<{
  total_api_keys: number;
  active_api_keys: number;
  total_requests: number;
  total_cost: number;
}> {
  const { data } = await apiClient.get<{
    total_api_keys: number;
    active_api_keys: number;
    total_requests: number;
    total_cost: number;
  }>(`/admin/groups/${id}/stats`);
  return data;
}

/**
 * Get API keys in a group
 * @param id - Group ID
 * @param page - Page number
 * @param pageSize - Items per page
 * @returns Paginated list of API keys in the group
 */
export async function getGroupApiKeys(
  id: number,
  page: number = 1,
  pageSize: number = 20
): Promise<PaginatedResponse<ApiKey>> {
  // 仅返回脱敏密钥字段（masked_key）。
  const { data } = await apiClient.get<PaginatedResponse<ApiKey>>(
    `/admin/groups/${id}/api-keys`,
    {
      params: { page, page_size: pageSize },
    }
  );
  return data;
}

export const groupsAPI = {
  list,
  getAll,
  getByPlatform,
  getById,
  create,
  update,
  delete: deleteGroup,
  toggleStatus,
  getStats,
  getGroupApiKeys,
};

export default groupsAPI;
