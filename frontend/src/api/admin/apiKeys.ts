/**
 * Admin API Keys API endpoints
 * Handles API key management for administrators
 */

import { apiClient REDACTED from '../client'
import type { ApiKey REDACTED from '@/types'

export interface UpdateApiKeyGroupResult {
  api_key: ApiKey
  auto_granted_group_access: boolean
  granted_group_id?: number
  granted_group_name?: string
REDACTED

/**
 * Update an API key's group binding
 * @param id - API Key ID
 * @param groupId - Group ID (0 to unbind, positive to bind, null/undefined to skip)
 * @returns Updated API key with auto-grant info
 */
export async function updateApiKeyGroup(id: number, groupId: number | null): Promise<UpdateApiKeyGroupResult> {
  const { data REDACTED = await apiClient.put<UpdateApiKeyGroupResult>(`/admin/api-keys/${idREDACTED`, {
    group_id: groupId === null ? 0 : groupId
  REDACTED)
  return data
REDACTED

export const apiKeysAPI = {
  updateApiKeyGroup
REDACTED

export default apiKeysAPI
