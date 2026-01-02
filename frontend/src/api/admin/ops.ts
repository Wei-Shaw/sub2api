/**
 * Admin Ops API endpoints
 * Provides stability metrics and error logs for ops dashboard
 */

import { apiClient REDACTED from '../client'

export type OpsSeverity = 'P0' | 'P1' | 'P2' | 'P3'
export type OpsPhase =
  | 'auth'
  | 'concurrency'
  | 'billing'
  | 'scheduling'
  | 'network'
  | 'upstream'
  | 'response'
  | 'internal'
export type OpsPlatform = 'gemini' | 'openai' | 'anthropic' | 'antigravity'

export interface OpsMetrics {
  window_minutes: number
  request_count: number
  success_count: number
  error_count: number
  success_rate: number
  error_rate: number
  p95_latency_ms: number
  p99_latency_ms: number
  http2_errors: number
  active_alerts: number
  cpu_usage_percent?: number
  memory_used_mb?: number
  memory_total_mb?: number
  memory_usage_percent?: number
  heap_alloc_mb?: number
  gc_pause_ms?: number
  concurrency_queue_depth?: number
  updated_at?: string
REDACTED

export interface OpsErrorLog {
  id: number
  created_at: string
  phase: OpsPhase
  type: string
  severity: OpsSeverity
  status_code: number
  platform: OpsPlatform
  model: string
  latency_ms: number | null
  request_id: string
  message: string
  user_id?: number | null
  api_key_id?: number | null
  account_id?: number | null
  group_id?: number | null
  client_ip?: string
  request_path?: string
  stream?: boolean
REDACTED

export interface OpsErrorListParams {
  start_time?: string
  end_time?: string
  platform?: OpsPlatform
  phase?: OpsPhase
  severity?: OpsSeverity
  q?: string
  /**
   * Max 500 (legacy endpoint uses a hard cap); use paginated /admin/ops/errors for larger result sets.
   */
  limit?: number
REDACTED

export interface OpsErrorListResponse {
  items: OpsErrorLog[]
  total?: number
REDACTED

export interface OpsMetricsHistoryParams {
  window_minutes?: number
  minutes?: number
  start_time?: string
  end_time?: string
  limit?: number
REDACTED

export interface OpsMetricsHistoryResponse {
  items: OpsMetrics[]
REDACTED

/**
 * Get latest ops metrics snapshot
 */
export async function getMetrics(): Promise<OpsMetrics> {
  const { data REDACTED = await apiClient.get<OpsMetrics>('/admin/ops/metrics')
  return data
REDACTED

/**
 * List metrics history for charts
 */
export async function listMetricsHistory(params?: OpsMetricsHistoryParams): Promise<OpsMetricsHistoryResponse> {
  const { data REDACTED = await apiClient.get<OpsMetricsHistoryResponse>('/admin/ops/metrics/history', { params REDACTED)
  return data
REDACTED

/**
 * List recent error logs with optional filters
 */
export async function listErrors(params?: OpsErrorListParams): Promise<OpsErrorListResponse> {
  const { data REDACTED = await apiClient.get<OpsErrorListResponse>('/admin/ops/error-logs', { params REDACTED)
  return data
REDACTED

export interface OpsDashboardOverview {
  timestamp: string
  health_score: number
  sla: {
    current: number
    threshold: number
    status: string
    trend: string
    change_24h: number
  REDACTED
  qps: {
    current: number
    peak_1h: number
    avg_1h: number
    change_vs_yesterday: number
  REDACTED
  tps: {
    current: number
    peak_1h: number
    avg_1h: number
  REDACTED
  latency: {
    p50: number
    p95: number
    p99: number
    p999: number
    avg: number
    max: number
    threshold_p99: number
    status: string
  REDACTED
  errors: {
    total_count: number
    error_rate: number
    '4xx_count': number
    '5xx_count': number
    timeout_count: number
    top_error?: {
      code: string
      message: string
      count: number
    REDACTED
  REDACTED
  resources: {
    cpu_usage: number
    memory_usage: number
    disk_usage: number
    goroutines: number
    db_connections: {
      active: number
      idle: number
      waiting: number
      max: number
    REDACTED
  REDACTED
  system_status: {
    redis: string
    database: string
    background_jobs: string
  REDACTED
REDACTED

export interface ProviderHealthData {
  name: string
  request_count: number
  success_rate: number
  error_rate: number
  latency_avg: number
  latency_p99: number
  status: string
  errors_by_type: {
    '4xx': number
    '5xx': number
    timeout: number
  REDACTED
REDACTED

export interface ProviderHealthResponse {
  providers: ProviderHealthData[]
  summary: {
    total_requests: number
    avg_success_rate: number
    best_provider: string
    worst_provider: string
  REDACTED
REDACTED

export interface LatencyHistogramResponse {
  buckets: {
    range: string
    count: number
    percentage: number
  REDACTED[]
  total_requests: number
  slow_request_threshold: number
REDACTED

export interface ErrorDistributionResponse {
  items: {
    code: string
    message: string
    count: number
    percentage: number
  REDACTED[]
REDACTED

/**
 * Get realtime ops dashboard overview
 */
export async function getDashboardOverview(timeRange = '1h'): Promise<OpsDashboardOverview> {
  const { data REDACTED = await apiClient.get<OpsDashboardOverview>('/admin/ops/dashboard/overview', {
    params: { time_range: timeRange REDACTED
  REDACTED)
  return data
REDACTED

/**
 * Get provider health comparison
 */
export async function getProviderHealth(timeRange = '1h'): Promise<ProviderHealthResponse> {
  const { data REDACTED = await apiClient.get<ProviderHealthResponse>('/admin/ops/dashboard/providers', {
    params: { time_range: timeRange REDACTED
  REDACTED)
  return data
REDACTED

/**
 * Get latency histogram
 */
export async function getLatencyHistogram(timeRange = '1h'): Promise<LatencyHistogramResponse> {
  const { data REDACTED = await apiClient.get<LatencyHistogramResponse>('/admin/ops/dashboard/latency-histogram', {
    params: { time_range: timeRange REDACTED
  REDACTED)
  return data
REDACTED

/**
 * Get error distribution
 */
export async function getErrorDistribution(timeRange = '1h'): Promise<ErrorDistributionResponse> {
  const { data REDACTED = await apiClient.get<ErrorDistributionResponse>('/admin/ops/dashboard/errors/distribution', {
    params: { time_range: timeRange REDACTED
  REDACTED)
  return data
REDACTED

/**
 * Subscribe to realtime QPS updates via WebSocket
 */
export function subscribeQPS(onMessage: (data: any) => void): () => void {
  let ws: WebSocket | null = null
  let reconnectAttempts = 0
  const maxReconnectAttempts = 5
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let shouldReconnect = true

  const connect = () => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const host = window.location.host
    ws = new WebSocket(`${protocolREDACTED//${hostREDACTED/api/v1/admin/ops/ws/qps`)

    ws.onopen = () => {
      console.log('[OpsWS] Connected')
      reconnectAttempts = 0
    REDACTED

    ws.onmessage = (e) => {
      const data = JSON.parse(e.data)
      onMessage(data)
    REDACTED

    ws.onerror = (error) => {
      console.error('[OpsWS] Connection error:', error)
    REDACTED

    ws.onclose = () => {
      console.log('[OpsWS] Connection closed')
      if (shouldReconnect && reconnectAttempts < maxReconnectAttempts) {
        const delay = Math.min(1000 * Math.pow(2, reconnectAttempts), 30000)
        console.log(`[OpsWS] Reconnecting in ${delayREDACTEDms...`)
        reconnectTimer = setTimeout(() => {
          reconnectAttempts++
          connect()
        REDACTED, delay)
      REDACTED
    REDACTED
  REDACTED

  connect()

  return () => {
    shouldReconnect = false
    if (reconnectTimer) clearTimeout(reconnectTimer)
    if (ws) ws.close()
  REDACTED
REDACTED

export const opsAPI = {
  getMetrics,
  listMetricsHistory,
  listErrors,
  getDashboardOverview,
  getProviderHealth,
  getLatencyHistogram,
  getErrorDistribution,
  subscribeQPS
REDACTED

export default opsAPI
