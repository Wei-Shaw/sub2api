/**
 * Admin Ops API endpoints (vNext)
 * - Error logs list/detail + retry (client/upstream)
 * - Dashboard overview (raw path)
 */

import { apiClient } from '../client'
import type { PaginatedResponse } from '@/types'

export type OpsRetryMode = 'client' | 'upstream'
export type OpsQueryMode = 'auto' | 'raw' | 'preagg'

export interface OpsRequestOptions {
  signal?: AbortSignal
}

export interface OpsRetryRequest {
  mode: OpsRetryMode
  pinned_account_id?: number
  force?: boolean
}

export interface OpsRetryAttempt {
  id: number
  created_at: string
  requested_by_user_id: number
  source_error_id: number
  mode: string
  pinned_account_id?: number | null
  pinned_account_name?: string

  status: string
  started_at?: string | null
  finished_at?: string | null
  duration_ms?: number | null

  success?: boolean | null
  http_status_code?: number | null
  upstream_request_id?: string | null
  used_account_id?: number | null
  used_account_name?: string
  response_preview?: string | null
  response_truncated?: boolean | null

  result_request_id?: string | null
  result_error_id?: number | null
  error_message?: string | null
}

export type OpsUpstreamErrorEvent = {
  at_unix_ms?: number
  platform?: string
  account_id?: number
  account_name?: string
  upstream_status_code?: number
  upstream_request_id?: string
  upstream_request_body?: string
  kind?: string
  message?: string
  detail?: string
}

export interface OpsRetryResult {
  attempt_id: number
  mode: OpsRetryMode
  status: 'running' | 'succeeded' | 'failed' | string

  pinned_account_id?: number | null
  used_account_id?: number | null

  http_status_code: number
  upstream_request_id: string

  response_preview: string
  response_truncated: boolean

  error_message: string

  started_at: string
  finished_at: string
  duration_ms: number
}

export interface OpsDashboardOverview {
  start_time: string
  end_time: string
  platform: string
  group_id?: number | null

  health_score?: number

  system_metrics?: OpsSystemMetricsSnapshot | null
  job_heartbeats?: OpsJobHeartbeat[] | null

  success_count: number
  error_count_total: number
  business_limited_count: number
  error_count_sla: number
  request_count_total: number
  request_count_sla: number

  token_consumed: number

  sla: number
  error_rate: number
  upstream_error_rate: number
  upstream_error_count_excl_429_529: number
  upstream_429_count: number
  upstream_529_count: number

  qps: {
    current: number
    peak: number
    avg: number
  }
  tps: {
    current: number
    peak: number
    avg: number
  }

  duration: OpsPercentiles
  ttft: OpsPercentiles
}

export interface OpsPercentiles {
  p50_ms?: number | null
  p90_ms?: number | null
  p95_ms?: number | null
  p99_ms?: number | null
  avg_ms?: number | null
  max_ms?: number | null
}

export interface OpsThroughputTrendPoint {
  bucket_start: string
  request_count: number
  token_consumed: number
  switch_count?: number
  qps: number
  tps: number
}

export interface OpsThroughputPlatformBreakdownItem {
  platform: string
  request_count: number
  token_consumed: number
}

export interface OpsThroughputGroupBreakdownItem {
  group_id: number
  group_name: string
  request_count: number
  token_consumed: number
}

export interface OpsThroughputTrendResponse {
  bucket: string
  points: OpsThroughputTrendPoint[]
  by_platform?: OpsThroughputPlatformBreakdownItem[]
  top_groups?: OpsThroughputGroupBreakdownItem[]
}

export type OpsRequestKind = 'success' | 'error'
export type OpsRequestDetailsKind = OpsRequestKind | 'all'
export type OpsRequestDetailsSort = 'created_at_desc' | 'duration_desc'

export interface OpsRequestDetail {
  kind: OpsRequestKind
  created_at: string
  request_id: string

  platform?: string
  model?: string
  duration_ms?: number | null
  status_code?: number | null

  error_id?: number | null
  phase?: string
  severity?: string
  message?: string

  user_id?: number | null
  api_key_id?: number | null
  account_id?: number | null
  group_id?: number | null

  stream?: boolean
}

export interface OpsRequestDetailsParams {
  time_range?: '5m' | '30m' | '1h' | '6h' | '24h'
  start_time?: string
  end_time?: string

  kind?: OpsRequestDetailsKind

  platform?: string
  group_id?: number | null

  user_id?: number
  api_key_id?: number
  account_id?: number

  model?: string
  request_id?: string
  q?: string

  min_duration_ms?: number
  max_duration_ms?: number

  sort?: OpsRequestDetailsSort

  page?: number
  page_size?: number
}

export type OpsRequestDetailsResponse = PaginatedResponse<OpsRequestDetail>

export interface OpsLatencyHistogramBucket {
  range: string
  count: number
}

export interface OpsLatencyHistogramResponse {
  start_time: string
  end_time: string
  platform: string
  group_id?: number | null

  total_requests: number
  buckets: OpsLatencyHistogramBucket[]
}

