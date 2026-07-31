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
  status?: 'active' | 'disabled' | string
}

export interface SetUserAccountVisibilityRequest {
  visibility: UserAccountVisibility | string
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
  }
): Promise<PaginatedResponse<Account>> {
  const { data } = await apiClient.get<PaginatedResponse<Account>>('/user/accounts', {
    params: {
      page,
      page_size: pageSize,
      sort_by: options?.sort_by ?? 'created_at',
      sort_order: options?.sort_order ?? 'desc'
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
 * Delete an owned account
 */
export async function remove(id: number): Promise<void> {
  await apiClient.delete(`/user/accounts/${id}`)
}

export const userAccountsAPI = {
  list,
  getById,
  create,
  update,
  setVisibility,
  remove
}

export default userAccountsAPI
