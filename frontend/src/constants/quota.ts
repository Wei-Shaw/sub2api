/**
 * 用户每日配额限制 — 前端常量
 *
 * 契约参考：docs/DAILY_QUOTA_CONTRACT.md §5.4 / §0.3
 * 单点定义：UsersView / Settings / Modal / QuotaStatusCard 全部 import 本文件
 */

// ---- 规则周期（首版只支持 daily；后端验证层会拒绝其他取值） ----
export const QUOTA_PERIOD_DAILY = 'daily' as const

// ---- 三态 override 语义 ----
export const QUOTA_OVERRIDE_FOLLOW_GLOBAL = null
export const QUOTA_OVERRIDE_ENABLED = true
export const QUOTA_OVERRIDE_DISABLED = false

// ---- 后端错误码（reason 字段） ----
export const QUOTA_ERR_EXCEEDED = 'USAGE_QUOTA_EXCEEDED'
export const QUOTA_ERR_RULE_OVERLAP = 'QUOTA_RULE_GROUPS_OVERLAP'
export const QUOTA_ERR_RULE_SUBSCRIPTION = 'QUOTA_RULE_GROUP_SUBSCRIPTION'
export const QUOTA_ERR_RULE_GROUP_NOT_FOUND = 'QUOTA_RULE_GROUP_NOT_FOUND'
export const QUOTA_ERR_RULE_NOT_FOUND = 'QUOTA_RULE_NOT_FOUND'
// Admin handler 层的结构化 400（params/JSON binding），对应 quota_handler.go 顶部常量
export const QUOTA_ERR_INVALID_USER_ID = 'QUOTA_INVALID_USER_ID'
export const QUOTA_ERR_INVALID_RULE_ID = 'QUOTA_INVALID_RULE_ID'
export const QUOTA_ERR_INVALID_REQUEST = 'QUOTA_INVALID_REQUEST'

// ---- metadata scope 值 ----
export const QUOTA_SCOPE_TOTAL = 'total'
export const QUOTA_SCOPE_RULE = 'rule'
