/**
 * Admin Channel Monitor API endpoints
 * Handles channel monitor (uptime/health) management for administrators
 */

import { apiClient } from '../client'

export type Provider =
  | 'openai'
  | 'anthropic'
  | 'gemini'
  | 'grok'
  | 'antigravity'
  | 'kimi'
  | 'zhipu'
  | 'deepseek'
export type MonitorStatus = 'operational' | 'degraded' | 'failed' | 'error'
export type BodyOverrideMode = 'off' | 'merge' | 'replace'
export type APIMode = 'chat_completions' | 'responses'
/**
 * probe = LLM 探活（默认）；quota = 仅查关联账号用量（零 LLM 成本）；
 * quota_probe = 探活 + 配额快照挂主模型行。
 */
export type CheckMode = 'probe' | 'quota' | 'quota_probe'

/** 配额快照中的单个用量窗口（与后端 domain.MonitorQuotaTier 一致）。 */
export interface MonitorQuotaTier {
  /** 5h | 7d | 7d-sonnet | 7d-fable | 30d | daily | weekly | total */
  window: string
  /** 同窗口多档时的机器标识（gemini shared/pro/flash、grok requests/tokens、antigravity 模型名） */
  label?: string
  used_percent: number
  used?: number
  limit?: number
  /** RFC3339；空表示无重置时间 */
  reset_at?: string
}

export interface MonitorBalance {
  currency: string
  balance: number
}

/** 归一化配额快照（与后端 domain.MonitorQuotaSnapshot 一致）。 */
export interface MonitorQuotaSnapshot {
  /** usage | cn_quota | cn_balance */
  source: string
  success: boolean
  tiers?: MonitorQuotaTier[]
  balance?: number | null
  balances?: MonitorBalance[]
  currency?: string
  plan_level?: string
  /** 401/403 鉴权失败标记（推导为 failed 状态） */
  credential_invalid?: boolean
  error?: string
  fetched_at: string
}

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
  /** 检测模式：probe（默认）/ quota / quota_probe */
  check_mode: CheckMode
  /** 配额模式关联的账号 ID；探活模式为 null */
  account_id: number | null
  /** 主模型最近一次配额快照（配额模式；无历史时为 null） */
  latest_quota?: MonitorQuotaSnapshot | null
}

export interface ExtraModelStatus {
  model: string
  status: MonitorStatus | ''
  latency_ms: number | null
}

export interface ListParams {
  page?: number
  page_size?: number
  provider?: Provider
  enabled?: boolean
  search?: string
}

