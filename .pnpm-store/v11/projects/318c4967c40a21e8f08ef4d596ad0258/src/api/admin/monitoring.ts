import { apiClient } from '../client'

export interface GroupHealth {
  group_id: number
  group_name: string
  total_accounts: number
  active_accounts: number
  error_accounts: number
  rate_limited: number
  overload: number
  disabled: number
}

export interface ModelLatency {
  model: string
  request_count: number
  success_count: number
  error_count: number
  avg_latency_ms: number
  p50_latency_ms: number
  p95_latency_ms: number
  p99_latency_ms: number
  avg_first_token_ms: number
}

export interface GroupModelStats {
  group_id: number
  group_name: string
  model: string
  request_count: number
  success_count: number
  error_count: number
  avg_latency_ms: number
  p50_latency_ms: number
  p95_latency_ms: number
  avg_ttft: number
}

export interface HourlyStats {
  hour: string
  total: number
  success: number
}

export interface ModelHourlyStats {
  group_id: number
  model: string
  hour: string
  total: number
  success: number
}

export interface ErrorAccount {
  account_id: number
  account_name: string
  group_name: string
  status: string
  error_message: string
  rate_limited_at?: string
  overload_until?: string
}

export interface MonitoringOverview {
  groups: GroupHealth[]
  group_models: GroupModelStats[]
  model_latencies: ModelLatency[]
  error_accounts: ErrorAccount[]
  hourly_stats: HourlyStats[]
  model_hourly_stats: ModelHourlyStats[]
  total_requests_today: number
  success_count_today: number
  error_count_today: number
  avg_latency_ms_today: number
}

export interface MonitoringSummary {
  groups: GroupHealth[]
  error_accounts: ErrorAccount[]
  hourly_stats: HourlyStats[]
  total_requests_today: number
  success_count_today: number
  error_count_today: number
  avg_latency_ms_today: number
}

export interface MonitoringGroupModels {
  group_models: GroupModelStats[]
  model_hourly_stats: ModelHourlyStats[]
}

export interface MonitoringModelLatency {
  model_latencies: ModelLatency[]
}

export async function getMonitoringOverview(): Promise<MonitoringOverview> {
  const { data } = await apiClient.get<MonitoringOverview>('/monitoring/overview')
  return data
}

export async function getMonitoringSummary(): Promise<MonitoringSummary> {
  const { data } = await apiClient.get<MonitoringSummary>('/monitoring/summary')
  return data
}

export async function getMonitoringGroupModels(): Promise<MonitoringGroupModels> {
  const { data } = await apiClient.get<MonitoringGroupModels>('/monitoring/group-models')
  return data
}

export async function getMonitoringModelLatency(): Promise<MonitoringModelLatency> {
  const { data } = await apiClient.get<MonitoringModelLatency>('/monitoring/model-latency')
  return data
}

export const monitoringAPI = {
  getOverview: getMonitoringOverview,
  getSummary: getMonitoringSummary,
  getGroupModels: getMonitoringGroupModels,
  getModelLatency: getMonitoringModelLatency,
}

export default monitoringAPI
