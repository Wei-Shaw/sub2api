import { apiClient } from '../client'
import type { PaginatedResponse } from '@/types'

export interface AuditLog {
  id: number
  request_id: string
  session_id: string
  request_count: number
  user_id?: number | null
  user_email: string
  api_key_id?: number | null
  api_key_name: string
  group_id?: number | null
  group_name: string
  platform: string
  endpoint: string
  method: string
  model: string
  status_code: number
  request_body: string
  response_body: string
  request_truncated: boolean
  response_truncated: boolean
  duration_ms: number
  ip_address: string
  user_agent: string
  created_at: string
  updated_at: string
}

export interface AuditListParams {
  page?: number
  page_size?: number
  search?: string
  platform?: string
  model?: string
  endpoint?: string
  from?: string
  to?: string
}

export async function list(params: AuditListParams = {}, options?: { signal?: AbortSignal }): Promise<PaginatedResponse<AuditLog>> {
  const { data } = await apiClient.get<PaginatedResponse<AuditLog>>('/admin/audit', {
    params,
    signal: options?.signal,
  })
  return data
}

export default {
  list,
}
