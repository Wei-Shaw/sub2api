/**
 * User Groups API endpoints (non-admin)
 * Handles group-related operations for regular users
 */

import { apiClient } from './client'
import type { Group } from '@/types'

/** Group with optional permission-source metadata (returned by /groups/available/profile). */
export interface AvailableGroup extends Group {
  permission_source?: string // "direct" | "pool" | "public" | "subscription"
  pool_id?: number
  pool_name?: string
}

/** Response shape of GET /groups/available/profile */
export interface AvailableGroupsProfile {
  bindable_groups: AvailableGroup[]
  grant_effective_groups: number[]
}

/**
 * Get available groups that the current user can bind to API keys
 * This returns groups based on user's permissions:
 * - Standard groups: public (non-exclusive) or explicitly allowed
 * - Subscription groups: user has active subscription
 * @returns List of available groups
 * @deprecated Prefer getAvailableProfile() which includes Pool-derived permissions.
 */
export async function getAvailable(): Promise<Group[]> {
  const { data } = await apiClient.get<Group[]>('/groups/available')
  return data
}

/**
 * Get available groups with permission-source metadata (Pool-aware).
 * Returns bindable_groups (AvailableGroup[]) and grant_effective_groups (number[]).
 */
export async function getAvailableProfile(): Promise<AvailableGroupsProfile> {
  const { data } = await apiClient.get<AvailableGroupsProfile>('/groups/available/profile')
  return data
}

/**
 * Get current user's custom group rate multipliers
 * @returns Map of group_id to custom rate_multiplier
 */
export async function getUserGroupRates(): Promise<Record<number, number>> {
  const { data } = await apiClient.get<Record<number, number> | null>('/groups/rates')
  return data || {}
}

/**
 * Per-group capacity summary: aggregated active/rate-limited account counts
 * plus runtime concurrency / sessions / RPM (used vs max).
 * 通过单次 API 调用同时拿到账号计数与运行时容量，前端不再依赖 Group DTO 上的
 * active_account_count / rate_limited_account_count 字段。
 */
export interface GroupCapacitySummary {
  group_id: number
  active_account_count: number
  rate_limited_account_count: number
  concurrency_used: number
  concurrency_max: number
  sessions_used: number
  sessions_max: number
  rpm_used: number
  rpm_max: number
}

/**
 * Get per-group capacity summary for groups visible to the current user.
 * 仅返回用户可见的分组（与 GetAvailableGroups 可见性一致）。
 */
export async function getCapacitySummary(): Promise<GroupCapacitySummary[]> {
  const { data } = await apiClient.get<GroupCapacitySummary[] | null>('/groups/capacity-summary')
  return data ?? []
}

export const userGroupsAPI = {
  getAvailable,
  getAvailableProfile,
  getUserGroupRates,
  getCapacitySummary
}

export default userGroupsAPI
