/**
 * Admin API Keys API endpoints
 * Handles API key management for administrators
 */

import { apiClient } from '../client'
import type { AdminUser, ApiKey, PaginatedResponse } from '@/types'

export interface UpdateApiKeyGroupResult {
  api_key: ApiKey
  auto_granted_group_access: boolean
  granted_group_id?: number
  granted_group_name?: string
}

export interface ManagedKey {
  user: AdminUser
  api_key: ApiKey | null
  ip_lock?: {
    mode: 'off' | 'auto_single_ip'
    locked_ip?: string
  }
}

export interface ManagedKeyDelivery {
  api_key: string
  authorization_header: string
  base_url: string
  openai_base_url: string
  claude_base_url: string
  gemini_base_url: string
}

export interface ManagedKeyResponse {
  user: AdminUser
  api_key: ApiKey
  delivery: ManagedKeyDelivery
}

export interface CreateManagedKeyRequest {
  customer_name: string
  contact?: string
  key_name?: string
  group_id?: number | null
  balance?: number
  concurrency?: number
  rpm_limit?: number
  quota?: number
  expires_in_days?: number | null
  custom_key?: string | null
  ip_whitelist?: string[]
  ip_blacklist?: string[]
  ip_lock_mode?: 'off' | 'auto_single_ip'
  limit_action?: 'hard_block' | 'soft_throttle'
  rate_limit_5h?: number
  rate_limit_1d?: number
  rate_limit_7d?: number
  rate_limit_1mo?: number
  notes?: string
}

/**
 * Update an API key's group binding
 * @param id - API Key ID
 * @param groupId - Group ID (0 to unbind, positive to bind, null/undefined to skip)
 * @returns Updated API key with auto-grant info
 */
export async function updateApiKeyGroup(id: number, groupId: number | null): Promise<UpdateApiKeyGroupResult> {
  const { data } = await apiClient.put<UpdateApiKeyGroupResult>(`/admin/api-keys/${id}`, {
    group_id: groupId === null ? 0 : groupId
  })
  return data
}

export async function listManagedKeys(
  page: number = 1,
  pageSize: number = 20
): Promise<PaginatedResponse<ManagedKey>> {
  const { data } = await apiClient.get<PaginatedResponse<ManagedKey>>('/admin/managed-keys', {
    params: {
      page,
      page_size: pageSize
    }
  })
  return data
}

export async function createManagedKey(payload: CreateManagedKeyRequest): Promise<ManagedKeyResponse> {
  const { data } = await apiClient.post<ManagedKeyResponse>('/admin/managed-keys', payload)
  return data
}

export async function getManagedKeyDelivery(id: number): Promise<ManagedKeyResponse> {
  const { data } = await apiClient.get<ManagedKeyResponse>(`/admin/managed-keys/${id}/delivery`)
  return data
}

export async function resetManagedKeyIPLock(id: number): Promise<void> {
  await apiClient.post(`/admin/managed-keys/${id}/reset-ip-lock`)
}

export const apiKeysAPI = {
  updateApiKeyGroup,
  listManagedKeys,
  createManagedKey,
  getManagedKeyDelivery,
  resetManagedKeyIPLock
}

export default apiKeysAPI
