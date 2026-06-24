/**
 * Admin OIDC Provider API endpoints.
 *
 * 覆盖三组资源（均挂在 `/api/v1/admin/oidc/*`，走标准 apiClient + admin 鉴权）：
 *   - clients：第三方 RP 客户端 CRUD + reset-secret
 *   - settings：8 个 oidc_provider.* 全局设置
 *   - signing-keys：签名密钥列表 / 轮换 / 删除
 */

import { apiClient } from '../client'

// ==================== Clients ====================

export interface OidcClient {
  id: number
  client_id: string
  client_name: string
  redirect_uris: string[]
  allowed_scopes: string[]
  grant_types: string[]
  consent_required: boolean
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface CreateOidcClientRequest {
  client_name: string
  redirect_uris: string[]
  allowed_scopes: string[]
  consent_required: boolean
  enabled: boolean
}

export interface UpdateOidcClientRequest {
  client_name?: string
  redirect_uris?: string[]
  allowed_scopes?: string[]
  consent_required?: boolean
  enabled?: boolean
}

/** Create 响应：携带一次性明文 secret。 */
export interface CreateOidcClientResponse extends OidcClient {
  client_secret: string
}

export interface ResetSecretResponse {
  client_secret: string
}

export async function listClients(filters?: {
  only_enabled?: boolean
  name?: string
}): Promise<OidcClient[]> {
  const params: Record<string, string> = {}
  if (filters?.only_enabled) params.only_enabled = 'true'
  if (filters?.name) params.name = filters.name
  const { data } = await apiClient.get<OidcClient[]>('/admin/oidc/clients', { params })
  return data
}

export async function getClient(id: number): Promise<OidcClient> {
  const { data } = await apiClient.get<OidcClient>(`/admin/oidc/clients/${id}`)
  return data
}

export async function createClient(req: CreateOidcClientRequest): Promise<CreateOidcClientResponse> {
  const { data } = await apiClient.post<CreateOidcClientResponse>('/admin/oidc/clients', req)
  return data
}

export async function updateClient(id: number, req: UpdateOidcClientRequest): Promise<OidcClient> {
  const { data } = await apiClient.patch<OidcClient>(`/admin/oidc/clients/${id}`, req)
  return data
}

export async function deleteClient(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/oidc/clients/${id}`)
  return data
}

export async function resetClientSecret(id: number): Promise<ResetSecretResponse> {
  const { data } = await apiClient.post<ResetSecretResponse>(`/admin/oidc/clients/${id}/reset-secret`, {})
  return data
}

// ==================== Settings ====================

export interface OidcProviderSettings {
  enabled: boolean
  issuer_url: string
  access_token_ttl_seconds: number
  id_token_ttl_seconds: number
  refresh_token_ttl_seconds: number
  code_ttl_seconds: number
  sso_cookie_max_age_seconds: number
  sso_cookie_domain: string
}

export type UpdateOidcProviderSettings = Partial<OidcProviderSettings>

export async function getProviderSettings(): Promise<OidcProviderSettings> {
  const { data } = await apiClient.get<OidcProviderSettings>('/admin/oidc/settings')
  return data
}

export async function updateProviderSettings(
  req: UpdateOidcProviderSettings
): Promise<OidcProviderSettings> {
  const { data } = await apiClient.put<OidcProviderSettings>('/admin/oidc/settings', req)
  return data
}

// ==================== Signing Keys ====================

export interface OidcSigningKey {
  kid: string
  is_active: boolean
  created_at: string
  retired_at?: string | null
  removable: boolean
}

export async function listSigningKeys(): Promise<OidcSigningKey[]> {
  const { data } = await apiClient.get<OidcSigningKey[]>('/admin/oidc/signing-keys')
  return data
}

export async function rotateSigningKey(): Promise<{ active_kid: string }> {
  const { data } = await apiClient.post<{ active_kid: string }>('/admin/oidc/signing-keys/rotate', {})
  return data
}

export async function deleteSigningKey(kid: string): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(
    `/admin/oidc/signing-keys/${encodeURIComponent(kid)}`
  )
  return data
}

/** 6 个合法授权范围（与后端 AllowedOidcProviderScopes 对齐）。 */
export const OIDC_ALLOWED_SCOPES = [
  'openid',
  'profile',
  'email',
  'offline_access',
  'sub2api:apikey'
] as const

/** 敏感授权范围（需红字警示 + 二次确认）。 */
export const OIDC_SENSITIVE_SCOPES = ['sub2api:apikey'] as const

export function isSensitiveScope(scope: string): boolean {
  return (OIDC_SENSITIVE_SCOPES as readonly string[]).includes(scope)
}
