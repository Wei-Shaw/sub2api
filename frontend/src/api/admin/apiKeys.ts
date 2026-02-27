/**
 * Admin API Keys API endpoints
 * Handles API key management for administrators
 */

import { apiClient REDACTED from '../client'
import type { ApiKey REDACTED from '@/types'

/**
 * Update an API key's group binding
 * @param id - API Key ID
 * @param groupId - Group ID (0 to unbind, positive to bind, null/undefined to skip)
 * @returns Updated API key
 */
export async function updateApiKeyGroup(id: number, groupId: number | null): Promise<ApiKey> {
  const { data REDACTED = await apiClient.put<ApiKey>(`/admin/api-keys/${idREDACTED`, {
    group_id: groupId === null ? 0 : groupId
  REDACTED)
  return data
REDACTED

export const apiKeysAPI = {
  updateApiKeyGroup
REDACTED

export default apiKeysAPI
