/**
 * Admin Billing Pools API endpoints
 */

import { apiClient } from '../client'
import type {
  BillingPool,
  BillingPoolListResponse,
  BillingPoolLookup,
  BillingPoolPlatformScope,
  BillingPoolStatus,
  CreateBillingPoolRequest,
  ReplaceBillingPoolMembersRequest,
  UpdateBillingPoolRequest
} from '@/types'

export async function list(
  page: number = 1,
  pageSize: number = 20,
  filters?: {
    status?: BillingPoolStatus | ''
    search?: string
    platform_scope?: BillingPoolPlatformScope | ''
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  },
  options?: {
    signal?: AbortSignal
  }
): Promise<BillingPoolListResponse> {
  const { data } = await apiClient.get<BillingPoolListResponse>('/admin/billing-pools', {
    params: { page, page_size: pageSize, ...filters },
    signal: options?.signal
  })
  return data
}

export async function lookup(): Promise<BillingPoolLookup[]> {
  const { data } = await apiClient.get<BillingPoolLookup[]>('/admin/billing-pools/lookup')
  return data
}

export async function getById(id: number): Promise<BillingPool> {
  const { data } = await apiClient.get<BillingPool>(`/admin/billing-pools/${id}`)
  return data
}

export async function create(request: CreateBillingPoolRequest): Promise<BillingPool> {
  const { data } = await apiClient.post<BillingPool>('/admin/billing-pools', request)
  return data
}

export async function update(id: number, request: UpdateBillingPoolRequest): Promise<BillingPool> {
  const { data } = await apiClient.put<BillingPool>(`/admin/billing-pools/${id}`, request)
  return data
}

export async function deletePool(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/billing-pools/${id}`)
  return data
}

export async function replaceMembers(
  id: number,
  request: ReplaceBillingPoolMembersRequest
): Promise<BillingPool> {
  const { data } = await apiClient.put<BillingPool>(`/admin/billing-pools/${id}/members`, request)
  return data
}

const billingPoolsAPI = {
  list,
  lookup,
  getById,
  create,
  update,
  delete: deletePool,
  replaceMembers
}

export default billingPoolsAPI