export interface OpsErrorTrendPoint {
  bucket_start: string
  error_count_total: number
  business_limited_count: number
  error_count_sla: number
  upstream_error_count_excl_429_529: number
  upstream_429_count: number
  upstream_529_count: number
}

export interface OpsErrorTrendResponse {
  bucket: string
  points: OpsErrorTrendPoint[]
}

export interface OpsErrorDistributionItem {
  status_code: number
  total: number
  sla: number
  business_limited: number
}

export interface OpsErrorDistributionResponse {
  total: number
  items: OpsErrorDistributionItem[]
}

export interface OpsDashboardSnapshotV2Response {
  generated_at: string
  overview: OpsDashboardOverview
  throughput_trend: OpsThroughputTrendResponse
  error_trend: OpsErrorTrendResponse
}

export type OpsOpenAITokenStatsTimeRange = '30m' | '1h' | '1d' | '15d' | '30d'

export interface OpsOpenAITokenStatsItem {
  model: string
  request_count: number
  avg_tokens_per_sec?: number | null
  avg_first_token_ms?: number | null
  total_output_tokens: number
  avg_duration_ms: number
  requests_with_first_token: number
}

export interface OpsOpenAITokenStatsResponse {
  time_range: OpsOpenAITokenStatsTimeRange
  start_time: string
  end_time: string
  platform?: string
  group_id?: number | null
  items: OpsOpenAITokenStatsItem[]
  total: number
  page?: number
  page_size?: number
  top_n?: number | null
}

export interface OpsOpenAITokenStatsParams {
  time_range?: OpsOpenAITokenStatsTimeRange
  platform?: string
  group_id?: number | null
  page?: number
  page_size?: number
  top_n?: number
}

export interface OpsSystemMetricsSnapshot {
  id: number
  created_at: string
  window_minutes: number

  cpu_usage_percent?: number | null
  memory_used_mb?: number | null
  memory_total_mb?: number | null
  memory_usage_percent?: number | null

  db_ok?: boolean | null
  redis_ok?: boolean | null

  // Config-derived limits (best-effort) for rendering "current vs max".
  db_max_open_conns?: number | null
  redis_pool_size?: number | null

  redis_conn_total?: number | null
  redis_conn_idle?: number | null

  db_conn_active?: number | null
  db_conn_idle?: number | null
  db_conn_waiting?: number | null

  goroutine_count?: number | null
  concurrency_queue_depth?: number | null
  account_switch_count?: number | null
}

export interface OpsJobHeartbeat {
  job_name: string
  last_run_at?: string | null
  last_success_at?: string | null
  last_error_at?: string | null
  last_error?: string | null
  last_duration_ms?: number | null
  last_result?: string | null
  updated_at: string
}

export interface PlatformConcurrencyInfo {
  platform: string
  current_in_use: number
  max_capacity: number
  load_percentage: number
  waiting_in_queue: number
}

export interface GroupConcurrencyInfo {
  group_id: number
  group_name: string
  platform: string
  current_in_use: number
  max_capacity: number
  load_percentage: number
  waiting_in_queue: number
}

export interface AccountConcurrencyInfo {
  account_id: number
  account_name?: string
  platform: string
  group_id: number
  group_name: string
  current_in_use: number
  max_capacity: number
  load_percentage: number
  waiting_in_queue: number
}

export interface OpsConcurrencyStatsResponse {
  enabled: boolean
  platform: Record<string, PlatformConcurrencyInfo>
  group: Record<string, GroupConcurrencyInfo>
  account: Record<string, AccountConcurrencyInfo>
  timestamp?: string
}

export interface UserConcurrencyInfo {
  user_id: number
  user_email: string
  username: string
  current_in_use: number
  max_capacity: number
  load_percentage: number
  waiting_in_queue: number
}

export interface OpsUserConcurrencyStatsResponse {
  enabled: boolean
  user: Record<string, UserConcurrencyInfo>
  timestamp?: string
}

export async function getConcurrencyStats(platform?: string, groupId?: number | null): Promise<OpsConcurrencyStatsResponse> {
  const params: Record<string, any> = {}
  if (platform) {
    params.platform = platform
  }
  if (typeof groupId === 'number' && groupId > 0) {
    params.group_id = groupId
  }

  const { data } = await apiClient.get<OpsConcurrencyStatsResponse>('/admin/ops/concurrency', { params })
  return data
}

export async function getUserConcurrencyStats(): Promise<OpsUserConcurrencyStatsResponse> {
  const { data } = await apiClient.get<OpsUserConcurrencyStatsResponse>('/admin/ops/user-concurrency')
  return data
}

export interface PlatformAvailability {
  platform: string
  total_accounts: number
  available_count: number
  rate_limit_count: number
  error_count: number
}

export interface GroupAvailability {
  group_id: number
  group_name: string
  platform: string
  total_accounts: number
  available_count: number
  rate_limit_count: number
  error_count: number
}

export interface AccountAvailability {
  account_id: number
  account_name: string
  platform: string
  group_id: number
  group_name: string
  status: string
  is_available: boolean
  is_rate_limited: boolean
  rate_limit_reset_at?: string
  rate_limit_remaining_sec?: number
  is_overloaded: boolean
  overload_until?: string
  overload_remaining_sec?: number
  has_error: boolean
  error_message?: string
}

