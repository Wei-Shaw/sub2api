import { apiClient } from './client'

export interface CheckInConfig {
  enabled: boolean
  standard_min: number
  standard_max: number
  reduced_threshold: number
  reduced_min: number
  reduced_max: number
  updated_at: string
}

export interface CheckInRecord {
  id: number
  date: string
  reward: number
  mode: 'standard' | 'reduced'
  cycle_reward_after: number
  created_at: string
}

export interface CheckInStatus {
  config: CheckInConfig
  checked_today: boolean
  today_reward: number
  cycle_reward: number
  total_reward: number
  next_reward_mode: 'standard' | 'reduced'
  next_reward_min: number
  next_reward_max: number
  recent_checkins: CheckInRecord[]
  server_date: string
  server_timezone: string
}

export interface CheckInResult {
  record: CheckInRecord
  already_checked: boolean
  new_balance: number
}

export async function getCheckInStatus(): Promise<CheckInStatus> {
  const { data } = await apiClient.get<CheckInStatus>('/user/checkin')
  return data
}

export async function checkIn(): Promise<CheckInResult> {
  const { data } = await apiClient.post<CheckInResult>('/user/checkin')
  return data
}

export default {
  getStatus: getCheckInStatus,
  checkIn
}
