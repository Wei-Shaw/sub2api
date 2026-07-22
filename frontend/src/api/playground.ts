import { apiClient } from './client'

export interface PlaygroundChatRequest {
  api_key_id?: number | null
  model: string
  prompt: string
  max_tokens?: number
}

export interface PlaygroundError {
  code: string
  message: string
  suggestion?: string
}

export interface PlaygroundChatResponse {
  success: boolean
  api_key_id: number
  api_key_name: string
  model: string
  resolved_model?: string
  endpoint: string
  duration_ms: number
  text?: string
  usage?: Record<string, unknown>
  balance_before: number
  balance_after: number
  cost: number
  billing_settled: boolean
  raw_status: number
  error?: PlaygroundError
}

export interface PlaygroundModelsResponse {
  api_key_id: number
  api_key_name: string
  group_name?: string
  platform?: string
  models: string[]
  default_model?: string
  source: string
}

export async function testChat(payload: PlaygroundChatRequest): Promise<PlaygroundChatResponse> {
  const { data } = await apiClient.post<PlaygroundChatResponse>('/playground/chat', payload)
  return data
}

export async function getModels(apiKeyId?: number | null): Promise<PlaygroundModelsResponse> {
  const { data } = await apiClient.get<PlaygroundModelsResponse>('/playground/models', {
    params: apiKeyId ? { api_key_id: apiKeyId } : undefined,
  })
  return data
}

export const playgroundAPI = {
  getModels,
  testChat,
}

export default playgroundAPI