export interface OpsAccountAvailabilityStatsResponse {
  enabled: boolean
  platform: Record<string, PlatformAvailability>
  group: Record<string, GroupAvailability>
  account: Record<string, AccountAvailability>
  timestamp?: string
}

export async function getAccountAvailabilityStats(platform?: string, groupId?: number | null): Promise<OpsAccountAvailabilityStatsResponse> {
  const params: Record<string, any> = {}
  if (platform) {
    params.platform = platform
  }
  if (typeof groupId === 'number' && groupId > 0) {
    params.group_id = groupId
  }
  const { data } = await apiClient.get<OpsAccountAvailabilityStatsResponse>('/admin/ops/account-availability', { params })
  return data
}

export interface OpsRateSummary {
  current: number
  peak: number
  avg: number
}

export interface OpsRealtimeTrafficSummary {
  window: string
  start_time: string
  end_time: string
  platform: string
  group_id?: number | null
  qps: OpsRateSummary
  tps: OpsRateSummary
}

export interface OpsRealtimeTrafficSummaryResponse {
  enabled: boolean
  summary: OpsRealtimeTrafficSummary | null
  timestamp?: string
}

export async function getRealtimeTrafficSummary(
  window: string,
  platform?: string,
  groupId?: number | null
): Promise<OpsRealtimeTrafficSummaryResponse> {
  const params: Record<string, any> = { window }
  if (platform) {
    params.platform = platform
  }
  if (typeof groupId === 'number' && groupId > 0) {
    params.group_id = groupId
  }

  const { data } = await apiClient.get<OpsRealtimeTrafficSummaryResponse>('/admin/ops/realtime-traffic', { params })
  return data
}

/**
 * Subscribe to realtime QPS updates via WebSocket.
 *
 * Note: browsers cannot set Authorization headers for WebSockets.
 * We authenticate via Sec-WebSocket-Protocol using a prefixed token item:
 *   ["sub2api-admin", "jwt.<token>"]
 */
export interface SubscribeQPSOptions {
  token?: string | null
  onOpen?: () => void
  onClose?: (event: CloseEvent) => void
  onError?: (event: Event) => void
  /**
   * Called when the server closes with an application close code that indicates
   * reconnecting is not useful (e.g. feature flag disabled).
   */
  onFatalClose?: (event: CloseEvent) => void
  /**
   * More granular status updates for UI (connecting/reconnecting/offline/etc).
   */
  onStatusChange?: (status: OpsWSStatus) => void
  /**
   * Called when a reconnect is scheduled (helps display "retry in Xs").
   */
  onReconnectScheduled?: (info: { attempt: number, delayMs: number }) => void
  wsBaseUrl?: string
  /**
   * Maximum reconnect attempts. Defaults to Infinity to keep the dashboard live.
   * Set to 0 to disable reconnect.
   */
  maxReconnectAttempts?: number
  reconnectBaseDelayMs?: number
  reconnectMaxDelayMs?: number
  /**
   * Stale connection detection (heartbeat-by-observation).
   * If no messages are received within this window, the socket is closed to trigger a reconnect.
   * Set to 0 to disable.
   */
  staleTimeoutMs?: number
  /**
   * How often to check staleness. Only used when `staleTimeoutMs > 0`.
   */
  staleCheckIntervalMs?: number
}

export type OpsWSStatus = 'connecting' | 'connected' | 'reconnecting' | 'offline' | 'closed'

export const OPS_WS_CLOSE_CODES = {
  REALTIME_DISABLED: 4001
} as const

const OPS_WS_BASE_PROTOCOL = 'sub2api-admin'

