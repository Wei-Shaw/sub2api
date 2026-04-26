/**
 * User-facing Channel Monitor API endpoints
 * Read-only views for end users to inspect channel availability/status.
 */

import { apiClient } from './client'
import type { Provider, MonitorStatus } from './admin/channelMonitor'

export type { Provider, MonitorStatus } from './admin/channelMonitor'

export interface UserMonitorExtraModel {
  model: string
  status: MonitorStatus
  latency_ms: number | null
}

export interface MonitorTimelinePoint {
  status: MonitorStatus
  latency_ms: number | null
  ping_latency_ms: number | null
  checked_at: string
}

export interface UserMonitorView {
  id: number
  name: string
  provider: Provider
  group_name: string
  primary_model: string
  primary_status: MonitorStatus
  primary_latency_ms: number | null
  primary_ping_latency_ms: number | null
  availability_7d: number
  extra_models: UserMonitorExtraModel[]
  timeline: MonitorTimelinePoint[]
}

export interface UserMonitorListResponse {
  items: UserMonitorView[]
}

export interface UserMonitorModelDetail {
  model: string
  latest_status: MonitorStatus
  latest_latency_ms: number | null
  availability_7d: number
  availability_15d: number
  availability_30d: number
  avg_latency_7d_ms: number | null
}

export interface UserMonitorDetail {
  id: number
  name: string
  provider: Provider
  group_name: string
  models: UserMonitorModelDetail[]
}

/**
 * List all monitor views available to the current user.
 */
export async function list(options?: { signal?: AbortSignal }): Promise<UserMonitorListResponse> {
  const { data } = await apiClient.get<UserMonitorListResponse>('/channel-monitors', {
    signal: options?.signal,
  })
  return data
}

/**
 * Get detailed status (multi-window availability + latency) for a single monitor.
 */
export async function status(id: number): Promise<UserMonitorDetail> {
  const { data } = await apiClient.get<UserMonitorDetail>(`/channel-monitors/${id}/status`)
  return data
}

export const channelMonitorUserAPI = {
  list,
  status,
}

export default channelMonitorUserAPI
