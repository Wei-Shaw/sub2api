/**
 * Admin Devin (Cognition) API endpoints
 * Handles Devin PKCE login flows for administrators:
 * generate auth-url → user authorizes in browser → paste code/token → exchange.
 */

import { apiClient } from '../client'

export interface DevinAuthUrlResponse {
  auth_url: string
  session_id: string
  state: string
}

export interface DevinAuthUrlRequest {
  proxy_id?: number
}

export interface DevinExchangeCodeRequest {
  session_id?: string
  state?: string
  code: string
  proxy_id?: number
}

export interface DevinTokenInfo {
  access_token?: string
  api_server_url?: string
  client_version?: string
  name?: string
  email?: string
  org_id?: string
  team_id?: string
  plan_name?: string
  account_display_name?: string
  can_use_cli?: boolean
  is_enterprise?: boolean
  [key: string]: unknown
}

export async function generateAuthUrl(
  payload: DevinAuthUrlRequest
): Promise<DevinAuthUrlResponse> {
  const { data } = await apiClient.post<DevinAuthUrlResponse>(
    '/admin/devin/oauth/auth-url',
    payload
  )
  return data
}

export async function exchangeCode(
  payload: DevinExchangeCodeRequest
): Promise<DevinTokenInfo> {
  const { data } = await apiClient.post<DevinTokenInfo>(
    '/admin/devin/oauth/exchange-code',
    payload
  )
  return data
}

export default { generateAuthUrl, exchangeCode }