export function subscribeQPS(onMessage: (data: any) => void, options: SubscribeQPSOptions = {}): () => void {
  let ws: WebSocket | null = null
  let reconnectAttempts = 0
  const maxReconnectAttempts = Number.isFinite(options.maxReconnectAttempts as number)
    ? (options.maxReconnectAttempts as number)
    : Infinity
  const baseDelayMs = options.reconnectBaseDelayMs ?? 1000
  const maxDelayMs = options.reconnectMaxDelayMs ?? 30000
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let shouldReconnect = true
  let isConnecting = false
  let hasConnectedOnce = false
  let lastMessageAt = 0
  const staleTimeoutMs = options.staleTimeoutMs ?? 120_000
  const staleCheckIntervalMs = options.staleCheckIntervalMs ?? 30_000
  let staleTimer: ReturnType<typeof setInterval> | null = null

  const setStatus = (status: OpsWSStatus) => {
    options.onStatusChange?.(status)
  }

  const clearReconnectTimer = () => {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
  }

  const clearStaleTimer = () => {
    if (staleTimer) {
      clearInterval(staleTimer)
      staleTimer = null
    }
  }

  const startStaleTimer = () => {
    clearStaleTimer()
    if (!staleTimeoutMs || staleTimeoutMs <= 0) return
    staleTimer = setInterval(() => {
      if (!shouldReconnect) return
      if (!ws || ws.readyState !== WebSocket.OPEN) return
      if (!lastMessageAt) return
      const ageMs = Date.now() - lastMessageAt
      if (ageMs > staleTimeoutMs) {
        // Treat as a half-open connection; closing triggers the normal reconnect path.
        ws.close()
      }
    }, staleCheckIntervalMs)
  }

  const scheduleReconnect = () => {
    if (!shouldReconnect) return
    if (hasConnectedOnce && reconnectAttempts >= maxReconnectAttempts) return

    // If we're offline, wait for the browser to come back online.
    if (typeof navigator !== 'undefined' && 'onLine' in navigator && !navigator.onLine) {
      setStatus('offline')
      return
    }

    const expDelay = baseDelayMs * Math.pow(2, reconnectAttempts)
    const delay = Math.min(expDelay, maxDelayMs)
    const jitter = Math.floor(Math.random() * 250)
    clearReconnectTimer()
    reconnectTimer = setTimeout(() => {
      reconnectAttempts++
      connect()
    }, delay + jitter)
    options.onReconnectScheduled?.({ attempt: reconnectAttempts + 1, delayMs: delay + jitter })
  }

  const handleOnline = () => {
    if (!shouldReconnect) return
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return
    connect()
  }

  const handleOffline = () => {
    setStatus('offline')
  }

  const connect = () => {
    if (!shouldReconnect) return
    if (isConnecting) return
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return
    if (hasConnectedOnce && reconnectAttempts >= maxReconnectAttempts) return

    isConnecting = true
    setStatus(hasConnectedOnce ? 'reconnecting' : 'connecting')
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsBaseUrl = options.wsBaseUrl || import.meta.env.VITE_WS_BASE_URL || window.location.host
    const wsURL = new URL(`${protocol}//${wsBaseUrl}/api/v1/admin/ops/ws/qps`)

    // Do NOT put admin JWT in the URL query string (it can leak via access logs, proxies, etc).
    // Browsers cannot set Authorization headers for WebSockets, so we pass the token via
    // Sec-WebSocket-Protocol (subprotocol list): ["sub2api-admin", "jwt.<token>"].
    const rawToken = String(options.token ?? localStorage.getItem('auth_token') ?? '').trim()
    const protocols: string[] = [OPS_WS_BASE_PROTOCOL]
    if (rawToken) protocols.push(`jwt.${rawToken}`)

    ws = new WebSocket(wsURL.toString(), protocols)

    ws.onopen = () => {
      reconnectAttempts = 0
      isConnecting = false
      hasConnectedOnce = true
      clearReconnectTimer()
      lastMessageAt = Date.now()
      startStaleTimer()
      setStatus('connected')
      options.onOpen?.()
    }

    ws.onmessage = (e) => {
      try {
        const data = JSON.parse(e.data)
        lastMessageAt = Date.now()
        onMessage(data)
      } catch (err) {
        console.warn('[OpsWS] Failed to parse message:', err)
      }
    }

    ws.onerror = (error) => {
      console.error('[OpsWS] Connection error:', error)
      options.onError?.(error)
    }

    ws.onclose = (event) => {
      isConnecting = false
      options.onClose?.(event)
      clearStaleTimer()
      ws = null

      // If the server explicitly tells us to stop reconnecting, honor it.
      if (event && typeof event.code === 'number' && event.code === OPS_WS_CLOSE_CODES.REALTIME_DISABLED) {
        shouldReconnect = false
        clearReconnectTimer()
        setStatus('closed')
        options.onFatalClose?.(event)
        return
      }

      scheduleReconnect()
    }
  }

  window.addEventListener('online', handleOnline)
  window.addEventListener('offline', handleOffline)
  connect()

  return () => {
    shouldReconnect = false
    window.removeEventListener('online', handleOnline)
    window.removeEventListener('offline', handleOffline)
    clearReconnectTimer()
    clearStaleTimer()
    if (ws) ws.close()
    ws = null
    setStatus('closed')
  }
}

export type OpsSeverity = string
export type OpsPhase = string

export type AlertSeverity = 'critical' | 'warning' | 'info'
export type ThresholdMode = 'count' | 'percentage' | 'both'
export type MetricType =
  | 'success_rate'
  | 'error_rate'
  | 'upstream_error_rate'
  | 'cpu_usage_percent'
  | 'memory_usage_percent'
  | 'concurrency_queue_depth'
  | 'group_available_accounts'
  | 'group_available_ratio'
  | 'group_rate_limit_ratio'
  | 'account_rate_limited_count'
  | 'account_error_count'
  | 'account_error_ratio'
  | 'overload_account_count'
export type Operator = '>' | '>=' | '<' | '<=' | '==' | '!='

export interface AlertRule {
  id?: number
  name: string
  description?: string
  enabled: boolean
  metric_type: MetricType
  operator: Operator
  threshold: number
  window_minutes: number
  sustained_minutes: number
  severity: OpsSeverity
  cooldown_minutes: number
  notify_email: boolean
  filters?: Record<string, any>
  created_at?: string
  updated_at?: string
  last_triggered_at?: string | null
}

