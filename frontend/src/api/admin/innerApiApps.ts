/**
 * Admin 内部 API 接入方管理 API。
 *
 * 挂在 `/admin/inner-api-apps`，走标准 apiClient + admin 鉴权。
 * 鉴权采用无状态 token：创建时返回一次性 token（之后无法再取），库里不存。
 */

import { apiClient } from '../client'

export interface InnerAPIApp {
  id: number
  app_id: string
  app_name: string
  enabled: boolean
  permissions: InnerAPIPermission[]
}

export const INNER_API_PERMISSIONS = [
  'balance:write',
  'balance:read',
  'materials:read',
  'materials:write'
] as const

export type InnerAPIPermission = (typeof INNER_API_PERMISSIONS)[number]

/** Create 响应：携带一次性 token。 */
export interface CreateInnerAPIAppResponse extends InnerAPIApp {
  token: string
}

export async function listInnerAPIApps(): Promise<InnerAPIApp[]> {
  const { data } = await apiClient.get<InnerAPIApp[]>('/admin/inner-api-apps')
  return data
}

export async function createInnerAPIApp(
  appName: string,
  permissions: InnerAPIPermission[]
): Promise<CreateInnerAPIAppResponse> {
  const { data } = await apiClient.post<CreateInnerAPIAppResponse>('/admin/inner-api-apps', {
    app_name: appName,
    permissions
  })
  return data
}

export async function setInnerAPIAppPermissions(
  appId: string,
  permissions: InnerAPIPermission[]
): Promise<{ app_id: string; permissions: InnerAPIPermission[] }> {
  const { data } = await apiClient.patch<{ app_id: string; permissions: InnerAPIPermission[] }>(
    `/admin/inner-api-apps/${encodeURIComponent(appId)}/permissions`,
    { permissions }
  )
  return data
}

export async function setInnerAPIAppEnabled(
  appId: string,
  enabled: boolean
): Promise<{ app_id: string; enabled: boolean }> {
  const { data } = await apiClient.patch<{ app_id: string; enabled: boolean }>(
    `/admin/inner-api-apps/${encodeURIComponent(appId)}/enabled`,
    { enabled }
  )
  return data
}

/** 刷新 token：旧 token 立即失效，返回一次性新 token。 */
export async function refreshInnerAPIAppToken(
  appId: string
): Promise<{ app_id: string; token: string }> {
  const { data } = await apiClient.post<{ app_id: string; token: string }>(
    `/admin/inner-api-apps/${encodeURIComponent(appId)}/refresh-token`,
    {}
  )
  return data
}

export async function deleteInnerAPIApp(appId: string): Promise<{ app_id: string }> {
  const { data } = await apiClient.delete<{ app_id: string }>(
    `/admin/inner-api-apps/${encodeURIComponent(appId)}`
  )
  return data
}

export interface InnerAPIAppStats {
  app_id: string
  total_deducted: number
  total_refunded: number
  net_deducted: number
  deduct_count: number
  refund_count: number
}

export async function getInnerAPIAppStats(appId: string): Promise<InnerAPIAppStats> {
  const { data } = await apiClient.get<InnerAPIAppStats>(
    `/admin/inner-api-apps/${encodeURIComponent(appId)}/stats`
  )
  return data
}
