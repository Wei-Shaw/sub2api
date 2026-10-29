import { apiClient } from './client'

export interface OAuthAuthorizationTransaction {
  id: string
  client_id: string
  client_name: string
  client_description?: string
  client_logo_url?: string
  scope: string[]
  expires_at: string
}

export interface OAuthApprovalResponse {
  redirect_uri: string
}

export async function getOAuthAuthorizationTransaction(id: string): Promise<OAuthAuthorizationTransaction> {
  const { data } = await apiClient.get<OAuthAuthorizationTransaction>(`/oauth/authorize/transaction/${encodeURIComponent(id)}`)
  return data
}

export async function approveOAuthAuthorization(
  transactionId: string,
  decision: 'approve' | 'deny'
): Promise<OAuthApprovalResponse> {
  const { data } = await apiClient.post<OAuthApprovalResponse>(
    '/oauth/authorize/approve',
    { transaction_id: transactionId, decision },
    { headers: { Accept: 'application/json' } }
  )
  return data
}

export interface OAuthGrantView {
  client_id: string
  client_name: string
  client_logo_url?: string
  scopes: string[]
  approved_at: string
}

export async function listOAuthGrants(): Promise<OAuthGrantView[]> {
  const { data } = await apiClient.get<{ grants: OAuthGrantView[] }>('/oauth/grants')
  return data.grants ?? []
}

export async function revokeOAuthGrant(clientId: string): Promise<void> {
  await apiClient.delete(`/oauth/grants/${encodeURIComponent(clientId)}`)
}

// ==================== Admin: OAuth client management ====================

export interface OAuthClientAdminView {
  client_id: string
  client_type: string
  name: string
  description: string
  logo_url: string
  redirect_uris: string[]
  allowed_scopes: string[]
  pkce_required: boolean
  status: string
  created_at: string
  updated_at: string
}

export interface OAuthClientInput {
  name: string
  description: string
  logo_url: string
  redirect_uris: string[]
  allowed_scopes: string[]
  status: string
}

export async function listOAuthClients(): Promise<OAuthClientAdminView[]> {
  const { data } = await apiClient.get<{ clients: OAuthClientAdminView[] }>('/admin/oauth-clients')
  return data.clients ?? []
}

export async function upsertOAuthClient(clientId: string, input: OAuthClientInput): Promise<OAuthClientAdminView> {
  const { data } = await apiClient.put<OAuthClientAdminView>(
    `/admin/oauth-clients/${encodeURIComponent(clientId)}`,
    input
  )
  return data
}

export async function deleteOAuthClient(clientId: string): Promise<void> {
  await apiClient.delete(`/admin/oauth-clients/${encodeURIComponent(clientId)}`)
}
