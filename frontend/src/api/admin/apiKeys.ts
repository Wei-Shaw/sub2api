/**
 * Admin API Keys API endpoints
 * Handles API key management for administrators
 */

import { apiClient } from '../client'
import type { ApiKey, ChatMessage, ChatSession, ChatSessionDetail, PaginatedResponse } from '@/types'

export interface UpdateApiKeyGroupResult {
  api_key: ApiKey
  auto_granted_group_access: boolean
  granted_group_id?: number
  granted_group_name?: string
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

export async function listChatSessions(
  id: number,
  userId: number,
  page: number = 1,
  pageSize: number = 20
): Promise<PaginatedResponse<ChatSession>> {
  const { data } = await apiClient.get<PaginatedResponse<ChatSession>>(`/admin/api-keys/${id}/chat-sessions`, {
    params: {
      user_id: userId,
      page,
      page_size: pageSize
    }
  })
  return data
}

export async function getChatSession(
  id: number,
  userId: number,
  sessionId: number,
  page: number = 1,
  pageSize: number = 20
): Promise<ChatSessionDetail> {
  const { data } = await apiClient.get<ChatSessionDetail>(`/admin/api-keys/${id}/chat-sessions/${sessionId}`, {
    params: {
      user_id: userId,
      page,
      page_size: pageSize
    }
  })
  return data
}

export async function getChatMessage(
  id: number,
  userId: number,
  sessionId: number,
  messageId: number
): Promise<ChatMessage> {
  const { data } = await apiClient.get<ChatMessage>(`/admin/api-keys/${id}/chat-sessions/${sessionId}/messages/${messageId}`, {
    params: {
      user_id: userId
    }
  })
  return data
}

export const apiKeysAPI = {
  updateApiKeyGroup,
  listChatSessions,
  getChatSession,
  getChatMessage
}

export default apiKeysAPI
