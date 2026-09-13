import { apiClient } from '../client'

export interface DevinAuthUrlRequest {
  proxy_id?: number
}

export interface DevinAuthUrlResponse {
  auth_url: string
  session_id: string
  state: string
  redirect_uri: string
}

export interface DevinExchangeCodeRequest {
  session_id: string
  code: string
  state?: string
  proxy_id?: number
}

export interface DevinTokenInfo {
  access_token?: string
  refresh_token?: string
  expires_at?: number | string
  api_endpoint?: string
  enterprise_url?: string
}

export async function generateAuthUrl(payload: DevinAuthUrlRequest): Promise<DevinAuthUrlResponse> {
  const { data } = await apiClient.post<DevinAuthUrlResponse>('/admin/devin/oauth/auth-url', payload)
  return data
}

export async function exchangeCode(payload: DevinExchangeCodeRequest): Promise<DevinTokenInfo> {
  const { data } = await apiClient.post<DevinTokenInfo>('/admin/devin/oauth/exchange-code', payload)
  return data
}

export async function importToken(sessionToken: string): Promise<DevinTokenInfo> {
  const { data } = await apiClient.post<DevinTokenInfo>('/admin/devin/oauth/import-token', {
    session_token: sessionToken
  })
  return data
}

export default {
  generateAuthUrl,
  exchangeCode,
  importToken
}
