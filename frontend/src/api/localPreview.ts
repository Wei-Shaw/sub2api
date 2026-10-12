import type { AxiosAdapter, AxiosRequestConfig, AxiosResponse } from 'axios'

const previewUser = {
  id: 2,
  username: 'preview-user',
  email: 'preview@example.com',
  role: 'user' as const,
  balance: 128.5,
  frozen_balance: 0,
  concurrency: 8,
  status: 'active' as const,
  allowed_groups: null,
  balance_notify_enabled: false,
  balance_notify_threshold: null,
  balance_notify_extra_emails: [],
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z'
}

const adminUser = { ...previewUser, id: 1, username: 'preview-admin', role: 'admin' as const }

const group = {
  id: 1,
  name: 'Claude Pro',
  description: 'Local preview group',
  platform: 'anthropic',
  rate_multiplier: 1,
  rpm_limit: 0,
  is_exclusive: false,
  status: 'active',
  subscription_type: 'standard',
  daily_limit_usd: null,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  long_context_pricing_enabled: true,
  allow_image_generation: false,
  allow_batch_image_generation: false,
  image_rate_independent: false,
  image_rate_multiplier: 1,
  batch_image_discount_multiplier: 1,
  batch_image_hold_multiplier: 1,
  image_price_1k: null,
  image_price_2k: null,
  image_price_4k: null,
  video_rate_independent: false,
  video_rate_multiplier: 1,
  video_price_480p: null,
  video_price_720p: null,
  video_price_1080p: null,
  web_search_price_per_call: null,
  search_price_per_1k: null,
  audio_realtime_price_per_min: null,
  audio_tts_price_per_million_chars: null,
  audio_stt_price_per_hour: null,
  peak_rate_enabled: false,
  peak_start: '00:00',
  peak_end: '00:00',
  peak_rate_multiplier: 1,
  claude_code_only: false,
  fallback_group_id: null,
  fallback_group_id_on_invalid_request: null,
  allow_live: false,
  require_oauth_only: false,
  require_privacy_set: false,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  model_pricing: [],
  profit_control_enabled: false,
  profit_min_margin: 0,
  profit_safety_buffer: 0,
  model_routing: null,
  model_routing_enabled: false,
  mcp_xml_inject: false,
  sort_order: 1,
  account_count: 2,
  active_account_count: 2,
  rate_limited_account_count: 0
}

const apiKey = {
  id: 1,
  user_id: 2,
  key: 'sk-preview-7f3c9b2a',
  name: '本地预览密钥',
  group_id: 1,
  status: 'active',
  ip_whitelist: [],
  ip_blacklist: [],
  last_used_at: '2026-08-30T08:30:00Z',
  last_used_ip: '127.0.0.1',
  quota: 0,
  quota_used: 12.84,
  expires_at: null,
  created_at: '2026-01-05T00:00:00Z',
  updated_at: '2026-08-30T08:30:00Z',
  current_concurrency: 0,
  group,
  rate_limit_5h: 0,
  rate_limit_1d: 0,
  rate_limit_7d: 0,
  usage_5h: 0,
  usage_1d: 0,
  usage_7d: 0,
  window_5h_start: null,
  window_1d_start: null,
  window_7d_start: null,
  reset_5h_at: null,
  reset_1d_at: null,
  reset_7d_at: null
}

const usageLog = {
  id: 1,
  user_id: 2,
  api_key_id: 1,
  account_id: 1,
  request_id: 'preview-request-001',
  model: 'gpt-image-2',
  service_tier: null,
  reasoning_effort: null,
  inbound_endpoint: '/v1/images/generations',
  upstream_endpoint: '/v1/images/generations',
  group_id: 1,
  subscription_id: null,
  input_tokens: 4820,
  output_tokens: 1260,
  cache_creation_tokens: 0,
  cache_read_tokens: 420,
  cache_creation_5m_tokens: 0,
  cache_creation_1h_tokens: 0,
  input_cost: 0.014,
  output_cost: 0.019,
  cache_creation_cost: 0,
  cache_read_cost: 0.001,
  total_cost: 0.034,
  actual_cost: 0.028,
  rate_multiplier: 1,
  long_context_billing_applied: false,
  billing_type: 1,
  request_type: 'image_generation',
  stream: false,
  duration_ms: 832,
  first_token_ms: 214,
  image_count: 1,
  image_size: '1024x1024',
  image_input_size: null,
  image_output_size: '1024x1024',
  image_size_source: 'output',
  image_size_breakdown: { '1024x1024': 1 },
  image_input_tokens: 0,
  image_input_cost: 0,
  image_output_tokens: 0,
  image_output_cost: 0,
  user_agent: 'local-preview',
  ip_address: '127.0.0.1',
  cache_ttl_overridden: false,
  billing_mode: 'image',
  created_at: '2026-08-30T08:30:00Z',
  user: previewUser,
  api_key: apiKey,
  group
}

