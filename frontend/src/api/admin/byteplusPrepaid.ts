import { apiClient } from '../client'

export interface PrepaidPool {
  id: string
  name: string
  api_base: string
  enforced: boolean
  frozen: boolean
  safety_lock: boolean
  reason: string
  reconcile_after: string | null
}
export interface PrepaidPolicy { model: string; name: string; minimum_validity_seconds: number }
export interface PrepaidGrant {
  verified_at?: string
  eligibility_reason?: string
  id: string
  model: string
  capacity_tokens: string
  spent_tokens: string
  reserved_tokens: string
  available_tokens: string
  expires_at: string
  eligible: boolean
  needs_verification: boolean
}
export interface PrepaidJob {
  id: string
  model: string
  state: string
  task_id: string | null
  reserved_tokens: string
  ceiling_tokens: string
  actual_tokens: string | null
  created_at: string
  valid_until: string
}
export interface PrepaidStatus {
  observed_at?: string
  models?: { model: string; spendable_tokens: string; held_tokens: string; next_expiry: string | null }[]
  pool: PrepaidPool | null
  credential_matches: boolean
  policies: PrepaidPolicy[]
  grants: PrepaidGrant[]
  jobs: PrepaidJob[]
  open_jobs: number
  bound_accounts: number
  credentials: number
}
export interface PrepaidInput {
  pool_id?: string
  name?: string
  pack_id?: string
  model?: string
  remaining_tokens?: string
  expires_at?: string
  confirmed?: boolean
  allow_pay_as_you_go?: boolean
}
export type PrepaidAction = 'enable' | 'link' | 'grant' | 'verify-grant' | 'freeze' | 'resume' | 'disable'
const root = (id: number) => `/admin/accounts/${id}/byteplus-prepaid`
export async function getStatus(id: number): Promise<PrepaidStatus> {
  return (await apiClient.get<PrepaidStatus>(root(id))).data
}
export async function listPools(id: number): Promise<PrepaidPool[]> {
  return (await apiClient.get<PrepaidPool[]>(`${root(id)}/pools`)).data
}
export async function update(id: number, action: PrepaidAction, input: PrepaidInput = {}): Promise<PrepaidStatus> {
  return (await apiClient.post<PrepaidStatus>(`${root(id)}/${action}`, input)).data
}
export async function reconcile(id: number, reservation: string): Promise<PrepaidStatus> {
  return (await apiClient.post<PrepaidStatus>(`${root(id)}/reservations/${encodeURIComponent(reservation)}/reconcile`)).data
}

export interface BillingSource { state: string; observed_at: string | null; attempted_at: string | null; data: Record<string, unknown> | Record<string, unknown>[] | null }
export interface BillingStatus { configured: boolean; setup: string; balance: BillingSource; packages: BillingSource; coupons: BillingSource }
export async function providerStatus(id: number): Promise<BillingStatus> {
 return (await apiClient.get<BillingStatus>(`${root(id)}/provider-status`)).data
}
export async function queryProvider(id: number): Promise<BillingStatus> {
 return (await apiClient.post<BillingStatus>(`${root(id)}/provider-query`, undefined, { timeout: 45000 })).data
}