export interface AlertEvent {
  id: number
  rule_id: number
  severity: OpsSeverity | string
  status: 'firing' | 'resolved' | 'manual_resolved' | string
  title?: string
  description?: string
  metric_value?: number
  threshold_value?: number
  dimensions?: Record<string, any>
  fired_at: string
  resolved_at?: string | null
  email_sent: boolean
  created_at: string
}

export interface EmailNotificationConfig {
  alert: {
    enabled: boolean
    recipients: string[]
    min_severity: AlertSeverity | ''
    rate_limit_per_hour: number
    batching_window_seconds: number
    include_resolved_alerts: boolean
  }
  report: {
    enabled: boolean
    recipients: string[]
    daily_summary_enabled: boolean
    daily_summary_schedule: string
    weekly_summary_enabled: boolean
    weekly_summary_schedule: string
    error_digest_enabled: boolean
    error_digest_schedule: string
    error_digest_min_count: number
    account_health_enabled: boolean
    account_health_schedule: string
    account_health_error_rate_threshold: number
  }
}

export interface OpsMetricThresholds {
  sla_percent_min?: number | null                 // SLA低于此值变红
  ttft_p99_ms_max?: number | null                 // TTFT P99高于此值变红
  request_error_rate_percent_max?: number | null  // 请求错误率高于此值变红
  upstream_error_rate_percent_max?: number | null // 上游错误率高于此值变红
}

export interface OpsDistributedLockSettings {
  enabled: boolean
  key: string
  ttl_seconds: number
}

export interface OpsAlertRuntimeSettings {
  evaluation_interval_seconds: number
  distributed_lock: OpsDistributedLockSettings
  silencing: {
    enabled: boolean
    global_until_rfc3339: string
    global_reason: string
    entries?: Array<{
      rule_id?: number
      severities?: Array<OpsSeverity | string>
      until_rfc3339: string
      reason: string
    }>
  }
  thresholds: OpsMetricThresholds // 指标阈值配置
}

export interface OpsAdvancedSettings {
  data_retention: OpsDataRetentionSettings
  aggregation: OpsAggregationSettings
  ignore_count_tokens_errors: boolean
  ignore_context_canceled: boolean
  ignore_no_available_accounts: boolean
  ignore_invalid_api_key_errors: boolean
  ignore_insufficient_balance_errors: boolean
  display_openai_token_stats: boolean
  display_alert_events: boolean
  auto_refresh_enabled: boolean
  auto_refresh_interval_seconds: number
}

export interface OpsDataRetentionSettings {
  cleanup_enabled: boolean
  cleanup_schedule: string
  error_log_retention_days: number
  minute_metrics_retention_days: number
  hourly_metrics_retention_days: number
}

export interface OpsAggregationSettings {
  aggregation_enabled: boolean
}

export interface OpsRuntimeLogConfig {
  level: 'debug' | 'info' | 'warn' | 'error'
  enable_sampling: boolean
  sampling_initial: number
  sampling_thereafter: number
  caller: boolean
  stacktrace_level: 'none' | 'error' | 'fatal'
  retention_days: number
  source?: string
  updated_at?: string
  updated_by_user_id?: number
}

export interface OpsSystemLog {
  id: number
  created_at: string
  level: string
  component: string
  message: string
  request_id?: string
  client_request_id?: string
  user_id?: number | null
  account_id?: number | null
  platform?: string
  model?: string
  extra?: Record<string, any>
}

export type OpsSystemLogListResponse = PaginatedResponse<OpsSystemLog>

export interface OpsSystemLogQuery {
  page?: number
  page_size?: number
  time_range?: '5m' | '30m' | '1h' | '6h' | '24h' | '7d' | '30d'
  start_time?: string
  end_time?: string
  level?: string
  component?: string
  request_id?: string
  client_request_id?: string
  user_id?: number | null
  account_id?: number | null
  platform?: string
  model?: string
  q?: string
}

export interface OpsSystemLogCleanupRequest {
  start_time?: string
  end_time?: string
  level?: string
  component?: string
  request_id?: string
  client_request_id?: string
  user_id?: number | null
  account_id?: number | null
  platform?: string
  model?: string
  q?: string
}

export interface OpsSystemLogSinkHealth {
  queue_depth: number
  queue_capacity: number
  dropped_count: number
  write_failed_count: number
  written_count: number
  avg_write_delay_ms: number
  last_error?: string
}

export interface OpsErrorLog {
  id: number
  created_at: string

  // Standardized classification
  phase: OpsPhase
  type: string
  error_owner: 'client' | 'provider' | 'platform' | string
  error_source: 'client_request' | 'upstream_http' | 'gateway' | string

  severity: OpsSeverity
  status_code: number
  platform: string
  model: string

  is_retryable: boolean
  retry_count: number

  resolved: boolean
  resolved_at?: string | null
  resolved_by_user_id?: number | null
  resolved_retry_id?: number | null

  client_request_id: string
  request_id: string
  message: string

  user_id?: number | null
  user_email: string
  api_key_id?: number | null
  account_id?: number | null
  account_name: string
  group_id?: number | null
  group_name: string

  client_ip?: string | null
  request_path?: string
  stream?: boolean
}

export interface OpsErrorDetail extends OpsErrorLog {
  error_body: string
  user_agent: string

