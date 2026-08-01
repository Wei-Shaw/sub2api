/**
 * Admin Cloudflare WARP gateway API
 */

import { apiClient } from '../client'

export interface WarpInstance {
  id: string
  name: string
  listen_host: string
  listen_port: number
  status: string
  exit_ip?: string
  exit_colo?: string
  last_error?: string
}

export interface WarpPoolSnapshot {
  instances: WarpInstance[]
  socks_urls: string[]
  unhealthy_ids: string[]
  duplicate_exit_ips: Record<string, string[]>
  healthy_count: number
  total_count: number
}

export interface WarpProxySpec {
  name: string
  protocol: string
  host: string
  port: number
  warp_id: string
  exit_ip: string
  status: string
}

export interface WarpSyncResult {
  snapshot?: WarpPoolSnapshot
  plan?: {
    proxy_specs?: WarpProxySpec[]
    detach_proxy_names?: string[]
    duplicate_exit_ips?: Record<string, string[]>
    suggested_group_name?: string
  }
  created_proxies?: { id: number; name: string; status: string }[]
  updated_proxies?: { id: number; name: string; status: string }[]
  group?: { id: number; name: string; proxy_count?: number }
  member_ids?: number[]
  detached_ids?: number[]
  alerts?: string[]
}

export interface WarpBindResult {
  group_id: number
  group_name: string
  updated_ids: number[]
  skipped_ids?: number[]
  failed?: string[]
}

export async function status(): Promise<{ enabled: boolean }> {
  const { data } = await apiClient.get<{ enabled: boolean }>('/admin/warp/status')
  return data
}

export async function snapshot(): Promise<WarpPoolSnapshot> {
  const { data } = await apiClient.get<WarpPoolSnapshot>('/admin/warp/snapshot')
  return data
}

export async function listInstances(): Promise<WarpInstance[]> {
  const { data } = await apiClient.get<{ instances: WarpInstance[] }>('/admin/warp/instances')
  return data.instances || []
}

export async function sync(groupName?: string): Promise<WarpSyncResult> {
  const { data } = await apiClient.post<WarpSyncResult>('/admin/warp/sync', {
    group_name: groupName || undefined
  })
  return data
}

export async function createPool(payload: {
  name_prefix?: string
  count: number
  group_name?: string
  register?: boolean
}): Promise<WarpSyncResult> {
  const { data } = await apiClient.post<WarpSyncResult>('/admin/warp/pools', payload)
  return data
}

/** One-click free WARP register + pool + sync */
export async function registerPool(payload: {
  name_prefix?: string
  count: number
  group_name?: string
}): Promise<WarpSyncResult> {
  const { data } = await apiClient.post<WarpSyncResult>('/admin/warp/register-pool', payload)
  return data
}

export async function bindAccounts(payload: {
  account_ids?: number[]
  group_name?: string
  bind_all_active?: boolean
}): Promise<WarpBindResult> {
  const { data } = await apiClient.post<WarpBindResult>('/admin/warp/bind-accounts', payload)
  return data
}

export async function healthSync(groupName?: string): Promise<WarpSyncResult> {
  const { data } = await apiClient.post<WarpSyncResult>('/admin/warp/health-sync', {
    group_name: groupName || undefined
  })
  return data
}

export async function rotate(id: string, groupName?: string): Promise<WarpSyncResult> {
  const { data } = await apiClient.post<WarpSyncResult>(`/admin/warp/instances/${id}/rotate`, {
    group_name: groupName || undefined
  })
  return data
}

export async function attachPlan(groupName?: string): Promise<{
  snapshot: WarpPoolSnapshot
  plan: WarpSyncResult['plan']
}> {
  const { data } = await apiClient.get('/admin/warp/attach-plan', {
    params: groupName ? { group_name: groupName } : undefined
  })
  return data
}

export const warpAPI = {
  status,
  snapshot,
  listInstances,
  sync,
  createPool,
  registerPool,
  bindAccounts,
  healthSync,
  rotate,
  attachPlan
}

export default warpAPI
