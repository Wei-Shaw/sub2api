import { apiClient } from '../client'
import type { PaginatedResponse } from '@/types'

export type BackgroundTaskStatus =
  | 'pending'
  | 'running'
  | 'retry_wait'
  | 'succeeded'
  | 'skipped'
  | 'failed'
  | 'canceled'
  | 'indeterminate'

export interface BackgroundTask {
  id: number
  task_type: string
  resource_type: string
  resource_id: string
  account_id?: number | null
  account_name?: string
  credit_expires_at?: string | null
  run_at: string
  status: BackgroundTaskStatus
  attempt_count: number
  dispatch_count: number
  first_dispatch_at?: string | null
  last_dispatch_at?: string | null
  result_code?: string | null
  result?: Record<string, unknown> | null
  last_error_code?: string | null
  last_error_message?: string | null
  can_cancel: boolean
  can_retry: boolean
  canceled_at?: string | null
  started_at?: string | null
  finished_at?: string | null
  created_at: string
  updated_at: string
}

export interface BackgroundTaskListParams {
  task_type?: string
  status?: BackgroundTaskStatus
  resource_type?: string
  resource_id?: string
  page?: number
  page_size?: number
}

export interface CreateOpenAIQuotaResetTaskRequest {
  expected_expires_at: string
  lead_time_minutes: 10 | 30 | 60
}

export interface CreateBackgroundTaskResponse {
  task: BackgroundTask
  created: boolean
}

const quotaResetCreationKeys = new Map<number, string>()

function quotaResetCreationStorageKey(accountId: number): string {
  return `sub2api:admin:openai-quota-reset-task:${accountId}`
}

function readQuotaResetCreationKey(accountId: number): string | null {
  try {
    return globalThis.sessionStorage?.getItem(quotaResetCreationStorageKey(accountId)) ?? null
  } catch {
    return null
  }
}

function storeQuotaResetCreationKey(accountId: number, key: string | null): void {
  try {
    if (key) globalThis.sessionStorage?.setItem(quotaResetCreationStorageKey(accountId), key)
    else globalThis.sessionStorage?.removeItem(quotaResetCreationStorageKey(accountId))
  } catch {
    // In-memory retry protection still works when browser storage is unavailable.
  }
}

export async function list(params: BackgroundTaskListParams = {}): Promise<PaginatedResponse<BackgroundTask>> {
  const { data } = await apiClient.get<PaginatedResponse<BackgroundTask>>('/admin/background-tasks', { params })
  return data
}

export async function createOpenAIQuotaReset(
  accountId: number,
  request: CreateOpenAIQuotaResetTaskRequest
): Promise<CreateBackgroundTaskResponse> {
  let idempotencyKey = quotaResetCreationKeys.get(accountId) ?? readQuotaResetCreationKey(accountId)
  if (!idempotencyKey) {
    const requestID = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
    idempotencyKey = `openai-quota-reset-task-${accountId}-${requestID}`
  }
  quotaResetCreationKeys.set(accountId, idempotencyKey)
  storeQuotaResetCreationKey(accountId, idempotencyKey)
  const { data } = await apiClient.post<CreateBackgroundTaskResponse>(
    `/admin/openai/accounts/${accountId}/quota-reset-tasks`,
    request,
    { headers: { 'Idempotency-Key': idempotencyKey } }
  )
  quotaResetCreationKeys.delete(accountId)
  storeQuotaResetCreationKey(accountId, null)
  return data
}

export async function cancel(id: number): Promise<BackgroundTask> {
  const { data } = await apiClient.post<BackgroundTask>(`/admin/background-tasks/${id}/cancel`)
  return data
}

export async function retry(id: number): Promise<BackgroundTask> {
  const { data } = await apiClient.post<BackgroundTask>(`/admin/background-tasks/${id}/retry`)
  return data
}

export const backgroundTasksAPI = {
  list,
  createOpenAIQuotaReset,
  cancel,
  retry
}

export default backgroundTasksAPI
