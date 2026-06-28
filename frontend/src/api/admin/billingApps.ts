/**
 * Admin 余额 RPC 接入方（扣费 app）管理 API。
 *
 * 挂在 `/admin/billing-apps`，走标准 apiClient + admin 鉴权。
 * 鉴权采用无状态 token：创建时返回一次性 token（之后无法再取），库里不存。
 */

import { apiClient } from '../client'

export interface BillingApp {
  id: number
  app_id: string
  app_name: string
  enabled: boolean
}

/** Create 响应：携带一次性 token。 */
export interface CreateBillingAppResponse extends BillingApp {
  token: string
}

export async function listBillingApps(): Promise<BillingApp[]> {
  const { data } = await apiClient.get<BillingApp[]>('/admin/billing-apps')
  return data
}

export async function createBillingApp(appName: string): Promise<CreateBillingAppResponse> {
  const { data } = await apiClient.post<CreateBillingAppResponse>('/admin/billing-apps', {
    app_name: appName
  })
  return data
}

export async function setBillingAppEnabled(
  appId: string,
  enabled: boolean
): Promise<{ app_id: string; enabled: boolean }> {
  const { data } = await apiClient.patch<{ app_id: string; enabled: boolean }>(
    `/admin/billing-apps/${encodeURIComponent(appId)}/enabled`,
    { enabled }
  )
  return data
}

/** 刷新 token：旧 token 立即失效，返回一次性新 token。 */
export async function refreshBillingAppToken(
  appId: string
): Promise<{ app_id: string; token: string }> {
  const { data } = await apiClient.post<{ app_id: string; token: string }>(
    `/admin/billing-apps/${encodeURIComponent(appId)}/refresh-token`,
    {}
  )
  return data
}

export async function deleteBillingApp(appId: string): Promise<{ app_id: string }> {
  const { data } = await apiClient.delete<{ app_id: string }>(
    `/admin/billing-apps/${encodeURIComponent(appId)}`
  )
  return data
}

export interface BillingAppStats {
  app_id: string
  total_deducted: number
  total_refunded: number
  net_deducted: number
  deduct_count: number
  refund_count: number
}

export async function getBillingAppStats(appId: string): Promise<BillingAppStats> {
  const { data } = await apiClient.get<BillingAppStats>(
    `/admin/billing-apps/${encodeURIComponent(appId)}/stats`
  )
  return data
}
