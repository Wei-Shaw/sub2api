import { apiClient } from './client'

export type GroupAvailability = 'available' | 'degraded' | 'rate_limited' | 'unavailable'

export interface PublicGroupStatus {
  id: number
  name: string
  description: string
  platform: string
  subscription_type: string
  rate_multiplier: number
  peak_rate_enabled: boolean
  peak_start: string
  peak_end: string
  peak_rate_multiplier: number
  account_count: number
  available_account_count: number
  rate_limited_account_count: number
  status: string
  availability: GroupAvailability
  available: boolean
}

export interface GroupsStatusSummary {
  group_count: number
  available_group_count: number
  account_count: number
  available_account_count: number
  rate_limited_account_count: number
}

export interface GroupsStatusResponse {
  groups: PublicGroupStatus[]
  summary: GroupsStatusSummary
}

/** Public aggregate status endpoint. No authentication is required. */
export async function getGroupsStatus(options?: { signal?: AbortSignal }): Promise<GroupsStatusResponse> {
  const { data } = await apiClient.get<GroupsStatusResponse>('/groups-status', {
    signal: options?.signal
  })
  return data
}

export const groupsStatusAPI = { getGroupsStatus }

export default groupsStatusAPI
