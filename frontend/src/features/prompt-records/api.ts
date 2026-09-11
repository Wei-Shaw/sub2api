import { apiClient } from '@/api/client'

export interface PromptRecordSummary {
	id: number
  request_id: string
  turn_no: number
  stage: string
  user_id: number
  username: string
  user_email: string
	api_key_id: number
	api_key_name: string
	group_id?: number
	group_name: string
	provider: string
  endpoint: string
  protocol: string
	model: string
	prompt_hash: string
	prompt_length: number
	message_count: number
	risk_status: string
	risk_checked_at?: string
	created_at: string
	expires_at?: string
}

export interface PromptRecord extends PromptRecordSummary {
	prompt_text: string
	risk_result: string
	response_text: string
	response_length: number
	response_truncated: boolean
	response_captured_at?: string
}

export interface PromptRecordQueueStats {
  queue_length: number
  queue_capacity: number
  overflow_length: number
  overflow_capacity: number
  worker_count: number
  dropped_total: number
  persist_failed_total: number
  last_dropped_at?: string
}

export interface PromptRecordPage {
	items: PromptRecordSummary[]
  page: number
  page_size: number
  total: number
  total_pages: number
  queue: PromptRecordQueueStats
}

export interface PromptRecordingConfig {
	enabled: boolean
}

export async function getPromptRecordingConfig() {
	const { data } = await apiClient.get<PromptRecordingConfig>('/admin/prompt-records/recording')
	return data
}

export async function updatePromptRecordingConfig(enabled: boolean) {
	const { data } = await apiClient.put<PromptRecordingConfig>('/admin/prompt-records/recording', { enabled })
	return data
}

export async function listPromptRecords(params: Record<string, string | number | undefined>) {
  const { data } = await apiClient.get<PromptRecordPage>('/admin/prompt-records', { params })
  return data
}

export async function getPromptRecord(id: number) {
	const { data } = await apiClient.get<PromptRecord>(`/admin/prompt-records/${id}`)
	return data
}

export async function deletePromptRecord(id: number) {
  await apiClient.delete(`/admin/prompt-records/${id}`)
}

export async function batchDeletePromptRecords(ids: number[]) {
	const { data } = await apiClient.post<{ deleted: number }>('/admin/prompt-records/batch-delete', { ids })
	return data
}
