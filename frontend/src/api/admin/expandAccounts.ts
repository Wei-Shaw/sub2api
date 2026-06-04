import { apiClient } from '../client'
import type { PaginatedResponse } from '@/types'

export interface ExpandAccount {
  id: number
  email: string
  platform: string
  subscription_type: string
  country: string
  session_key: string
  used: boolean
  account_id?: number | null
  login_status: number
  device_id?: string
  api_key?: string
  created_at: string
  updated_at: string
}

export interface CreateExpandAccountRequest {
  email: string
  platform: string
  subscription_type: string
  country: string
  session_key: string
  used?: boolean
}

export interface UpdateExpandAccountRequest extends CreateExpandAccountRequest {}

export async function list(
  page: number,
  pageSize: number,
  filters?: {
    search?: string
    used?: string
    login_status?: number
    account_type?: 'old' | 'new'
  }
): Promise<PaginatedResponse<ExpandAccount>> {
  const { data } = await apiClient.get<PaginatedResponse<ExpandAccount>>('/admin/expand-accounts', {
    params: {
      page,
      page_size: pageSize,
      ...filters
    }
  })
  return data
}

export async function getById(id: number): Promise<ExpandAccount> {
  const { data } = await apiClient.get<ExpandAccount>(`/admin/expand-accounts/${id}`)
  return data
}

export async function create(payload: CreateExpandAccountRequest): Promise<ExpandAccount> {
  const { data } = await apiClient.post<ExpandAccount>('/admin/expand-accounts', payload)
  return data
}

export async function update(id: number, payload: UpdateExpandAccountRequest): Promise<ExpandAccount> {
  const { data } = await apiClient.put<ExpandAccount>(`/admin/expand-accounts/${id}`, payload)
  return data
}

export async function deleteExpandAccount(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/expand-accounts/${id}`)
  return data
}

export async function markUsed(id: number): Promise<ExpandAccount> {
  const { data } = await apiClient.post<ExpandAccount>(`/admin/expand-accounts/${id}/mark-used`)
  return data
}

const expandAccountsAPI = {
  list,
  getById,
  create,
  update,
  deleteAccount: deleteExpandAccount,
  markUsed
}

export default expandAccountsAPI

