import { apiClient } from '../client'

export type AIAgentProtocol = 'chat_completions' | 'responses' | 'messages'

export interface AIAgentConfig {
  enabled: boolean
  base_url: string
  model: string
  api_key_set: boolean
  auto_approve: boolean
  protocol: AIAgentProtocol
  thinking_mode: string
  process_display: 'off' | 'compact' | 'full'
  catalog_size: number
  context_window: string
  context_window_tokens: number
  streaming: boolean
  response_cache: boolean
  execution_topology: 'single_instance'
  multi_instance_safe: false
}

export interface AIAgentRollbackCapability {
  endpoint_key: string
  level: 'conditional' | 'assisted' | 'unavailable'
  strategy?: 'restore_fields' | 'delete_created' | 'rollback_plan'
  conditions?: string[]
  limitations?: string[]
}

export interface AIAgentMessage {
  id: string
  run_id?: string
  role: 'user' | 'assistant'
  content: string
  event?: string
  metadata?: Record<string, unknown>
  streaming?: boolean
  created_at: string
}

export interface AIAgentChange {
  field: string
  before: unknown
  after: unknown
}

export interface AIAgentPlanNode {
  id: string
  endpoint_key: string
  operation: string
  action?: 'create' | 'update' | 'delete'
  resource?: string
  depends_on?: string[]
  preview?: AIAgentChange[]
  status: 'planned' | 'running' | 'succeeded' | 'failed' | 'blocked' | 'rolled_back' | 'rollback_failed'
  error?: string
  sensitive?: boolean
  requires_step_up?: boolean
}

export interface AIAgentExecutionPlan {
  id: string
  title: string
  failure_policy: 'stop_on_failure' | 'continue_independent' | 'rollback_on_failure'
  status: 'awaiting_confirmation' | 'running' | 'succeeded' | 'partial_failure' | 'failed' | 'stopped'
  nodes: AIAgentPlanNode[]
  sensitive?: boolean
  requires_session?: boolean
  created_at: string
  updated_at: string
}

export interface AIAgentPendingAction {
  id: string
  operation: string
  action?: 'create' | 'update' | 'delete' | 'plan'
  resource?: string
  target_label?: string
  method: string
  path: string
  changes?: AIAgentChange[]
  preview?: AIAgentChange[]
  sensitive?: boolean
  requires_step_up?: boolean
  sensitive_fields?: string[]
  plan?: AIAgentExecutionPlan
  created_at: string
  expires_at: string
}

export type AIAgentRollbackStrategy = 'restore_fields' | 'delete_created' | 'rollback_plan'
export type AIAgentRollbackStatus = 'available' | 'running' | 'completed' | 'failed' | 'partial_failure'

export interface AIAgentRollback {
  id: string
  operation: string
  strategy: AIAgentRollbackStrategy
  status: AIAgentRollbackStatus
  resource?: string
  target_label?: string
  target_id?: string
  method: string
  path: string
  changes?: AIAgentChange[]
  child_count?: number
  plan_id?: string
  sensitive?: boolean
  requires_step_up?: boolean
  error?: string
  resolution?: 'agent_recovery'
  created_at: string
  updated_at?: string
  completed_at?: string
}

export type AIAgentRollbackPreviewStatus = 'safe' | 'review_required' | 'conflict' | 'already_restored' | 'unavailable' | 'running' | 'completed'

export interface AIAgentRollbackFieldPreview {
  field: string
  before?: unknown
  after?: unknown
  current?: unknown
  result?: unknown
  status: 'will_restore' | 'already_restored' | 'conflict'
  sensitive?: boolean
  operation?: string
  resource?: string
  target_label?: string
  target_id?: string
}

export interface AIAgentRollbackPreview {
  rollback: AIAgentRollback
  status: AIAgentRollbackPreviewStatus
  action: AIAgentRollbackStrategy
  can_execute: boolean
  requires_step_up?: boolean
  fields?: AIAgentRollbackFieldPreview[]
  conflict_count?: number
  change_count?: number
  checked_at: string
  message?: string
}

export type AIAgentConversationStatus = 'idle' | 'running' | 'stopping' | 'stopped' | 'error'

export interface AIAgentConversationSummary {
  id: string
  title: string
  status: AIAgentConversationStatus
  created_at: string
  updated_at: string
}

export interface AIAgentProcessEvent {
  id: string
  run_id?: string
  kind: string
  summary: string
  detail?: string
  metadata?: Record<string, unknown>
  created_at: string
}

export interface AIAgentConversationList {
  active_id?: string
  conversations: AIAgentConversationSummary[]
}

