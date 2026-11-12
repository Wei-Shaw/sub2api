/**
 * Account traceability API (backend: internal/handler/admin/ops_handler.go)
 * - GET /admin/ops/trace/:request_id        按请求反查（failover 链）
 * - GET /admin/ops/accounts/:id/trace       按账号反查（时间窗）
 * - GET /admin/ops/accounts/:id/ledger      订阅台账（完整历史）
 * - GET /admin/ops/expiring-accounts        到期巡检
 *
 * 所有追溯 UI 模块共用这一份类型与请求函数，模块内不要再重复定义。
 */

import { apiClient } from '../client'

/** 归一化失败类型，与 backend internal/upstream.FailureKind 对齐 */
export type TraceFailureKind = 'none' | 'auth' | 'rate_limit' | 'overload' | 'transient' | 'client'

/** upstream_attempts 一行：一次上游尝试（failover 的每一跳） */
export interface TraceAttempt {
  attempt_no: number
  account_id?: number
  /** 上一跳账号；首跳为空。非空即表示发生了 failover */
  retry_from?: number
  platform: string
  model_req?: string
  model_upstream?: string
  upstream_url?: string
  upstream_status?: number
  failure_kind: TraceFailureKind | string
  error_code?: string
  error_msg?: string
  account_state?: string
  cooling_until?: string
  /** 凭据指纹（sha256 前 8 字节 hex），用于区分「账号坏了」与「这版凭据坏了」 */
  credential_fp?: string
  proxy_id?: number
  created_at: string
  /**
   * 所属请求（追加字段，可选）。按账号反查接口当前未返回该列，
   * 后端补上 traceAttemptColumns 后面板即可直接展示 / 复制。
   */
  request_id?: string
}

/** account_status_events 一行：调度态变化（冷却 / 摘除） */
export interface TraceStatusEvent {
  account_id: number
  /** 持久化态：active / error / disabled */
  from_state?: string
  /** 调度态：cooling / invalid */
  to_state: string
  reason?: string
  error_code?: string
  cooling_until?: string
  /** classify / refresh / manual / expiry / scheduled_test */
  source: string
  request_id?: string
  created_at: string
}

export interface TraceUsage {
  user_id: number
  api_key_id: number
  account_id: number
  model: string
  requested_model?: string
  upstream_model?: string
  input_tokens: number
  output_tokens: number
  total_cost: number
  duration_ms: number
  first_token_ms?: number
  created_at: string
}

export interface TraceError {
  id: number
  error_phase: string
  error_type: string
  status_code?: number
  upstream_status_code?: number
  error_message?: string
  created_at: string
}

export interface RequestTrace {
  request_id: string
  usage?: TraceUsage
  attempts: TraceAttempt[]
  status_events: TraceStatusEvent[]
  error?: TraceError
}

/** 台账事件类型（由 accounts 表上的数据库触发器写入） */
export type LifecycleEventType =
  | 'created'
  | 'deleted'
  | 'restored'
  | 'expired'
  | 'renewed'
  | 'expiry_shortened'
  | 'expiry_set'
  | 'expiry_cleared'
  | 'plan_changed'
  | 'quota_changed'
  | 'disabled'
  | 'errored'
  | 'recovered'
  | 'enabled'
  | 'credential_rotated'
  | 'token_refreshed'
  | 'unschedulable'
  | 'schedulable'
  | 'updated'

/** changes 字段：{ "字段名": { from, to } }；created 事件为快照对象 */
export type LifecycleChanges = Record<string, { from?: unknown; to?: unknown } | unknown>

export interface AccountLifecycleEvent {
  id: number
  account_id: number
  event_type: LifecycleEventType | string
  changes: LifecycleChanges
  status_after?: string
  expires_at_before?: string
  expires_at_after?: string
  credential_fp_before?: string
  credential_fp_after?: string
  source: string
  operator_id?: number
  created_at: string
}

export interface AccountTrace {
  account_id: number
  since: string
  attempts: TraceAttempt[]
  status_events: TraceStatusEvent[]
  lifecycle_events: AccountLifecycleEvent[]
}

export interface AccountLedger {
  account_id: number
  events: AccountLifecycleEvent[]
}

export interface ExpiringAccount {
  id: number
  name: string
  platform: string
  type: string
  status: string
  schedulable: boolean
  expires_at: string
  auto_pause_on_expired: boolean
  expired: boolean
  hours_left: number
}

export async function getRequestTrace(requestId: string): Promise<RequestTrace> {
  const { data } = await apiClient.get<RequestTrace>(`/admin/ops/trace/${encodeURIComponent(requestId)}`)
  return data
}

export async function getAccountTrace(
  accountId: number,
  params: { hours?: number; limit?: number } = {}
): Promise<AccountTrace> {
  const { data } = await apiClient.get<AccountTrace>(`/admin/ops/accounts/${accountId}/trace`, { params })
  return data
}

export async function getAccountLedger(accountId: number, params: { limit?: number } = {}): Promise<AccountLedger> {
  const { data } = await apiClient.get<AccountLedger>(`/admin/ops/accounts/${accountId}/ledger`, { params })
  return data
}

export async function listExpiringAccounts(params: { days?: number; limit?: number } = {}): Promise<ExpiringAccount[]> {
  const { data } = await apiClient.get<ExpiringAccount[]>('/admin/ops/expiring-accounts', { params })
  return data
}

export const opsTraceAPI = {
  getRequestTrace,
  getAccountTrace,
  getAccountLedger,
  listExpiringAccounts
}

export default opsTraceAPI