const previewImageSvg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 640"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop stop-color="#dbeafe"/><stop offset="1" stop-color="#ccfbf1"/></linearGradient></defs><rect width="640" height="640" rx="36" fill="url(#g)"/><circle cx="320" cy="270" r="126" fill="#0f172a" opacity=".9"/><path d="M205 424c48-83 182-83 230 0" fill="none" stroke="#0f172a" stroke-width="30" stroke-linecap="round"/><circle cx="280" cy="246" r="14" fill="#fff"/><circle cx="360" cy="246" r="14" fill="#fff"/><path d="M276 311c28 22 60 22 88 0" fill="none" stroke="#fff" stroke-width="12" stroke-linecap="round"/></svg>`

const previewSubscription = {
  id: 1,
  user_id: 2,
  group_id: 1,
  status: 'active',
  starts_at: '2026-08-01T00:00:00Z',
  daily_usage_usd: 2.1,
  weekly_usage_usd: 8.4,
  monthly_usage_usd: 18.2,
  daily_window_start: '2026-08-30T08:30:00Z',
  weekly_window_start: '2026-08-30T08:30:00Z',
  monthly_window_start: '2026-08-30T08:30:00Z',
  created_at: '2026-08-01T00:00:00Z',
  updated_at: '2026-08-30T08:30:00Z',
  expires_at: '2026-09-01T00:00:00Z',
  user: previewUser,
  group
}

const previewOrders = [
  {
    id: 1001, user_id: 2, amount: 100, pay_amount: 100, currency: 'CNY', fee_rate: 0,
    payment_type: 'alipay', out_trade_no: 'PREVIEW20260830001', status: 'COMPLETED',
    order_type: 'balance', created_at: '2026-08-30T07:20:00Z', expires_at: '2026-08-30T07:35:00Z',
    paid_at: '2026-08-30T07:21:00Z', completed_at: '2026-08-30T07:21:02Z', refund_amount: 0
  },
  {
    id: 1002, user_id: 2, amount: 19.9, pay_amount: 143.28, currency: 'CNY', fee_rate: 0,
    payment_type: 'alipay', out_trade_no: 'PREVIEW20260830002', status: 'PENDING',
    order_type: 'subscription', created_at: '2026-08-30T08:10:00Z', expires_at: '2026-08-30T08:25:00Z',
    refund_amount: 0, plan_id: 1
  }
]

const publicSettings = {
  registration_enabled: true,
  email_verify_enabled: false,
  force_email_on_third_party_signup: false,
  registration_email_suffix_whitelist: [],
  promo_code_enabled: true,
  password_reset_enabled: true,
  invitation_code_enabled: false,
  login_agreement_enabled: false,
  turnstile_enabled: false,
  turnstile_site_key: '',
  site_name: 'BuaGa API',
  site_logo: '',
  site_subtitle: 'AI API Gateway',
  api_base_url: '/v1',
  contact_info: 'support@example.com',
  doc_url: 'https://docs.example.com',
  home_content: '',
  compact_home_enabled: false,
  hide_ccs_import_button: false,
  payment_enabled: true,
  risk_control_enabled: true,
  table_default_page_size: 20,
  table_page_size_options: [10, 20, 50, 100],
  custom_menu_items: [],
  custom_endpoints: [],
  linuxdo_oauth_enabled: false,
  wechat_oauth_enabled: false,
  oidc_oauth_enabled: false,
  oidc_oauth_provider_name: '',
  github_oauth_enabled: false,
  google_oauth_enabled: false,
  backend_mode_enabled: false,
  version: 'v0.1.176-custom',
  balance_low_notify_enabled: false,
  account_quota_notify_enabled: false,
  balance_low_notify_threshold: 10,
  channel_monitor_enabled: true,
  channel_monitor_mode: 'v2',
  channel_monitor_default_interval_seconds: 60,
  available_channels_enabled: true,
  model_plaza_enabled: true,
  model_plaza_require_auth: false,
  plugin_management_enabled: true,
  service_quota_enabled: true,
  affiliate_enabled: true,
  allow_user_view_error_requests: true
}

