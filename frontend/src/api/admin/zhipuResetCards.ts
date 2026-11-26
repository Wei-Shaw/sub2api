import { apiClient } from '../client'

export type ZhipuResetWindow = 'FIVE_HOUR' | 'WEEK'

export interface ZhipuResetCardStatus {
  available_five_hour_resets: { expire_at: number }[]
  available_week_resets: { expire_at: number }[]
  latest_five_hour_reset_history?: { used_at: number }
  latest_week_reset_history?: { used_at: number }
}

export interface ZhipuResetState {
  status: 'unbound' | 'bound' | 'unauthorized'
  binding_id?: string
  cards?: ZhipuResetCardStatus
}

export interface ZhipuResetDevicePrompt {
  attempt_id: string
  authorize_url: string
  expires_at: string
  poll_interval: number
}

export interface ZhipuResetDeviceStatus {
  status: 'pending' | 'expired' | 'rejected' | 'bound'
  state?: ZhipuResetState
}

export interface ZhipuResetActionResult {
  outcome: 'granted' | 'not_eligible' | 'used'
  next_try_at?: number
  cards?: ZhipuResetCardStatus
}

const base = (id: number) => `/admin/accounts/${id}/zhipu-reset-cards`

export async function getZhipuResetCards(id: number, signal?: AbortSignal) {
  const { data } = await apiClient.get<ZhipuResetState>(base(id), { signal })
  return data
}

export async function startZhipuResetAuthorization(id: number, signal?: AbortSignal) {
  const { data } = await apiClient.post<ZhipuResetDevicePrompt>(`${base(id)}/device/start`, undefined, { signal })
  return data
}

export async function pollZhipuResetAuthorization(id: number, attemptID: string, signal?: AbortSignal) {
  const { data } = await apiClient.post<ZhipuResetDeviceStatus>(
    `${base(id)}/device/poll`, { attempt_id: attemptID }, { signal }
  )
  return data
}

export async function unbindZhipuResetAuthorization(id: number, signal?: AbortSignal) {
  await apiClient.delete(`${base(id)}/session`, { signal })
}

export async function actZhipuResetCard(
  id: number, bindingID: string, action: 'claim' | ZhipuResetWindow, key: string
) {
  const claim = action === 'claim'
  const { data } = await apiClient.post<ZhipuResetActionResult>(
    `${base(id)}/${claim ? 'claim' : 'use'}`,
    { binding_id: bindingID, ...(claim ? {} : { reset_type: action }) },
    { headers: { 'Idempotency-Key': key }, timeout: 40000 }
  )
  return data
}
