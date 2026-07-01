/**
 * 管理端 TLS 指纹路由器 API。
 * 用于维护基于 User-Agent 匹配的 TLS 指纹路由器。
 */

import { apiClient } from '../client'

export type TLSFingerprintRouterMatchType = 'contains' | 'prefix' | 'exact' | 'regex'

export interface TLSFingerprintRouterRule {
  name: string
  enabled: boolean
  match_type: TLSFingerprintRouterMatchType
  pattern: string
  case_sensitive: boolean
  tls_fingerprint_profile_id: number
  upstream_user_agent?: string
  upstream_originator?: string
}

export interface TLSFingerprintRouter {
  id: number
  name: string
  description: string | null
  enabled: boolean
  chatgpt_oauth_token_user_agent?: string
  chatgpt_oauth_token_tls_fingerprint_profile_id?: number | null
  codex_invite_reset_user_agent?: string
  codex_invite_reset_tls_fingerprint_profile_id?: number | null
  rules: TLSFingerprintRouterRule[]
  created_at: string
  updated_at: string
}

export interface CreateTLSFingerprintRouterRequest {
  name: string
  description?: string | null
  enabled?: boolean
  chatgpt_oauth_token_user_agent?: string
  chatgpt_oauth_token_tls_fingerprint_profile_id?: number | null
  codex_invite_reset_user_agent?: string
  codex_invite_reset_tls_fingerprint_profile_id?: number | null
  rules?: TLSFingerprintRouterRule[]
}

export interface UpdateTLSFingerprintRouterRequest {
  name?: string
  description?: string | null
  enabled?: boolean
  chatgpt_oauth_token_user_agent?: string
  chatgpt_oauth_token_tls_fingerprint_profile_id?: number | null
  codex_invite_reset_user_agent?: string
  codex_invite_reset_tls_fingerprint_profile_id?: number | null
  rules?: TLSFingerprintRouterRule[]
}

export async function list(): Promise<TLSFingerprintRouter[]> {
  const { data } = await apiClient.get<TLSFingerprintRouter[]>('/admin/tls-fingerprint-routers')
  return data
}

export async function getById(id: number): Promise<TLSFingerprintRouter> {
  const { data } = await apiClient.get<TLSFingerprintRouter>(`/admin/tls-fingerprint-routers/${id}`)
  return data
}

export async function create(payload: CreateTLSFingerprintRouterRequest): Promise<TLSFingerprintRouter> {
  const { data } = await apiClient.post<TLSFingerprintRouter>('/admin/tls-fingerprint-routers', payload)
  return data
}

export async function update(id: number, payload: UpdateTLSFingerprintRouterRequest): Promise<TLSFingerprintRouter> {
  const { data } = await apiClient.put<TLSFingerprintRouter>(`/admin/tls-fingerprint-routers/${id}`, payload)
  return data
}

export async function deleteRouter(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/tls-fingerprint-routers/${id}`)
  return data
}

export const tlsFingerprintRouterAPI = {
  list,
  getById,
  create,
  update,
  delete: deleteRouter
}

export default tlsFingerprintRouterAPI
