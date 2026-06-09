/**
 * Admin Groups API endpoints
 * Handles API key group management for administrators
 */

import { apiClient REDACTED from '../client'
import type {
  AdminGroup,
  GroupPlatform,
  CreateGroupRequest,
  UpdateGroupRequest,
  PaginatedResponse
REDACTED from '@/types'

/**
 * List all groups with pagination
 * @param page - Page number (default: 1)
 * @param pageSize - Items per page (default: 20)
 * @param filters - Optional filters (platform, status, is_exclusive, search)
 * @returns Paginated list of groups
 */
export async function list(
  page: number = 1,
  pageSize: number = 20,
  filters?: {
    platform?: GroupPlatform
    status?: 'active' | 'inactive'
    is_exclusive?: boolean
    search?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  REDACTED,
  options?: {
    signal?: AbortSignal
  REDACTED
): Promise<PaginatedResponse<AdminGroup>> {
  const { data REDACTED = await apiClient.get<PaginatedResponse<AdminGroup>>('/admin/groups', {
    params: {
      page,
      page_size: pageSize,
      ...filters
    REDACTED,
    signal: options?.signal
  REDACTED)
  return data
REDACTED

/**
 * Get all active groups (without pagination)
 * @param platform - Optional platform filter
 * @returns List of all active groups
 */
export async function getAll(platform?: GroupPlatform): Promise<AdminGroup[]> {
  const { data REDACTED = await apiClient.get<AdminGroup[]>('/admin/groups/all', {
    params: platform ? { platform REDACTED : undefined
  REDACTED)
  return data
REDACTED

/**
 * Get ALL groups including disabled ones — used by the API Key group filter so
 * that admins can filter users whose keys are still bound to a now-disabled group.
 */
export async function getAllIncludingInactive(): Promise<AdminGroup[]> {
  const { data REDACTED = await apiClient.get<AdminGroup[]>('/admin/groups/all', {
    params: { include_inactive: true REDACTED
  REDACTED)
  return data
REDACTED

/**
 * Get active groups by platform
 * @param platform - Platform to filter by
 * @returns List of groups for the specified platform
 */
export async function getByPlatform(platform: GroupPlatform): Promise<AdminGroup[]> {
  return getAll(platform)
REDACTED

/**
 * Get group by ID
 * @param id - Group ID
 * @returns Group details
 */
export async function getById(id: number): Promise<AdminGroup> {
  const { data REDACTED = await apiClient.get<AdminGroup>(`/admin/groups/${idREDACTED`)
  return data
REDACTED

/**
 * Get candidate models for custom /v1/models list.
 * id=0 returns platform default models for create flow.
 */
export async function getModelsListCandidates(
  id: number,
  platform?: GroupPlatform
): Promise<string[]> {
  const { data REDACTED = await apiClient.get<{ models: string[] REDACTED>(
    `/admin/groups/${idREDACTED/models-list-candidates`,
    {
      params: platform ? { platform REDACTED : undefined
    REDACTED
  )
  return data.models || []
REDACTED

/**
 * Create new group
 * @param groupData - Group data
 * @returns Created group
 */
export async function create(groupData: CreateGroupRequest): Promise<AdminGroup> {
  const { data REDACTED = await apiClient.post<AdminGroup>('/admin/groups', groupData)
  return data
REDACTED

/**
 * Update group
 * @param id - Group ID
 * @param updates - Fields to update
 * @returns Updated group
 */
export async function update(id: number, updates: UpdateGroupRequest): Promise<AdminGroup> {
  const { data REDACTED = await apiClient.put<AdminGroup>(`/admin/groups/${idREDACTED`, updates)
  return data
REDACTED

/**
 * Delete group
 * @param id - Group ID
 * @returns Success confirmation
 */
export async function deleteGroup(id: number): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.delete<{ message: string REDACTED>(`/admin/groups/${idREDACTED`)
  return data
REDACTED

/**
 * Toggle group status
 * @param id - Group ID
 * @param status - New status
 * @returns Updated group
 */
export async function toggleStatus(id: number, status: 'active' | 'inactive'): Promise<AdminGroup> {
  return update(id, { status REDACTED)
REDACTED

/**
 * Get group statistics
 * @param id - Group ID
 * @returns Group usage statistics
 */
export async function getStats(id: number): Promise<{
  total_api_keys: number
  active_api_keys: number
  total_requests: number
  total_cost: number
REDACTED> {
  const { data REDACTED = await apiClient.get<{
    total_api_keys: number
    active_api_keys: number
    total_requests: number
    total_cost: number
  REDACTED>(`/admin/groups/${idREDACTED/stats`)
  return data
REDACTED

/**
 * Get API keys in a group
 * @param id - Group ID
 * @param page - Page number
 * @param pageSize - Items per page
 * @returns Paginated list of API keys in the group
 */
export async function getGroupApiKeys(
  id: number,
  page: number = 1,
  pageSize: number = 20
): Promise<PaginatedResponse<any>> {
  const { data REDACTED = await apiClient.get<PaginatedResponse<any>>(`/admin/groups/${idREDACTED/api-keys`, {
    params: { page, page_size: pageSize REDACTED
  REDACTED)
  return data
REDACTED

/**
 * Rate multiplier entry for a user in a group
 */
export interface GroupRateMultiplierEntry {
  user_id: number
  user_name: string
  user_email: string
  user_notes: string
  user_status: string
  rate_multiplier?: number | null
  rpm_override?: number | null
REDACTED

/**
 * Get rate multipliers for users in a group
 * @param id - Group ID
 * @returns List of user rate multiplier entries
 */
export async function getGroupRateMultipliers(id: number): Promise<GroupRateMultiplierEntry[]> {
  const { data REDACTED = await apiClient.get<GroupRateMultiplierEntry[]>(
    `/admin/groups/${idREDACTED/rate-multipliers`
  )
  return data
REDACTED

/**
 * Update group sort orders
 * @param updates - Array of { id, sort_order REDACTED objects
 * @returns Success confirmation
 */
export async function updateSortOrder(
  updates: Array<{ id: number; sort_order: number REDACTED>
): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.put<{ message: string REDACTED>('/admin/groups/sort-order', {
    updates
  REDACTED)
  return data
REDACTED

/**
 * Clear all rate multipliers for a group
 * @param id - Group ID
 * @returns Success confirmation
 */
export async function clearGroupRateMultipliers(id: number): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.delete<{ message: string REDACTED>(`/admin/groups/${idREDACTED/rate-multipliers`)
  return data
REDACTED

/**
 * Batch set rate multipliers for users in a group
 * Only touches rate_multiplier column; preserves rpm_override on existing rows.
 */
export async function batchSetGroupRateMultipliers(
  id: number,
  entries: Array<{ user_id: number; rate_multiplier: number REDACTED>
): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.put<{ message: string REDACTED>(
    `/admin/groups/${idREDACTED/rate-multipliers`,
    { entries REDACTED
  )
  return data
REDACTED

/**
 * RPM override entry for a user in a group
 */
export interface GroupRPMOverrideEntry {
  user_id: number
  user_name: string
  user_email: string
  user_notes: string
  user_status: string
  rpm_override: number
REDACTED

/**
 * Get RPM overrides for users in a group (subset of rate-multipliers endpoint).
 */
export async function getGroupRPMOverrides(id: number): Promise<GroupRPMOverrideEntry[]> {
  const { data REDACTED = await apiClient.get<GroupRateMultiplierEntry[]>(
    `/admin/groups/${idREDACTED/rate-multipliers`
  )
  return data
    .filter(e => e.rpm_override != null)
    .map(e => ({
      user_id: e.user_id,
      user_name: e.user_name,
      user_email: e.user_email,
      user_notes: e.user_notes,
      user_status: e.user_status,
      rpm_override: e.rpm_override as number
    REDACTED))
REDACTED

/**
 * Batch set RPM overrides for users in a group.
 * Only touches rpm_override column; preserves rate_multiplier on existing rows.
 */
export async function batchSetGroupRPMOverrides(
  id: number,
  entries: Array<{ user_id: number; rpm_override: number REDACTED>
): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.put<{ message: string REDACTED>(
    `/admin/groups/${idREDACTED/rpm-overrides`,
    { entries REDACTED
  )
  return data
REDACTED

/**
 * Clear all RPM overrides for a group (preserves rate_multiplier).
 */
export async function clearGroupRPMOverrides(id: number): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.delete<{ message: string REDACTED>(`/admin/groups/${idREDACTED/rpm-overrides`)
  return data
REDACTED

/**
 * Get usage summary (today + cumulative cost) for all groups
 * @param timezone - IANA timezone string (e.g. "Asia/Shanghai")
 * @returns Array of group usage summaries
 */
export async function getUsageSummary(
  timezone?: string
): Promise<{ group_id: number; today_cost: number; total_cost: number REDACTED[]> {
  const { data REDACTED = await apiClient.get<
    { group_id: number; today_cost: number; total_cost: number REDACTED[]
  >('/admin/groups/usage-summary', {
    params: timezone ? { timezone REDACTED : undefined
  REDACTED)
  return data
REDACTED

/**
 * Get capacity summary (concurrency/sessions/RPM) for all active groups
 */
export async function getCapacitySummary(): Promise<
  { group_id: number; concurrency_used: number; concurrency_max: number; sessions_used: number; sessions_max: number; rpm_used: number; rpm_max: number REDACTED[]
> {
  const { data REDACTED = await apiClient.get<
    { group_id: number; concurrency_used: number; concurrency_max: number; sessions_used: number; sessions_max: number; rpm_used: number; rpm_max: number REDACTED[]
  >('/admin/groups/capacity-summary')
  return data
REDACTED

export const groupsAPI = {
  list,
  getAll,
  getByPlatform,
  getAllIncludingInactive,
  getById,
  getModelsListCandidates,
  create,
  update,
  delete: deleteGroup,
  toggleStatus,
  getStats,
  getGroupApiKeys,
  getGroupRateMultipliers,
  clearGroupRateMultipliers,
  batchSetGroupRateMultipliers,
  getGroupRPMOverrides,
  clearGroupRPMOverrides,
  batchSetGroupRPMOverrides,
  updateSortOrder,
  getUsageSummary,
  getCapacitySummary
REDACTED

export default groupsAPI
