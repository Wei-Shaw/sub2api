import { apiClient } from '../client'
import type { DynamicProxyPool } from '@/types'

export interface DynamicProxyPoolListParams {
  page?: number
  page_size?: number
  search?: string
  enabled?: boolean
}

export interface CreateDynamicProxyPoolRequest {
  name: string
  source_type?: string
  subscription_id?: number | null
  extract_url?: string
  protocol?: string
  auth_mode?: string
  username?: string
  password?: string
  response_format?: string
  line_separator?: string
  ip_field_path?: string
  port_field_path?: string
  refresh_interval_sec?: number
  ip_duration_sec?: number
  extract_count?: number
  min_alive?: number
  health_check_interval_sec?: number
}

export interface UpdateDynamicProxyPoolRequest {
  name?: string
  enabled?: boolean
  source_type?: string
  subscription_id?: number | null
  extract_url?: string
  protocol?: string
  auth_mode?: string
  username?: string
  password?: string
  response_format?: string
  line_separator?: string
  ip_field_path?: string
  port_field_path?: string
  refresh_interval_sec?: number
  ip_duration_sec?: number
  extract_count?: number
  min_alive?: number
  health_check_interval_sec?: number
}

export interface DynamicProxyPoolExtractResult {
  created: number
  failed: number
  alive_count: number
}

const BASE = '/admin/dynamic-proxy-pools'

export const dynamicProxyPoolsAPI = {
  async list(params: DynamicProxyPoolListParams = {}) {
    const q = new URLSearchParams()
    if (params.page) q.set('page', String(params.page))
    if (params.page_size) q.set('page_size', String(params.page_size))
    if (params.search) q.set('search', params.search)
    if (params.enabled !== undefined) q.set('enabled', params.enabled ? 'true' : 'false')
    const res = await apiClient.get(`${BASE}?${q.toString()}`)
    return res.data as { items: DynamicProxyPool[]; total: number; page: number; page_size: number }
  },

  async getById(id: number) {
    const res = await apiClient.get(`${BASE}/${id}`)
    return res.data as DynamicProxyPool
  },

  async create(data: CreateDynamicProxyPoolRequest) {
    const res = await apiClient.post(BASE, data)
    return res.data as DynamicProxyPool
  },

  async update(id: number, data: UpdateDynamicProxyPoolRequest) {
    const res = await apiClient.put(`${BASE}/${id}`, data)
    return res.data as DynamicProxyPool
  },

  async delete(id: number) {
    await apiClient.delete(`${BASE}/${id}`)
  },

  async extract(id: number) {
    const res = await apiClient.post(`${BASE}/${id}/extract`)
    return res.data as DynamicProxyPoolExtractResult
  },

  async startEntryProxy(id: number, bindAddr: string) {
    const res = await apiClient.post(`${BASE}/${id}/entry-proxy/start`, { bind_addr: bindAddr })
    return res.data as { bind_addr: string; status: string }
  },

  async stopEntryProxy(id: number) {
    const res = await apiClient.post(`${BASE}/${id}/entry-proxy/stop`)
    return res.data as { status: string }
  },

  async listProxies(id: number) {
    const res = await apiClient.get(`${BASE}/${id}/proxies`)
    return res.data as { items: any[]; total: number }
  },

  async associateProxies(id: number, proxyIds: number[]) {
    const res = await apiClient.post(`${BASE}/${id}/proxies`, { proxy_ids: proxyIds })
    return res.data as { created: number; failed: number; alive_count: number }
  },

  async disassociateProxies(id: number, proxyIds: number[]) {
    const res = await apiClient.delete(`${BASE}/${id}/proxies`, { data: { proxy_ids: proxyIds } })
    return res.data as { alive_count: number }
  },

  async previewNodes(id: number) {
    const res = await apiClient.post(`${BASE}/${id}/preview-nodes`)
    return res.data as { nodes: Array<{ identity: string; name: string; type: string; server: string; port: string }>; total: number }
  },

  async addNodes(id: number, identities: string[]) {
    const res = await apiClient.post(`${BASE}/${id}/add-nodes`, { identities })
    return res.data as { created: number; ids: number[] }
  },

  async testPoolProxy(id: number, proxyId: number) {
    const res = await apiClient.post(`${BASE}/${id}/proxies/${proxyId}/test`)
    return res.data as { success: boolean; message: string; latency_ms?: number }
  },

  async listEntryProxies() {
    const res = await apiClient.get(`${BASE}/entry-proxies`)
    return res.data as Array<{ id: number; name: string; url: string }>
  }
}

export default dynamicProxyPoolsAPI