  // Upstream context (optional; enriched by gateway services)
  upstream_status_code?: number | null
  upstream_error_message?: string
  upstream_error_detail?: string
  upstream_errors?: string

  auth_latency_ms?: number | null
  routing_latency_ms?: number | null
  upstream_latency_ms?: number | null
  response_latency_ms?: number | null
  time_to_first_token_ms?: number | null

  request_body: string
  request_body_truncated: boolean
  request_body_bytes?: number | null

  is_business_limited: boolean
}

export type OpsErrorLogsResponse = PaginatedResponse<OpsErrorLog>

export async function getDashboardOverview(
  params: {
  time_range?: '5m' | '30m' | '1h' | '6h' | '24h'
  start_time?: string
  end_time?: string
  platform?: string
  group_id?: number | null
  mode?: OpsQueryMode
  },
  options: OpsRequestOptions = {}
): Promise<OpsDashboardOverview> {
  const { data } = await apiClient.get<OpsDashboardOverview>('/admin/ops/dashboard/overview', {
    params,
    signal: options.signal
  })
  return data
}

export async function getDashboardSnapshotV2(
  params: {
  time_range?: '5m' | '30m' | '1h' | '6h' | '24h'
  start_time?: string
  end_time?: string
  platform?: string
  group_id?: number | null
  mode?: OpsQueryMode
  },
  options: OpsRequestOptions = {}
): Promise<OpsDashboardSnapshotV2Response> {
  const { data } = await apiClient.get<OpsDashboardSnapshotV2Response>('/admin/ops/dashboard/snapshot-v2', {
    params,
    signal: options.signal
  })
  return data
}

export async function getThroughputTrend(
  params: {
  time_range?: '5m' | '30m' | '1h' | '6h' | '24h'
  start_time?: string
  end_time?: string
  platform?: string
  group_id?: number | null
  mode?: OpsQueryMode
  },
  options: OpsRequestOptions = {}
): Promise<OpsThroughputTrendResponse> {
  const { data } = await apiClient.get<OpsThroughputTrendResponse>('/admin/ops/dashboard/throughput-trend', {
    params,
    signal: options.signal
  })
  return data
}

export async function getLatencyHistogram(
  params: {
  time_range?: '5m' | '30m' | '1h' | '6h' | '24h'
  start_time?: string
  end_time?: string
  platform?: string
  group_id?: number | null
  mode?: OpsQueryMode
  },
  options: OpsRequestOptions = {}
): Promise<OpsLatencyHistogramResponse> {
  const { data } = await apiClient.get<OpsLatencyHistogramResponse>('/admin/ops/dashboard/latency-histogram', {
    params,
    signal: options.signal
  })
  return data
}

export async function getErrorTrend(
  params: {
  time_range?: '5m' | '30m' | '1h' | '6h' | '24h'
  start_time?: string
  end_time?: string
  platform?: string
  group_id?: number | null
  mode?: OpsQueryMode
  },
  options: OpsRequestOptions = {}
): Promise<OpsErrorTrendResponse> {
  const { data } = await apiClient.get<OpsErrorTrendResponse>('/admin/ops/dashboard/error-trend', {
    params,
    signal: options.signal
  })
  return data
}

export async function getErrorDistribution(
  params: {
  time_range?: '5m' | '30m' | '1h' | '6h' | '24h'
  start_time?: string
  end_time?: string
  platform?: string
  group_id?: number | null
  mode?: OpsQueryMode
  },
  options: OpsRequestOptions = {}
): Promise<OpsErrorDistributionResponse> {
  const { data } = await apiClient.get<OpsErrorDistributionResponse>('/admin/ops/dashboard/error-distribution', {
    params,
    signal: options.signal
  })
  return data
}

export async function getOpenAITokenStats(
  params: OpsOpenAITokenStatsParams,
  options: OpsRequestOptions = {}
): Promise<OpsOpenAITokenStatsResponse> {
  const { data } = await apiClient.get<OpsOpenAITokenStatsResponse>('/admin/ops/dashboard/openai-token-stats', {
    params,
    signal: options.signal
  })
  return data
}

export type OpsErrorListView = 'errors' | 'excluded' | 'all'

export type OpsErrorListQueryParams = {
  page?: number
  page_size?: number
  time_range?: string
  start_time?: string
  end_time?: string
  platform?: string
  group_id?: number | null
  account_id?: number | null

  phase?: string
  error_owner?: string
  error_source?: string
  resolved?: string
  view?: OpsErrorListView

  q?: string
  status_codes?: string
  status_codes_other?: string
}

// Legacy unified endpoints
export async function listErrorLogs(params: OpsErrorListQueryParams): Promise<OpsErrorLogsResponse> {
  const { data } = await apiClient.get<OpsErrorLogsResponse>('/admin/ops/errors', { params })
  return data
}

export async function getErrorLogDetail(id: number): Promise<OpsErrorDetail> {
  const { data } = await apiClient.get<OpsErrorDetail>(`/admin/ops/errors/${id}`)
  return data
}

