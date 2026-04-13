/** WebSearch emulation mode values (must match backend WebSearchMode* constants in account.go) */
export const WEB_SEARCH_MODE_DEFAULT = 'default' as const
export const WEB_SEARCH_MODE_ENABLED = 'enabled' as const
export const WEB_SEARCH_MODE_DISABLED = 'disabled' as const
export type WebSearchMode = typeof WEB_SEARCH_MODE_DEFAULT | typeof WEB_SEARCH_MODE_ENABLED | typeof WEB_SEARCH_MODE_DISABLED

/** Quota notification threshold type values (must match thresholdType* constants in balance_notify_service.go) */
export const QUOTA_THRESHOLD_TYPE_FIXED = 'fixed' as const
export const QUOTA_THRESHOLD_TYPE_PERCENTAGE = 'percentage' as const
export type QuotaThresholdType = typeof QUOTA_THRESHOLD_TYPE_FIXED | typeof QUOTA_THRESHOLD_TYPE_PERCENTAGE

/** Quota reset mode values */
export const QUOTA_RESET_MODE_ROLLING = 'rolling' as const
export const QUOTA_RESET_MODE_FIXED = 'fixed' as const
export type QuotaResetMode = typeof QUOTA_RESET_MODE_ROLLING | typeof QUOTA_RESET_MODE_FIXED
