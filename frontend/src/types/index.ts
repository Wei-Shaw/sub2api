/**
 * Core Type Definitions for Sub2API Frontend
 */

// ==================== Common Types ====================

export interface SelectOption {
  value: string | number | boolean | null
  label: string
  [key: string]: any // Support extra properties for custom templates
}

export interface BasePaginationResponse<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface FetchOptions {
  signal?: AbortSignal
}

// ==================== Notification Types ====================

/** Notification email entry with enable/disable and verification state.
 *  email="" is a placeholder for the primary email (user's registration email or admin email). */
export interface NotifyEmailEntry {
  email: string
  disabled: boolean
  verified: boolean
}

// ==================== User & Auth Types ====================

export type UserAuthProvider = 'email' | 'linuxdo' | 'oidc' | 'wechat'

export interface UserAuthBindingStatus {
  bound?: boolean
  bound_count?: number
  provider?: UserAuthProvider | string
  provider_key?: string | null
  provider_subject?: string | null
  issuer?: string | null
  label?: string | null
  provider_label?: string | null
  display_name?: string | null
  subject_hint?: string | null
  verified_at?: string | null
  bind_start_path?: string | null
  can_bind?: boolean
  can_unbind?: boolean
  note_key?: string | null
  note?: string | null
  metadata?: Record<string, unknown>
}

export interface UserProfileSourceContext {
  provider?: UserAuthProvider | string
  source?: string | null
  label?: string | null
  provider_label?: string | null
}

export interface User {
  id: number
  username: string
  email: string
  avatar_url?: string | null
  avatar_source?: string | UserProfileSourceContext | null
  username_source?: string | UserProfileSourceContext | null
  display_name_source?: string | UserProfileSourceContext | null
  nickname_source?: string | UserProfileSourceContext | null
  profile_sources?: {
    avatar?: string | UserProfileSourceContext | null
    username?: string | UserProfileSourceContext | null
    display_name?: string | UserProfileSourceContext | null
    nickname?: string | UserProfileSourceContext | null
  }
  auth_bindings?: Partial<Record<UserAuthProvider, boolean | UserAuthBindingStatus>>
  identity_bindings?: Partial<Record<UserAuthProvider, boolean | UserAuthBindingStatus>>
  email_bound?: boolean
  linuxdo_bound?: boolean
  oidc_bound?: boolean
  wechat_bound?: boolean
  role: 'admin' | 'user' // User role for authorization
  balance: number // User balance for API usage
  concurrency: number // Allowed concurrent requests
  rpm_limit?: number // User-level RPM cap (0 = unlimited); effective as fallback when group has no rpm_limit
  status: 'active' | 'disabled' // Account status
  allowed_groups: number[] | null // Allowed group IDs (null = all non-exclusive groups)
  balance_notify_enabled: boolean
  balance_notify_threshold: number | null
  balance_notify_extra_emails: NotifyEmailEntry[]
  subscriptions?: UserSubscription[] // User's active subscriptions
  last_active_at?: string | null
  created_at: string
  updated_at: string
}

export interface AdminUser extends User {
  // 管理员备注（普通用户接口不返回）
  notes: string
  last_used_at?: string | null
  // 用户专属分组倍率配置 (group_id -> rate_multiplier)
  group_rates?: Record<number, number>
  // 当前并发数（仅管理员列表接口返回）
  current_concurrency?: number
}

export interface LoginRequest {
  email: string
  password: string
  turnstile_token?: string
}

export interface RegisterRequest {
  email: string
  password: string
  verify_code?: string
  turnstile_token?: string
  promo_code?: string
  invitation_code?: string
  aff_code?: string
}

export interface AffiliateInvitee {
  user_id: number
  email: string
  username: string
  created_at?: string
  total_rebate: number
}

export interface UserAffiliateDetail {
  user_id: number
  aff_code: string
  inviter_id?: number | null
  aff_count: number
  aff_quota: number
  aff_frozen_quota: number
  aff_history_quota: number
  /** 当前用户作为邀请人时实际生效的返利比例（专属覆盖全局）。0-100。 */
  effective_rebate_rate_percent: number
  invitees: AffiliateInvitee[]
}

export interface AffiliateTransferResponse {
  transferred_quota: number
  balance: number
}

export interface SendVerifyCodeRequest {
  email: string
  turnstile_token?: string
  pending_auth_token?: string
  pending_oauth_token?: string
}

export interface SendVerifyCodeResponse {
  message: string
  countdown: number
}

export interface CustomMenuItem {
  id: string
  label: string
  icon_svg: string
  url: string
  visibility: 'user' | 'admin'
  sort_order: number
}

export interface CustomEndpoint {
  name: string
  endpoint: string
  description: string
}

export interface PublicSettings {
  registration_enabled: boolean
  email_verify_enabled: boolean
  force_email_on_third_party_signup: boolean
  registration_email_suffix_whitelist: string[]
  promo_code_enabled: boolean
  password_reset_enabled: boolean
  invitation_code_enabled: boolean
  turnstile_enabled: boolean
  turnstile_site_key: string
  site_name: string
  site_logo: string
  site_subtitle: string
  api_base_url: string
  contact_info: string
  doc_url: string
  home_content: string
  hide_ccs_import_button: boolean
  payment_enabled: boolean
  table_default_page_size: number
  table_page_size_options: number[]
  custom_menu_items: CustomMenuItem[]
  custom_endpoints: CustomEndpoint[]
  linuxdo_oauth_enabled: boolean
  wechat_oauth_enabled: boolean
  wechat_oauth_open_enabled?: boolean
  wechat_oauth_mp_enabled?: boolean
  wechat_oauth_mobile_enabled?: boolean
  oidc_oauth_enabled: boolean
  oidc_oauth_provider_name: string
  backend_mode_enabled: boolean
  version: string
  balance_low_notify_enabled: boolean
  account_quota_notify_enabled: boolean
  balance_low_notify_threshold: number
  channel_monitor_enabled: boolean
  channel_monitor_default_interval_seconds: number
  available_channels_enabled: boolean
  affiliate_enabled: boolean
}

