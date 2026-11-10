/**
 * Admin Payment FX Sync API
 * Manages the optional daily USD/CNY rate sync for the balance recharge multiplier.
 */

import { apiClient } from '../client'

export interface PaymentFXSyncConfig {
  enabled: boolean
  urls: string[]
  interval_hours: number
  markup_percent: number
  update_subscription_rate: boolean
  last_attempt_at?: string
  last_success_at?: string
  last_rate?: number
  last_multiplier?: number
  last_subscription_rate?: number
  last_source?: string
  last_error?: string
  next_run_at?: string
}

export interface UpdatePaymentFXSyncRequest {
  enabled: boolean
  urls: string[]
  interval_hours: number
  markup_percent: number
  update_subscription_rate: boolean
}

export async function getPaymentFXSyncConfig(): Promise<PaymentFXSyncConfig> {
  const { data } = await apiClient.get<PaymentFXSyncConfig>('/admin/settings/payment-fx')
  return data
}

export async function updatePaymentFXSyncConfig(
  payload: UpdatePaymentFXSyncRequest,
): Promise<PaymentFXSyncConfig> {
  const { data } = await apiClient.put<PaymentFXSyncConfig>('/admin/settings/payment-fx', payload)
  return data
}

export async function syncPaymentFXNow(): Promise<PaymentFXSyncConfig> {
  const { data } = await apiClient.post<PaymentFXSyncConfig>(
    '/admin/settings/payment-fx/sync',
    undefined,
    { timeout: 60000 },
  )
  return data
}
