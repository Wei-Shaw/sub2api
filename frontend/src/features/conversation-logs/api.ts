import { apiClient } from '@/api/client'

export interface CaptureConfig {
  enabled: boolean
  retention_days: number
  max_body_bytes: number
  redact_sensitive: boolean
}
export interface CaptureRuntime {
  config: CaptureConfig
  dropped_records: number
  failed_writes: number
  saved_records: number
  queue_depth: number
  websocket_capture: boolean
}
export interface ConversationRecord {
  id: number
  request_id: string
  created_at: string
  user_id: number
  api_key_id: number
  user_label: string
  key_name: string
  endpoint: string
  model: string
  stream: boolean
  status_code: number
  outcome: string
  duration_ms: number
  prompt: string
  answer: string
  request_body?: string
  response_body?: string
  tool_calls?: unknown[]
  request_truncated: boolean
  response_truncated: boolean
  request_bytes: number
  response_bytes: number
  redacted: boolean
}
export interface ListResponse {
  items: ConversationRecord[]
  total: number
  page: number
  page_size: number
}
const base = '/admin/conversation-logs'
export const conversationAPI = {
  config: async () => (await apiClient.get<CaptureRuntime>(`${base}/config`)).data,
  save: async (config: CaptureConfig) => (await apiClient.put<CaptureConfig>(`${base}/config`, config)).data,
  list: async (params: Record<string, unknown>) => (await apiClient.get<ListResponse>(base, { params })).data,
  detail: async (id: number) => (await apiClient.get<ConversationRecord>(`${base}/${id}`)).data,
  remove: async (id: number) => { await apiClient.delete(`${base}/${id}`) }
}