const now = new Date().toISOString()
const trend = Array.from({ length: 7 }, (_, index) => ({
  date: `2026-08-${String(24 + index).padStart(2, '0')}`,
  requests: 1200 + index * 140,
  input_tokens: 420000 + index * 30000,
  output_tokens: 170000 + index * 18000,
  cache_creation_tokens: 0,
  cache_read_tokens: 40000 + index * 4000,
  total_tokens: 630000 + index * 52000,
  cost: 18.2 + index * 2.1,
  actual_cost: 15.4 + index * 1.7
}))

const dashboardStats = {
  total_users: 1284,
  today_new_users: 18,
  active_users: 382,
  hourly_active_users: 94,
  stats_updated_at: now,
  stats_stale: false,
  total_api_keys: 2670,
  active_api_keys: 2512,
  total_accounts: 42,
  normal_accounts: 38,
  error_accounts: 2,
  ratelimit_accounts: 1,
  overload_accounts: 1,
  total_requests: 812842,
  total_input_tokens: 214000000,
  total_output_tokens: 92000000,
  total_cache_creation_tokens: 0,
  total_cache_read_tokens: 18000000,
  total_tokens: 324000000,
  total_cost: 48210.4,
  total_actual_cost: 42680.2,
  total_account_cost: 33720.5,
  today_requests: 12842,
  today_input_tokens: 6120000,
  today_output_tokens: 2340000,
  today_cache_creation_tokens: 0,
  today_cache_read_tokens: 420000,
  today_tokens: 8460000,
  today_cost: 48.21,
  today_actual_cost: 42.68,
  today_account_cost: 33.72,
  average_duration_ms: 1240,
  uptime: 864000,
  rpm: 148,
  tpm: 98000
}

const monitorMetric = {
  success_requests: 1240, error_requests: 6, request_count: 1246, token_count: 8460000,
  rpm: 148, tpm: 98000, error_rate: 0.48, cache_rate: 12.4,
  cache_rate_numerator: 420000, cache_rate_denominator: 3380000,
  ttft: { sample_count: 1246, p50_ms: 214, p95_ms: 680, avg_ms: 248 },
  duration: { sample_count: 1246, p50_ms: 832, p95_ms: 1860, avg_ms: 940 },
  upstream_affected_requests: 2, upstream_attempt_count: 1252
}
const monitorHealth = {
  overall: 'healthy', error_rate: 'healthy', ttft: 'healthy', cache: 'healthy',
  score: 96, error_rate_score: 98, ttft_score: 94, cache_score: 90, minimum_sample: 20,
  thresholds: { minimum_sample: 20, warning_error_rate: 2, critical_error_rate: 5, target_ttft_ms: 500,
    warning_ttft_ms: 1000, critical_ttft_ms: 3000, warning_cache_rate: 5, critical_cache_rate: 2,
    error_weight: 0.4, ttft_weight: 0.4, cache_weight: 0.2 }
}
const monitorCoverage = {
  requested_start: '2026-08-29T08:30:00Z', requested_end: '2026-08-30T08:30:00Z',
  coverage_start: '2026-08-29T08:30:00Z', data_through: '2026-08-30T08:30:00Z',
  computed_at: '2026-08-30T08:30:00Z', aggregation_lag_seconds: 12, coverage_complete: true, bucket_seconds: 3600
}
const monitorConfig = {
  version: 1, enabled: true, refresh_interval_seconds: 60,
  platforms: [{ platform: 'anthropic', enabled: true, models: ['claude-sonnet-4'] }],
  group_ids: [1],
  health_thresholds: { minimum_sample: 20, warning_error_rate: 2, critical_error_rate: 5,
    target_ttft_ms: 500, warning_ttft_ms: 1000, critical_ttft_ms: 3000, warning_cache_rate: 5,
    critical_cache_rate: 2, error_weight: 0.4, ttft_weight: 0.4, cache_weight: 0.2 },
  ignored_error_categories: []
}

