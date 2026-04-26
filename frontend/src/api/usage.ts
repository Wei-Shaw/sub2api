/**
 * Usage tracking API endpoints
 * Handles usage logs and statistics retrieval
 */

import { apiClient REDACTED from './client'
import type {
  UsageLog,
  UsageQueryParams,
  UsageStatsResponse,
  PaginatedResponse,
  TrendDataPoint,
  ModelStat,
  GroupStat,
  UsageRequestType,
  UserErrorRequest,
  UserErrorRequestDetail,
  UserErrorListParams
REDACTED from '@/types'

// ==================== Dashboard Types ====================

export interface PlatformDashboardStats {
  platform: string
  total_requests: number
  total_tokens: number
  total_actual_cost: number
  today_requests: number
  today_tokens: number
  today_actual_cost: number
REDACTED

export interface UserDashboardStats {
  total_api_keys: number
  active_api_keys: number
  total_requests: number
  total_input_tokens: number
  total_output_tokens: number
  total_cache_creation_tokens: number
  total_cache_read_tokens: number
  total_tokens: number
  total_cost: number // 标准计费
  total_actual_cost: number // 实际扣除
  today_requests: number
  today_input_tokens: number
  today_output_tokens: number
  today_cache_creation_tokens: number
  today_cache_read_tokens: number
  today_tokens: number
  today_cost: number // 今日标准计费
  today_actual_cost: number // 今日实际扣除
  average_duration_ms: number
  rpm: number // 近5分钟平均每分钟请求数
  tpm: number // 近5分钟平均每分钟Token数
  by_platform?: PlatformDashboardStats[]
REDACTED

export interface TrendParams {
  start_date?: string
  end_date?: string
  granularity?: 'day' | 'hour'
  api_key_id?: number
  model?: string
  group_id?: number
  request_type?: UsageRequestType
  stream?: boolean
  billing_type?: number | null
  billing_mode?: string | null
  timezone?: string
REDACTED

export interface TrendResponse {
  trend: TrendDataPoint[]
  start_date: string
  end_date: string
  granularity: string
REDACTED

export interface ModelStatsResponse {
  models: ModelStat[]
  start_date: string
  end_date: string
REDACTED

export interface ApiKeyDailyUsagePoint {
  date: string
  requests: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  total_tokens: number
  cost: number
  actual_cost: number
REDACTED

export interface ApiKeyDailyUsageResponse {
  items: ApiKeyDailyUsagePoint[]
  days: number
  start_date: string
  end_date: string
REDACTED

export interface UsageDashboardSnapshotV2Params extends TrendParams {
  include_trend?: boolean
  include_model_stats?: boolean
  include_group_stats?: boolean
REDACTED

export interface UsageDashboardSnapshotV2Response {
  generated_at: string
  start_date: string
  end_date: string
  granularity: string
  trend?: TrendDataPoint[]
  models?: ModelStat[]
  groups?: GroupStat[]
REDACTED

/**
 * List usage logs with optional filters
 * @param page - Page number (default: 1)
 * @param pageSize - Items per page (default: 20)
 * @param apiKeyId - Filter by API key ID
 * @returns Paginated list of usage logs
 */
export async function list(
  page: number = 1,
  pageSize: number = 20,
  apiKeyId?: number
): Promise<PaginatedResponse<UsageLog>> {
  const params: UsageQueryParams = {
    page,
    page_size: pageSize
  REDACTED

  if (apiKeyId !== undefined) {
    params.api_key_id = apiKeyId
  REDACTED

  const { data REDACTED = await apiClient.get<PaginatedResponse<UsageLog>>('/usage', {
    params
  REDACTED)
  return data
REDACTED

/**
 * Get usage logs with advanced query parameters
 * @param params - Query parameters for filtering and pagination
 * @returns Paginated list of usage logs
 */
export async function query(
  params: UsageQueryParams & { sort_by?: string; sort_order?: 'asc' | 'desc' REDACTED,
  config: { signal?: AbortSignal REDACTED = {REDACTED
): Promise<PaginatedResponse<UsageLog>> {
  const { data REDACTED = await apiClient.get<PaginatedResponse<UsageLog>>('/usage', {
    ...config,
    params
  REDACTED)
  return data
REDACTED

/**
 * Get usage statistics for a specific period
 * @param period - Time period ('today', 'week', 'month', 'year')
 * @param apiKeyId - Optional API key ID filter
 * @returns Usage statistics
 */
export async function getStats(
  paramsOrPeriod: (UsageQueryParams & { period?: string; timezone?: string REDACTED) | string = 'today',
  apiKeyId?: number
): Promise<UsageStatsResponse> {
  const params: Record<string, unknown> = typeof paramsOrPeriod === 'string'
    ? { period: paramsOrPeriod REDACTED
    : { ...paramsOrPeriod REDACTED

  if (apiKeyId !== undefined) {
    params.api_key_id = apiKeyId
  REDACTED

  const { data REDACTED = await apiClient.get<UsageStatsResponse>('/usage/stats', {
    params
  REDACTED)
  return data
REDACTED

/**
 * Get usage statistics for a date range
 * @param startDate - Start date (YYYY-MM-DD format)
 * @param endDate - End date (YYYY-MM-DD format)
 * @param apiKeyId - Optional API key ID filter
 * @returns Usage statistics
 */
export async function getStatsByDateRange(
  startDate: string,
  endDate: string,
  apiKeyId?: number
): Promise<UsageStatsResponse> {
  const params: Record<string, unknown> = {
    start_date: startDate,
    end_date: endDate
  REDACTED

  if (apiKeyId !== undefined) {
    params.api_key_id = apiKeyId
  REDACTED

  const { data REDACTED = await apiClient.get<UsageStatsResponse>('/usage/stats', {
    params
  REDACTED)
  return data
REDACTED

/**
 * Get usage by date range
 * @param startDate - Start date (YYYY-MM-DD format)
 * @param endDate - End date (YYYY-MM-DD format)
 * @param apiKeyId - Optional API key ID filter
 * @returns Usage logs within date range
 */
export async function getByDateRange(
  startDate: string,
  endDate: string,
  apiKeyId?: number
): Promise<PaginatedResponse<UsageLog>> {
  const params: UsageQueryParams = {
    start_date: startDate,
    end_date: endDate,
    page: 1,
    page_size: 100
  REDACTED

  if (apiKeyId !== undefined) {
    params.api_key_id = apiKeyId
  REDACTED

  const { data REDACTED = await apiClient.get<PaginatedResponse<UsageLog>>('/usage', {
    params
  REDACTED)
  return data
REDACTED

/**
 * Get detailed usage log by ID
 * @param id - Usage log ID
 * @returns Usage log details
 */
export async function getById(id: number): Promise<UsageLog> {
  const { data REDACTED = await apiClient.get<UsageLog>(`/usage/${idREDACTED`)
  return data
REDACTED

// ==================== Dashboard API ====================

/**
 * Get user dashboard statistics
 * @returns Dashboard statistics for current user
 */
export async function getDashboardStats(): Promise<UserDashboardStats> {
  const { data REDACTED = await apiClient.get<UserDashboardStats>('/usage/dashboard/stats')
  return data
REDACTED

/**
 * Get user usage trend data
 * @param params - Query parameters for filtering
 * @returns Usage trend data for current user
 */
export async function getDashboardTrend(params?: TrendParams): Promise<TrendResponse> {
  const { data REDACTED = await apiClient.get<TrendResponse>('/usage/dashboard/trend', { params REDACTED)
  return data
REDACTED

/**
 * Get user model usage statistics
 * @param params - Query parameters for filtering
 * @returns Model usage statistics for current user
 */
export async function getDashboardModels(params?: {
  start_date?: string
  end_date?: string
  api_key_id?: number
  model?: string
  model_source?: 'requested'
  group_id?: number
  request_type?: UsageRequestType
  stream?: boolean
  billing_type?: number | null
  billing_mode?: string | null
  timezone?: string
REDACTED): Promise<ModelStatsResponse> {
  const { data REDACTED = await apiClient.get<ModelStatsResponse>('/usage/dashboard/models', { params REDACTED)
  return data
REDACTED

/**
 * Get daily usage details for one API key owned by the current user.
 * @param apiKeyId - API key ID
 * @param days - Number of days to include (1-90)
 * @returns Daily usage detail rows
 */
export async function getMyApiKeyDailyUsage(
  apiKeyId: number,
  days: number = 30
): Promise<ApiKeyDailyUsageResponse> {
  const { data REDACTED = await apiClient.get<ApiKeyDailyUsageResponse>(
    `/user/api-keys/${apiKeyIdREDACTED/usage/daily`,
    { params: { days REDACTED REDACTED
  )
  return data
REDACTED

export async function getDashboardSnapshotV2(
  params?: UsageDashboardSnapshotV2Params
): Promise<UsageDashboardSnapshotV2Response> {
  const { data REDACTED = await apiClient.get<UsageDashboardSnapshotV2Response>(
    '/usage/dashboard/snapshot-v2',
    { params REDACTED
  )
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
 * Get batch usage stats for user's own API keys
 * @param apiKeyIds - Array of API key IDs
 * @param options - Optional request options
 * @returns Usage stats map keyed by API key ID
 */
export async function getDashboardApiKeysUsage(
  apiKeyIds: number[],
  options?: {
    signal?: AbortSignal
  REDACTED
): Promise<BatchApiKeysUsageResponse> {
  const { data REDACTED = await apiClient.post<BatchApiKeysUsageResponse>(
    '/usage/dashboard/api-keys-usage',
    {
      api_key_ids: apiKeyIds
    REDACTED,
    {
      signal: options?.signal
    REDACTED
  )
  return data
REDACTED

export async function listMyErrorRequests(
  params: UserErrorListParams
): Promise<PaginatedResponse<UserErrorRequest>> {
  const { data REDACTED = await apiClient.get<PaginatedResponse<UserErrorRequest>>('/usage/errors', {
    params
  REDACTED)
  return data
REDACTED

export async function getMyErrorDetail(id: number): Promise<UserErrorRequestDetail> {
  const { data REDACTED = await apiClient.get<UserErrorRequestDetail>(`/usage/errors/${idREDACTED`)
  return data
REDACTED

export const usageAPI = {
  list,
  query,
  getStats,
  getStatsByDateRange,
  getByDateRange,
  getById,
  // Dashboard
  getDashboardStats,
  getDashboardTrend,
  getDashboardModels,
  getMyApiKeyDailyUsage,
  getDashboardSnapshotV2,
  getDashboardApiKeysUsage,
  // Error requests
  listMyErrorRequests,
  getMyErrorDetail
REDACTED

export default usageAPI
