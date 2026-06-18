/**
 * Admin Usage API endpoints
 * Handles admin-level usage logs and statistics retrieval
 */

import { apiClient REDACTED from '../client'
import type { AdminUsageLog, UsageQueryParams, PaginatedResponse, UsageRequestType REDACTED from '@/types'
import type { EndpointStat REDACTED from '@/types'

// ==================== Types ====================

export interface AdminUsageStatsResponse {
  total_requests: number
  total_input_tokens: number
  total_output_tokens: number
  total_cache_tokens: number
  total_cache_creation_tokens: number
  total_cache_read_tokens: number
  total_tokens: number
  total_cost: number
  total_actual_cost: number
  total_account_cost: number
  average_duration_ms: number
  endpoints?: EndpointStat[]
  upstream_endpoints?: EndpointStat[]
  endpoint_paths?: EndpointStat[]
REDACTED

export interface SimpleUser {
  id: number
  email: string
  deleted: boolean
REDACTED

export interface SimpleApiKey {
  id: number
  name: string
  user_id: number
REDACTED

export interface UsageCleanupFilters {
  start_time: string
  end_time: string
  user_id?: number
  api_key_id?: number
  account_id?: number
  group_id?: number
  model?: string | null
  request_type?: UsageRequestType | null
  stream?: boolean | null
  billing_type?: number | null
REDACTED

export interface UsageCleanupTask {
  id: number
  status: string
  filters: UsageCleanupFilters
  created_by: number
  deleted_rows: number
  error_message?: string | null
  canceled_by?: number | null
  canceled_at?: string | null
  started_at?: string | null
  finished_at?: string | null
  created_at: string
  updated_at: string
REDACTED

export interface CreateUsageCleanupTaskRequest {
  start_date: string
  end_date: string
  user_id?: number
  api_key_id?: number
  account_id?: number
  group_id?: number
  model?: string | null
  request_type?: UsageRequestType | null
  stream?: boolean | null
  billing_type?: number | null
  timezone?: string
REDACTED

export interface AdminUsageQueryParams extends UsageQueryParams {
  user_id?: number
  exact_total?: boolean
  billing_mode?: string
  sort_by?: string
  sort_order?: 'asc' | 'desc'
REDACTED

// ==================== API Functions ====================

/**
 * List all usage logs with optional filters (admin only)
 * @param params - Query parameters for filtering and pagination
 * @returns Paginated list of usage logs
 */
export async function list(
  params: AdminUsageQueryParams,
  options?: { signal?: AbortSignal REDACTED
): Promise<PaginatedResponse<AdminUsageLog>> {
  const { data REDACTED = await apiClient.get<PaginatedResponse<AdminUsageLog>>('/admin/usage', {
    params,
    signal: options?.signal
  REDACTED)
  return data
REDACTED

/**
 * Get usage statistics with optional filters (admin only)
 * @param params - Query parameters for filtering
 * @returns Usage statistics
 */
export async function getStats(params: {
  user_id?: number
  api_key_id?: number
  account_id?: number
  group_id?: number
  model?: string
  request_type?: UsageRequestType
  stream?: boolean
  period?: string
  start_date?: string
  end_date?: string
  timezone?: string
  nocache?: number
REDACTED): Promise<AdminUsageStatsResponse> {
  const { data REDACTED = await apiClient.get<AdminUsageStatsResponse>('/admin/usage/stats', {
    params
  REDACTED)
  return data
REDACTED

/**
 * Search users by email keyword (admin only)
 * @param keyword - Email keyword to search
 * @returns List of matching users (max 30)
 */
export async function searchUsers(keyword: string): Promise<SimpleUser[]> {
  const { data REDACTED = await apiClient.get<SimpleUser[]>('/admin/usage/search-users', {
    params: { q: keyword REDACTED
  REDACTED)
  return data
REDACTED

/**
 * Search API keys by user ID and/or keyword (admin only)
 * @param userId - Optional user ID to filter by
 * @param keyword - Optional keyword to search in key name
 * @returns List of matching API keys (max 30)
 */
export async function searchApiKeys(userId?: number, keyword?: string): Promise<SimpleApiKey[]> {
  const params: Record<string, unknown> = {REDACTED
  if (userId !== undefined) {
    params.user_id = userId
  REDACTED
  if (keyword) {
    params.q = keyword
  REDACTED
  const { data REDACTED = await apiClient.get<SimpleApiKey[]>('/admin/usage/search-api-keys', {
    params
  REDACTED)
  return data
REDACTED

/**
 * List usage cleanup tasks (admin only)
 * @param params - Query parameters for pagination
 * @returns Paginated list of cleanup tasks
 */
export async function listCleanupTasks(
  params: { page?: number; page_size?: number REDACTED,
  options?: { signal?: AbortSignal REDACTED
): Promise<PaginatedResponse<UsageCleanupTask>> {
  const { data REDACTED = await apiClient.get<PaginatedResponse<UsageCleanupTask>>('/admin/usage/cleanup-tasks', {
    params,
    signal: options?.signal
  REDACTED)
  return data
REDACTED

/**
 * Create a usage cleanup task (admin only)
 * @param payload - Cleanup task parameters
 * @returns Created cleanup task
 */
export async function createCleanupTask(payload: CreateUsageCleanupTaskRequest): Promise<UsageCleanupTask> {
  const { data REDACTED = await apiClient.post<UsageCleanupTask>('/admin/usage/cleanup-tasks', payload)
  return data
REDACTED

/**
 * Cancel a usage cleanup task (admin only)
 * @param taskId - Task ID to cancel
 */
export async function cancelCleanupTask(taskId: number): Promise<{ id: number; status: string REDACTED> {
  const { data REDACTED = await apiClient.post<{ id: number; status: string REDACTED>(
    `/admin/usage/cleanup-tasks/${taskIdREDACTED/cancel`
  )
  return data
REDACTED

export const adminUsageAPI = {
  list,
  getStats,
  searchUsers,
  searchApiKeys,
  listCleanupTasks,
  createCleanupTask,
  cancelCleanupTask
REDACTED

export default adminUsageAPI