export interface AuthResponse {
  access_token: string
  refresh_token?: string  // New: Refresh Token for token renewal
  expires_in?: number     // New: Access Token expiry time in seconds
  token_type: string
  user: User & { run_mode?: 'standard' | 'simple' }
}

export interface CurrentUserResponse extends User {
  run_mode?: 'standard' | 'simple'
}

// ==================== Subscription Types ====================

export interface Subscription {
  id: number
  user_id: number
  name: string
  url: string
  type: 'clash' | 'v2ray' | 'surge' | 'quantumult' | 'shadowrocket'
  update_interval: number // in hours
  last_updated: string | null
  node_count: number
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface CreateSubscriptionRequest {
  name: string
  url: string
  type: Subscription['type']
  update_interval?: number
}

export interface UpdateSubscriptionRequest {
  name?: string
  url?: string
  type?: Subscription['type']
  update_interval?: number
  is_active?: boolean
}

// ==================== Announcement Types ====================

export type AnnouncementStatus = 'draft' | 'active' | 'archived'
export type AnnouncementNotifyMode = 'silent' | 'popup'

export type AnnouncementConditionType = 'subscription' | 'balance'

export type AnnouncementOperator = 'in' | 'gt' | 'gte' | 'lt' | 'lte' | 'eq'

export interface AnnouncementCondition {
  type: AnnouncementConditionType
  operator: AnnouncementOperator
  group_ids?: number[]
  value?: number
}

export interface AnnouncementConditionGroup {
  all_of?: AnnouncementCondition[]
}

export interface AnnouncementTargeting {
  any_of?: AnnouncementConditionGroup[]
}

export interface Announcement {
  id: number
  title: string
  content: string
  status: AnnouncementStatus
  notify_mode: AnnouncementNotifyMode
  targeting: AnnouncementTargeting
  starts_at?: string
  ends_at?: string
  created_by?: number
  updated_by?: number
  created_at: string
  updated_at: string
}

export interface UserAnnouncement {
  id: number
  title: string
  content: string
  notify_mode: AnnouncementNotifyMode
  starts_at?: string
  ends_at?: string
  read_at?: string
  created_at: string
  updated_at: string
}

export interface CreateAnnouncementRequest {
  title: string
  content: string
  status?: AnnouncementStatus
  notify_mode?: AnnouncementNotifyMode
  targeting: AnnouncementTargeting
  starts_at?: number
  ends_at?: number
}

export interface UpdateAnnouncementRequest {
  title?: string
  content?: string
  status?: AnnouncementStatus
  notify_mode?: AnnouncementNotifyMode
  targeting?: AnnouncementTargeting
  starts_at?: number
  ends_at?: number
}

export interface AnnouncementUserReadStatus {
  user_id: number
  email: string
  username: string
  balance: number
  eligible: boolean
  read_at?: string
}

// ==================== Proxy Node Types ====================

export interface ProxyNode {
  id: number
  subscription_id: number
  name: string
  type: 'ss' | 'ssr' | 'vmess' | 'vless' | 'trojan' | 'hysteria' | 'hysteria2'
  server: string
  port: number
  config: Record<string, unknown> // JSON configuration specific to proxy type
  latency: number | null // in milliseconds
  last_checked: string | null
  is_available: boolean
  created_at: string
  updated_at: string
}

// ==================== Conversion Types ====================

export interface ConversionRequest {
  subscription_ids: number[]
  target_type: 'clash' | 'v2ray' | 'surge' | 'quantumult' | 'shadowrocket'
  filter?: {
    name_pattern?: string
    types?: ProxyNode['type'][]
    min_latency?: number
    max_latency?: number
    available_only?: boolean
  }
  sort?: {
    by: 'name' | 'latency' | 'type'
    order: 'asc' | 'desc'
  }
}

export interface ConversionResult {
  url: string // URL to download the converted subscription
  expires_at: string
  node_count: number
}

// ==================== Statistics Types ====================

export interface SubscriptionStats {
  subscription_id: number
  total_nodes: number
  available_nodes: number
  avg_latency: number | null
  by_type: Record<ProxyNode['type'], number>
  last_update: string
}

export interface UserStats {
  total_subscriptions: number
  total_nodes: number
  active_subscriptions: number
  total_conversions: number
  last_conversion: string | null
}

// ==================== API Response Types ====================

export interface ApiResponse<T = unknown> {
  code: number
  message: string
  data: T
}

export interface ApiError {
  detail: string
  code?: string
  field?: string
}

export interface PaginatedResponse<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  pages: number
}

// ==================== UI State Types ====================

export type ToastType = 'success' | 'error' | 'info' | 'warning'

export interface Toast {
  id: string
  type: ToastType
  message: string
  title?: string
  duration?: number // in milliseconds, undefined means no auto-dismiss
  startTime?: number // timestamp when toast was created, for progress bar
}

