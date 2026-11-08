/**
 * Admin Usage API endpoints
 * Handles admin-level usage logs and statistics retrieval
 */

import { apiClient } from '../client'
import type { AdminUsageLog, UsageQueryParams, PaginatedResponse, UsageRequestType } from '@/types'
import type { EndpointStat } from '@/types'

// ==================== 上游拉黑（A6 侧处置） ====================

/**
 * 在上游 A6 侧拉黑「整个商户」或「某渠道的某个模型」。
 *
 * ⚠️ 有副作用的写操作：拉黑整个商户会让该商户名下所有渠道退出路由，
 * 且**该商户名下的固定绑定不会自动改绑**（会悬空）。调用方必须先让操作者二次确认。
 *
 * 参数由调用方从那一行已有数据里直接取（supplier_id / channel_id / model 都显示在页面上），
 * 后端不再回查，避免为一次处置多开一条读库路径。
 */
export interface UpstreamBlockPayload {
  /** "supplier" = 整个商户；"channel_model" = 某渠道的某个模型。 */
  scope: 'supplier' | 'channel_model'
  supplier_id?: number
  channel_id?: number
  model?: string
}

export async function blockUpstream(payload: UpstreamBlockPayload): Promise<{ scope: string }> {
  const { data } = await apiClient.post<{ data: { scope: string } }>('/admin/usage/upstream-block', payload)
  return data.data
}

/**
 * 读取上游当前黑名单，用来把已拉黑的按钮置灰。
 *
 * 每次页面加载现问上游、本地不缓存：黑名单的权威在上游，本地存一份会在
 * 管理员直接去上游后台拉黑/恢复时与上游漂移 —— 而"以为已经拉黑了"比
 * "显示未拉黑"更危险。
 *
 * 后端在读取失败时返回空集合而非报错（置灰只是锦上添花，不该拖垮主列表），
 * 所以这里不需要额外兜底。
 */
export interface UpstreamBlocks {
  supplier_ids: number[]
  /** "channelID|model" 形式的复合键；上游的渠道级拉黑是按模型生效的。 */
  channel_model_keys: string[]
}

export async function fetchUpstreamBlocks(): Promise<UpstreamBlocks> {
  const { data } = await apiClient.get<{ data: UpstreamBlocks }>('/admin/usage/upstream-blocks')
  return data.data ?? { supplier_ids: [], channel_model_keys: [] }
}

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
  /** 上游真实扣费合计（原币，A6 为美元）。**只含已反查到的**，不做汇率换算。 */
  total_upstream_cost: number
  /** 尚未反查到成本的记录数。>0 表示上面的毛利是偏乐观的下界。 */
  upstream_cost_missing: number
  /** 毛利 = total_actual_cost − total_upstream_cost，由后端算好，前端不重复算。 */
  total_profit: number
  average_duration_ms: number
  endpoints?: EndpointStat[]
  upstream_endpoints?: EndpointStat[]
  endpoint_paths?: EndpointStat[]
}

export interface SimpleUser {
  id: number
  email: string
  deleted: boolean
}

export interface SimpleApiKey {
  id: number
  name: string
  user_id: number
}

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
}

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
}

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
}

export interface AdminUsageQueryParams extends UsageQueryParams {
  user_id?: number
  exact_total?: boolean
  billing_mode?: string
  upstream_model_mismatch?: boolean
  sort_by?: string
  sort_order?: 'asc' | 'desc'
  // 错误请求 tab 专属筛选(仅传给错误列表接口;共用同一 filters 对象)
  error_phase?: string | null
  error_category?: string | null
  status_code?: number | null
}

// ==================== API Functions ====================

/**
 * List all usage logs with optional filters (admin only)
 * @param params - Query parameters for filtering and pagination
 * @returns Paginated list of usage logs
 */
export async function list(
  params: AdminUsageQueryParams,
  options?: { signal?: AbortSignal }
): Promise<PaginatedResponse<AdminUsageLog>> {
  const { data } = await apiClient.get<PaginatedResponse<AdminUsageLog>>('/admin/usage', {
    params,
    signal: options?.signal
  })
  return data
}

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
  native_compaction_v2?: boolean | null
  upstream_model_mismatch?: boolean
  period?: string
  start_date?: string
  end_date?: string
  timezone?: string
  nocache?: number
}): Promise<AdminUsageStatsResponse> {
  const { data } = await apiClient.get<AdminUsageStatsResponse>('/admin/usage/stats', {
    params
  })
  return data
}

/**
 * Search users by email keyword (admin only)
 * @param keyword - Email keyword to search
 * @returns List of matching users (max 30)
 */
export async function searchUsers(keyword: string): Promise<SimpleUser[]> {
  const { data } = await apiClient.get<SimpleUser[]>('/admin/usage/search-users', {
    params: { q: keyword }
  })
  return data
}

/**
 * Search API keys by user ID and/or keyword (admin only)
 * @param userId - Optional user ID to filter by
 * @param keyword - Optional keyword to search in key name
 * @returns List of matching API keys (max 30)
 */