const promptAuditConfig = {
  enabled: true, blocking_enabled: false, blocking_latest_turn_only: true, store_pass_events: true,
  effective_mode: 'async_audit', strategy: 'priority', worker_count: 2, queue_capacity: 128,
  scanners: ['prompt_injection', 'pii'], all_groups: true, group_ids: [], endpoints: [],
  config_version: 3, updated_at: '2026-08-30T08:30:00Z', updated_by: 1, change_summary: 'Local preview'
}
const promptAuditRuntime = {
  process_status: 'running', effective_mode: 'async_audit', expected_config_version: 3,
  active_config_version: 3, config_loaded_at: '2026-08-30T08:30:00Z', worker_total: 2, worker_active: 1,
  worker_heartbeat_at: '2026-08-30T08:30:00Z', queue_capacity: 128,
  queue: { staging: 0, queued: 2, processing: 1, retry: 0, done: 246, failed: 2, active: 3 },
  processed_total: 248, failed_total: 2, enqueued_total: 250, dropped_total: 0,
  last_processed_at: '2026-08-30T08:29:50Z', last_error_code: '', last_error_message: '',
  database_status: 'ok', redis_status: 'ok', endpoints: {},
  guard_metrics: { total: 248, allowed: 220, flagged: 20, blocked: 6, unavailable: 2, invalid: 0,
    timeouts: 1, failovers: 3, bulkhead_full: 0, record_failed: 0, latency_avg_ms: 184,
    latency_p50_ms: 150, latency_p95_ms: 420, latency_p99_ms: 680, latency_max_ms: 920 }
}

function paginated<T>(items: T[], config: AxiosRequestConfig) {
  const params = (config.params || {}) as Record<string, unknown>
  return { items, total: items.length, page: Number(params.page || 1), page_size: Number(params.page_size || 20), pages: 1 }
}

function pathOf(config: AxiosRequestConfig): string {
  const raw = String(config.url || '')
  try {
    return new URL(raw, 'http://local').pathname.replace(/^\/api\/v1/, '') || '/'
  } catch {
    return raw.replace(/^\/api\/v1/, '') || '/'
  }
}

