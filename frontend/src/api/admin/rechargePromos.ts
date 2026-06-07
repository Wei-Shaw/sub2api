/**
 * Admin Recharge Promo Activities API endpoints
 *
 * 充值赠送活动以列表 CRUD 形式管理：每条记录是一个活动，全表至多一行 enabled=TRUE
 * （DB partial unique 兜底，业务层 toggle 在事务内自动互斥）。
 */

import { apiClient } from '../client'
import type { BasePaginationResponse } from '@/types'

export interface RechargePromoTier {
  min_amount: number
  bonus_rate: number
}

export interface RechargePromoActivity {
  id: number
  name: string
  enabled: boolean
  valid_from?: string | null
  valid_until?: string | null
  tiers: RechargePromoTier[]
  operator: string
  note?: string | null
  created_at: string
  updated_at: string
}

export interface CreateOrUpdateRechargePromoRequest {
  name: string
  enabled: boolean
  valid_from?: string | null
  valid_until?: string | null
  tiers: RechargePromoTier[]
  note?: string | null
}

export async function list(
  page: number = 1,
  pageSize: number = 20,
  options?: { signal?: AbortSignal }
): Promise<BasePaginationResponse<RechargePromoActivity>> {
  const { data } = await apiClient.get<BasePaginationResponse<RechargePromoActivity>>(
    '/admin/recharge-promos',
    { params: { page, page_size: pageSize }, signal: options?.signal }
  )
  return data
}

export async function getById(id: number): Promise<RechargePromoActivity> {
  const { data } = await apiClient.get<RechargePromoActivity>(`/admin/recharge-promos/${id}`)
  return data
}

export async function create(
  request: CreateOrUpdateRechargePromoRequest
): Promise<RechargePromoActivity> {
  const { data } = await apiClient.post<RechargePromoActivity>('/admin/recharge-promos', request)
  return data
}

export async function update(
  id: number,
  request: CreateOrUpdateRechargePromoRequest
): Promise<RechargePromoActivity> {
  const { data } = await apiClient.put<RechargePromoActivity>(
    `/admin/recharge-promos/${id}`,
    request
  )
  return data
}

export async function toggle(
  id: number,
  enabled: boolean
): Promise<RechargePromoActivity> {
  const { data } = await apiClient.post<RechargePromoActivity>(
    `/admin/recharge-promos/${id}/toggle`,
    { enabled }
  )
  return data
}

export async function deleteById(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/recharge-promos/${id}`)
  return data
}

const rechargePromosAPI = {
  list,
  getById,
  create,
  update,
  toggle,
  delete: deleteById
}

export default rechargePromosAPI