export interface AppState {
  sidebarCollapsed: boolean
  loading: boolean
  toasts: Toast[]
}

// ==================== Validation Types ====================

export interface ValidationError {
  field: string
  message: string
}

// ==================== Table/List Types ====================

export interface SortConfig {
  key: string
  order: 'asc' | 'desc'
}

export interface FilterConfig {
  [key: string]: string | number | boolean | null | undefined
}

export interface PaginationConfig {
  page: number
  page_size: number
}

// ==================== API Key & Group Types ====================

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
  rpm_limit?: number // Group-level RPM cap (0 = unlimited); overrides user-level rpm_limit when set
  is_exclusive: boolean
  status: 'active' | 'inactive'
  subscription_type: SubscriptionType
  daily_limit_usd: number | null
  weekly_limit_usd: number | null
  monthly_limit_usd: number | null
  // 图片生成计费配置（仅 antigravity 平台使用）
  image_price_1k: number | null
  image_price_2k: number | null
  image_price_4k: number | null
  // Claude Code 客户端限制
  claude_code_only: boolean
  fallback_group_id: number | null
  fallback_group_id_on_invalid_request: number | null
  // OpenAI Messages 调度开关（用户侧需要此字段判断是否展示 Claude Code 教程）
  allow_messages_dispatch?: boolean
  default_mapped_model?: string
  messages_dispatch_model_config?: OpenAIMessagesDispatchModelConfig
  require_oauth_only: boolean
  require_privacy_set: boolean
  created_at: string
  updated_at: string
}

export interface AdminGroup extends Group {
  // 模型路由配置（仅管理员可见，内部信息）
  model_routing: Record<string, number[]> | null
  model_routing_enabled: boolean

  // MCP XML 协议注入（仅 antigravity 平台使用）
  mcp_xml_inject: boolean

  // 支持的模型系列（仅 antigravity 平台使用）
  supported_model_scopes?: string[]

  // 分组下账号数量（仅管理员可见）
  account_count?: number
  active_account_count?: number
  rate_limited_account_count?: number

  // OpenAI Messages 调度配置（仅 openai 平台使用）
  default_mapped_model?: string
  messages_dispatch_model_config?: OpenAIMessagesDispatchModelConfig

  // 分组排序
  sort_order: number
}

export interface ApiKey {
  id: number
  user_id: number
  key: string
  name: string
  group_id: number | null
  status: 'active' | 'inactive' | 'quota_exhausted' | 'expired'
  ip_whitelist: string[]
  ip_blacklist: string[]
  last_used_at: string | null
  quota: number // Quota limit in USD (0 = unlimited)
  quota_used: number // Used quota amount in USD
  expires_at: string | null // Expiration time (null = never expires)
  created_at: string
  updated_at: string
  group?: Group
  rate_limit_5h: number
  rate_limit_1d: number
  rate_limit_7d: number
  usage_5h: number
  usage_1d: number
  usage_7d: number
  window_5h_start: string | null
  window_1d_start: string | null
  window_7d_start: string | null
  reset_5h_at: string | null
  reset_1d_at: string | null
  reset_7d_at: string | null
}

export interface CreateApiKeyRequest {
  name: string
  group_id?: number | null
  custom_key?: string // Optional custom API Key
  ip_whitelist?: string[]
  ip_blacklist?: string[]
  quota?: number // Quota limit in USD (0 = unlimited)
  expires_in_days?: number // Days until expiry (null = never expires)
  rate_limit_5h?: number
  rate_limit_1d?: number
  rate_limit_7d?: number
}

export interface UpdateApiKeyRequest {
  name?: string
  group_id?: number | null
  status?: 'active' | 'inactive'
  ip_whitelist?: string[]
  ip_blacklist?: string[]
  quota?: number // Quota limit in USD (null = no change, 0 = unlimited)
  expires_at?: string | null // Expiration time (null = no change)
  reset_quota?: boolean // Reset quota_used to 0
  rate_limit_5h?: number
  rate_limit_1d?: number
  rate_limit_7d?: number
  reset_rate_limit_usage?: boolean
}

export interface CreateGroupRequest {
  name: string
  description?: string | null
  platform?: GroupPlatform
  rate_multiplier?: number
  is_exclusive?: boolean
  subscription_type?: SubscriptionType
  daily_limit_usd?: number | null
  weekly_limit_usd?: number | null
  monthly_limit_usd?: number | null
  image_price_1k?: number | null
  image_price_2k?: number | null
  image_price_4k?: number | null
  claude_code_only?: boolean
  fallback_group_id?: number | null
  fallback_group_id_on_invalid_request?: number | null
  mcp_xml_inject?: boolean
  supported_model_scopes?: string[]
  require_oauth_only?: boolean
  require_privacy_set?: boolean
  // 从指定分组复制账号
  copy_accounts_from_group_ids?: number[]
}

export interface UpdateGroupRequest {
  name?: string
  description?: string | null
  platform?: GroupPlatform
  rate_multiplier?: number
  is_exclusive?: boolean
  status?: 'active' | 'inactive'
  subscription_type?: SubscriptionType
  daily_limit_usd?: number | null
  weekly_limit_usd?: number | null
  monthly_limit_usd?: number | null
  image_price_1k?: number | null
  image_price_2k?: number | null
  image_price_4k?: number | null
  claude_code_only?: boolean
  fallback_group_id?: number | null
  fallback_group_id_on_invalid_request?: number | null
  mcp_xml_inject?: boolean
  supported_model_scopes?: string[]
  require_oauth_only?: boolean
  require_privacy_set?: boolean
  copy_accounts_from_group_ids?: number[]
}

