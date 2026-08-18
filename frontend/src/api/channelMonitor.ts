/**
 * User-facing Channel Monitor API endpoints
 * Read-only views for end users to inspect channel availability/status.
 */

import { apiClient REDACTED from './client'
import type { MonitorQuotaSnapshot, Provider, MonitorStatus REDACTED from './admin/channelMonitor'

export type { Provider, MonitorStatus REDACTED from './admin/channelMonitor'

export interface UserMonitorExtraModel {
  model: string
  status: MonitorStatus
  latency_ms: number | null
REDACTED

export interface MonitorTimelinePoint {
  status: MonitorStatus
  latency_ms: number | null
  ping_latency_ms: number | null
  checked_at: string
REDACTED

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
  /**
   * 主模型最近配额快照。仅当系统开启 channel_monitor_show_quota 时
   * 服务端才会下发（关闭时服务端已剥离，前端 flag 仅作纵深防御）。
   */
  latest_quota?: MonitorQuotaSnapshot | null
REDACTED

export interface UserMonitorListResponse {
  items: UserMonitorView[]
REDACTED

export interface UserMonitorModelDetail {
  model: string
  latest_status: MonitorStatus
  latest_latency_ms: number | null
  availability_7d: number
  availability_15d: number
  availability_30d: number
  avg_latency_7d_ms: number | null
REDACTED

export interface UserMonitorDetail {
  id: number
  name: string
  provider: Provider
  group_name: string
  models: UserMonitorModelDetail[]
REDACTED

/**
 * List all monitor views available to the current user.
 */
export async function list(options?: { signal?: AbortSignal REDACTED): Promise<UserMonitorListResponse> {
  const { data REDACTED = await apiClient.get<UserMonitorListResponse>('/channel-monitors', {
    signal: options?.signal,
  REDACTED)
  return data
REDACTED

/**
 * Get detailed status (multi-window availability + latency) for a single monitor.
 */
export async function status(id: number): Promise<UserMonitorDetail> {
  const { data REDACTED = await apiClient.get<UserMonitorDetail>(`/channel-monitors/${idREDACTED/status`)
  return data
REDACTED

export const channelMonitorUserAPI = {
  list,
  status,
REDACTED

export default channelMonitorUserAPI
