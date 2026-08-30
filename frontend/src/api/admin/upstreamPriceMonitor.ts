import { apiClient } from '../client'

export type UpstreamPriceMonitorMode = 'observe' | 'auto_apply'
export type UpstreamPriceMonitorRuntimeStatus =
  | 'idle'
  | 'running'
  | 'degraded'
  | 'failed'
  | 'disabled'
export type UpstreamPriceEvidenceStatus =
  | 'trusted'
  | 'pending'
  | 'mismatch'
  | 'stale'
  | 'unobservable'
export type UpstreamPriceEvidenceSource = 'user_request' | 'active_probe' | 'price_page'
export type UpstreamPriceRunStatus =
  | 'running'
  | 'completed'
  | 'partial'
  | 'failed'

export interface UpstreamPriceMonitorConfig {
  enabled: boolean
  mode: UpstreamPriceMonitorMode
  interval_minutes: number
  markup: number
  display_multiplier_decimals: number
  account_ids: number[]
  channel_ids: number[]
  domestic_models: string[]
  passive_sample_max_age_minutes: number
  active_probe_enabled: boolean
  updated_at?: string
}

export interface UpstreamPriceCoverage {
  trusted: number
  total: number
}

export interface UpstreamPriceMonitorRuntime {
  status: UpstreamPriceMonitorRuntimeStatus
  last_run_at: string | null
  next_run_at: string | null
  consecutive_failures: number
  last_error: string
  today_probe_cost: number
  coverage: UpstreamPriceCoverage
  current_run_id?: number | null
  key_exclusive?: boolean | null
  reconciliation_status?: string
}

export interface UpstreamPriceValues {
  input_per_million?: number | null
  output_per_million?: number | null
  cache_write_per_million?: number | null
  cache_read_per_million?: number | null
  per_request_lte_256k?: number | null
  per_request_256k_512k?: number | null
  per_request_gt_512k?: number | null
}

export interface UpstreamPriceUsageCounters {
  requests: number
  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
  actual_cost: number
}

export interface UpstreamPriceEvidence {
  model: string
  account_id: number
  billing_mode?: 'token' | 'per_request'
  status: UpstreamPriceEvidenceStatus
  source: UpstreamPriceEvidenceSource
  reconciliation_status?: string
  observed_at: string | null
  sample_count: number
  prices?: UpstreamPriceValues | null
  remote_delta?: UpstreamPriceUsageCounters | null
  local_delta?: UpstreamPriceUsageCounters | null
  current_prices?: UpstreamPriceValues | null
  suggested_prices?: UpstreamPriceValues | null
  display_multiplier_current?: number | null
  display_multiplier_suggested?: number | null
  last_error?: string
}

export interface UpstreamPriceRunSummary {
  changed_models?: number
  skipped_models?: number
  failed_models?: number
  applied_models?: number
  [key: string]: unknown
}

export interface UpstreamPriceMonitorRun {
  id: number
  trigger: 'scheduled' | 'manual' | 'active_probe' | string
  status: UpstreamPriceRunStatus
  mode: UpstreamPriceMonitorMode | string
  dry_run?: boolean
  started_at: string
  finished_at?: string | null
  matched_models: number
  mismatched_models: number
  probed_models?: number
  probe_cost: number
  snapshot_hash?: string
  applied_at?: string | null
  rollback_available?: boolean
  error?: string
  summary?: UpstreamPriceRunSummary | null
}

export interface UpstreamPriceEvidenceListResponse {
  items: UpstreamPriceEvidence[]
}

export interface UpstreamPriceRunListParams {
  page?: number
  page_size?: number
  status?: UpstreamPriceRunStatus | ''
}

export interface UpstreamPriceRunListResponse {
  items: UpstreamPriceMonitorRun[]
  total: number
  page?: number
  page_size?: number
}

export interface CreateUpstreamPriceRunRequest {
  dry_run: true
  model_names?: string[]
}

export interface ApplyUpstreamPriceRunRequest {
  snapshot_hash: string
}

export type UpstreamPriceModelStatus =
  | 'managed'
  | 'discovered'
  | 'suspected_retired'
  | 'ignored'
  | 'retired'

