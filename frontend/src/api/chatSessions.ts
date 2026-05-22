import { apiClient } from './client'

export type ChatSessionStatus = 'active' | 'archived'
export type ChatMessageStatus = 'pending' | 'streaming' | 'completed' | 'stopped' | 'failed'
export type ChatMessageRole = 'system' | 'developer' | 'user' | 'assistant'

export interface ChatMessageAttachmentRecord {
  type: 'image' | string
  image_url: string
  mime_type: string
  name?: string
  size?: number
}

export interface ChatSessionRecord {
  id: number
  api_key_id: number
  title: string
  model: string
  status: ChatSessionStatus | string
  expires_at: string
  created_at: string
  updated_at: string
  deleted_at?: string | null
}

export interface ChatMessageRecord {
  id: number
  session_id: number
  role: ChatMessageRole
  content: string
  attachments?: ChatMessageAttachmentRecord[]
  status: ChatMessageStatus | string
  model?: string | null
  duration_ms?: number | null
  usage_log_id?: number | null
  actual_cost?: number | null
  error_message?: string | null
  created_at: string
  updated_at: string
}

export interface CreateChatSessionPayload {
  api_key_id: number
  title?: string
  model: string
}

export interface UpdateChatSessionPayload {
  title?: string
  status?: ChatSessionStatus | string
  model?: string
}

export interface CreateChatMessagePayload {
  role: ChatMessageRole
  content: string
  attachments?: ChatMessageAttachmentRecord[]
  status?: ChatMessageStatus | string
  model?: string
  duration_ms?: number
  usage_log_id?: number
  actual_cost?: number
  error_message?: string
}

export type UpdateChatMessagePayload = Partial<Omit<CreateChatMessagePayload, 'role'>>

export async function listChatSessions(): Promise<ChatSessionRecord[]> {
  const { data } = await apiClient.get('/chat/sessions')
  return data
}

export async function createChatSession(payload: CreateChatSessionPayload): Promise<ChatSessionRecord> {
  const { data } = await apiClient.post('/chat/sessions', payload)
  return data
}

export async function updateChatSession(sessionId: number, payload: UpdateChatSessionPayload): Promise<ChatSessionRecord> {
  const { data } = await apiClient.patch(`/chat/sessions/${sessionId}`, payload)
  return data
}

export async function deleteChatSession(sessionId: number): Promise<{ deleted: boolean }> {
  const { data } = await apiClient.delete(`/chat/sessions/${sessionId}`)
  return data
}

export async function getChatSessionMessages(sessionId: number): Promise<ChatMessageRecord[]> {
  const { data } = await apiClient.get(`/chat/sessions/${sessionId}/messages`)
  return data
}

export async function createChatMessage(sessionId: number, payload: CreateChatMessagePayload): Promise<ChatMessageRecord> {
  const { data } = await apiClient.post(`/chat/sessions/${sessionId}/messages`, payload)
  return data
}

export async function updateChatMessage(
  sessionId: number,
  messageId: number,
  payload: UpdateChatMessagePayload,
): Promise<ChatMessageRecord> {
  const { data } = await apiClient.patch(`/chat/sessions/${sessionId}/messages/${messageId}`, payload)
  return data
}
