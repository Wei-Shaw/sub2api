import { apiClient } from '../client'

export type UpstreamBalanceType = 'sub2api' | 'newapi'
export type ProbeStatus = 'ok' | 'failed' | 'pending'
export type UpstreamCredentialMode = 'password' | 'token'

export interface UpstreamRate { name: string; description?: string; ratio: number }
export interface UpstreamRateChange { group: string; old_ratio: number; new_ratio: number; changed_at: string }

export interface UpstreamBalanceMonitor {
  id: number
  name: string
  type: UpstreamBalanceType
  base_url: string
  api_key_masked: string
  credential_mode: UpstreamCredentialMode
  username?: string
  enabled: boolean
  display_order: number
  probe_interval_minutes: number
  low_balance_threshold_usd: number
  last_probe_at: string | null
  last_probe_status: ProbeStatus
  last_probe_error: string | null
  balance_display?: {
    quota_remaining_usd?: number
    used_quota_usd?: number
    request_count?: number
    group?: string
    username?: string
    email?: string
    rates?: UpstreamRate[]
    rate_changes?: UpstreamRateChange[]
  }
  next_probe_at?: string | null
  created_at: string
  updated_at: string
}

export interface UpstreamBalanceMonitorInput {
  name: string
  type: UpstreamBalanceType
  base_url: string
  api_key?: string
  cookie?: string
  user_id?: string
  credential_mode: UpstreamCredentialMode
  username?: string
  password?: string
  enabled: boolean
  display_order: number
  probe_interval_minutes: number
  low_balance_threshold_usd: number
}

export async function list(): Promise<UpstreamBalanceMonitor[]> {
  const { data } = await apiClient.get<UpstreamBalanceMonitor[]>('/admin/upstream-balance-monitors')
  return data || []
}

export async function create(input: UpstreamBalanceMonitorInput): Promise<UpstreamBalanceMonitor> {
  const { data } = await apiClient.post<UpstreamBalanceMonitor>('/admin/upstream-balance-monitors', input)
  return data
}

export async function update(id: number, input: UpstreamBalanceMonitorInput): Promise<UpstreamBalanceMonitor> {
  const { data } = await apiClient.put<UpstreamBalanceMonitor>(`/admin/upstream-balance-monitors/${id}`, input)
  return data
}

export async function remove(id: number): Promise<void> {
  await apiClient.delete(`/admin/upstream-balance-monitors/${id}`)
}

export async function probe(id: number): Promise<UpstreamBalanceMonitor> {
  const { data } = await apiClient.post<UpstreamBalanceMonitor>(`/admin/upstream-balance-monitors/${id}/probe`)
  return data
}

export async function probeAll(): Promise<UpstreamBalanceMonitor[]> {
  const { data } = await apiClient.post<UpstreamBalanceMonitor[]>('/admin/upstream-balance-monitors/probe-all')
  return data || []
}

export default { list, create, update, remove, probe, probeAll }
