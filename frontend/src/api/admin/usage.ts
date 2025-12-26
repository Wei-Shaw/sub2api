/**
 * Admin Usage API endpoints
 * Handles admin-level usage logs and statistics retrieval
 */

import { apiClient REDACTED from '../client'
import type { UsageLog, UsageQueryParams, PaginatedResponse REDACTED from '@/types'

// ==================== Types ====================

export interface AdminUsageStatsResponse {
  total_requests: number
  total_input_tokens: number
  total_output_tokens: number
  total_cache_tokens: number
  total_tokens: number
  total_cost: number
  total_actual_cost: number
  average_duration_ms: number
REDACTED

export interface SimpleUser {
  id: number
  email: string
REDACTED

export interface SimpleApiKey {
  id: number
  name: string
  user_id: number
REDACTED

export interface AdminUsageQueryParams extends UsageQueryParams {
  user_id?: number
REDACTED

// ==================== API Functions ====================

/**
 * List all usage logs with optional filters (admin only)
 * @param params - Query parameters for filtering and pagination
 * @returns Paginated list of usage logs
 */
export async function list(params: AdminUsageQueryParams): Promise<PaginatedResponse<UsageLog>> {
  const { data REDACTED = await apiClient.get<PaginatedResponse<UsageLog>>('/admin/usage', {
    params
  REDACTED)
  return data
REDACTED

/**
 * Get usage statistics with optional filters (admin only)
 * @param params - Query parameters (user_id, api_key_id, period/date range)
 * @returns Usage statistics
 */
export async function getStats(params: {
  user_id?: number
  api_key_id?: number
  period?: string
  start_date?: string
  end_date?: string
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

export const adminUsageAPI = {
  list,
  getStats,
  searchUsers,
  searchApiKeys
REDACTED

export default adminUsageAPI