export interface UpstreamPriceModelCatalogEntry {
  model: string
  status: UpstreamPriceModelStatus
  domestic_candidate: boolean
  seen_account_count: number
  expected_account_count: number
  missing_runs: number
  discovery_complete: boolean
  first_seen_at?: string | null
  last_seen_at?: string | null
  last_missing_at?: string | null
  updated_at: string
}

const basePath = '/admin/upstream-price-monitor'

export async function getConfig(options?: { signal?: AbortSignal }): Promise<UpstreamPriceMonitorConfig> {
  const { data } = await apiClient.get<UpstreamPriceMonitorConfig>(`${basePath}/config`, {
    signal: options?.signal,
  })
  return data
}

export async function updateConfig(
  config: UpstreamPriceMonitorConfig,
): Promise<UpstreamPriceMonitorConfig> {
  const { data } = await apiClient.put<UpstreamPriceMonitorConfig>(`${basePath}/config`, config)
  return data
}

export async function getRuntime(options?: { signal?: AbortSignal }): Promise<UpstreamPriceMonitorRuntime> {
  const { data } = await apiClient.get<UpstreamPriceMonitorRuntime>(`${basePath}/runtime`, {
    signal: options?.signal,
  })
  return data
}

export async function listEvidence(options?: { signal?: AbortSignal }): Promise<UpstreamPriceEvidenceListResponse> {
  const { data } = await apiClient.get<UpstreamPriceEvidenceListResponse>(`${basePath}/evidence`, {
    signal: options?.signal,
  })
  return data
}

export async function listRuns(
  params: UpstreamPriceRunListParams = {},
  options?: { signal?: AbortSignal },
): Promise<UpstreamPriceRunListResponse> {
  const { data } = await apiClient.get<UpstreamPriceRunListResponse>(`${basePath}/runs`, {
    params,
    signal: options?.signal,
  })
  return data
}

export async function listModels(options?: { signal?: AbortSignal }): Promise<{ items: UpstreamPriceModelCatalogEntry[] }> {
  const { data } = await apiClient.get<{ items: UpstreamPriceModelCatalogEntry[] }>(`${basePath}/models`, {
    signal: options?.signal,
  })
  return data
}

export async function updateModelStatus(
  model: string,
  status: UpstreamPriceModelStatus,
): Promise<UpstreamPriceModelCatalogEntry> {
  const { data } = await apiClient.post<UpstreamPriceModelCatalogEntry>(`${basePath}/models/status`, { model, status })
  return data
}

export async function discoverModels(): Promise<{ items: UpstreamPriceModelCatalogEntry[] }> {
  const { data } = await apiClient.post<{ items: UpstreamPriceModelCatalogEntry[] }>(`${basePath}/models/discover`)
  return data
}

export async function createRun(
  request: CreateUpstreamPriceRunRequest = { dry_run: true },
): Promise<UpstreamPriceMonitorRun> {
  const { data } = await apiClient.post<UpstreamPriceMonitorRun>(`${basePath}/runs`, request)
  return data
}

export async function applyRun(
  id: number,
  request: ApplyUpstreamPriceRunRequest,
): Promise<UpstreamPriceMonitorRun> {
  const { data } = await apiClient.post<UpstreamPriceMonitorRun>(
    `${basePath}/runs/${encodeURIComponent(String(id))}/apply`,
    request,
  )
  return data
}

export async function rollbackRun(
  id: number,
  request: ApplyUpstreamPriceRunRequest,
): Promise<UpstreamPriceMonitorRun> {
  const { data } = await apiClient.post<UpstreamPriceMonitorRun>(
    `${basePath}/runs/${encodeURIComponent(String(id))}/rollback`,
    request,
  )
  return data
}

export const upstreamPriceMonitorAPI = {
  getConfig,
  updateConfig,
  getRuntime,
  listEvidence,
  listRuns,
  listModels,
  updateModelStatus,
  discoverModels,
  createRun,
  applyRun,
  rollbackRun,
}

export default upstreamPriceMonitorAPI
