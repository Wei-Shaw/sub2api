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
  rate_multiplier: number
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
 * @param id - Group ID
 * @param entries - Array of { user_id, rate_multiplier REDACTED
 * @returns Success confirmation
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

export const groupsAPI = {
  list,
  getAll,
  getByPlatform,
  getById,
  create,
  update,
  delete: deleteGroup,
  toggleStatus,
  getStats,
  getGroupApiKeys,
  getGroupRateMultipliers,
  clearGroupRateMultipliers,
  batchSetGroupRateMultipliers,
  updateSortOrder
REDACTED

export default groupsAPI
