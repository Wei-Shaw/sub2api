/**
 * Admin Channel Monitor API endpoints
 * Handles channel monitor (uptime/health) management for administrators
 */

import { apiClient REDACTED from '../client'

export type Provider = 'openai' | 'anthropic' | 'gemini' | 'grok'
export type MonitorStatus = 'operational' | 'degraded' | 'failed' | 'error'
export type BodyOverrideMode = 'off' | 'merge' | 'replace'
export type APIMode = 'chat_completions' | 'responses'

export interface ChannelMonitor {
  id: number
  name: string
  provider: Provider
  api_mode: APIMode
  endpoint: string
  api_key_masked: string
  /**
   * True when the stored encrypted API key cannot be decrypted (e.g. the
   * encryption key has changed). Admin must re-edit the monitor to provide
   * a fresh key. Backend skips checks for these monitors.
   */
  api_key_decrypt_failed?: boolean
  primary_model: string
  extra_models: string[]
  group_name: string
  enabled: boolean
  interval_seconds: number
  /** 每次调度在 interval 基础上 ± [0, jitter] 的随机偏移（秒），0 = 固定间隔 */
  jitter_seconds: number
  last_checked_at: string | null
  created_by: number
  created_at: string
  updated_at: string
  /** Latest status of the primary model (empty when no history yet) */
  primary_status: MonitorStatus | ''
  /** Latest latency of the primary model in ms (null when no history yet) */
  primary_latency_ms: number | null
  /** Primary model 7-day availability percentage (0-100) */
  availability_7d: number
  /** Latest status per extra model (used for hover tooltip) */
  extra_models_status: ExtraModelStatus[]
  /** 请求自定义快照字段（高级设置） */
  template_id: number | null
  extra_headers: Record<string, string>
  body_override_mode: BodyOverrideMode
  body_override: Record<string, unknown> | null
REDACTED

export interface ExtraModelStatus {
  model: string
  status: MonitorStatus | ''
  latency_ms: number | null
REDACTED

export interface ListParams {
  page?: number
  page_size?: number
  provider?: Provider
  enabled?: boolean
  search?: string
REDACTED

export interface ListResponse {
  items: ChannelMonitor[]
  total: number
  page: number
  page_size: number
  pages: number
REDACTED

export interface CreateParams {
  name: string
  provider: Provider
  api_mode?: APIMode
  endpoint: string
  api_key: string
  primary_model: string
  extra_models?: string[]
  group_name?: string
  enabled?: boolean
  interval_seconds: number
  jitter_seconds?: number
  template_id?: number | null
  extra_headers?: Record<string, string>
  body_override_mode?: BodyOverrideMode
  body_override?: Record<string, unknown> | null
REDACTED

// Update request: api_key 空串 = 不修改；clear_template=true 时把 template_id 置空
export type UpdateParams = Partial<CreateParams> & {
  clear_template?: boolean
REDACTED

export interface CheckResult {
  model: string
  status: MonitorStatus
  latency_ms: number | null
  ping_latency_ms: number | null
  message: string
  checked_at: string
REDACTED

export interface RunNowResponse {
  results: CheckResult[]
REDACTED

export interface HistoryItem {
  id: number
  model: string
  status: MonitorStatus
  latency_ms: number | null
  ping_latency_ms: number | null
  message: string
  checked_at: string
REDACTED

export interface HistoryParams {
  model?: string
  limit?: number
REDACTED

export interface HistoryResponse {
  items: HistoryItem[]
REDACTED

/**
 * List channel monitors with pagination and filters
 */
export async function list(
  params: ListParams = {REDACTED,
  options?: { signal?: AbortSignal REDACTED
): Promise<ListResponse> {
  const { data REDACTED = await apiClient.get<ListResponse>('/admin/channel-monitors', {
    params,
    signal: options?.signal,
  REDACTED)
  return data
REDACTED

/**
 * Get a channel monitor by ID
 */
export async function get(id: number): Promise<ChannelMonitor> {
  const { data REDACTED = await apiClient.get<ChannelMonitor>(`/admin/channel-monitors/${idREDACTED`)
  return data
REDACTED

/**
 * Create a new channel monitor
 */
export async function create(params: CreateParams): Promise<ChannelMonitor> {
  const { data REDACTED = await apiClient.post<ChannelMonitor>('/admin/channel-monitors', params)
  return data
REDACTED

/**
 * Duplicate a monitor without exposing its stored API key to the browser.
 * Keep the operation key after ambiguous failures so a retry replays the
 * original server-side operation instead of creating another monitor.
 */
const duplicateOperationKeys = new Map<string, string>()

interface DuplicateOperationScope {
  adminID: string
  key: string
REDACTED

function getCurrentAdminID(): string | null {
  try {
    const rawUser = globalThis.localStorage?.getItem('auth_user')
    if (!rawUser) return null

    const user: unknown = JSON.parse(rawUser)
    if (typeof user !== 'object' || user === null) return null

    const id = (user as { id?: unknown REDACTED).id
    if (typeof id !== 'number' || !Number.isSafeInteger(id) || id <= 0) return null
    return String(id)
  REDACTED catch {
    return null
  REDACTED
REDACTED

function duplicateOperationScope(id: number): DuplicateOperationScope | null {
  const adminID = getCurrentAdminID()
  if (!adminID) return null

  return {
    adminID,
    key: `sub2api:admin:channel-monitor-duplicate:${adminIDREDACTED:${idREDACTED`,
  REDACTED
REDACTED

function getStoredDuplicateOperationKey(storageKey: string): string | null {
  try {
    return globalThis.sessionStorage?.getItem(storageKey) ?? null
  REDACTED catch {
    return null
  REDACTED
REDACTED

function storeDuplicateOperationKey(storageKey: string, key: string | null): void {
  try {
    if (key) globalThis.sessionStorage?.setItem(storageKey, key)
    else globalThis.sessionStorage?.removeItem(storageKey)
  REDACTED catch {
    // In-memory retry protection still works when browser storage is unavailable.
  REDACTED
REDACTED

export async function duplicate(id: number): Promise<ChannelMonitor> {
  const scope = duplicateOperationScope(id)
  let idempotencyKey = scope
    ? duplicateOperationKeys.get(scope.key) ?? getStoredDuplicateOperationKey(scope.key)
    : null
  if (!idempotencyKey) {
    const requestID = globalThis.crypto?.randomUUID?.() ?? `${Date.now()REDACTED-${Math.random().toString(36).slice(2)REDACTED`
    idempotencyKey = `channel-monitor-duplicate-${scope?.adminID ?? 'unknown-admin'REDACTED-${idREDACTED-${requestIDREDACTED`
  REDACTED
  if (scope) {
    duplicateOperationKeys.set(scope.key, idempotencyKey)
    storeDuplicateOperationKey(scope.key, idempotencyKey)
  REDACTED

  const { data REDACTED = await apiClient.post<ChannelMonitor>(
    `/admin/channel-monitors/${idREDACTED/duplicate`,
    undefined,
    { headers: { 'Idempotency-Key': idempotencyKey REDACTED REDACTED
  )

  if (scope) {
    duplicateOperationKeys.delete(scope.key)
    storeDuplicateOperationKey(scope.key, null)
  REDACTED
  return data
REDACTED

/**
 * Update an existing channel monitor.
 * api_key field: empty string means "do not modify".
 */
export async function update(id: number, params: UpdateParams): Promise<ChannelMonitor> {
  const { data REDACTED = await apiClient.put<ChannelMonitor>(`/admin/channel-monitors/${idREDACTED`, params)
  return data
REDACTED

/**
 * Delete a channel monitor
 */
export async function del(id: number): Promise<void> {
  await apiClient.delete(`/admin/channel-monitors/${idREDACTED`)
REDACTED

/**
 * Trigger an immediate manual check for a channel monitor.
 * Returns the latest check results for primary + extra models.
 */
export async function runNow(id: number): Promise<RunNowResponse> {
  const { data REDACTED = await apiClient.post<RunNowResponse>(`/admin/channel-monitors/${idREDACTED/run`)
  return data
REDACTED

/**
 * List historical check results for a monitor.
 */
export async function listHistory(
  id: number,
  params: HistoryParams = {REDACTED
): Promise<HistoryResponse> {
  const { data REDACTED = await apiClient.get<HistoryResponse>(
    `/admin/channel-monitors/${idREDACTED/history`,
    { params REDACTED
  )
  return data
REDACTED

export const channelMonitorAPI = {
  list,
  get,
  create,
  duplicate,
  update,
  del,
  runNow,
  listHistory,
REDACTED

export default channelMonitorAPI
