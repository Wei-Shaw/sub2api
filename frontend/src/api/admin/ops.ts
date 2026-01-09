/**
 * Admin Ops API endpoints (vNext)
 * - Error logs list/detail + retry (client/upstream)
 * - Dashboard overview (raw path)
 */

import { apiClient REDACTED from '../client'
import type { PaginatedResponse REDACTED from '@/types'

export type OpsRetryMode = 'client' | 'upstream'
export type OpsQueryMode = 'auto' | 'raw' | 'preagg'

export interface OpsRequestOptions {
  signal?: AbortSignal
REDACTED

export interface OpsRetryRequest {
  mode: OpsRetryMode
  pinned_account_id?: number
REDACTED

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
REDACTED

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
  REDACTED
  tps: {
    current: number
    peak: number
    avg: number
  REDACTED

  duration: OpsPercentiles
  ttft: OpsPercentiles
REDACTED

export interface OpsPercentiles {
  p50_ms?: number | null
  p90_ms?: number | null
  p95_ms?: number | null
  p99_ms?: number | null
  avg_ms?: number | null
  max_ms?: number | null
REDACTED

export interface OpsThroughputTrendPoint {
  bucket_start: string
  request_count: number
  token_consumed: number
  qps: number
  tps: number
REDACTED

export interface OpsThroughputPlatformBreakdownItem {
  platform: string
  request_count: number
  token_consumed: number
REDACTED

export interface OpsThroughputGroupBreakdownItem {
  group_id: number
  group_name: string
  request_count: number
  token_consumed: number
REDACTED

export interface OpsThroughputTrendResponse {
  bucket: string
  points: OpsThroughputTrendPoint[]
  by_platform?: OpsThroughputPlatformBreakdownItem[]
  top_groups?: OpsThroughputGroupBreakdownItem[]
REDACTED

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
REDACTED

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
REDACTED

export type OpsRequestDetailsResponse = PaginatedResponse<OpsRequestDetail>

export interface OpsLatencyHistogramBucket {
  range: string
  count: number
REDACTED

export interface OpsLatencyHistogramResponse {
  start_time: string
  end_time: string
  platform: string
  group_id?: number | null

  total_requests: number
  buckets: OpsLatencyHistogramBucket[]
REDACTED

export interface OpsErrorTrendPoint {
  bucket_start: string
  error_count_total: number
  business_limited_count: number
  error_count_sla: number
  upstream_error_count_excl_429_529: number
  upstream_429_count: number
  upstream_529_count: number
REDACTED

export interface OpsErrorTrendResponse {
  bucket: string
  points: OpsErrorTrendPoint[]
REDACTED

export interface OpsErrorDistributionItem {
  status_code: number
  total: number
  sla: number
  business_limited: number
REDACTED

export interface OpsErrorDistributionResponse {
  total: number
  items: OpsErrorDistributionItem[]
REDACTED

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
REDACTED

export interface OpsJobHeartbeat {
  job_name: string
  last_run_at?: string | null
  last_success_at?: string | null
  last_error_at?: string | null
  last_error?: string | null
  last_duration_ms?: number | null
  updated_at: string
REDACTED

export interface PlatformConcurrencyInfo {
  platform: string
  current_in_use: number
  max_capacity: number
  load_percentage: number
  waiting_in_queue: number
REDACTED

export interface GroupConcurrencyInfo {
  group_id: number
  group_name: string
  platform: string
  current_in_use: number
  max_capacity: number
  load_percentage: number
  waiting_in_queue: number
REDACTED

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
REDACTED

export interface OpsConcurrencyStatsResponse {
  enabled: boolean
  platform: Record<string, PlatformConcurrencyInfo>
  group: Record<string, GroupConcurrencyInfo>
  account: Record<string, AccountConcurrencyInfo>
  timestamp?: string
REDACTED

export async function getConcurrencyStats(platform?: string, groupId?: number | null): Promise<OpsConcurrencyStatsResponse> {
  const params: Record<string, any> = {REDACTED
  if (platform) {
    params.platform = platform
  REDACTED
  if (typeof groupId === 'number' && groupId > 0) {
    params.group_id = groupId
  REDACTED

  const { data REDACTED = await apiClient.get<OpsConcurrencyStatsResponse>('/admin/ops/concurrency', { params REDACTED)
  return data
REDACTED

export interface PlatformAvailability {
  platform: string
  total_accounts: number
  available_count: number
  rate_limit_count: number
  error_count: number
REDACTED

export interface GroupAvailability {
  group_id: number
  group_name: string
  platform: string
  total_accounts: number
  available_count: number
  rate_limit_count: number
  error_count: number
REDACTED

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
REDACTED

export interface OpsAccountAvailabilityStatsResponse {
  enabled: boolean
  platform: Record<string, PlatformAvailability>
  group: Record<string, GroupAvailability>
  account: Record<string, AccountAvailability>
  timestamp?: string
REDACTED

export async function getAccountAvailabilityStats(platform?: string, groupId?: number | null): Promise<OpsAccountAvailabilityStatsResponse> {
  const params: Record<string, any> = {REDACTED
  if (platform) {
    params.platform = platform
  REDACTED
  if (typeof groupId === 'number' && groupId > 0) {
    params.group_id = groupId
  REDACTED
  const { data REDACTED = await apiClient.get<OpsAccountAvailabilityStatsResponse>('/admin/ops/account-availability', { params REDACTED)
  return data
REDACTED

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
  onReconnectScheduled?: (info: { attempt: number, delayMs: number REDACTED) => void
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
REDACTED

export type OpsWSStatus = 'connecting' | 'connected' | 'reconnecting' | 'offline' | 'closed'

export const OPS_WS_CLOSE_CODES = {
  REALTIME_DISABLED: 4001
REDACTED as const

const OPS_WS_BASE_PROTOCOL = 'sub2api-admin'

export function subscribeQPS(onMessage: (data: any) => void, options: SubscribeQPSOptions = {REDACTED): () => void {
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
  REDACTED

  const clearReconnectTimer = () => {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    REDACTED
  REDACTED

  const clearStaleTimer = () => {
    if (staleTimer) {
      clearInterval(staleTimer)
      staleTimer = null
    REDACTED
  REDACTED

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
      REDACTED
    REDACTED, staleCheckIntervalMs)
  REDACTED

  const scheduleReconnect = () => {
    if (!shouldReconnect) return
    if (hasConnectedOnce && reconnectAttempts >= maxReconnectAttempts) return

    // If we're offline, wait for the browser to come back online.
    if (typeof navigator !== 'undefined' && 'onLine' in navigator && !navigator.onLine) {
      setStatus('offline')
      return
    REDACTED

    const expDelay = baseDelayMs * Math.pow(2, reconnectAttempts)
    const delay = Math.min(expDelay, maxDelayMs)
    const jitter = Math.floor(Math.random() * 250)
    clearReconnectTimer()
    reconnectTimer = setTimeout(() => {
      reconnectAttempts++
      connect()
    REDACTED, delay + jitter)
    options.onReconnectScheduled?.({ attempt: reconnectAttempts + 1, delayMs: delay + jitter REDACTED)
  REDACTED

  const handleOnline = () => {
    if (!shouldReconnect) return
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return
    connect()
  REDACTED

  const handleOffline = () => {
    setStatus('offline')
  REDACTED

  const connect = () => {
    if (!shouldReconnect) return
    if (isConnecting) return
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return
    if (hasConnectedOnce && reconnectAttempts >= maxReconnectAttempts) return

    isConnecting = true
    setStatus(hasConnectedOnce ? 'reconnecting' : 'connecting')
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsBaseUrl = options.wsBaseUrl || import.meta.env.VITE_WS_BASE_URL || window.location.host
    const wsURL = new URL(`${protocolREDACTED//${wsBaseUrlREDACTED/api/v1/admin/ops/ws/qps`)

    // Do NOT put admin JWT in the URL query string (it can leak via access logs, proxies, etc).
    // Browsers cannot set Authorization headers for WebSockets, so we pass the token via
    // Sec-WebSocket-Protocol (subprotocol list): ["sub2api-admin", "jwt.<token>"].
    const rawToken = String(options.token ?? localStorage.getItem('auth_token') ?? '').trim()
    const protocols: string[] = [OPS_WS_BASE_PROTOCOL]
    if (rawToken) protocols.push(`jwt.${rawTokenREDACTED`)

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
    REDACTED

    ws.onmessage = (e) => {
      try {
        const data = JSON.parse(e.data)
        lastMessageAt = Date.now()
        onMessage(data)
      REDACTED catch (err) {
        console.warn('[OpsWS] Failed to parse message:', err)
      REDACTED
    REDACTED

    ws.onerror = (error) => {
      console.error('[OpsWS] Connection error:', error)
      options.onError?.(error)
    REDACTED

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
      REDACTED

      scheduleReconnect()
    REDACTED
  REDACTED

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
  REDACTED
REDACTED

export type OpsSeverity = string
export type OpsPhase = string

export type AlertSeverity = 'critical' | 'warning' | 'info'
export type ThresholdMode = 'count' | 'percentage' | 'both'
export type MetricType =
  | 'success_rate'
  | 'error_rate'
  | 'upstream_error_rate'
  | 'p95_latency_ms'
  | 'p99_latency_ms'
  | 'cpu_usage_percent'
  | 'memory_usage_percent'
  | 'concurrency_queue_depth'
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
REDACTED

export interface AlertEvent {
  id: number
  rule_id: number
  severity: OpsSeverity | string
  status: 'firing' | 'resolved' | string
  title?: string
  description?: string
  metric_value?: number
  threshold_value?: number
  dimensions?: Record<string, any>
  fired_at: string
  resolved_at?: string | null
  email_sent: boolean
  created_at: string
REDACTED

export interface EmailNotificationConfig {
  alert: {
    enabled: boolean
    recipients: string[]
    min_severity: AlertSeverity | ''
    rate_limit_per_hour: number
    batching_window_seconds: number
    include_resolved_alerts: boolean
  REDACTED
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
  REDACTED
REDACTED

export interface OpsDistributedLockSettings {
  enabled: boolean
  key: string
  ttl_seconds: number
REDACTED

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
    REDACTED>
  REDACTED
REDACTED

export interface OpsErrorLog {
  id: number
  created_at: string
  phase: OpsPhase
  type: string
  severity: OpsSeverity
  status_code: number
  platform: string
  model: string
  latency_ms?: number | null
  client_request_id: string
  request_id: string
  message: string

  user_id?: number | null
  api_key_id?: number | null
  account_id?: number | null
  group_id?: number | null

  client_ip?: string | null
  request_path?: string
  stream?: boolean
REDACTED

export interface OpsErrorDetail extends OpsErrorLog {
  error_body: string
  user_agent: string

  auth_latency_ms?: number | null
  routing_latency_ms?: number | null
  upstream_latency_ms?: number | null
  response_latency_ms?: number | null
  time_to_first_token_ms?: number | null

  request_body: string
  request_body_truncated: boolean
  request_body_bytes?: number | null

  is_business_limited: boolean
REDACTED

export type OpsErrorLogsResponse = PaginatedResponse<OpsErrorLog>

export async function getDashboardOverview(
  params: {
  time_range?: '5m' | '30m' | '1h' | '6h' | '24h'
  start_time?: string
  end_time?: string
  platform?: string
  group_id?: number | null
  mode?: OpsQueryMode
  REDACTED,
  options: OpsRequestOptions = {REDACTED
): Promise<OpsDashboardOverview> {
  const { data REDACTED = await apiClient.get<OpsDashboardOverview>('/admin/ops/dashboard/overview', {
    params,
    signal: options.signal
  REDACTED)
  return data
REDACTED

export async function getThroughputTrend(
  params: {
  time_range?: '5m' | '30m' | '1h' | '6h' | '24h'
  start_time?: string
  end_time?: string
  platform?: string
  group_id?: number | null
  mode?: OpsQueryMode
  REDACTED,
  options: OpsRequestOptions = {REDACTED
): Promise<OpsThroughputTrendResponse> {
  const { data REDACTED = await apiClient.get<OpsThroughputTrendResponse>('/admin/ops/dashboard/throughput-trend', {
    params,
    signal: options.signal
  REDACTED)
  return data
REDACTED

export async function getLatencyHistogram(
  params: {
  time_range?: '5m' | '30m' | '1h' | '6h' | '24h'
  start_time?: string
  end_time?: string
  platform?: string
  group_id?: number | null
  mode?: OpsQueryMode
  REDACTED,
  options: OpsRequestOptions = {REDACTED
): Promise<OpsLatencyHistogramResponse> {
  const { data REDACTED = await apiClient.get<OpsLatencyHistogramResponse>('/admin/ops/dashboard/latency-histogram', {
    params,
    signal: options.signal
  REDACTED)
  return data
REDACTED

export async function getErrorTrend(
  params: {
  time_range?: '5m' | '30m' | '1h' | '6h' | '24h'
  start_time?: string
  end_time?: string
  platform?: string
  group_id?: number | null
  mode?: OpsQueryMode
  REDACTED,
  options: OpsRequestOptions = {REDACTED
): Promise<OpsErrorTrendResponse> {
  const { data REDACTED = await apiClient.get<OpsErrorTrendResponse>('/admin/ops/dashboard/error-trend', {
    params,
    signal: options.signal
  REDACTED)
  return data
REDACTED

export async function getErrorDistribution(
  params: {
  time_range?: '5m' | '30m' | '1h' | '6h' | '24h'
  start_time?: string
  end_time?: string
  platform?: string
  group_id?: number | null
  mode?: OpsQueryMode
  REDACTED,
  options: OpsRequestOptions = {REDACTED
): Promise<OpsErrorDistributionResponse> {
  const { data REDACTED = await apiClient.get<OpsErrorDistributionResponse>('/admin/ops/dashboard/error-distribution', {
    params,
    signal: options.signal
  REDACTED)
  return data
REDACTED

export async function listErrorLogs(params: {
  page?: number
  page_size?: number
  time_range?: string
  start_time?: string
  end_time?: string
  platform?: string
  group_id?: number | null
  account_id?: number | null
  phase?: string
  q?: string
  status_codes?: string
REDACTED): Promise<OpsErrorLogsResponse> {
  const { data REDACTED = await apiClient.get<OpsErrorLogsResponse>('/admin/ops/errors', { params REDACTED)
  return data
REDACTED

export async function getErrorLogDetail(id: number): Promise<OpsErrorDetail> {
  const { data REDACTED = await apiClient.get<OpsErrorDetail>(`/admin/ops/errors/${idREDACTED`)
  return data
REDACTED

export async function retryErrorRequest(id: number, req: OpsRetryRequest): Promise<OpsRetryResult> {
  const { data REDACTED = await apiClient.post<OpsRetryResult>(`/admin/ops/errors/${idREDACTED/retry`, req)
  return data
REDACTED

export async function listRequestDetails(params: OpsRequestDetailsParams): Promise<OpsRequestDetailsResponse> {
  const { data REDACTED = await apiClient.get<OpsRequestDetailsResponse>('/admin/ops/requests', { params REDACTED)
  return data
REDACTED

// Alert rules
export async function listAlertRules(): Promise<AlertRule[]> {
  const { data REDACTED = await apiClient.get<AlertRule[]>('/admin/ops/alert-rules')
  return data
REDACTED

export async function createAlertRule(rule: AlertRule): Promise<AlertRule> {
  const { data REDACTED = await apiClient.post<AlertRule>('/admin/ops/alert-rules', rule)
  return data
REDACTED

export async function updateAlertRule(id: number, rule: Partial<AlertRule>): Promise<AlertRule> {
  const { data REDACTED = await apiClient.put<AlertRule>(`/admin/ops/alert-rules/${idREDACTED`, rule)
  return data
REDACTED

export async function deleteAlertRule(id: number): Promise<void> {
  await apiClient.delete(`/admin/ops/alert-rules/${idREDACTED`)
REDACTED

export async function listAlertEvents(limit = 100): Promise<AlertEvent[]> {
  const { data REDACTED = await apiClient.get<AlertEvent[]>('/admin/ops/alert-events', { params: { limit REDACTED REDACTED)
  return data
REDACTED

// Email notification config
export async function getEmailNotificationConfig(): Promise<EmailNotificationConfig> {
  const { data REDACTED = await apiClient.get<EmailNotificationConfig>('/admin/ops/email-notification/config')
  return data
REDACTED

export async function updateEmailNotificationConfig(config: EmailNotificationConfig): Promise<EmailNotificationConfig> {
  const { data REDACTED = await apiClient.put<EmailNotificationConfig>('/admin/ops/email-notification/config', config)
  return data
REDACTED

// Runtime settings (DB-backed)
export async function getAlertRuntimeSettings(): Promise<OpsAlertRuntimeSettings> {
  const { data REDACTED = await apiClient.get<OpsAlertRuntimeSettings>('/admin/ops/runtime/alert')
  return data
REDACTED

export async function updateAlertRuntimeSettings(config: OpsAlertRuntimeSettings): Promise<OpsAlertRuntimeSettings> {
  const { data REDACTED = await apiClient.put<OpsAlertRuntimeSettings>('/admin/ops/runtime/alert', config)
  return data
REDACTED

export const opsAPI = {
  getDashboardOverview,
  getThroughputTrend,
  getLatencyHistogram,
  getErrorTrend,
  getErrorDistribution,
  getConcurrencyStats,
  getAccountAvailabilityStats,
  subscribeQPS,
  listErrorLogs,
  getErrorLogDetail,
  retryErrorRequest,
  listRequestDetails,
  listAlertRules,
  createAlertRule,
  updateAlertRule,
  deleteAlertRule,
  listAlertEvents,
  getEmailNotificationConfig,
  updateEmailNotificationConfig,
  getAlertRuntimeSettings,
  updateAlertRuntimeSettings
REDACTED

export default opsAPI
