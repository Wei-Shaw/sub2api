/**
 * Admin Meta Muse API endpoints.
 * Handles the Meta Muse device flow used to provision OAuth accounts.
 */

import { apiClient } from '../client'

export type MetaDeviceFlowState = 'pending' | 'slow_down' | 'authorized' | 'denied' | 'expired'

export interface MetaDeviceStartRequest {
  proxy_id?: number
}

export interface MetaDeviceStartResponse {
  device_code_id: string
  user_code: string
  verification_uri: string
  verification_uri_complete: string
  expires_in: number
  interval: number
}

export interface MetaDeviceAccountOptions {
  name?: string
  notes?: string
  concurrency?: number
  priority?: number
  group_ids?: number[]
  proxy_id?: number
  rate_multiplier?: number
  load_factor?: number
  expires_at?: number | null
  auto_pause_on_expired?: boolean
  credentials?: Record<string, unknown>
  extra?: Record<string, unknown>
}

export interface MetaDevicePollRequest extends MetaDeviceAccountOptions {
  device_code_id: string
}

export interface MetaDevicePollResponse {
  state: MetaDeviceFlowState
  interval: number
  message?: string
  account?: Record<string, unknown>
}

export async function startDeviceFlow(payload: MetaDeviceStartRequest = {}): Promise<MetaDeviceStartResponse> {
  const { data } = await apiClient.post<MetaDeviceStartResponse>('/admin/meta/oauth/device/start', payload)
  return data
}

export async function pollDeviceFlow(payload: MetaDevicePollRequest): Promise<MetaDevicePollResponse> {
  const { data } = await apiClient.post<MetaDevicePollResponse>('/admin/meta/oauth/device/poll', payload)
  return data
}

export default {
  startDeviceFlow,
  pollDeviceFlow,
}
