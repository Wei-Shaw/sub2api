/**
 * User-owned upstream accounts API
 * CRUD + visibility for accounts where owner_user_id = current user.
 * Requires public setting user_owned_accounts_enabled.
 */

import { apiClient } from './client'
import type { Account, AccountPlatform, AccountType, PaginatedResponse } from '@/types'

export type UserAccountVisibility = 'private' | 'public'

export interface CreateUserAccountRequest {
  name: string
  platform: AccountPlatform | string
  type: AccountType | string
  credentials?: Record<string, unknown>
  /** 非敏感扩展（privacy_mode、load_code_assist 等） */
  extra?: Record<string, unknown>
  /** private | public；探测失败时后端可能强制 private */
  visibility?: UserAccountVisibility | string
  /** 账号并发数（>=1） */
  concurrency?: number
}

export interface UpdateUserAccountRequest {
  name?: string
  notes?: string
  credentials?: Record<string, unknown>
  /** active | inactive | disabled（后端将 disabled 规范为 inactive） */
  status?: 'active' | 'inactive' | 'disabled' | string
  concurrency?: number
  schedulable?: boolean
  rate_multiplier?: number
  /** 浅合并到现有 extra */
  extra?: Record<string, unknown>
}

export interface SetUserAccountVisibilityRequest {
  visibility: UserAccountVisibility | string
}

export interface UserAccountBatchDeleteResult {
  deleted_ids: number[]
  failed_ids: number[]
  deleted: number
  failed: number
}

export interface UserAccountBatchSchedulableResult {
  success_ids: number[]
  failed_ids: number[]
  success: number
  failed: number
}

/**
 * List current user's owned accounts
 */
export async function list(
  page: number = 1,
  pageSize: number = 20,
  options?: {
    signal?: AbortSignal
    sort_by?: string
    sort_order?: 'asc' | 'desc'
    platform?: string
    status?: string
    search?: string
  }
): Promise<PaginatedResponse<Account>> {
  const { data } = await apiClient.get<PaginatedResponse<Account>>('/user/accounts', {
    params: {
      page,
      page_size: pageSize,
      sort_by: options?.sort_by ?? 'created_at',
      sort_order: options?.sort_order ?? 'desc',
      // 预留筛选；后端 v1 可能忽略
      platform: options?.platform || undefined,
      status: options?.status || undefined,
      search: options?.search || undefined
    },
    signal: options?.signal
  })
  return data
}

/**
 * Get one owned account by id
 */
export async function getById(id: number): Promise<Account> {
  const { data } = await apiClient.get<Account>(`/user/accounts/${id}`)
  return data
}

/**
 * Create a user-owned account (minimal: platform + type + credentials paste)
 */
export async function create(payload: CreateUserAccountRequest): Promise<Account> {
  const { data } = await apiClient.post<Account>('/user/accounts', payload)
  return data
}

/**
 * Update allowlisted fields on an owned account
 */
export async function update(id: number, payload: UpdateUserAccountRequest): Promise<Account> {
  const { data } = await apiClient.patch<Account>(`/user/accounts/${id}`, payload)
  return data
}

/**
 * Toggle private ↔ public visibility
 */
export async function setVisibility(
  id: number,
  visibility: UserAccountVisibility | string
): Promise<Account> {
  const { data } = await apiClient.put<Account>(`/user/accounts/${id}/visibility`, {
    visibility
  } satisfies SetUserAccountVisibilityRequest)
  return data
}

/**
 * Toggle schedulable on an owned account
 */
export async function setSchedulable(id: number, schedulable: boolean): Promise<Account> {
  const { data } = await apiClient.put<Account>(`/user/accounts/${id}/schedulable`, {
    schedulable
  })
  return data
}

/**
 * Delete an owned account
 */
export async function remove(id: number): Promise<void> {
  await apiClient.delete(`/user/accounts/${id}`)
}

/**
 * Batch delete owned accounts
 */
export async function batchDelete(ids: number[]): Promise<UserAccountBatchDeleteResult> {
  const { data } = await apiClient.post<UserAccountBatchDeleteResult>(
    '/user/accounts/batch-delete',
    { ids }
  )
  return data
}

/**
 * Batch set schedulable on owned accounts
 */
export async function batchSetSchedulable(
  ids: number[],
  schedulable: boolean
): Promise<UserAccountBatchSchedulableResult> {
  const { data } = await apiClient.post<UserAccountBatchSchedulableResult>(
    '/user/accounts/batch-set-schedulable',
    { ids, schedulable }
  )
  return data
}

export interface UserAccountDataPayload {
  type?: string
  version?: number
  exported_at: string
  proxies?: unknown[]
  accounts: Array<{
    name: string
    notes?: string | null
    platform: string
    type: string
    credentials: Record<string, unknown>
    extra?: Record<string, unknown>
    concurrency?: number
    rate_multiplier?: number
    visibility?: string
    upstream_plan?: string
  }>
}

export interface UserAccountDataImportResult {
  account_created: number
  account_failed: number
  errors?: Array<{ kind: string; name?: string; message: string }>
  proxy_created?: number
  proxy_reused?: number
  proxy_failed?: number
}

/** Export owned accounts (no proxies). Optional ids filter. */
export async function exportData(options?: {
  ids?: number[]
}): Promise<UserAccountDataPayload> {
  const params: Record<string, string> = {}
  if (options?.ids && options.ids.length > 0) {
    params.ids = options.ids.join(',')
  }
  const { data } = await apiClient.get<UserAccountDataPayload>('/user/accounts/data', { params })
  return data
}

/** Import as owned accounts (proxies ignored). */
export async function importData(payload: {
  data: UserAccountDataPayload
}): Promise<UserAccountDataImportResult> {
  const { data } = await apiClient.post<UserAccountDataImportResult>('/user/accounts/data', {
    data: payload.data
  })
  return data
}

export const userAccountsAPI = {
  list,
  getById,
  create,
  update,
  setVisibility,
  setSchedulable,
  remove,
  batchDelete,
  batchSetSchedulable,
  exportData,
  importData
}

export default userAccountsAPI
