import { apiClient } from '../client'
import type {
  ProxyPool,
  ProxyPoolWithStats,
  ProxyPoolProxy,
  ProxyPoolRebindLog,
  ProxyPoolBindResult
} from '@/types'

export async function list(): Promise<ProxyPoolWithStats[]> {
  const { data } = await apiClient.get<ProxyPoolWithStats[]>('/admin/proxy-pools')
  return data
}

export async function get(id: number): Promise<ProxyPool> {
  const { data } = await apiClient.get<ProxyPool>(`/admin/proxy-pools/${id}`)
  return data
}

export async function create(input: {
  name: string
  description?: string | null
  status?: 'active' | 'disabled'
  health_interval_seconds?: number
  failure_threshold?: number
  auto_rebind?: boolean
}): Promise<ProxyPool> {
  const { data } = await apiClient.post<ProxyPool>('/admin/proxy-pools', input)
  return data
}

export async function update(id: number, input: Partial<{
  name: string
  description: string | null
  status: 'active' | 'disabled'
  health_interval_seconds: number
  failure_threshold: number
  auto_rebind: boolean
}>): Promise<ProxyPool> {
  const { data } = await apiClient.put<ProxyPool>(`/admin/proxy-pools/${id}`, input)
  return data
}

export async function remove(id: number): Promise<void> {
  await apiClient.delete(`/admin/proxy-pools/${id}`)
}

export async function listProxies(id: number): Promise<ProxyPoolProxy[]> {
  const { data } = await apiClient.get<ProxyPoolProxy[]>(`/admin/proxy-pools/${id}/proxies`)
  return data
}

export async function assignProxies(id: number, proxyIds: number[]): Promise<number> {
  const { data } = await apiClient.post<{ assigned: number }>(`/admin/proxy-pools/${id}/proxies`, { proxy_ids: proxyIds })
  return data.assigned ?? 0
}

export async function removeProxies(id: number, proxyIds: number[]): Promise<number> {
  const { data } = await apiClient.delete<{ removed: number }>(`/admin/proxy-pools/${id}/proxies`, { data: { proxy_ids: proxyIds } })
  return data.removed ?? 0
}

export async function bindAccounts(id: number, accountIds: number[]): Promise<ProxyPoolBindResult> {
  const { data } = await apiClient.post<ProxyPoolBindResult>(`/admin/proxy-pools/${id}/accounts`, { account_ids: accountIds })
  return data
}

export async function unbindAccounts(id: number, accountIds: number[]): Promise<number> {
  const { data } = await apiClient.delete<{ unbound: number }>(`/admin/proxy-pools/${id}/accounts`, { data: { account_ids: accountIds } })
  return data.unbound ?? 0
}

export async function rebind(id: number): Promise<number> {
  const { data } = await apiClient.post<{ rebound_accounts: number }>(`/admin/proxy-pools/${id}/rebind`)
  return data.rebound_accounts ?? 0
}

export async function rebindLogs(id: number, limit = 50): Promise<ProxyPoolRebindLog[]> {
  const { data } = await apiClient.get<ProxyPoolRebindLog[]>(`/admin/proxy-pools/${id}/rebind-logs`, { params: { limit } })
  return data
}

export default {
  list,
  get,
  create,
  update,
  remove,
  listProxies,
  assignProxies,
  removeProxies,
  bindAccounts,
  unbindAccounts,
  rebind,
  rebindLogs
}
