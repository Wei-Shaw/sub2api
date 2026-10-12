import { apiClient } from '../client'
import type { PaginatedResponse } from '@/types'

export const videoTaskStatuses = [
  'in_progress', 'delayed', 'query_retry', 'billing_pending',
  'completed', 'failed', 'submission_unknown', 'review_required'
] as const

export type VideoTaskStatus = typeof videoTaskStatuses[number]

export interface VideoTaskAdminItem {
  kind: 'task' | 'submission'
  task_id: string
  user_id: number | null
  user_email: string
  api_key_id: number
  api_key_name: string
  account_id: number | null
  account_name: string
  group_id: number | null
  group_name: string
  model: string | null
  state: string
  display_status: string
  reason_code: string
  created_at: string
  updated_at: string
  completed_at: string | null
  next_attempt_at: string | null
  attempts: number
  expected_cost: number | null
  charged_cost: number | null
  usage_log_id: number | null
  billing_status: 'not_charged' | 'pending' | 'charged' | 'review'
  video_duration_seconds: number | null
  video_resolution: string
}

export interface VideoTaskAdminQuery {
  page?: number
  page_size?: number
  search?: string
  status?: VideoTaskStatus
  days?: 1 | 7 | 30
}

export async function list(
  params: VideoTaskAdminQuery = {},
  signal?: AbortSignal
): Promise<PaginatedResponse<VideoTaskAdminItem>> {
  const { data } = await apiClient.get<PaginatedResponse<VideoTaskAdminItem>>('/admin/video-tasks', {
    params: {
      ...params,
      page: Math.max(1, Math.floor(params.page || 1)),
      page_size: Math.min(100, Math.max(1, Math.floor(params.page_size || 20))),
      search: params.search ? Array.from(params.search).slice(0, 100).join('') : undefined,
      days: params.days ?? 7
    },
    signal
  })
  return data
}

export const videoTasksAPI = { list }
