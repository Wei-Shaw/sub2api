/**
 * Usage tracking API endpoints
 * Handles usage logs and statistics retrieval
 */

import { apiClient } from './client';
import type {
  UsageLog,
  UsageQueryParams,
  UsageStatsResponse,
  PaginatedResponse,
  TrendDataPoint,
  ModelStat,
} from '@/types';

// ==================== Dashboard Types ====================

export interface UserDashboardStats {
  total_api_keys: number;
  active_api_keys: number;
  total_requests: number;
  total_input_tokens: number;
  total_output_tokens: number;
  total_cache_creation_tokens: number;
  total_cache_read_tokens: number;
  total_tokens: number;
  total_cost: number;         // 标准计费
  total_actual_cost: number;  // 实际扣除
  today_requests: number;
  today_input_tokens: number;
  today_output_tokens: number;
  today_cache_creation_tokens: number;
  today_cache_read_tokens: number;
  today_tokens: number;
  today_cost: number;         // 今日标准计费
  today_actual_cost: number;  // 今日实际扣除
  average_duration_ms: number;
  rpm: number;                // 近5分钟平均每分钟请求数
  tpm: number;                // 近5分钟平均每分钟Token数
}

export interface TrendParams {
  start_date?: string;
  end_date?: string;
  granularity?: 'day' | 'hour';
}

export interface TrendResponse {
  trend: TrendDataPoint[];
  start_date: string;
  end_date: string;
  granularity: string;
}

export interface ModelStatsResponse {
  models: ModelStat[];
  start_date: string;
  end_date: string;
}

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
    page_size: pageSize,
  };

  if (apiKeyId !== undefined) {
    params.api_key_id = apiKeyId;
  }

  const { data } = await apiClient.get<PaginatedResponse<UsageLog>>('/usage', {
    params,
  });
  return data;
}

/**
 * Get usage logs with advanced query parameters
 * @param params - Query parameters for filtering and pagination
 * @returns Paginated list of usage logs
 */
export async function query(params: UsageQueryParams): Promise<PaginatedResponse<UsageLog>> {
  const { data } = await apiClient.get<PaginatedResponse<UsageLog>>('/usage', {
    params,
  });
  return data;
}

/**
 * Get usage statistics for a specific period
 * @param period - Time period ('today', 'week', 'month', 'year')
 * @param apiKeyId - Optional API key ID filter
 * @returns Usage statistics
 */
export async function getStats(
  period: string = 'today',
  apiKeyId?: number
): Promise<UsageStatsResponse> {
  const params: Record<string, unknown> = { period };

  if (apiKeyId !== undefined) {
    params.api_key_id = apiKeyId;
  }

  const { data } = await apiClient.get<UsageStatsResponse>('/usage/stats', {
    params,
  });
  return data;
}

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
    end_date: endDate,
  };

  if (apiKeyId !== undefined) {
    params.api_key_id = apiKeyId;
  }

  const { data } = await apiClient.get<UsageStatsResponse>('/usage/stats', {
    params,
  });
  return data;
}

/**
 * Get usage by date range
 * @param startDate - Start date (ISO format)
 * @param endDate - End date (ISO format)
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
    page_size: 100,
  };

  if (apiKeyId !== undefined) {
    params.api_key_id = apiKeyId;
  }

  const { data } = await apiClient.get<PaginatedResponse<UsageLog>>('/usage', {
    params,
  });
  return data;
}

/**
 * Get detailed usage log by ID
 * @param id - Usage log ID
 * @returns Usage log details
 */
export async function getById(id: number): Promise<UsageLog> {
  const { data } = await apiClient.get<UsageLog>(`/usage/${id}`);
  return data;
}

// ==================== Dashboard API ====================

/**
 * Get user dashboard statistics
 * @returns Dashboard statistics for current user
 */
export async function getDashboardStats(): Promise<UserDashboardStats> {
  const { data } = await apiClient.get<UserDashboardStats>('/usage/dashboard/stats');
  return data;
}

/**
 * Get user usage trend data
 * @param params - Query parameters for filtering
 * @returns Usage trend data for current user
 */
export async function getDashboardTrend(params?: TrendParams): Promise<TrendResponse> {
  const { data } = await apiClient.get<TrendResponse>('/usage/dashboard/trend', { params });
  return data;
}

/**
 * Get user model usage statistics
 * @param params - Query parameters for filtering
 * @returns Model usage statistics for current user
 */
export async function getDashboardModels(params?: { start_date?: string; end_date?: string }): Promise<ModelStatsResponse> {
  const { data } = await apiClient.get<ModelStatsResponse>('/usage/dashboard/models', { params });
  return data;
}

export interface BatchApiKeyUsageStats {
  api_key_id: number;
  today_actual_cost: number;
  total_actual_cost: number;
}

export interface BatchApiKeysUsageResponse {
  stats: Record<string, BatchApiKeyUsageStats>;
}

/**
 * Get batch usage stats for user's own API keys
 * @param apiKeyIds - Array of API key IDs
 * @returns Usage stats map keyed by API key ID
 */
export async function getDashboardApiKeysUsage(apiKeyIds: number[]): Promise<BatchApiKeysUsageResponse> {
  const { data } = await apiClient.post<BatchApiKeysUsageResponse>('/usage/dashboard/api-keys-usage', {
    api_key_ids: apiKeyIds,
  });
  return data;
}

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
  getDashboardApiKeysUsage,
};

export default usageAPI;
