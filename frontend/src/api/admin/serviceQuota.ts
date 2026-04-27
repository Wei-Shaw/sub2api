import apiClient from '@/api/client'

export interface ServiceQuotaRuleUserRef {
  id: number
  email: string
}

/**
 * TokenComponent 与后端 ServiceQuotaTokenComponent* 常量保持一致。
 *
 * 仅 TPM/TPD 限流器使用此配置；决定哪几种 token 计入限流计数。
 * 默认 = TOKEN_COMPONENTS_DEFAULT（与 Anthropic 3.7+ / Bedrock 3.7+ 口径对齐：
 * input + output + cache_creation，剔除 cache_read）。
 */
export const TokenComponent = {
  Input: 'input',
  Output: 'output',
  CacheCreation: 'cache_creation',
  CacheRead: 'cache_read',
} as const

export type TokenComponent = (typeof TokenComponent)[keyof typeof TokenComponent]

/** 与后端 ServiceQuotaTokenComponentsDefault 一致 */
export const TOKEN_COMPONENTS_DEFAULT: TokenComponent[] = [
  TokenComponent.Input,
  TokenComponent.Output,
  TokenComponent.CacheCreation,
]

/** 全部合法 component；UI 校验 + checkbox 渲染顺序 */
export const TOKEN_COMPONENTS_ALL: TokenComponent[] = [
  TokenComponent.Input,
  TokenComponent.Output,
  TokenComponent.CacheCreation,
  TokenComponent.CacheRead,
]

/** TPM/TPD 限流器才用 token_components 字段；其他类型后端会忽略 */
export function limiterUsesTokenComponents(limiterType: string): boolean {
  return limiterType === 'tpm' || limiterType === 'tpd'
}

export interface ServiceQuotaLimiterDef {
  id: number
  rule_id: number
  limiter_type: string
  window_mode: string
  limit_value: number
  /** 仅 TPM/TPD 返回；其他类型后端置 null/缺省 */
  token_components?: TokenComponent[] | null
  /**
   * 仅 RPM 限流器使用：true 表示请求到达即计入 RPM；false（默认）仅成功请求计入。
   * 其他 limiter 类型后端忽略此字段。
   */
  count_on_arrival?: boolean
}

export interface ServiceQuotaLimiterInput {
  limiter_type: string
  window_mode: string
  limit_value: number
  /** 仅 TPM/TPD 提交；其他类型 strip 掉 */
  token_components?: TokenComponent[]
  /** 仅 RPM 提交；其他类型 strip 掉 */
  count_on_arrival?: boolean
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
 * reset_at_unix_ms 是该计数器/窗口下次"自然清零"的 Unix 毫秒时间戳：
 *   - fixed window：key 真实 PTTL 推算
 *   - rolling window：now + 窗口长度（窗口右端）
 *   - concurrency 或 key 不存在：0
 * 前端展示"X 秒后重置"用：max(0, ceil((reset_at_unix_ms - Date.now())/1000))。
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
  reset_at_unix_ms?: number
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

/**
 * 手动重置某个 limiter 的 Redis 计数器。
 *
 * scopeUserID 可选：shared 计数器（counter_mode=shared 或 per_user 未限定用户）传 null/undefined；
 * 用户独立计数（counter_mode=user 命中或 per_user 限定到某用户）传该 user_id。
 */
export async function resetServiceQuotaCounter(payload: {
  rule_id: number
  path_id: number
  limiter_type: string
  scope_user_id?: number | null
}): Promise<void> {
  await apiClient.post('/admin/service-quotas/reset', payload)
}