// ==================== Account & Proxy Types ====================

export type AccountPlatform = 'anthropic' | 'openai' | 'gemini' | 'antigravity'
export type AccountType = 'oauth' | 'setup-token' | 'apikey' | 'upstream' | 'bedrock' | 'service_account'
export type OAuthAddMethod = 'oauth' | 'setup-token'
export type ProxyProtocol = 'http' | 'https' | 'socks5' | 'socks5h'

// Claude Model type (returned by /v1/models and account models API)
export interface ClaudeModel {
  id: string
  type: string
  display_name: string
  created_at: string
}

export interface Proxy {
  id: number
  name: string
  protocol: ProxyProtocol
  host: string
  port: number
  username: string | null
  password?: string | null
  status: 'active' | 'inactive'
  account_count?: number // Number of accounts using this proxy
  latency_ms?: number
  latency_status?: 'success' | 'failed'
  latency_message?: string
  ip_address?: string
  country?: string
  country_code?: string
  region?: string
  city?: string
  quality_status?: 'healthy' | 'warn' | 'challenge' | 'failed'
  quality_score?: number
  quality_grade?: string
  quality_summary?: string
  quality_checked?: number
  created_at: string
  updated_at: string
}

export interface ProxyAccountSummary {
  id: number
  name: string
  platform: AccountPlatform
  type: AccountType
  notes?: string | null
}

export interface ProxyQualityCheckItem {
  target: string
  status: 'pass' | 'warn' | 'fail' | 'challenge'
  http_status?: number
  latency_ms?: number
  message?: string
  cf_ray?: string
}

export interface ProxyQualityCheckResult {
  proxy_id: number
  score: number
  grade: string
  summary: string
  exit_ip?: string
  country?: string
  country_code?: string
  base_latency_ms?: number
  passed_count: number
  warn_count: number
  failed_count: number
  challenge_count: number
  checked_at: number
  items: ProxyQualityCheckItem[]
}

// Gemini credentials structure for OAuth and API Key authentication
export interface GeminiCredentials {
  // API Key authentication
  api_key?: string

  // OAuth authentication
  access_token?: string
  refresh_token?: string
  oauth_type?: 'code_assist' | 'google_one' | 'ai_studio' | string
  tier_id?:
    | 'google_one_free'
    | 'google_ai_pro'
    | 'google_ai_ultra'
    | 'gcp_standard'
    | 'gcp_enterprise'
    | 'aistudio_free'
    | 'aistudio_paid'
    | 'LEGACY'
    | 'PRO'
    | 'ULTRA'
    | string
  project_id?: string
  token_type?: string
  scope?: string
  expires_at?: string
  model_mapping?: Record<string, string>
}

export interface TempUnschedulableRule {
  error_code: number
  keywords: string[]
  duration_minutes: number
  description: string
}

export interface TempUnschedulableState {
  until_unix: number
  triggered_at_unix: number
  status_code: number
  matched_keyword: string
  rule_index: number
  error_message: string
}

export interface TempUnschedulableStatus {
  active: boolean
  state?: TempUnschedulableState
}

export interface Account {
  id: number
  name: string
  notes?: string | null
  platform: AccountPlatform
  type: AccountType
  credentials?: Record<string, unknown>
  // Extra fields including Codex usage, OpenAI compact capability, and model-level rate limits.
  extra?: (CodexUsageSnapshot & OpenAICompactState & {
    model_rate_limits?: Record<string, { rate_limited_at: string; rate_limit_reset_at: string }>
    antigravity_credits_overages?: Record<string, { activated_at: string; active_until: string }>
  } & Record<string, unknown>)
  proxy_id: number | null
  concurrency: number
  load_factor?: number | null
  current_concurrency?: number // Real-time concurrency count from Redis
  priority: number
  rate_multiplier?: number // Account billing multiplier (>=0, 0 means free)
  status: 'active' | 'inactive' | 'error'
  error_message: string | null
  last_used_at: string | null
  expires_at: number | null
  auto_pause_on_expired: boolean
  created_at: string
  updated_at: string
  proxy?: Proxy
  group_ids?: number[] // Groups this account belongs to
  groups?: Group[] // Preloaded group objects

  // Rate limit & scheduling fields
  schedulable: boolean
  rate_limited_at: string | null
  rate_limit_reset_at: string | null
  overload_until: string | null
  temp_unschedulable_until: string | null
  temp_unschedulable_reason: string | null

  // Session window fields (5-hour window)
  session_window_start: string | null
  session_window_end: string | null
  session_window_status: 'allowed' | 'allowed_warning' | 'rejected' | null

  // 5h窗口费用控制（仅 Anthropic OAuth/SetupToken 账号有效）
  window_cost_limit?: number | null
  window_cost_sticky_reserve?: number | null

  // 会话数量控制（仅 Anthropic OAuth/SetupToken 账号有效）
  max_sessions?: number | null
  session_idle_timeout_minutes?: number | null

  // RPM 限制（仅 Anthropic OAuth/SetupToken 账号有效）
  base_rpm?: number | null
  rpm_strategy?: string | null
  rpm_sticky_buffer?: number | null
  user_msg_queue_mode?: string | null  // "serialize" | "throttle" | null

