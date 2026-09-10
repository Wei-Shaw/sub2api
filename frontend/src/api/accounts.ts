import { apiClient } from './client'
import type { AccountPlatform, AccountType, AccountUsageStatsResponse, ClaudeModel, PaginatedResponse } from '@/types'

export interface VisibleAccountGroup {
  id: number
  name: string
  platform: string
}

export interface VisibleAccount {
  id: number
  name: string
  platform: AccountPlatform
  type: AccountType
  concurrency: number
  priority: number
  status: 'active' | 'inactive' | 'error'
  schedulable: boolean
  last_used_at: string | null
  expires_at: number | null
  created_at: string
  rate_limit_reset_at: string | null
  overload_until: string | null
  temp_unschedulable_until: string | null
  groups: VisibleAccountGroup[]
  group_ids: number[]
}

export interface VisibleAccountFilters {
  platform?: string
  type?: string
  status?: string
  group?: string
  search?: string
  sort_by?: string
  sort_order?: 'asc' | 'desc'
}

export async function list(
  page = 1,
  pageSize = 20,
  filters?: VisibleAccountFilters,
  options?: { signal?: AbortSignal }
): Promise<PaginatedResponse<VisibleAccount>> {
  const { data } = await apiClient.get<PaginatedResponse<VisibleAccount>>('/accounts', {
    params: { page, page_size: pageSize, ...filters },
    signal: options?.signal
  })
  return data
}

export async function listGroups(options?: { signal?: AbortSignal }): Promise<VisibleAccountGroup[]> {
  const { data } = await apiClient.get<VisibleAccountGroup[]>('/accounts/groups', {
    signal: options?.signal
  })
  return data
}

export async function getStats(id: number, days = 30): Promise<AccountUsageStatsResponse> {
  const { data } = await apiClient.get<AccountUsageStatsResponse>(`/accounts/${id}/stats`, { params: { days } })
  return data
}

export async function getAvailableModels(id: number): Promise<ClaudeModel[]> {
  const { data } = await apiClient.get<ClaudeModel[]>(`/accounts/${id}/models`)
  return data
}

export const accountsAPI = { list, listGroups, getStats, getAvailableModels }

export default accountsAPI