export async function retryErrorRequest(id: number, req: OpsRetryRequest): Promise<OpsRetryResult> {
  const { data } = await apiClient.post<OpsRetryResult>(`/admin/ops/errors/${id}/retry`, req)
  return data
}

export async function listRetryAttempts(errorId: number, limit = 50): Promise<OpsRetryAttempt[]> {
  const { data } = await apiClient.get<OpsRetryAttempt[]>(`/admin/ops/errors/${errorId}/retries`, { params: { limit } })
  return data
}

export async function updateErrorResolved(errorId: number, resolved: boolean): Promise<void> {
  await apiClient.put(`/admin/ops/errors/${errorId}/resolve`, { resolved })
}

// New split endpoints
export async function listRequestErrors(params: OpsErrorListQueryParams): Promise<OpsErrorLogsResponse> {
  const { data } = await apiClient.get<OpsErrorLogsResponse>('/admin/ops/request-errors', { params })
  return data
}

export async function listUpstreamErrors(params: OpsErrorListQueryParams): Promise<OpsErrorLogsResponse> {
  const { data } = await apiClient.get<OpsErrorLogsResponse>('/admin/ops/upstream-errors', { params })
  return data
}

export async function getRequestErrorDetail(id: number): Promise<OpsErrorDetail> {
  const { data } = await apiClient.get<OpsErrorDetail>(`/admin/ops/request-errors/${id}`)
  return data
}

export async function getUpstreamErrorDetail(id: number): Promise<OpsErrorDetail> {
  const { data } = await apiClient.get<OpsErrorDetail>(`/admin/ops/upstream-errors/${id}`)
  return data
}

export async function retryRequestErrorClient(id: number): Promise<OpsRetryResult> {
  const { data } = await apiClient.post<OpsRetryResult>(`/admin/ops/request-errors/${id}/retry-client`, {})
  return data
}

export async function retryRequestErrorUpstreamEvent(id: number, idx: number): Promise<OpsRetryResult> {
  const { data } = await apiClient.post<OpsRetryResult>(`/admin/ops/request-errors/${id}/upstream-errors/${idx}/retry`, {})
  return data
}

export async function retryUpstreamError(id: number): Promise<OpsRetryResult> {
  const { data } = await apiClient.post<OpsRetryResult>(`/admin/ops/upstream-errors/${id}/retry`, {})
  return data
}

export async function updateRequestErrorResolved(errorId: number, resolved: boolean): Promise<void> {
  await apiClient.put(`/admin/ops/request-errors/${errorId}/resolve`, { resolved })
}

export async function updateUpstreamErrorResolved(errorId: number, resolved: boolean): Promise<void> {
  await apiClient.put(`/admin/ops/upstream-errors/${errorId}/resolve`, { resolved })
}

export async function listRequestErrorUpstreamErrors(
  id: number,
  params: OpsErrorListQueryParams = {},
  options: { include_detail?: boolean } = {}
): Promise<PaginatedResponse<OpsErrorDetail>> {
  const query: Record<string, any> = { ...params }
  if (options.include_detail) query.include_detail = '1'
  const { data } = await apiClient.get<PaginatedResponse<OpsErrorDetail>>(`/admin/ops/request-errors/${id}/upstream-errors`, { params: query })
  return data
}

export async function listRequestDetails(params: OpsRequestDetailsParams): Promise<OpsRequestDetailsResponse> {
  const { data } = await apiClient.get<OpsRequestDetailsResponse>('/admin/ops/requests', { params })
  return data
}

// Alert rules
export async function listAlertRules(): Promise<AlertRule[]> {
  const { data } = await apiClient.get<AlertRule[]>('/admin/ops/alert-rules')
  return data
}

export async function createAlertRule(rule: AlertRule): Promise<AlertRule> {
  const { data } = await apiClient.post<AlertRule>('/admin/ops/alert-rules', rule)
  return data
}

export async function updateAlertRule(id: number, rule: Partial<AlertRule>): Promise<AlertRule> {
  const { data } = await apiClient.put<AlertRule>(`/admin/ops/alert-rules/${id}`, rule)
  return data
}

export async function deleteAlertRule(id: number): Promise<void> {
  await apiClient.delete(`/admin/ops/alert-rules/${id}`)
}

export interface AlertEventsQuery {
  limit?: number
  status?: string
  severity?: string
  email_sent?: boolean
  time_range?: string
  start_time?: string
  end_time?: string
  before_fired_at?: string
  before_id?: number
  platform?: string
  group_id?: number
}

export async function listAlertEvents(params: AlertEventsQuery = {}): Promise<AlertEvent[]> {
  const { data } = await apiClient.get<AlertEvent[]>('/admin/ops/alert-events', { params })
  return data
}

export async function getAlertEvent(id: number): Promise<AlertEvent> {
  const { data } = await apiClient.get<AlertEvent>(`/admin/ops/alert-events/${id}`)
  return data
}

export async function updateAlertEventStatus(id: number, status: 'resolved' | 'manual_resolved'): Promise<void> {
  await apiClient.put(`/admin/ops/alert-events/${id}/status`, { status })
}