  // TLS指纹伪装（仅 Anthropic OAuth/SetupToken 账号有效）
  enable_tls_fingerprint?: boolean | null
  tls_fingerprint_profile_id?: number | null

  // 会话ID伪装（仅 Anthropic OAuth/SetupToken 账号有效）
  // 启用后将在15分钟内固定 metadata.user_id 中的 session ID
  session_id_masking_enabled?: boolean | null

  // 缓存 TTL 强制替换（仅 Anthropic OAuth/SetupToken 账号有效）
  cache_ttl_override_enabled?: boolean | null
  cache_ttl_override_target?: string | null

  // 自定义 Base URL 中继转发（仅 Anthropic OAuth/SetupToken 账号有效）
  custom_base_url_enabled?: boolean | null
  custom_base_url?: string | null

  // API Key 账号配额限制
  quota_limit?: number | null
  quota_used?: number | null
  quota_daily_limit?: number | null
  quota_daily_used?: number | null
  quota_weekly_limit?: number | null
  quota_weekly_used?: number | null

  // 配额固定时间重置配置
  quota_daily_reset_mode?: 'rolling' | 'fixed' | null
  quota_daily_reset_hour?: number | null
  quota_weekly_reset_mode?: 'rolling' | 'fixed' | null
  quota_weekly_reset_day?: number | null
  quota_weekly_reset_hour?: number | null
  quota_reset_timezone?: string | null
  quota_daily_reset_at?: string | null
  quota_weekly_reset_at?: string | null

  // 运行时状态（仅当启用对应限制时返回）
  current_window_cost?: number | null // 当前窗口费用
  active_sessions?: number | null // 当前活跃会话数
  current_rpm?: number | null // 当前分钟 RPM 计数
}

// Account Usage types
export interface WindowStats {
  requests: number
  tokens: number
  cost: number // Account cost (account multiplier)
  standard_cost?: number
  user_cost?: number
}

export interface UsageProgress {
  utilization: number // Percentage (0-100+, 100 = 100%)
  resets_at: string | null
  remaining_seconds: number
  window_stats?: WindowStats | null // 窗口期统计（从窗口开始到当前的使用量）
  used_requests?: number
  limit_requests?: number
}

// Antigravity 单个模型的配额信息
export interface AntigravityModelQuota {
  utilization: number // 使用率 0-100
  reset_time: string  // 重置时间 ISO8601
}

export interface AccountUsageInfo {
  source?: 'passive' | 'active'
  updated_at: string | null
  five_hour: UsageProgress | null
  seven_day: UsageProgress | null
  seven_day_sonnet: UsageProgress | null
  gemini_shared_daily?: UsageProgress | null
  gemini_pro_daily?: UsageProgress | null
  gemini_flash_daily?: UsageProgress | null
  gemini_shared_minute?: UsageProgress | null
  gemini_pro_minute?: UsageProgress | null
  gemini_flash_minute?: UsageProgress | null
  antigravity_quota?: Record<string, AntigravityModelQuota> | null
  ai_credits?: Array<{
    credit_type?: string
    amount?: number
    minimum_balance?: number
  }> | null
  // Antigravity 403 forbidden 状态
  is_forbidden?: boolean
  forbidden_reason?: string
  forbidden_type?: string   // "validation" | "violation" | "forbidden"
  validation_url?: string   // 验证/申诉链接

  // 状态标记（后端自动推导）
  needs_verify?: boolean    // 需要人工验证（forbidden_type=validation）
  is_banned?: boolean       // 账号被封（forbidden_type=violation）
  needs_reauth?: boolean    // token 失效需重新授权（401）

  // 机器可读错误码：forbidden / unauthenticated / rate_limited / network_error
  error_code?: string

  error?: string            // usage 获取失败时的错误信息
}

// OpenAI Codex usage snapshot (from response headers)
export interface CodexUsageSnapshot {
  // Legacy fields (kept for backwards compatibility)
  // NOTE: The naming is ambiguous - actual window type is determined by window_minutes value
  codex_primary_used_percent?: number // Usage percentage (check window_minutes for actual window type)
  codex_primary_reset_after_seconds?: number // Seconds until reset
  codex_primary_window_minutes?: number // Window in minutes
  codex_secondary_used_percent?: number // Usage percentage (check window_minutes for actual window type)
  codex_secondary_reset_after_seconds?: number // Seconds until reset
  codex_secondary_window_minutes?: number // Window in minutes
  codex_primary_over_secondary_percent?: number // Overflow ratio

  // Canonical fields (normalized by backend, use these preferentially)
  codex_5h_used_percent?: number // 5-hour window usage percentage
  codex_5h_reset_after_seconds?: number // Seconds until 5h window reset
  codex_5h_reset_at?: string // 5-hour window absolute reset time (RFC3339)
  codex_5h_window_minutes?: number // 5h window in minutes (should be ~300)
  codex_7d_used_percent?: number // 7-day window usage percentage
  codex_7d_reset_after_seconds?: number // Seconds until 7d window reset
  codex_7d_reset_at?: string // 7-day window absolute reset time (RFC3339)
  codex_7d_window_minutes?: number // 7d window in minutes (should be ~10080)

  codex_usage_updated_at?: string // Last update timestamp
}

export type OpenAICompactMode = 'auto' | 'force_on' | 'force_off'

export interface OpenAICompactState {
  openai_compact_mode?: OpenAICompactMode
  openai_compact_supported?: boolean
  openai_compact_checked_at?: string
  openai_compact_last_status?: number
  openai_compact_last_error?: string
}

