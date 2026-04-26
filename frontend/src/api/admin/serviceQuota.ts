import apiClient from '@/api/client'

export interface ServiceQuotaRuleUserRef {
  id: number
  email: string
}

export interface ServiceQuotaLimiterDef {
  id: number
  rule_id: number
  limiter_type: string
  window_mode: string
  limit_value: number
}

export interface ServiceQuotaLimiterInput {
  limiter_type: string
  window_mode: string
  limit_value: number
  // 仅前端使用：v-for stable key，提交前会被 strip 掉，不会发到 backend
  uid?: string
}

export interface ServiceQuotaPathDef {
  id: number
  rule_id: number
  platform?: string | null
  channel_id?: number | null
  group_id?: number | null
  account_id?: number | null
  model_pattern?: string | null
}

export interface ServiceQuotaPathInput {
  platform?: string | null
  channel_id?: number | null
  group_id?: number | null
  account_id?: number | null
  model_pattern?: string | null
  // 仅前端使用：v-for stable key，提交前会被 strip 掉，不会发到 backend
  uid?: string
}

export interface ServiceQuotaRule {
  id: number
  enabled: boolean
  name?: string | null
  counter_mode: string
  is_fallback: boolean
  limiters: ServiceQuotaLimiterDef[]
  paths: ServiceQuotaPathDef[]
  target_user_ids?: number[] | null
  target_users?: ServiceQuotaRuleUserRef[] | null
  created_at: string
  updated_at: string
}

export interface ServiceQuotaRuleInput {
  enabled: boolean
  name?: string | null
  counter_mode: string
  is_fallback: boolean
  limiters: ServiceQuotaLimiterInput[]
  paths: ServiceQuotaPathInput[]
  target_user_ids?: number[] | null
}

export async function listServiceQuotaRules(): Promise<ServiceQuotaRule[]> {
  const { data } = await apiClient.get<{ items: ServiceQuotaRule[] }>('/admin/service-quotas')
  return data.items || []
}

export async function createServiceQuotaRule(input: ServiceQuotaRuleInput): Promise<ServiceQuotaRule> {
  const { data } = await apiClient.post<ServiceQuotaRule>('/admin/service-quotas', input)
  return data
}

export async function updateServiceQuotaRule(id: number, input: ServiceQuotaRuleInput): Promise<ServiceQuotaRule> {
  const { data } = await apiClient.put<ServiceQuotaRule>(`/admin/service-quotas/${id}`, input)
  return data
}

export async function deleteServiceQuotaRule(id: number): Promise<void> {
  await apiClient.delete(`/admin/service-quotas/${id}`)
}

// ==================== 限额监控（Phase B 后端契约） ====================

/** 路径摘要：用于在监控行上显示 limiter 命中的路径维度。null 字段表示该维度不限制；
 *  用户视角下后端会抹空所有字段，故全部 optional */
export interface LimiterRuntimePathSummary {
  platform?: string | null
  channel_id?: number | null
  group_id?: number | null
  account_id?: number | null
  model_pattern?: string | null
}

/**
 * 单个 limiter 的运行时快照。
 * snake_case 与后端 json tag 对齐。
 *
 * per_user_unbound=true 表示：counter_mode=per_user 且 admin 未提供 user 筛选时
 * 后端返回的"占位行"。该行 exists 恒为 false，前端应展示提示而非真实数字与进度条。
 */
export interface LimiterRuntime {
  rule_id: number
  rule_name: string
  path_id: number
  path_index: number
  path_summary?: LimiterRuntimePathSummary | null
  limiter_type: string // rpm/tpm/tpd/daily_usd/concurrency
  window_mode: string // fixed/rolling/none
  limit_value: number
  current: number
  utilization_pct: number
  counter_mode?: string // shared/user/per_user；用户视角下后端会抹空
  scope_user_id?: number | null
  is_fallback: boolean
  exists: boolean
  per_user_unbound?: boolean
}

export interface ServiceQuotaMonitorSnapshot {
  enabled: boolean
  as_of_unix_ms: number
  items: LimiterRuntime[]
  truncated: boolean
}

export interface ServiceQuotaMonitorFilter {
  rule_id?: number
  user_id?: number
  channel_id?: number
  group_id?: number
  account_id?: number
  platform?: string
}

/**
 * 拉取 admin 端服务限额监控快照
 * 自动剔除空字符串/undefined/null 的 filter 字段，避免 ?rule_id= 这种空参
 */
export async function getServiceQuotaMonitorSnapshot(
  filter: ServiceQuotaMonitorFilter
): Promise<ServiceQuotaMonitorSnapshot> {
  const params: Record<string, string | number> = {}
  for (const [k, v] of Object.entries(filter)) {
    if (v === undefined || v === null) continue
    if (typeof v === 'string' && v.trim() === '') continue
    params[k] = v
  }
  const { data } = await apiClient.get<ServiceQuotaMonitorSnapshot>(
    '/admin/service-quotas/monitor',
    { params }
  )
  return data
}
