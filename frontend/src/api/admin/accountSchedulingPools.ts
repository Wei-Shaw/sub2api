import { apiClient } from '../client'

export interface AccountSchedulingMember {
  account_id: number
  pool_id: number | null
  position: number
  name: string
  state: string
  reason: string
  exit_group: string
  queued_at?: string | null
}

export interface AccountSchedulingPool {
  id: number
  name: string
  enabled: boolean
  created_at: string
  updated_at: string
  members?: AccountSchedulingMember[]
}

export interface AccountSchedulingPolicy {
  enabled: boolean
  max_active_accounts: number
  proxy_exit_groups: Record<string, string>
}

const base = '/admin/account-scheduling-pools'
const pendingCreates = new Map<string, string>()

export const accountSchedulingPoolsAPI = {
  async list(): Promise<AccountSchedulingPool[]> {
    const { data } = await apiClient.get<AccountSchedulingPool[]>(base)
    return data
  },
  async create(input: { name: string; enabled: boolean }): Promise<AccountSchedulingPool> {
    const request = JSON.stringify(input)
    const key = pendingCreates.get(request) ?? globalThis.crypto?.randomUUID?.() ?? `pool-${Date.now()}-${Math.random().toString(36).slice(2)}`
    pendingCreates.set(request, key)
    const { data } = await apiClient.post<AccountSchedulingPool>(base, input, {
      headers: { 'Idempotency-Key': key }
    })
    pendingCreates.delete(request)
    return data
  },
  async update(id: number, input: { name: string; enabled: boolean }): Promise<AccountSchedulingPool> {
    const { data } = await apiClient.put<AccountSchedulingPool>(`${base}/${id}`, input)
    return data
  },
  async remove(id: number): Promise<void> {
    await apiClient.delete(`${base}/${id}`)
  },
  async members(poolId: number | null): Promise<AccountSchedulingMember[]> {
    const { data } = await apiClient.get<AccountSchedulingMember[]>(`${base}/members`, {
      params: poolId === null ? {} : { pool_id: poolId }
    })
    return data
  },
  async assign(poolId: number | null, accountIds: number[]): Promise<void> {
    await apiClient.put(`${base}/members`, { pool_id: poolId, account_ids: accountIds })
  },
  async policy(): Promise<AccountSchedulingPolicy> {
    const { data } = await apiClient.get<AccountSchedulingPolicy>(`${base}/policy`)
    return data
  },
  async updatePolicy(policy: AccountSchedulingPolicy): Promise<void> {
    await apiClient.put(`${base}/policy`, policy)
  }
}