export interface CreateAccountRequest {
  name: string
  notes?: string | null
  platform: AccountPlatform
  type: AccountType
  credentials: Record<string, unknown>
  extra?: Record<string, unknown>
  proxy_id?: number | null
  concurrency?: number
  load_factor?: number | null
  priority?: number
  rate_multiplier?: number // Account billing multiplier (>=0, 0 means free)
  group_ids?: number[]
  expires_at?: number | null
  auto_pause_on_expired?: boolean
  confirm_mixed_channel_risk?: boolean
}

export interface UpdateAccountRequest {
  name?: string
  notes?: string | null
  type?: AccountType
  credentials?: Record<string, unknown>
  extra?: Record<string, unknown>
  proxy_id?: number | null
  concurrency?: number
  load_factor?: number | null
  priority?: number
  rate_multiplier?: number // Account billing multiplier (>=0, 0 means free)
  schedulable?: boolean
  status?: 'active' | 'inactive' | 'error'
  group_ids?: number[]
  expires_at?: number | null
  auto_pause_on_expired?: boolean
  confirm_mixed_channel_risk?: boolean
}

export interface CheckMixedChannelRequest {
  platform: AccountPlatform
  group_ids: number[]
  account_id?: number
}

export interface MixedChannelWarningDetails {
  group_id: number
  group_name: string
  current_platform: string
  other_platform: string
}

export interface CheckMixedChannelResponse {
  has_risk: boolean
  error?: string
  message?: string
  details?: MixedChannelWarningDetails
}

export interface CreateProxyRequest {
  name: string
  protocol: ProxyProtocol
  host: string
  port: number
  username?: string | null
  password?: string | null
}

export interface UpdateProxyRequest {
  name?: string
  protocol?: ProxyProtocol
  host?: string
  port?: number
  username?: string | null
  password?: string | null
  status?: 'active' | 'inactive'
}

export interface AdminDataPayload {
  type?: string
  version?: number
  exported_at: string
  proxies: AdminDataProxy[]
  accounts: AdminDataAccount[]
}

export interface AdminDataProxy {
  proxy_key: string
  name: string
  protocol: ProxyProtocol
  host: string
  port: number
  username?: string | null
  password?: string | null
  status: 'active' | 'inactive'
}

export interface AdminDataAccount {
  name: string
  notes?: string | null
  platform: AccountPlatform
  type: AccountType
  credentials: Record<string, unknown>
  extra?: Record<string, unknown>
  proxy_key?: string | null
  concurrency: number
  priority: number
  rate_multiplier?: number | null
  expires_at?: number | null
  auto_pause_on_expired?: boolean
}

export interface AdminDataImportError {
  kind: 'proxy' | 'account'
  name?: string
  proxy_key?: string
  message: string
}

export interface AdminDataImportResult {
  proxy_created: number
  proxy_reused: number
  proxy_failed: number
  account_created: number
  account_failed: number
  errors?: AdminDataImportError[]
}

// ==================== Usage & Redeem Types ====================

export type RedeemCodeType = 'balance' | 'concurrency' | 'subscription' | 'invitation'
export type UsageRequestType = 'unknown' | 'sync' | 'stream' | 'ws_v2'

export interface UsageLog {
  id: number
  user_id: number
  api_key_id: number
  account_id: number | null
  request_id: string
  model: string
  service_tier?: string | null
  reasoning_effort?: string | null
  inbound_endpoint?: string | null
  upstream_endpoint?: string | null

  group_id: number | null
  subscription_id: number | null

  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
  cache_creation_5m_tokens: number
  cache_creation_1h_tokens: number

  input_cost: number
  output_cost: number
  cache_creation_cost: number
  cache_read_cost: number
  total_cost: number
  actual_cost: number
  rate_multiplier: number
  billing_type: number

  request_type?: UsageRequestType
  stream: boolean
  openai_ws_mode?: boolean
  duration_ms: number
  first_token_ms: number | null

  // 图片生成字段
  image_count: number
  image_size: string | null

  // User-Agent
  user_agent: string | null

  // Cache TTL Override
  cache_ttl_overridden: boolean

  // 计费模式
  billing_mode?: string | null

  created_at: string

  user?: User
  api_key?: ApiKey
  group?: Group
  subscription?: UserSubscription
}

export interface UsageLogAccountSummary {
  id: number
  name: string
}

export interface AdminUsageLog extends UsageLog {
  upstream_model?: string | null
  model_mapping_chain?: string | null

  // 账号计费倍率（仅管理员可见）
  account_rate_multiplier?: number | null
  // 自定义定价规则计算的账号统计费用（nil 时使用 total_cost * multiplier）
  account_stats_cost?: number | null

  // 渠道 ID 和计费等级（仅管理员可见）
  channel_id?: number | null
  billing_tier?: string | null

  // 用户请求 IP（仅管理员可见）
  ip_address?: string | null

