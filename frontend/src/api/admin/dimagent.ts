import { apiClient } from '../client'
import type { Account } from '@/types'

export interface DimAgentAuthURLResponse {
  auth_url: string
  session_id: string
  redirect_uri: string
}

export interface CreateDimAgentOAuthAccountRequest {
  session_id: string
  callback_url: string
  proxy_id?: number | null
  name?: string
  concurrency?: number
  priority?: number
  group_ids?: number[]
  credential_extras?: Record<string, unknown>
}

export async function generateAuthUrl(proxyId?: number | null): Promise<DimAgentAuthURLResponse> {
  const payload: Record<string, unknown> = {}
  if (proxyId) payload.proxy_id = proxyId
  const { data } = await apiClient.post<DimAgentAuthURLResponse>('/admin/dimagent/oauth/auth-url', payload)
  return data
}

export async function createFromCallback(payload: CreateDimAgentOAuthAccountRequest): Promise<Account> {
  const { data } = await apiClient.post<Account>('/admin/dimagent/oauth/create-from-callback', payload)
  return data
}

export async function refreshAccount(id: number): Promise<Account> {
  const { data } = await apiClient.post<Account>(`/admin/dimagent/accounts/${id}/refresh`)
  return data
}

export default { generateAuthUrl, createFromCallback, refreshAccount }
