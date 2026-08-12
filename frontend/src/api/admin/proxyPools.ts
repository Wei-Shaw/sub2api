/**
 * Admin Proxy Pools API endpoints
 * 代理池管理：池 CRUD、成员分配、健康探测触发、重绑日志
 */

import { apiClient } from '../client'
import type {
  Proxy,
  ProxyPool,
  ProxyPoolWithStats,
  ProxyPoolRebindLog,
  ProxyPoolAccountSummary,
  PaginatedResponse
} from '@/types'

/**
 * List all proxy pools with health stats
 */
export async function list(): Promise<ProxyPoolWithStats[]> {
  const { data } = await apiClient.get<ProxyPoolWithStats[]>('/admin/proxy-pools')
  return data
}

/**
 * Get a single proxy pool
 */
export async function get(id: number): Promise<ProxyPool> {
  const { data } = await apiClient.get<ProxyPool>(`/admin/proxy-pools/${id}`)
  return data
}

/**
 * Create a proxy pool
 */
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

/**
 * Update a proxy pool (omit fields to keep unchanged)
 */
export async function update(
  id: number,
  input: Partial<{
    name: string
    description: string | null
    status: 'active' | 'disabled'
    health_interval_seconds: number
    failure_threshold: number
    auto_rebind: boolean
  }>
): Promise<ProxyPool> {
  const { data } = await apiClient.put<ProxyPool>(`/admin/proxy-pools/${id}`, input)
  return data
}

/**
 * Delete a proxy pool
 */
export async function remove(id: number): Promise<void> {
  await apiClient.delete(`/admin/proxy-pools/${id}`)
}

/**
 * List proxies inside a pool (with account count + latency)
 */
export async function listProxies(id: number): Promise<Proxy[]> {
  const { data } = await apiClient.get<Proxy[]>(`/admin/proxy-pools/${id}/proxies`)
  return data
}

/**
 * List accounts assigned to a pool, including each account's current proxy.
 */
export async function listAccounts(
  id: number,
  page = 1,
  pageSize = 10
): Promise<PaginatedResponse<ProxyPoolAccountSummary>> {
  const { data } = await apiClient.get<PaginatedResponse<ProxyPoolAccountSummary>>(
    `/admin/proxy-pools/${id}/accounts`,
    { params: { page, page_size: pageSize } }
  )
  return data
}

/**
 * Assign proxies to a pool
 */
export async function assignProxies(id: number, proxyIds: number[]): Promise<number> {
  const { data } = await apiClient.post<{ assigned: number }>(`/admin/proxy-pools/${id}/proxies`, {
    proxy_ids: proxyIds
  })
  return data.assigned ?? 0
}

/**
 * Remove proxies from a pool
 */
export async function removeProxies(id: number, proxyIds: number[]): Promise<number> {
  const { data } = await apiClient.delete<{ removed: number }>(`/admin/proxy-pools/${id}/proxies`, {
    data: { proxy_ids: proxyIds }
  })
  return data.removed ?? 0
}

export interface ProxyPoolRebindResult {
  reboundAccounts: number
  partialFailure: boolean
  failedProxies: number
}

/**
 * Trigger one round of health probe + auto rebind manually
 */
export async function rebind(id: number): Promise<ProxyPoolRebindResult> {
  const { data } = await apiClient.post<{
    rebound_accounts: number
    partial_failure: boolean
    failed_proxies: number
  }>(`/admin/proxy-pools/${id}/rebind`)
  return {
    reboundAccounts: data.rebound_accounts ?? 0,
    partialFailure: data.partial_failure ?? false,
    failedProxies: data.failed_proxies ?? 0
  }
}

/**
 * Recent rebind logs of a pool
 */
export async function rebindLogs(id: number, limit = 50): Promise<ProxyPoolRebindLog[]> {
  const { data } = await apiClient.get<ProxyPoolRebindLog[]>(`/admin/proxy-pools/${id}/rebind-logs`, {
    params: { limit }
  })
  return data
}

export default {
  list,
  get,
  create,
  update,
  remove,
  listProxies,
  listAccounts,
  assignProxies,
  removeProxies,
  rebind,
  rebindLogs
}