  // 最小账号信息（仅管理员接口返回）
  account?: UsageLogAccountSummary
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

export interface RedeemCode {
  id: number
  code: string
  type: RedeemCodeType
  value: number
  status: 'active' | 'used' | 'expired' | 'unused'
  used_by: number | null
  used_at: string | null
  created_at: string
  updated_at?: string
  group_id?: number | null // 订阅类型专用
  validity_days?: number // 订阅类型专用
  user?: User
  group?: Group // 关联的分组
}

export interface GenerateRedeemCodesRequest {
  count: number
  type: RedeemCodeType
  value: number
  group_id?: number | null // 订阅类型专用
  validity_days?: number // 订阅类型专用
}

export interface RedeemCodeRequest {
  code: string
}

// ==================== Dashboard & Statistics ====================

export interface DashboardStats {
  // 用户统计
  total_users: number
  today_new_users: number // 今日新增用户数
  active_users: number // 今日有请求的用户数
  hourly_active_users: number // 当前小时活跃用户数（UTC）
  stats_updated_at: string // 统计更新时间（UTC RFC3339）
  stats_stale: boolean // 统计是否过期

  // API Key 统计
  total_api_keys: number
  active_api_keys: number // 状态为 active 的 API Key 数

  // 账户统计
  total_accounts: number
  normal_accounts: number // 正常账户数
  error_accounts: number // 异常账户数
  ratelimit_accounts: number // 限流账户数
  overload_accounts: number // 过载账户数

  // 累计 Token 使用统计
  total_requests: number
  total_input_tokens: number
  total_output_tokens: number
  total_cache_creation_tokens: number
  total_cache_read_tokens: number
  total_tokens: number
  total_cost: number // 累计标准计费
  total_actual_cost: number // 累计实际扣除
  total_account_cost: number // 累计账号成本

  // 今日 Token 使用统计
  today_requests: number
  today_input_tokens: number
  today_output_tokens: number
  today_cache_creation_tokens: number
  today_cache_read_tokens: number
  today_tokens: number
  today_cost: number // 今日标准计费
  today_actual_cost: number // 今日实际扣除
  today_account_cost: number // 今日账号成本

  // 系统运行统计
  average_duration_ms: number // 平均响应时间
  uptime: number // 系统运行时间(秒)

  // 性能指标
  rpm: number // 近5分钟平均每分钟请求数
  tpm: number // 近5分钟平均每分钟Token数
}

export interface UsageStatsResponse {
  period?: string
  total_requests: number
  total_input_tokens: number
  total_output_tokens: number
  total_cache_tokens: number
  total_tokens: number
  total_cost: number // 标准计费
  total_actual_cost: number // 实际扣除
  average_duration_ms: number
  models?: Record<string, number>
}

// ==================== Trend & Chart Types ====================

export interface TrendDataPoint {
  date: string
  requests: number
  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
  total_tokens: number
  cost: number // 标准计费
  actual_cost: number // 实际扣除
}

export interface ModelStat {
  model: string
  requests: number
  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
  total_tokens: number
  cost: number // 标准计费
  actual_cost: number // 实际扣除
  account_cost: number // 账号成本
}

export interface EndpointStat {
  endpoint: string
  requests: number
  total_tokens: number
  cost: number
  actual_cost: number
}

export interface GroupStat {
  group_id: number
  group_name: string
  requests: number
  total_tokens: number
  cost: number // 标准计费
  actual_cost: number // 实际扣除
  account_cost: number // 账号成本
}

export interface UserBreakdownItem {
  user_id: number
  email: string
  requests: number
  total_tokens: number
  cost: number
  actual_cost: number
  account_cost: number
}

export interface UserUsageTrendPoint {
  date: string
  user_id: number
  email: string
  username: string
  requests: number
  tokens: number
  cost: number // 标准计费
  actual_cost: number // 实际扣除
}

export interface UserSpendingRankingItem {
  user_id: number
  email: string
  actual_cost: number
  requests: number
  tokens: number
}

export interface UserSpendingRankingResponse {
  ranking: UserSpendingRankingItem[]
  total_actual_cost: number
  total_requests: number
  total_tokens: number
  start_date: string
  end_date: string
}

export interface ApiKeyUsageTrendPoint {
  date: string
  api_key_id: number
  key_name: string
  requests: number
  tokens: number
}

// ==================== Admin User Management ====================

export interface UpdateUserRequest {
  email?: string
  password?: string
  username?: string
  notes?: string
  role?: 'admin' | 'user'
  balance?: number
  concurrency?: number
  status?: 'active' | 'disabled'
  allowed_groups?: number[] | null
  // 用户专属分组倍率配置 (group_id -> rate_multiplier | null)
  // null 表示删除该分组的专属倍率
  group_rates?: Record<number, number | null>
}

export interface ChangePasswordRequest {
  old_password: string
  new_password: string
}

// ==================== User Subscription Types ====================

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
  user?: User
  group?: Group
}

export interface SubscriptionProgress {
  subscription_id: number
  daily: {
    used: number
    limit: number | null
    percentage: number
    reset_in_seconds: number | null
  } | null
  weekly: {
    used: number
    limit: number | null
    percentage: number
    reset_in_seconds: number | null
  } | null
  monthly: {
    used: number
    limit: number | null
    percentage: number
    reset_in_seconds: number | null
  } | null
  expires_at: string | null
  days_remaining: number | null
}

export interface AssignSubscriptionRequest {
  user_id: number
  group_id: number
  validity_days?: number
}

export interface BulkAssignSubscriptionRequest {
  user_ids: number[]
  group_id: number
  validity_days?: number
}

export interface ExtendSubscriptionRequest {
  days: number
}

// ==================== Query Parameters ====================

export interface UsageQueryParams {
  page?: number
  page_size?: number
  api_key_id?: number
  user_id?: number
  account_id?: number
  group_id?: number
  model?: string
  request_type?: UsageRequestType
  stream?: boolean
  billing_type?: number | null
  start_date?: string
  end_date?: string
  sort_by?: string
  sort_order?: 'asc' | 'desc'
}

