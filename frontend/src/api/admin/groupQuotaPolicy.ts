import { apiClient } from '../client'

export interface QuotaCondition {
  window: '5h' | '7d'
  trigger_remaining: number
  recover_remaining: number
}

export interface QuotaPolicyRule {
  id: string
  name: string
  priority: number
  mode: 'disabled' | 'observe' | 'enforce'
  account_ids: number[]
  account_match: 'any' | 'all'
  window_match: 'any' | 'all'
  conditions: QuotaCondition[]
  recovery: 'auto' | 'manual'
  mappings: { source: string; target: string }[]
}

export interface QuotaPolicyConfig { rules: QuotaPolicyRule[] }
export interface QuotaPolicyAccount { id: number; name: string; status: string }
export interface QuotaPolicyWindow { remaining_percent: number; reset_at: number }
export interface QuotaPolicySnapshot {
  account_id: number
  sampled_at: number
  checked_at: number
  error_code?: string
  five_hour?: QuotaPolicyWindow
  seven_day?: QuotaPolicyWindow
}
export interface QuotaPolicyState {
  rule_id: string
  active: boolean
  suppressed: boolean
  status: string
  changed_at: number
  checked_at: number
}
export interface QuotaPolicyRuntime {
  snapshots: QuotaPolicySnapshot[]
  rules: QuotaPolicyState[]
  events: { at: number; rule_id: string; type: string; message: string }[]
}
export interface QuotaPolicyView {
  group_id: number
  revision: number
  config: QuotaPolicyConfig
  runtime: QuotaPolicyRuntime
  accounts: QuotaPolicyAccount[]
}
export interface QuotaPolicyPreview {
  runtime: QuotaPolicyRuntime
  resolution: {
    applied: boolean
    source_model: string
    target_model: string
    rule_id: string
    rule_name: string
    revision: number
  }
}

const endpoint = (groupId: number) => `/admin/groups/${groupId}/quota-policy`

export async function get(groupId: number): Promise<QuotaPolicyView> {
  return (await apiClient.get<QuotaPolicyView>(endpoint(groupId))).data
}

export async function save(groupId: number, revision: number, config: QuotaPolicyConfig): Promise<QuotaPolicyView> {
  return (await apiClient.put<QuotaPolicyView>(endpoint(groupId), { revision, config })).data
}

export async function preview(groupId: number, config: QuotaPolicyConfig, model: string): Promise<QuotaPolicyPreview> {
  return (await apiClient.post<QuotaPolicyPreview>(`${endpoint(groupId)}/preview`, { config, model })).data
}

export async function refresh(groupId: number): Promise<QuotaPolicyView> {
  return (await apiClient.post<QuotaPolicyView>(`${endpoint(groupId)}/refresh`, {}, { timeout: 60000 })).data
}

export async function release(groupId: number, revision: number, ruleId: string): Promise<QuotaPolicyView> {
  return (await apiClient.post<QuotaPolicyView>(`${endpoint(groupId)}/release`, { revision, rule_id: ruleId })).data
}

export interface QuotaModelCatalog {
  group_id: number
  checked_at: number
  models: { id: string; account_ids: number[]; accounts?: { account_id: number; upstream_model: string }[] }[]
  accounts: { id: number; name: string; status: string; schedulable: boolean; catalog_status?: string; error_code?: string; models: { id: string; upstream_model: string }[] }[]
  warnings?: string[]
}

export interface QuotaMappingCheck {
  source: string
  target: string
  status: 'passed' | 'partial' | 'failed' | 'untested'
  checked_at: number
  expires_at?: number
  accounts: {
    account_id: number
    account_name: string
    upstream_model: string
    status: 'passed' | 'failed' | 'unsupported' | 'skipped'
    latency_ms?: number
    error_code?: string
    message?: string
    cached?: boolean
  }[]
  warnings?: string[]
}

export async function getModels(groupId: number, signal?: AbortSignal): Promise<QuotaModelCatalog> {
  return (await apiClient.get<QuotaModelCatalog>(`${endpoint(groupId)}/models`, { signal, timeout: 40000 })).data
}

export async function validateMapping(groupId: number, mapping: { source: string; target: string; account_ids: number[]; force?: boolean }, signal?: AbortSignal): Promise<QuotaMappingCheck> {
  return (await apiClient.post<QuotaMappingCheck>(`${endpoint(groupId)}/validate-mapping`, mapping, { signal, timeout: 40000 })).data
}
