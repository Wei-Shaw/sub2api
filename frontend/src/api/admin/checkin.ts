import { apiClient } from '../client'
import type { CheckInConfig } from '../checkin'

export interface CheckInTrendPoint {
  date: string
  users: number
  reward: number
}

export interface CheckInAdminStats {
  config: CheckInConfig
  today_users: number
  today_reward: number
  total_users: number
  total_reward: number
  reduced_mode_users: number
  trend: CheckInTrendPoint[]
  server_date: string
  server_timezone: string
}

export interface CheckInResetResult {
  affected_users: number
  reset_at: string
}

export async function getStats(days = 30): Promise<CheckInAdminStats> {
  const { data } = await apiClient.get<CheckInAdminStats>('/admin/checkin/stats', {
    params: { days }
  })
  return data
}

export async function setEnabled(enabled: boolean): Promise<CheckInConfig> {
  const { data } = await apiClient.put<CheckInConfig>('/admin/checkin/enabled', { enabled })
  return data
}

export async function resetCycles(): Promise<CheckInResetResult> {
  const { data } = await apiClient.post<CheckInResetResult>('/admin/checkin/reset-cycles')
  return data
}

export default {
  getStats,
  setEnabled,
  resetCycles
}