// ==================== Account Usage Statistics ====================

export interface AccountUsageHistory {
  date: string
  label: string
  requests: number
  tokens: number
  cost: number
  actual_cost: number // Account cost (account multiplier)
  user_cost: number // User/API key billed cost (group multiplier)
}

export interface AccountUsageSummary {
  days: number
  actual_days_used: number
  total_cost: number // Account cost (account multiplier)
  total_user_cost: number
  total_standard_cost: number
  total_requests: number
  total_tokens: number
  avg_daily_cost: number // Account cost
  avg_daily_user_cost: number
  avg_daily_requests: number
  avg_daily_tokens: number
  avg_duration_ms: number
  today: {
    date: string
    cost: number
    user_cost: number
    requests: number
    tokens: number
  } | null
  highest_cost_day: {
    date: string
    label: string
    cost: number
    user_cost: number
    requests: number
  } | null
  highest_request_day: {
    date: string
    label: string
    requests: number
    cost: number
    user_cost: number
  } | null
}

export interface AccountUsageStatsResponse {
  history: AccountUsageHistory[]
  summary: AccountUsageSummary
  models: ModelStat[]
  endpoints: EndpointStat[]
  upstream_endpoints: EndpointStat[]
}

// ==================== User Attribute Types ====================

export type UserAttributeType = 'text' | 'textarea' | 'number' | 'email' | 'url' | 'date' | 'select' | 'multi_select'

export interface UserAttributeOption {
  value: string
  label: string
  [key: string]: unknown
}

export interface UserAttributeValidation {
  min_length?: number
  max_length?: number
  min?: number
  max?: number
  pattern?: string
  message?: string
}

export interface UserAttributeDefinition {
  id: number
  key: string
  name: string
  description: string
  type: UserAttributeType
  options: UserAttributeOption[]
  required: boolean
  validation: UserAttributeValidation
  placeholder: string
  display_order: number
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface UserAttributeValue {
  id: number
  user_id: number
  attribute_id: number
  value: string
  created_at: string
  updated_at: string
}

export interface CreateUserAttributeRequest {
  key: string
  name: string
  description?: string
  type: UserAttributeType
  options?: UserAttributeOption[]
  required?: boolean
  validation?: UserAttributeValidation
  placeholder?: string
  display_order?: number
  enabled?: boolean
}

export interface UpdateUserAttributeRequest {
  key?: string
  name?: string
  description?: string
  type?: UserAttributeType
  options?: UserAttributeOption[]
  required?: boolean
  validation?: UserAttributeValidation
  placeholder?: string
  display_order?: number
  enabled?: boolean
}

export interface UserAttributeValuesMap {
  [attributeId: number]: string
}

// ==================== Promo Code Types ====================

export interface PromoCode {
  id: number
  code: string
  bonus_amount: number
  max_uses: number
  used_count: number
  status: 'active' | 'disabled'
  expires_at: string | null
  notes: string | null
  created_at: string
  updated_at: string
}

export interface PromoCodeUsage {
  id: number
  promo_code_id: number
  user_id: number
  bonus_amount: number
  used_at: string
  user?: User
}

export interface CreatePromoCodeRequest {
  code?: string
  bonus_amount: number
  max_uses?: number
  expires_at?: number | null
  notes?: string
}

export interface UpdatePromoCodeRequest {
  code?: string
  bonus_amount?: number
  max_uses?: number
  status?: 'active' | 'disabled'
  expires_at?: number | null
  notes?: string
}

// ==================== TOTP (2FA) Types ====================

export interface TotpStatus {
  enabled: boolean
  enabled_at: number | null  // Unix timestamp in seconds
  feature_enabled: boolean
}

export interface TotpSetupRequest {
  email_code?: string
  password?: string
}

export interface TotpSetupResponse {
  secret: string
  qr_code_url: string
  setup_token: string
  countdown: number
}

export interface TotpEnableRequest {
  totp_code: string
  setup_token: string
}

export interface TotpEnableResponse {
  success: boolean
}

export interface TotpDisableRequest {
  email_code?: string
  password?: string
}

export interface TotpVerificationMethod {
  method: 'email' | 'password'
}

export interface TotpLoginResponse {
  requires_2fa: boolean
  temp_token?: string
  user_email_masked?: string
}

export interface TotpLogin2FARequest {
  temp_token: string
  totp_code: string
}

// ==================== Scheduled Test Types ====================

export interface ScheduledTestPlan {
  id: number
  account_id: number
  model_id: string
  cron_expression: string
  enabled: boolean
  max_results: number
  auto_recover: boolean
  last_run_at: string | null
  next_run_at: string | null
  created_at: string
  updated_at: string
}

export interface ScheduledTestResult {
  id: number
  plan_id: number
  status: string
  response_text: string
  error_message: string
  latency_ms: number
  started_at: string
  finished_at: string
  created_at: string
}

export interface CreateScheduledTestPlanRequest {
  account_id: number
  model_id: string
  cron_expression: string
  enabled?: boolean
  max_results?: number
  auto_recover?: boolean
}

export interface UpdateScheduledTestPlanRequest {
  model_id?: string
  cron_expression?: string
  enabled?: boolean
  max_results?: number
  auto_recover?: boolean
}

// Payment types
export type { SubscriptionPlan, PaymentOrder, CheckoutInfoResponse } from './payment'
