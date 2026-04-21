/**
 * 用户每日配额限制 — 类型定义
 *
 * 契约参考：docs/DAILY_QUOTA_CONTRACT.md §5.2
 * 所有字段与后端 DTO 一一对应，不新增前端独有偏差字段
 */

/** 规则周期（首版只支持 daily；后端验证层会拒绝其他取值） */
export type QuotaPeriod = 'daily'

/** 单条规则（对应 DB `user_usage_limit_rules` 行） */
export interface QuotaRule {
  id: number
  user_id: number
  group_ids: number[]
  daily_limit_usd: number
  period: QuotaPeriod
  created_at: string
  updated_at: string
}

/** 今日已用量快照 */
export interface QuotaUsageSnapshot {
  total_used_usd: number
  /** key 为 ruleID 字符串（JSON 键统一字符串化） */
  rules_used: Record<string, number>
  /** RFC3339 下一次重置时间（次日零点，配置时区） */
  reset_at: string
}

/** 合并全局默认 + 用户覆盖 + rules 后的生效配额 */
export interface ResolvedQuota {
  user_id: number
  enabled: boolean
  /** null 表示"不限"；正数才检查 */
  daily_limit: number | null
  rules: QuotaRule[]
  resolved_at: string
}

/** Admin GET /users/:id/quota 完整视图 */
export interface UserQuotaView {
  user_override: {
    usage_limit_enabled: boolean | null
    daily_usage_limit_usd: number | null
  }
  resolved: ResolvedQuota
  today_usage: QuotaUsageSnapshot
}

/**
 * Admin PUT /users/:id/quota 请求
 *
 * 三态语义：
 *   - `undefined`（字段不提供）：不改该字段
 *   - `null`：清空回"跟随全局/不限"
 *   - `true`/`false` / 数字：写入该值
 */
export interface UpdateUserQuotaRequest {
  usage_limit_enabled?: boolean | null
  daily_usage_limit_usd?: number | null
}

export interface CreateRuleRequest {
  group_ids: number[]
  daily_limit_usd: number
  period?: 'daily'
}

export interface UpdateRuleRequest {
  group_ids?: number[]
  daily_limit_usd?: number
}

/**
 * 批量替换规则的单项输入
 *
 * 用于 PUT /api/v1/admin/users/:id/quota/rules 批量替换接口
 * 后端全量覆盖（单事务幂等）：未出现的历史规则会被删除，出现的规则会被新建或更新
 */
export interface ReplaceRuleInput {
  group_ids: number[]
  daily_limit_usd: number
  period?: 'daily'
}

/**
 * 用户侧简化视图（只读）
 *
 * 注意：当后端 quotaService 未装配（feature flag 关闭或 wire 未注入）时，
 * `resolved` 和 `today_usage` 可能为 null。前端组件必须做防御性判断。
 */
export interface UserQuotaStatus {
  resolved: ResolvedQuota | null
  today_usage: QuotaUsageSnapshot | null
}