export async function searchApiKeys(userId?: number, keyword?: string): Promise<SimpleApiKey[]> {
  const params: Record<string, unknown> = {}
  if (userId !== undefined) {
    params.user_id = userId
  }
  if (keyword) {
    params.q = keyword
  }
  const { data } = await apiClient.get<SimpleApiKey[]>('/admin/usage/search-api-keys', {
    params
  })
  return data
}

/**
 * List usage cleanup tasks (admin only)
 * @param params - Query parameters for pagination
 * @returns Paginated list of cleanup tasks
 */
export async function listCleanupTasks(
  params: { page?: number; page_size?: number },
  options?: { signal?: AbortSignal }
): Promise<PaginatedResponse<UsageCleanupTask>> {
  const { data } = await apiClient.get<PaginatedResponse<UsageCleanupTask>>('/admin/usage/cleanup-tasks', {
    params,
    signal: options?.signal
  })
  return data
}

/**
 * Create a usage cleanup task (admin only)
 * @param payload - Cleanup task parameters
 * @returns Created cleanup task
 */
export async function createCleanupTask(payload: CreateUsageCleanupTaskRequest): Promise<UsageCleanupTask> {
  const { data } = await apiClient.post<UsageCleanupTask>('/admin/usage/cleanup-tasks', payload)
  return data
}

/**
 * Cancel a usage cleanup task (admin only)
 * @param taskId - Task ID to cancel
 */
export async function cancelCleanupTask(taskId: number): Promise<{ id: number; status: string }> {
  const { data } = await apiClient.post<{ id: number; status: string }>(
    `/admin/usage/cleanup-tasks/${taskId}/cancel`
  )
  return data
}

/**
 * 上游 A6 凭据的脱敏视图。
 *
 * **没有任何字段能承载明文令牌**：接口只回「是否已配置 + 脱敏提示」。
 * 要给这个类型加字段时请先想清楚这一条。
 */
export interface UpstreamCostSettings {
  a6_base_url: string
  a6_user_id: string
  a6_token_configured: boolean
  a6_token_mask: string
  /** 哪些键来自面板覆盖（用表单字段名，前端据此给输入框打「已覆盖」标记）。 */
  override_keys: string[]
}

export interface UpstreamCostSettingsUpdate {
  a6_base_url?: string
  a6_user_id?: string
  a6_access_token?: string
  clear_a6_access_token?: boolean
}

/**
 * 读取上游 A6 配置的生效状态。
 *
 * 这份配置只服务于「按请求 ID 反查上游真实成本」的后台任务；
 * 使用记录页的成本列为空时，答案就在这里。
 */
export async function getUpstreamCostSettings(): Promise<UpstreamCostSettings> {
  const { data } = await apiClient.get<UpstreamCostSettings>('/admin/usage/upstream-cost/settings')
  return data
}

/**
 * 保存上游 A6 配置覆盖值。
 *
 * 未出现在 payload 里的字段保持不动；`a6_access_token` 传空串等于清除覆盖。
 */
export async function updateUpstreamCostSettings(
  payload: UpstreamCostSettingsUpdate
): Promise<UpstreamCostSettings> {
  const { data } = await apiClient.put<UpstreamCostSettings>('/admin/usage/upstream-cost/settings', payload)
  return data
}

/**
 * 盈亏排除名单里的一个成员。
 *
 * email / username 为空**不代表这条记录无效**：用户可能已被删除或改名。
 * 界面据此显示成「用户 #id」而不是把它隐藏掉——隐藏会让面板看到的名单
 * 比后端实际生效的名单短，管理员将无法解释「为什么统计口径和界面对不上」。
 */
export interface ProfitExcludedUser {
  id: number
  email: string
  username: string
}

/**
 * 不计入盈亏的用户名单。
 *
 * 语义：名单里的用户，其**收入**不计入盈亏统计，但其**上游成本**仍然计入。
 * 内部人员的余额由管理员手工调整、并没有真实付款，所以收入是假的；
 * 可他们消耗掉的上游额度是真花钱，成本必须照实算。
 */
export interface UsageProfitExclusion {
  /** 名单本体，升序去重。 */
  user_ids: number[]
  /** user_ids 的展示信息，顺序与 user_ids 一致。 */
  users: ProfitExcludedUser[]
}

/**
 * 读取当前生效的盈亏排除名单。
 *
 * 名单为空 = 没有排除任何人（统计口径与改动前一致）。
 */
export async function getUsageProfitExclusion(): Promise<UsageProfitExclusion> {
  const { data } = await apiClient.get<UsageProfitExclusion>('/admin/usage/profit-exclusion')
  return data
}

/**
 * 覆盖盈亏排除名单（整份替换，不是增量）。
 *
 * 传空数组表示取消所有排除。
 */
export async function updateUsageProfitExclusion(payload: {
  user_ids: number[]
}): Promise<UsageProfitExclusion> {
  const { data } = await apiClient.put<UsageProfitExclusion>('/admin/usage/profit-exclusion', payload)
  return data
}

export const adminUsageAPI = {
  list,
  getStats,
  searchUsers,
  searchApiKeys,
  listCleanupTasks,
  createCleanupTask,
  cancelCleanupTask,
  getUpstreamCostSettings,
  updateUpstreamCostSettings,
  getUsageProfitExclusion,
  updateUsageProfitExclusion
}

export default adminUsageAPI
