/**
 * Admin Gemini API endpoints
 * Handles Gemini OAuth flows for administrators
 */

import { apiClient REDACTED from '../client'

export interface GeminiAuthUrlResponse {
  auth_url: string
  session_id: string
  state: string
REDACTED

export interface GeminiAuthUrlRequest {
  redirect_uri: string
  proxy_id?: number
REDACTED

export interface GeminiExchangeCodeRequest {
  session_id: string
  state: string
  code: string
  redirect_uri: string
  proxy_id?: number
REDACTED

export type GeminiTokenInfo = Record<string, unknown>

export async function generateAuthUrl(
  payload: GeminiAuthUrlRequest
): Promise<GeminiAuthUrlResponse> {
  const { data REDACTED = await apiClient.post<GeminiAuthUrlResponse>(
    '/admin/gemini/oauth/auth-url',
    payload
  )
  return data
REDACTED

export async function exchangeCode(payload: GeminiExchangeCodeRequest): Promise<GeminiTokenInfo> {
  const { data REDACTED = await apiClient.post<GeminiTokenInfo>(
    '/admin/gemini/oauth/exchange-code',
    payload
  )
  return data
REDACTED

export default { generateAuthUrl, exchangeCode REDACTED