export interface AIAgentSession {
  conversation: AIAgentConversationSummary
  messages: AIAgentMessage[]
  events: AIAgentProcessEvent[]
  pending?: AIAgentPendingAction
  rollbacks: AIAgentRollback[]
  error?: string
}

export interface UpdateAIAgentConfigInput {
  enabled?: boolean
  base_url?: string
  model?: string
  api_key?: string
  clear_api_key?: boolean
  auto_approve?: boolean
  protocol?: AIAgentProtocol
  thinking_mode?: string
  process_display?: 'off' | 'compact' | 'full'
  context_window?: string
}

const aiAgentAPI = {
  async getConfig(): Promise<AIAgentConfig> {
    const { data } = await apiClient.get<AIAgentConfig>('/admin/ai-agent/config')
    return data
  },

  async updateConfig(input: UpdateAIAgentConfigInput): Promise<AIAgentConfig> {
    const { data } = await apiClient.put<AIAgentConfig>('/admin/ai-agent/config', input)
    return data
  },

  async listModels(): Promise<string[]> {
    const { data } = await apiClient.get<{ models: string[] }>('/admin/ai-agent/models')
    return data.models
  },

  async rollbackCapabilities(): Promise<AIAgentRollbackCapability[]> {
    const { data } = await apiClient.get<{ operations: AIAgentRollbackCapability[] }>('/admin/ai-agent/rollback-capabilities')
    return data.operations
  },

  async listConversations(): Promise<AIAgentConversationList> {
    const { data } = await apiClient.get<AIAgentConversationList>('/admin/ai-agent/conversations')
    return data
  },

  async createConversation(): Promise<AIAgentSession> {
    const { data } = await apiClient.post<AIAgentSession>('/admin/ai-agent/conversations')
    return data
  },

  async deleteConversation(conversationId: string): Promise<void> {
    await apiClient.delete(`/admin/ai-agent/conversations/${encodeURIComponent(conversationId)}`)
  },

  async getSession(conversationId?: string): Promise<AIAgentSession> {
    const { data } = await apiClient.get<AIAgentSession>('/admin/ai-agent/session', { params: conversationId ? { conversation_id: conversationId } : undefined })
    return data
  },

  async clearSession(conversationId: string): Promise<void> {
    await apiClient.delete('/admin/ai-agent/session', { params: { conversation_id: conversationId } })
  },

  async chat(conversationId: string, message: string): Promise<AIAgentSession> {
    const { data } = await apiClient.post<AIAgentSession>('/admin/ai-agent/chat', { conversation_id: conversationId, message })
    return data
  },

  async stop(conversationId: string): Promise<boolean> {
    const { data } = await apiClient.post<{ stopping: boolean }>('/admin/ai-agent/stop', { conversation_id: conversationId })
    return data.stopping
  },

  async confirm(conversationId: string, actionId: string, requiresStepUp = false): Promise<{ message: AIAgentMessage; changes: AIAgentChange[]; rollback_available: boolean }> {
    const suffix = requiresStepUp ? 'confirm-sensitive' : 'confirm'
    const { data } = await apiClient.post(`/admin/ai-agent/actions/${encodeURIComponent(actionId)}/${suffix}`, undefined, { params: { conversation_id: conversationId } })
    return data
  },

  async cancel(conversationId: string, actionId: string): Promise<void> {
    await apiClient.delete(`/admin/ai-agent/actions/${encodeURIComponent(actionId)}`, { params: { conversation_id: conversationId } })
  },

  async previewRollback(conversationId: string, rollbackId: string): Promise<AIAgentRollbackPreview> {
    const { data } = await apiClient.get<AIAgentRollbackPreview>(`/admin/ai-agent/rollbacks/${encodeURIComponent(rollbackId)}/preview`, { params: { conversation_id: conversationId } })
    return data
  },

  async rollback(conversationId: string, rollbackId: string, requiresStepUp = false): Promise<void> {
    const suffix = requiresStepUp ? '/confirm-sensitive' : ''
    await apiClient.post(`/admin/ai-agent/rollbacks/${encodeURIComponent(rollbackId)}${suffix}`, undefined, { params: { conversation_id: conversationId } })
  },

  async assistRollback(conversationId: string, rollbackId: string, instruction = ''): Promise<AIAgentSession> {
    const { data } = await apiClient.post<AIAgentSession>(`/admin/ai-agent/rollbacks/${encodeURIComponent(rollbackId)}/assist`, { instruction }, { params: { conversation_id: conversationId } })
    return data
  }
}

export default aiAgentAPI