export async function createAlertSilence(payload: {
  rule_id: number
  platform: string
  group_id?: number | null
  region?: string | null
  until: string
  reason?: string
}): Promise<void> {
  await apiClient.post('/admin/ops/alert-silences', payload)
}

// Email notification config
export async function getEmailNotificationConfig(): Promise<EmailNotificationConfig> {
  const { data } = await apiClient.get<EmailNotificationConfig>('/admin/ops/email-notification/config')
  return data
}

export async function updateEmailNotificationConfig(config: EmailNotificationConfig): Promise<EmailNotificationConfig> {
  const { data } = await apiClient.put<EmailNotificationConfig>('/admin/ops/email-notification/config', config)
  return data
}

// Runtime settings (DB-backed)
export async function getAlertRuntimeSettings(): Promise<OpsAlertRuntimeSettings> {
  const { data } = await apiClient.get<OpsAlertRuntimeSettings>('/admin/ops/runtime/alert')
  return data
}

export async function updateAlertRuntimeSettings(config: OpsAlertRuntimeSettings): Promise<OpsAlertRuntimeSettings> {
  const { data } = await apiClient.put<OpsAlertRuntimeSettings>('/admin/ops/runtime/alert', config)
  return data
}

export async function getRuntimeLogConfig(): Promise<OpsRuntimeLogConfig> {
  const { data } = await apiClient.get<OpsRuntimeLogConfig>('/admin/ops/runtime/logging')
  return data
}

export async function updateRuntimeLogConfig(config: OpsRuntimeLogConfig): Promise<OpsRuntimeLogConfig> {
  const { data } = await apiClient.put<OpsRuntimeLogConfig>('/admin/ops/runtime/logging', config)
  return data
}

export async function resetRuntimeLogConfig(): Promise<OpsRuntimeLogConfig> {
  const { data } = await apiClient.post<OpsRuntimeLogConfig>('/admin/ops/runtime/logging/reset')
  return data
}

export async function listSystemLogs(params: OpsSystemLogQuery): Promise<OpsSystemLogListResponse> {
  const { data } = await apiClient.get<OpsSystemLogListResponse>('/admin/ops/system-logs', { params })
  return data
}

export async function cleanupSystemLogs(payload: OpsSystemLogCleanupRequest): Promise<{ deleted: number }> {
  const { data } = await apiClient.post<{ deleted: number }>('/admin/ops/system-logs/cleanup', payload)
  return data
}

export async function getSystemLogSinkHealth(): Promise<OpsSystemLogSinkHealth> {
  const { data } = await apiClient.get<OpsSystemLogSinkHealth>('/admin/ops/system-logs/health')
  return data
}

// Advanced settings (DB-backed)
export async function getAdvancedSettings(): Promise<OpsAdvancedSettings> {
  const { data } = await apiClient.get<OpsAdvancedSettings>('/admin/ops/advanced-settings')
  return data
}

export async function updateAdvancedSettings(config: OpsAdvancedSettings): Promise<OpsAdvancedSettings> {
  const { data } = await apiClient.put<OpsAdvancedSettings>('/admin/ops/advanced-settings', config)
  return data
}

// ==================== Metric Thresholds ====================

async function getMetricThresholds(): Promise<OpsMetricThresholds> {
  const { data } = await apiClient.get<OpsMetricThresholds>('/admin/ops/settings/metric-thresholds')
  return data
}

async function updateMetricThresholds(thresholds: OpsMetricThresholds): Promise<void> {
  await apiClient.put('/admin/ops/settings/metric-thresholds', thresholds)
}

export const opsAPI = {
  getDashboardSnapshotV2,
  getDashboardOverview,
  getThroughputTrend,
  getLatencyHistogram,
  getErrorTrend,
  getErrorDistribution,
  getOpenAITokenStats,
  getConcurrencyStats,
  getUserConcurrencyStats,
  getAccountAvailabilityStats,
  getRealtimeTrafficSummary,
  subscribeQPS,

  // Legacy unified endpoints
  listErrorLogs,
  getErrorLogDetail,
  retryErrorRequest,
  listRetryAttempts,
  updateErrorResolved,

  // New split endpoints
  listRequestErrors,
  listUpstreamErrors,
  getRequestErrorDetail,
  getUpstreamErrorDetail,
  retryRequestErrorClient,
  retryRequestErrorUpstreamEvent,
  retryUpstreamError,
  updateRequestErrorResolved,
  updateUpstreamErrorResolved,
  listRequestErrorUpstreamErrors,

  listRequestDetails,
  listAlertRules,
  createAlertRule,
  updateAlertRule,
  deleteAlertRule,
  listAlertEvents,
  getAlertEvent,
  updateAlertEventStatus,
  createAlertSilence,
  getEmailNotificationConfig,
  updateEmailNotificationConfig,
  getAlertRuntimeSettings,
  updateAlertRuntimeSettings,
  getRuntimeLogConfig,
  updateRuntimeLogConfig,
  resetRuntimeLogConfig,
  getAdvancedSettings,
  updateAdvancedSettings,
  getMetricThresholds,
  updateMetricThresholds,
  listSystemLogs,
  cleanupSystemLogs,
  getSystemLogSinkHealth
}

export default opsAPI
