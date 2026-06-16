/**
 * Minimal type shims for the payment plugin frontend.
 *
 * These types mirror the host frontend's `@/types` index but trimmed to only
 * what the migrated payment views/components reference. Keeping them local
 * avoids depending on host @/ aliases inside the plugin bundle.
 */

export interface BasePaginationResponse<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  pages: number
}

export type GroupPlatform = 'anthropic' | 'openai' | 'gemini' | 'antigravity'

export type SubscriptionType = 'standard' | 'subscription'

export interface OpenAIMessagesDispatchModelConfig {
  opus_mapped_model?: string
  sonnet_mapped_model?: string
  haiku_mapped_model?: string
  exact_model_mappings?: Record<string, string>
}

export interface Group {
  id: number
  name: string
  description: string | null
  platform: GroupPlatform
  rate_multiplier: number
  rpm_limit?: number
  is_exclusive: boolean
  status: 'active' | 'inactive'
  subscription_type: SubscriptionType
  daily_limit_usd: number | null
  weekly_limit_usd: number | null
  monthly_limit_usd: number | null
  image_price_1k: number | null
  image_price_2k: number | null
  image_price_4k: number | null
  claude_code_only: boolean
  fallback_group_id: number | null
  fallback_group_id_on_invalid_request: number | null
  allow_messages_dispatch?: boolean
  default_mapped_model?: string
  messages_dispatch_model_config?: OpenAIMessagesDispatchModelConfig
  require_oauth_only: boolean
  require_privacy_set: boolean
  created_at: string
  updated_at: string
}

export interface AdminGroup extends Group {
  model_routing: Record<string, number[]> | null
  model_routing_enabled: boolean
  mcp_xml_inject: boolean
  supported_model_scopes?: string[]
  account_count?: number
  active_account_count?: number
  rate_limited_account_count?: number
  default_mapped_model?: string
  messages_dispatch_model_config?: OpenAIMessagesDispatchModelConfig
  sort_order: number
}

export interface UserSubscription {
  id: number
  user_id: number
  group_id: number
  status: 'active' | 'expired' | 'revoked'
  daily_usage_usd: number
  weekly_usage_usd: number
  monthly_usage_usd: number
  daily_window_start: string | null
  weekly_window_start: string | null
  monthly_window_start: string | null
  created_at: string
  updated_at: string
  expires_at: string | null
  group?: Group
}