export interface ListResponse {
  items: ChannelMonitor[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface CreateParams {
  name: string
  provider: Provider
  api_mode?: APIMode
  /** 探活模式必填（base origin）；quota 模式可留空 */
  endpoint: string
  /** 探活模式必填；quota 模式可留空 */
  api_key: string
  /** 缺省 probe；antigravity 仅支持 quota */
  check_mode?: CheckMode
  /** 配额模式必填：数据源账号（provider 需与账号平台一致）。
   * update 语义：>0=换绑，0=解绑（切回 probe 模式时前端发 0 清空存量关联）；
   * create 绝不发 0——后端会把 0 存成 &0 触发外键违约。 */
  account_id?: number | null
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
}

// Update request: api_key 空串 = 不修改；clear_template=true 时把 template_id 置空；
// account_id=0 显式解绑关联账号（null = 不动，见 CreateParams 注释）
export type UpdateParams = Partial<CreateParams> & {
  clear_template?: boolean
}

export interface CheckResult {
  model: string
  status: MonitorStatus
  latency_ms: number | null
  ping_latency_ms: number | null
  message: string
  checked_at: string
  /** 配额模式（quota / quota_probe 主模型行）附带的配额快照 */
  quota?: MonitorQuotaSnapshot | null
}

export interface RunNowResponse {
  results: CheckResult[]
}

export interface HistoryItem {
  id: number
  model: string
  status: MonitorStatus
  latency_ms: number | null
  ping_latency_ms: number | null
  message: string
  checked_at: string
  /** 配额快照（配额模式行；探活行为空） */
  quota?: MonitorQuotaSnapshot | null
}

export interface HistoryParams {
  model?: string
  limit?: number
}

export interface HistoryResponse {
  items: HistoryItem[]
}

/**
 * List channel monitors with pagination and filters
 */
export async function list(
  params: ListParams = {},
  options?: { signal?: AbortSignal }
): Promise<ListResponse> {
  const { data } = await apiClient.get<ListResponse>('/admin/channel-monitors', {
    params,
    signal: options?.signal,
  })
  return data
}

/**
 * Get a channel monitor by ID
 */
export async function get(id: number): Promise<ChannelMonitor> {
  const { data } = await apiClient.get<ChannelMonitor>(`/admin/channel-monitors/${id}`)
  return data
}

/**
 * Create a new channel monitor
 */
export async function create(params: CreateParams): Promise<ChannelMonitor> {
  const { data } = await apiClient.post<ChannelMonitor>('/admin/channel-monitors', params)
  return data
}

/**
 * Duplicate a monitor without exposing its stored API key to the browser.
 * Keep the operation key after ambiguous failures so a retry replays the
 * original server-side operation instead of creating another monitor.
 */
const duplicateOperationKeys = new Map<string, string>()

interface DuplicateOperationScope {
  adminID: string
  key: string
}

function getCurrentAdminID(): string | null {
  try {
    const rawUser = globalThis.localStorage?.getItem('auth_user')
    if (!rawUser) return null

    const user: unknown = JSON.parse(rawUser)
    if (typeof user !== 'object' || user === null) return null

    const id = (user as { id?: unknown }).id
    if (typeof id !== 'number' || !Number.isSafeInteger(id) || id <= 0) return null
    return String(id)
  } catch {
    return null
  }
}

function duplicateOperationScope(id: number): DuplicateOperationScope | null {
  const adminID = getCurrentAdminID()
  if (!adminID) return null

  return {
    adminID,
    key: `sub2api:admin:channel-monitor-duplicate:${adminID}:${id}`,
  }
}

function getStoredDuplicateOperationKey(storageKey: string): string | null {
  try {
    return globalThis.sessionStorage?.getItem(storageKey) ?? null
  } catch {
    return null
  }
}

function storeDuplicateOperationKey(storageKey: string, key: string | null): void {
  try {
    if (key) globalThis.sessionStorage?.setItem(storageKey, key)
    else globalThis.sessionStorage?.removeItem(storageKey)
  } catch {
    // In-memory retry protection still works when browser storage is unavailable.
  }
}

export async function duplicate(id: number): Promise<ChannelMonitor> {
  const scope = duplicateOperationScope(id)
  let idempotencyKey = scope
    ? duplicateOperationKeys.get(scope.key) ?? getStoredDuplicateOperationKey(scope.key)
    : null
  if (!idempotencyKey) {
    const requestID = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
    idempotencyKey = `channel-monitor-duplicate-${scope?.adminID ?? 'unknown-admin'}-${id}-${requestID}`
  }
  if (scope) {
    duplicateOperationKeys.set(scope.key, idempotencyKey)
    storeDuplicateOperationKey(scope.key, idempotencyKey)
  }

  const { data } = await apiClient.post<ChannelMonitor>(
    `/admin/channel-monitors/${id}/duplicate`,
    undefined,
    { headers: { 'Idempotency-Key': idempotencyKey } }
  )

  if (scope) {
    duplicateOperationKeys.delete(scope.key)
    storeDuplicateOperationKey(scope.key, null)
  }
  return data
}

/**
 * Update an existing channel monitor.
 * api_key field: empty string means "do not modify".
 */
export async function update(id: number, params: UpdateParams): Promise<ChannelMonitor> {
  const { data } = await apiClient.put<ChannelMonitor>(`/admin/channel-monitors/${id}`, params)
  return data
}

/**
 * Delete a channel monitor
 */
export async function del(id: number): Promise<void> {
  await apiClient.delete(`/admin/channel-monitors/${id}`)
}

/**
 * Trigger an immediate manual check for a channel monitor.
 * Returns the latest check results for primary + extra models.
 */
export async function runNow(id: number): Promise<RunNowResponse> {
  const { data } = await apiClient.post<RunNowResponse>(`/admin/channel-monitors/${id}/run`)
  return data
}

/**
 * List historical check results for a monitor.
 */
export async function listHistory(
  id: number,
  params: HistoryParams = {}
): Promise<HistoryResponse> {
  const { data } = await apiClient.get<HistoryResponse>(
    `/admin/channel-monitors/${id}/history`,
    { params }
  )
  return data
}

export const channelMonitorAPI = {
  list,
  get,
  create,
  duplicate,
  update,
  del,
  runNow,
  listHistory,
}

export default channelMonitorAPI
