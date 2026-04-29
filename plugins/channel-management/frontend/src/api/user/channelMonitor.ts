/**
 * User-facing Channel Monitor API client.
 *
 * V5 W7.3 — vendored from host frontend `src/api/channelMonitor.ts`. Plugin
 * exposes a read-only user API at:
 *   - GET /api/v1/plugin/channel-management/monitors
 *   - GET /api/v1/plugin/channel-management/monitors/:id  (W6 stub: 501)
 *
 * 简化说明: 仅保留 plugin 用户视图实际使用的两条接口, 模型详情 (availability_15d /
 * availability_30d / avg_latency_7d_ms) 字段保留以便后端 W6 后续完善.
 */

import { getClient } from '../client'
import type { MonitorStatus, Provider } from '../admin/channelMonitor'

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
  timeline?: MonitorTimelinePoint[]
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

const BASE = '/plugin/channel-management/monitors'

export async function list(options?: {
  signal?: AbortSignal
}): Promise<UserMonitorListResponse> {
  const { data } = await getClient().get<UserMonitorListResponse>(BASE, {
    signal: options?.signal,
  })
  return data
}

/**
 * Get detailed status (multi-window availability + latency) for a single monitor.
 * Backend route: GET /plugin/channel-management/monitors/:id  (handler: UserHandler.GetStatus).
 */
export async function status(id: number): Promise<UserMonitorDetail> {
  const { data } = await getClient().get<UserMonitorDetail>(`${BASE}/${id}`)
  return data
}

export const channelMonitorUserAPI = {
  list,
  status,
}

export default channelMonitorUserAPI