function mockPayload(config: AxiosRequestConfig): unknown {
  const path = pathOf(config)

  if (path === '/admin/system/check-updates') return {
    current_version: '0.2.2', latest_version: '0.2.3', has_update: true,
    cached: false, build_type: 'release',
    release_info: { html_url: 'https://github.com/Wei-Shaw/sub2api/releases/tag/v0.2.3' }
  }

  if (path === '/auth/login') {
    return {
      access_token: 'local-preview-token',
      refresh_token: 'local-preview-refresh-token',
      expires_in: 3600,
      user: previewUser
    }
  }
  if (path === '/auth/me' || path === '/user/profile') return path === '/auth/me' ? previewUser : previewUser
  if (path === '/settings/public') return publicSettings
  if (path === '/keys') return paginated([apiKey], config)
  if (path === '/groups/available') return [group]
  if (path === '/groups/rates') return { 1: 1 }
  if (path === '/channels/available') {
    return [{ name: 'Claude Pro', description: 'Anthropic compatible channel', platforms: [{ platform: 'anthropic', groups: [group], supported_models: [{ name: 'claude-sonnet-4', platform: 'anthropic', pricing: { billing_mode: 'tokens', input_price: 3, output_price: 15, cache_write_price: 3.75, cache_read_price: 0.3, image_input_price: null, image_output_price: null, per_request_price: null, intervals: [] } }] }] }]
  }
  if (path === '/usage/dashboard/stats') return { ...dashboardStats, total_api_keys: 1, active_api_keys: 1, total_requests: 1842, today_requests: 126, today_tokens: 84600, total_tokens: 240000 }
  if (path === '/usage/dashboard/trend') return { trend, start_date: trend[0].date, end_date: trend[6].date, granularity: 'day' }
  if (path === '/usage/dashboard/models') return { models: [{ model: 'claude-sonnet-4', requests: 1260, input_tokens: 420000, output_tokens: 180000, cache_creation_tokens: 0, cache_read_tokens: 42000, total_tokens: 642000, cost: 24.8, actual_cost: 21.2 }], start_date: trend[0].date, end_date: trend[6].date }
  if (path === '/usage/dashboard/snapshot-v2') return { generated_at: now, start_date: trend[0].date, end_date: trend[6].date, granularity: 'day', trend, models: [] }
  if (path === '/usage') return paginated([usageLog], config)
  if (path === '/usage/stats') return { ...dashboardStats, period: 'month', total_cache_tokens: 42000, average_duration_ms: 832 }
  if (path === '/usage/errors') return paginated([], config)
  if (path === '/usage/dashboard/api-keys-usage') return { stats: { '1': { api_key_id: 1, today_actual_cost: 0.12, total_actual_cost: 12.84 } } }
  if (path === '/subscriptions' || path === '/subscriptions/active') return [previewSubscription]
  if (path === '/subscriptions/progress') return [{ subscription_id: 1, daily: { used: 0.42, limit: 2, percentage: 21, reset_in_seconds: 3600 }, weekly: { used: 2.4, limit: 10, percentage: 24, reset_in_seconds: 86400 }, monthly: { used: 8.2, limit: 30, percentage: 27, reset_in_seconds: 604800 }, expires_at: '2026-09-01T00:00:00Z', days_remaining: 2 }]
  if (path === '/subscriptions/summary') return { active_count: 1, subscriptions: [] }
  if (path === '/user/aff') return { user_id: 2, aff_code: 'PREVIEW', inviter_id: null, aff_count: 12, aff_quota: 18.4, aff_frozen_quota: 0, aff_history_quota: 42.8, effective_rebate_rate_percent: 10, invitees: [] }
  if (path === '/user/platform-quotas') return { platform_quotas: [] }
  if (path === '/redeem/history') return []
  if (path === '/payment/checkout-info') return {
    methods: {
      alipay: { currency: 'CNY', display_name: 'Alipay', daily_limit: 10000, daily_used: 0, daily_remaining: 10000, single_min: 1, single_max: 1000, fee_rate: 0, available: true }
    },
    global_min: 1,
    global_max: 1000,
    plans: [{ id: 1, group_id: 1, group_platform: 'anthropic', group_name: group.name, rate_multiplier: 1, name: 'Claude Pro 月卡', description: '本地预览套餐', price: 19.9, original_price: 29.9, validity_days: 30, validity_unit: 'day', features: ['Claude Sonnet 4', '每日额度'], for_sale: true, sort_order: 1 }],
    balance_disabled: false,
    balance_recharge_multiplier: 1,
    subscription_usd_to_cny_rate: 7.2,
    recharge_fee_rate: 0,
    help_text: '支付页面本地预览',
    help_image_url: '',
    stripe_publishable_key: ''
  }
  if (path === '/payment/orders/my') return paginated(previewOrders, config)
  if (path === '/payment/refund-eligible-providers') return []

  if (path === '/admin/settings/admin-api-key') return { exists: false, masked_key: '' }
  if (path === '/admin/settings/overload-cooldown') return { enabled: false, cooldown_minutes: 15 }
  if (path === '/admin/settings/rate-limit-429-cooldown') return { enabled: false, cooldown_seconds: 60 }
  if (path === '/admin/settings/panel-rate-limit') return { enabled: true, user_rpm: 120, heavy_rpm: 30, exempt_admin: true, public_ip_rpm: 60 }
  if (path === '/admin/settings/stream-timeout') return { enabled: false, action: 'none', temp_unsched_minutes: 10, threshold_count: 3, threshold_window_minutes: 10 }
  if (path === '/admin/settings/rectifier') return { enabled: true, thinking_signature_enabled: true, thinking_budget_enabled: true, apikey_signature_enabled: false, apikey_signature_patterns: [] }
  if (path === '/admin/settings/beta-policy') return { rules: [] }
  if (path === '/admin/settings/web-search-emulation') return { enabled: false, providers: [] }
  if (path === '/admin/settings/email-templates') return { events: [], locales: ['zh-CN', 'en-US'], templates: [], placeholders: [] }
  if (path === '/admin/accounts/upstream-billing-probe/settings') return { enabled: false, interval_minutes: 60 }
  if (path === '/admin/accounts/ollama-cloud-usage/settings') return { enabled: false, interval_minutes: 30, debounce_minutes: 5 }
  if (path === '/admin/payment/providers') return []
  if (path === '/admin/payment/config') return {
    enabled: true, min_amount: 1, max_amount: 1000, daily_limit: 10000, order_timeout_minutes: 15,
    max_pending_orders: 3, enabled_payment_types: ['alipay'], balance_disabled: false,
    balance_recharge_multiplier: 1, subscription_usd_to_cny_rate: 7.2, recharge_fee_rate: 0,
    load_balance_strategy: 'round-robin', product_name_prefix: 'BuaGa API', product_name_suffix: '',
    help_image_url: '', help_text: ''
  }
  if (path === '/admin/payment/orders') return paginated(previewOrders.map(order => ({ ...order, user: previewUser })), config)
  if (path === '/channel-monitor-v2/dimensions' || path === '/admin/channel-monitor-v2/dimensions') {
    return { platforms: [{ value: 'anthropic', label: 'Anthropic', request_count: 1246 }],
      groups: [{ id: 1, name: group.name, platform: 'anthropic', request_count: 1246 }],
      models: [{ value: 'claude-sonnet-4', label: 'claude-sonnet-4', platform: 'anthropic', request_count: 1246 }] }
  }
  if (path === '/channel-monitor-v2/snapshot' || path === '/admin/channel-monitor-v2/snapshot') {
    return { config: monitorConfig, coverage: monitorCoverage, metrics: monitorMetric, health: monitorHealth, trend: [] }
  }
  if (path === '/channel-monitor-v2/matrix' || path === '/admin/channel-monitor-v2/matrix') {
    return { coverage: monitorCoverage, group_by: 'platform_model', items: [{ platform: 'anthropic', group_id: 1,
      group_name: group.name, model: 'claude-sonnet-4', metrics: monitorMetric, health: monitorHealth, buckets: [] }] }
  }
  if (path === '/channel-monitor-v2/models' || path === '/admin/channel-monitor-v2/models') {
    return { coverage: monitorCoverage, items: [{ platform: 'anthropic', model: 'claude-sonnet-4', metrics: monitorMetric, health: monitorHealth }] }
  }
  if (path === '/channel-monitor-v2/errors' || path === '/admin/channel-monitor-v2/errors') return { coverage: monitorCoverage, items: [] }
  if (path === '/channel-monitor-v2/users' || path === '/admin/channel-monitor-v2/users') return { coverage: monitorCoverage, items: [] }
  if (path === '/admin/channel-monitor-v2/config') return monitorConfig
  if (path === '/admin/prompt-audit/config') return promptAuditConfig
  if (path === '/admin/prompt-audit/runtime') return promptAuditRuntime
  if (path === '/admin/prompt-audit/events') return paginated([], config)
  if (path === '/admin/affiliates/users') return paginated([], config)
  if (path === '/admin/backups/s3-config') return { endpoint: '', region: 'auto', bucket: '', access_key_id: '', prefix: 'backups/', force_path_style: false }
  if (path === '/admin/backups/image-storage') return {
    config: { enabled: false, reuse_backup_s3: true, bucket: '', prefix: 'images/', public_base_url: '',
      presign_expiry_hours: 24, max_download_bytes: 104857600, endpoint: '', region: 'auto',
      access_key_id: '', force_path_style: false }, secret_configured: false
  }
  if (path === '/admin/backups/schedule') return { enabled: false, cron_expr: '0 2 * * *', retain_days: 30, retain_count: 10 }
  if (path === '/admin/backups') return { items: [] }
  if (path === '/admin/dashboard/stats' || path === '/admin/dashboard/snapshot-v2') return path.endsWith('stats') ? dashboardStats : { generated_at: now, start_date: trend[0].date, end_date: trend[6].date, granularity: 'day', stats: dashboardStats, trend, models: [], groups: [], users_trend: [] }
  if (path.startsWith('/admin/dashboard/')) {
    if (path.endsWith('/trend')) return { trend, start_date: trend[0].date, end_date: trend[6].date, granularity: 'day' }
    if (path.endsWith('/models')) return { models: [], start_date: trend[0].date, end_date: trend[6].date }
    if (path.endsWith('/groups')) return { groups: [{ group_id: 1, group_name: group.name, requests: 12842, total_tokens: 8460000, cost: 48.21, actual_cost: 42.68, account_cost: 33.72 }], start_date: trend[0].date, end_date: trend[6].date }
    if (path.endsWith('/users-ranking')) return { ranking: [], total_actual_cost: 0, total_requests: 0, total_tokens: 0, start_date: trend[0].date, end_date: trend[6].date }
    if (path.endsWith('/users-trend')) return { trend: [] }
    if (path.endsWith('/realtime')) return { active_requests: 2, requests_per_minute: 148, average_response_time: 1.24, error_rate: 0.2 }
    if (path.endsWith('/user-breakdown')) return { users: [], start_date: trend[0].date, end_date: trend[6].date }
    if (path.endsWith('/api-keys-trend')) return { trend: [], start_date: trend[0].date, end_date: trend[6].date, granularity: 'day' }
  }

  if (path === '/admin/users' || path.startsWith('/admin/users?')) return paginated([{ ...adminUser, notes: 'Local preview administrator', group_rates: {}, restrict_public_groups: false, current_concurrency: 0 }], config)
  if (path === '/admin/user-attributes') return []
  if (path === '/admin/user-attributes/batch') return { attributes: {} }
  if (path === '/admin/groups') return paginated([group], config)
  if (path === '/admin/groups/all') return [group]
  if (path === '/admin/groups/usage-summary' || path === '/admin/groups/capacity-summary') return []
  if (path.startsWith('/admin/groups/')) {
    if (path.endsWith('/live-capability')) return { supported: true }
    if (path.endsWith('/models-list-candidates')) return { models: ['claude-sonnet-4', 'claude-opus-4'] }
    if (path.endsWith('/composite-routes')) return []
    if (path.endsWith('/api-keys')) return paginated([apiKey], config)
    return group
  }
  if (path === '/admin/accounts' || path === '/admin/accounts/upstream-billing-rates') return path.endsWith('rates') ? { rates: [] } : paginated([{ id: 1, name: 'Claude Pro account', notes: 'Local preview account', platform: 'anthropic', type: 'oauth', credentials_status: {}, proxy_id: null, concurrency: 4, priority: 1, rate_multiplier: 1, status: 'active', error_message: null, last_used_at: now, expires_at: null, auto_pause_on_expired: false, schedulable: true, rate_limited_at: null, rate_limit_reset_at: null, overload_until: null, temp_unschedulable_until: null, temp_unschedulable_reason: null, session_window_start: null, session_window_end: null, session_window_status: 'allowed', created_at: '2026-01-01T00:00:00Z', updated_at: now }], config)
  if (path === '/admin/proxies' || path === '/admin/proxies/all') return path.endsWith('/all') ? [] : paginated([], config)
  if (path === '/admin/subscriptions') return paginated([previewSubscription], config)
  if (path === '/admin/usage') return paginated([{ ...usageLog, image_previews: ['/api/v1/admin/usage/1/images/0'] }], config)
  if (path === '/admin/usage/stats') return { ...dashboardStats, total_cache_tokens: 42000 }
  if (path === '/admin/announcements') return paginated([{ id: 1, title: '本地预览公告', content: '这是本地页面预览数据。', status: 'published', notify_mode: 'banner', targeting: {}, created_at: now, updated_at: now }], config)
  if (path === '/admin/redeem-codes') return paginated([{ id: 1, code: 'PREVIEW-2026', type: 'balance', value: 10, status: 'unused', used_by: null, used_at: null, created_at: now, expires_at: null, notes: 'Local preview' }], config)
  if (path === '/admin/promo-codes') return paginated([{ id: 1, code: 'PREVIEW10', bonus_amount: 10, max_uses: 100, used_count: 4, status: 'active', expires_at: null, notes: 'Local preview', created_at: now, updated_at: now }], config)
  if (path === '/admin/audit-logs') return paginated([], config)
  if (path === '/admin/plugins') return []
  if (path === '/admin/settings') return {
    ...publicSettings,
    default_balance: 10,
    default_concurrency: 5,
    default_user_rpm_limit: 0,
    default_subscriptions: [],
    frontend_url: 'http://127.0.0.1:4179',
    totp_enabled: false,
    totp_encryption_key_configured: false,
    passkey_enabled: false,
    passkey_configured: false,
    passkey_rp_id: 'localhost',
    passkey_rp_origins: [],
    session_binding_enabled: false,
    step_up_enabled: false,
    audit_log_retention_days: 30,
    smtp_host: '', smtp_port: 587, smtp_username: '', smtp_password_configured: false,
    smtp_from_email: '', smtp_from_name: 'BuaGa API', smtp_use_tls: true,
    turnstile_secret_key_configured: false,
    tencent_captcha_app_id: '', tencent_captcha_enabled: false,
    aliyun_captcha_enabled: false, aliyun_captcha_access_key_id: '', aliyun_captcha_access_key_secret_configured: false,
    aliyun_captcha_scene_id: '', aliyun_captcha_prefix: '', aliyun_captcha_region: 'cn-hangzhou',
    login_agreement_documents: []
  }
  if (path === '/admin/risk-control/config') return {
    enabled: true, mode: 'observe', base_url: 'https://moderation.local', model: 'qwen3guard', proxy_id: null,
    api_key_configured: true, api_key_masked: 'sk-pr...view', api_key_count: 1, api_key_masks: ['sk-pr...view'],
    api_key_statuses: [{ index: 0, key_hash: 'preview', masked: 'sk-pr...view', status: 'ok', failure_count: 0, success_count: 42, last_error: '', last_latency_ms: 184, last_http_status: 200, last_tested: true, configured: true }],
    timeout_ms: 3000, sample_rate: 1, all_groups: true, group_ids: [], record_non_hits: true,
    thresholds: { sexual: 0.8, violence: 0.8, hate: 0.8 }, worker_count: 4, queue_size: 128, block_status: 403,
    block_message: 'Request blocked by content moderation.', email_on_hit: false, auto_ban_enabled: false, ban_threshold: 10,
    violation_window_hours: 24, retry_count: 1, hit_retention_days: 30, non_hit_retention_days: 3,
    pre_hash_check_enabled: true, blocked_keywords: [], keyword_blocking_mode: 'keyword_only', model_filter: { type: 'all', models: [] }, cyber_policy_exclude_from_ban_count: false
  }
  if (path === '/admin/risk-control/status') return {
    enabled: true, risk_control_enabled: true, mode: 'observe', worker_count: 4, max_workers: 4, active_workers: 1, idle_workers: 3,
    queue_size: 128, queue_length: 2, queue_usage_percent: 1.6, enqueued: 248, dropped: 0, processed: 246, errors: 2,
    pre_block_active: 0, pre_block_checked: 0, pre_block_allowed: 0, pre_block_blocked: 0, pre_block_errors: 0,
    pre_block_avg_latency_ms: 0, pre_block_api_key_active: 1, pre_block_api_key_available_count: 1, pre_block_api_key_total_calls: 42,
    pre_block_api_key_loads: [], api_key_statuses: [], flagged_hash_count: 0, last_cleanup_at: now,
    last_cleanup_deleted_hit: 0, last_cleanup_deleted_non_hit: 0
  }
  if (path === '/admin/risk-control/logs') return paginated([], config)
  if (path.startsWith('/admin/settings/') || path.startsWith('/admin/ops/') || path.startsWith('/admin/channel-monitor/')) return {}

  if (path.endsWith('/totp/status')) return { enabled: false, enabled_at: null, feature_enabled: false }
  if (path.endsWith('/refund-eligible-providers')) return []

  if (path.endsWith('/all') || path.endsWith('/available') || path.endsWith('/models')) return []
  if (path.includes('/search-')) return []
  if (path.includes('/cleanup-tasks')) return paginated([], config)
  if (path.includes('/data')) return { items: [], total: 0 }
  if (path.includes('/config') || path.includes('/settings') || path.includes('/state')) return {}
  if (path.includes('/progress')) return {}
  if (path.includes('/usage')) return paginated([], config)
  return {}
}

export const localPreviewAdapter: AxiosAdapter = async (config) => {
  const path = pathOf(config)
  if (config.responseType === 'blob' && /^\/admin\/usage\/\d+\/images\/\d+$/.test(path)) {
    return {
      data: new Blob([previewImageSvg], { type: 'image/svg+xml' }),
      status: 200,
      statusText: 'OK',
      headers: { 'content-type': 'image/svg+xml' },
      config,
      request: undefined
    }
  }
  const response: AxiosResponse = {
    data: { code: 0, message: 'local preview', data: mockPayload(config) },
    status: 200,
    statusText: 'OK',
    headers: { etag: 'local-preview' },
    config,
    request: undefined
  }
  return response
}
