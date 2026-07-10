/**
 * Admin Dashboard API endpoints
 * Provides system-wide statistics and metrics
 */

import { apiClient REDACTED from '../client'
import type {
  DashboardStats,
  TrendDataPoint,
  ModelStat,
  GroupStat,
  ApiKeyUsageTrendPoint,
  UserUsageTrendPoint,
  UserSpendingRankingResponse,
  UserBreakdownItem,
  UsageRequestType
REDACTED from '@/types'

/**
 * Get dashboard statistics
 * @returns Dashboard statistics including users, keys, accounts, and token usage
 */
export async function getStats(): Promise<DashboardStats> {
  const { data REDACTED = await apiClient.get<DashboardStats>('/admin/dashboard/stats')
  return data
REDACTED

/**
 * Get real-time metrics
 * @returns Real-time system metrics
 */
export async function getRealtimeMetrics(): Promise<{
  active_requests: number
  requests_per_minute: number
  average_response_time: number
  error_rate: number
REDACTED> {
  const { data REDACTED = await apiClient.get<{
    active_requests: number
    requests_per_minute: number
    average_response_time: number
    error_rate: number
  REDACTED>('/admin/dashboard/realtime')
  return data
REDACTED

export interface TrendParams {
  start_date?: string
  end_date?: string
  granularity?: 'day' | 'hour'
  user_id?: number
  api_key_id?: number
  model?: string
  account_id?: number
  group_id?: number
  request_type?: UsageRequestType
  stream?: boolean
  billing_type?: number | null
REDACTED

export interface TrendResponse {
  trend: TrendDataPoint[]
  start_date: string
  end_date: string
  granularity: string
REDACTED

/**
 * Get usage trend data
 * @param params - Query parameters for filtering
 * @returns Usage trend data
 */
export async function getUsageTrend(params?: TrendParams): Promise<TrendResponse> {
  const { data REDACTED = await apiClient.get<TrendResponse>('/admin/dashboard/trend', { params REDACTED)
  return data
REDACTED

export interface ModelStatsParams {
  start_date?: string
  end_date?: string
  user_id?: number
  api_key_id?: number
  model?: string
  model_source?: 'requested' | 'upstream' | 'mapping'
  account_id?: number
  group_id?: number
  request_type?: UsageRequestType
  stream?: boolean
  billing_type?: number | null
REDACTED

export interface ModelStatsResponse {
  models: ModelStat[]
  start_date: string
  end_date: string
REDACTED

/**
 * Get model usage statistics
 * @param params - Query parameters for filtering
 * @returns Model usage statistics
 */
export async function getModelStats(params?: ModelStatsParams): Promise<ModelStatsResponse> {
  const { data REDACTED = await apiClient.get<ModelStatsResponse>('/admin/dashboard/models', { params REDACTED)
  return data
REDACTED

export interface GroupStatsParams {
  start_date?: string
  end_date?: string
  user_id?: number
  api_key_id?: number
  account_id?: number
  group_id?: number
  request_type?: UsageRequestType
  stream?: boolean
  billing_type?: number | null
REDACTED

export interface GroupStatsResponse {
  groups: GroupStat[]
  start_date: string
  end_date: string
REDACTED

export interface DashboardSnapshotV2Params extends TrendParams {
  include_stats?: boolean
  include_trend?: boolean
  include_model_stats?: boolean
  include_group_stats?: boolean
  include_users_trend?: boolean
  users_trend_limit?: number
REDACTED

export interface DashboardSnapshotV2Stats extends DashboardStats {
  uptime: number
REDACTED

export interface DashboardSnapshotV2Response {
  generated_at: string
  start_date: string
  end_date: string
  granularity: string
  stats?: DashboardSnapshotV2Stats
  trend?: TrendDataPoint[]
  models?: ModelStat[]
  groups?: GroupStat[]
  users_trend?: UserUsageTrendPoint[]
REDACTED

/**
 * Get group usage statistics
 * @param params - Query parameters for filtering
 * @returns Group usage statistics
 */
export async function getGroupStats(params?: GroupStatsParams): Promise<GroupStatsResponse> {
  const { data REDACTED = await apiClient.get<GroupStatsResponse>('/admin/dashboard/groups', { params REDACTED)
  return data
REDACTED

export interface UserBreakdownParams {
  start_date?: string
  end_date?: string
  group_id?: number
  model?: string
  model_source?: 'requested' | 'upstream' | 'mapping'
  endpoint?: string
  endpoint_type?: 'inbound' | 'upstream' | 'path'
  limit?: number
  // Sort column for the ranking (allowlisted server-side; falls back to actual_cost)
  sort_by?: 'total_tokens' | 'input_tokens' | 'output_tokens' | 'cache_tokens' | 'requests' | 'cost' | 'actual_cost'
  // Additional filter conditions
  user_id?: number
  api_key_id?: number
  account_id?: number
  request_type?: UsageRequestType
  stream?: boolean
  billing_type?: number | null
REDACTED

export interface UserBreakdownResponse {
  users: UserBreakdownItem[]
  start_date: string
  end_date: string
REDACTED

export async function getUserBreakdown(params: UserBreakdownParams): Promise<UserBreakdownResponse> {
  const { data REDACTED = await apiClient.get<UserBreakdownResponse>('/admin/dashboard/user-breakdown', {
    params
  REDACTED)
  return data
REDACTED

/**
 * Get dashboard snapshot v2 (aggregated response for heavy admin pages).
 */
export async function getSnapshotV2(params?: DashboardSnapshotV2Params): Promise<DashboardSnapshotV2Response> {
  const { data REDACTED = await apiClient.get<DashboardSnapshotV2Response>('/admin/dashboard/snapshot-v2', {
    params
  REDACTED)
  return data
REDACTED

export interface ApiKeyTrendParams extends TrendParams {
  limit?: number
REDACTED

export interface ApiKeyTrendResponse {
  trend: ApiKeyUsageTrendPoint[]
  start_date: string
  end_date: string
  granularity: string
REDACTED

/**
 * Get API key usage trend data
 * @param params - Query parameters for filtering
 * @returns API key usage trend data
 */
export async function getApiKeyUsageTrend(
  params?: ApiKeyTrendParams
): Promise<ApiKeyTrendResponse> {
  const { data REDACTED = await apiClient.get<ApiKeyTrendResponse>('/admin/dashboard/api-keys-trend', {
    params
  REDACTED)
  return data
REDACTED

export interface UserTrendParams extends TrendParams {
  limit?: number
REDACTED

export interface UserTrendResponse {
  trend: UserUsageTrendPoint[]
  start_date: string
  end_date: string
  granularity: string
REDACTED

export interface UserSpendingRankingParams
  extends Pick<TrendParams, 'start_date' | 'end_date'> {
  limit?: number
REDACTED

/**
 * Get user usage trend data
 * @param params - Query parameters for filtering
 * @returns User usage trend data
 */
export async function getUserUsageTrend(params?: UserTrendParams): Promise<UserTrendResponse> {
  const { data REDACTED = await apiClient.get<UserTrendResponse>('/admin/dashboard/users-trend', {
    params
  REDACTED)
  return data
REDACTED

/**
 * Get user spending ranking data
 * @param params - Query parameters for filtering
 * @returns User spending ranking data
 */
export async function getUserSpendingRanking(
  params?: UserSpendingRankingParams
): Promise<UserSpendingRankingResponse> {
  const { data REDACTED = await apiClient.get<UserSpendingRankingResponse>('/admin/dashboard/users-ranking', {
    params
  REDACTED)
  return data
REDACTED

export interface PlatformUsage {
  platform: string
  today_actual_cost: number
  total_actual_cost: number
REDACTED

export interface BatchUserUsageStats {
  user_id: number
  today_actual_cost: number
  total_actual_cost: number
  by_platform?: PlatformUsage[]
REDACTED

export interface BatchUsersUsageResponse {
  stats: Record<string, BatchUserUsageStats>
REDACTED

/**
 * Get batch usage stats for multiple users
 * @param userIds - Array of user IDs
 * @returns Usage stats map keyed by user ID
 */
export async function getBatchUsersUsage(userIds: number[]): Promise<BatchUsersUsageResponse> {
  const { data REDACTED = await apiClient.post<BatchUsersUsageResponse>('/admin/dashboard/users-usage', {
    user_ids: userIds
  REDACTED)
  return data
REDACTED

export interface BatchApiKeyUsageStats {
  api_key_id: number
  today_actual_cost: number
  total_actual_cost: number
REDACTED

export interface BatchApiKeysUsageResponse {
  stats: Record<string, BatchApiKeyUsageStats>
REDACTED

/**
 * Get batch usage stats for multiple API keys
 * @param apiKeyIds - Array of API key IDs
 * @returns Usage stats map keyed by API key ID
 */
export async function getBatchApiKeysUsage(
  apiKeyIds: number[]
): Promise<BatchApiKeysUsageResponse> {
  const { data REDACTED = await apiClient.post<BatchApiKeysUsageResponse>(
    '/admin/dashboard/api-keys-usage',
    {
      api_key_ids: apiKeyIds
    REDACTED
  )
  return data
REDACTED

export const dashboardAPI = {
  getStats,
  getRealtimeMetrics,
  getUsageTrend,
  getModelStats,
  getGroupStats,
  getSnapshotV2,
  getApiKeyUsageTrend,
  getUserUsageTrend,
  getUserSpendingRanking,
  getBatchUsersUsage,
  getBatchApiKeysUsage
REDACTED

export default dashboardAPI
