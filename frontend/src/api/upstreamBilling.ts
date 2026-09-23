import { apiClient } from './client'

export type UpstreamBillingProvider = 'azure' | 'tencent' | 'anthropic' | 'aliyun' | 'volcengine'
export type UpstreamBillingAllocationMode = 'local_weighted' | 'official_only' | 'disabled'

export interface UpstreamBillingBinding {
  resource_id: string
  account_id: number
}

export interface UpstreamBillingConnection {
  id: number
  name: string
  provider: UpstreamBillingProvider
  settings: Record<string, string>
  enabled: boolean
  sync_interval_hours: number
  sync_lookback_months?: number
  allocation_mode?: UpstreamBillingAllocationMode
  last_synced_at?: string | null
  next_sync_at?: string | null
  last_error?: string
  has_credentials: boolean
  bindings: UpstreamBillingBinding[]
}

export interface UpstreamBillingConnectionInput {
  name: string
  provider: UpstreamBillingProvider
  settings: Record<string, string>
  secrets: Record<string, string>
  enabled: boolean
  sync_interval_hours: number
  sync_lookback_months: number
  allocation_mode: UpstreamBillingAllocationMode
  bindings: UpstreamBillingBinding[]
}

export interface UpstreamBillingAccount {
  id: number
  name: string
  platform: string
}

export interface UpstreamBillingAllocation {
  connection_id?: number
  connection_name?: string
  provider?: UpstreamBillingProvider
  resource_id?: string
  account_id?: number | null
  account_name?: string
  api_key_id: number | null
  api_key_name: string
  user_id?: number | null
  requests: number
  tokens: number
  local_cost: string
  allocated_cost: string
  currency: string
  method: 'local_cost_weighted' | 'token_weighted' | 'official_token_weighted' | 'unmatched'
  official_tokens?: number
  local_matched_tokens?: number
  token_status?: UpstreamBillingTokenStatus
  period_start: string
  period_end: string
}

export interface UpstreamBillingBill {
  id?: string
  source_id?: number
  has_raw_source?: boolean
  connection_id: number
  resource_id: string
  description: string
  amount: string
  allocated_cost?: string
  unmatched_cost?: string
  source_amount?: string
  raw_source?: unknown
  usage?: {
    model?: string
    token_type?: string
    tokens?: number | string
    context_window?: string
    service_tier?: string
    inference_geo?: string
    complete?: boolean
  }
  usage_status?: 'available' | 'unavailable' | 'unsupported'
  local_matched_tokens?: number
  token_difference?: number
  token_status?: UpstreamBillingTokenStatus
  currency: string
  period_start: string
  period_end: string
}

export type UpstreamBillingTokenStatus = 'verified' | 'local_exceeds_official' | 'local_category_unverified' | 'dimensions_unverified' | 'incomplete' | 'unavailable' | 'unsupported' | 'no_local_usage' | 'not_checked'

export interface UpstreamBillingBillSource {
  id: number
  raw_source: unknown
  source_amount: string
}

export interface UpstreamBillingTotal {
  currency: string
  official_cost?: string
  allocated_cost: string
  unmatched_cost?: string
}

export interface UpstreamBillingReport {
  month: string
  items: UpstreamBillingAllocation[]
  bills?: UpstreamBillingBill[]
  totals: UpstreamBillingTotal[]
  is_admin: boolean
}

export const upstreamBillingAPI = {
  async summary(month: string, signal?: AbortSignal): Promise<UpstreamBillingReport> {
    const { data } = await apiClient.get<UpstreamBillingReport>('/upstream-billing/summary', {
      params: { month }, signal
    })
    return data
  },
  async source(id: number, signal?: AbortSignal): Promise<UpstreamBillingBillSource> {
    const { data } = await apiClient.get<UpstreamBillingBillSource>(`/upstream-billing/bills/${id}/source`, { signal })
    return data
  },
  async report(month: string, signal?: AbortSignal): Promise<UpstreamBillingReport> {
    const { data } = await apiClient.get<UpstreamBillingReport>('/upstream-billing/report', {
      params: { month }, signal
    })
    return data
  },
  async connections(): Promise<UpstreamBillingConnection[]> {
    const { data } = await apiClient.get<UpstreamBillingConnection[]>('/upstream-billing/connections')
    return data
  },
  async accounts(): Promise<UpstreamBillingAccount[]> {
    const { data } = await apiClient.get<UpstreamBillingAccount[]>('/upstream-billing/accounts')
    return data
  },
  async save(input: UpstreamBillingConnectionInput, id?: number): Promise<UpstreamBillingConnection> {
    const { data } = id
      ? await apiClient.put<UpstreamBillingConnection>(`/upstream-billing/connections/${id}`, input)
      : await apiClient.post<UpstreamBillingConnection>('/upstream-billing/connections', input)
    return data
  },
  async sync(id: number, month: string): Promise<void> {
    // A paginated provider bill can take longer than the normal panel timeout.
    await apiClient.post(`/upstream-billing/connections/${id}/sync`, { month }, { timeout: 180000 })
  }
}
