import { apiClient } from '../client'

export interface UserPool {
  id: number
  name: string
  description: string
  status: string
  created_at: string
  updated_at: string
}

export interface UserPoolMember {
  pool_id: number
  user_id: number
  email: string
  username: string
  created_at: string
}

export interface UserPoolGroupGrant {
  pool_id: number
  group_id: number
  rate_multiplier: number | null
  rpm_override: number | null
  created_at: string
  updated_at: string
}

export interface UserPoolListResponse {
  items: UserPool[]
  total: number
  page: number
  page_size: number
}

export interface UserPoolMemberListResponse {
  items: UserPoolMember[]
  total: number
  page: number
  page_size: number
}

export interface UserPoolGroupGrantsResponse {
  grants: UserPoolGroupGrant[]
}

export interface CreateUserPoolRequest {
  name: string
  description?: string
  status?: string
}

export interface UpdateUserPoolRequest {
  name?: string
  description?: string
  status?: string
}

export interface PoolGroupGrantInput {
  group_id: number
  rate_multiplier?: number | null
  rpm_override?: number | null
}

export async function list(
  page: number = 1,
  limit: number = 20,
  status?: string,
  options?: { signal?: AbortSignal }
): Promise<UserPoolListResponse> {
  const { data } = await apiClient.get<UserPoolListResponse>('/admin/user-pools', {
    params: { page, limit, status: status || undefined },
    signal: options?.signal
  })
  return data
}

export async function getById(id: number): Promise<UserPool> {
  const { data } = await apiClient.get<UserPool>(`/admin/user-pools/${id}`)
  return data
}

export async function create(payload: CreateUserPoolRequest): Promise<UserPool> {
  const { data } = await apiClient.post<UserPool>('/admin/user-pools', payload)
  return data
}

export async function update(id: number, payload: UpdateUserPoolRequest): Promise<UserPool> {
  const { data } = await apiClient.put<UserPool>(`/admin/user-pools/${id}`, payload)
  return data
}

export async function deletePool(id: number): Promise<void> {
  await apiClient.delete(`/admin/user-pools/${id}`)
}

export async function addMembers(
  poolId: number,
  userIds: number[]
): Promise<{ added: number; skipped: number }> {
  const { data } = await apiClient.post<{ added: number; skipped: number }>(
    `/admin/user-pools/${poolId}/members`,
    { user_ids: userIds }
  )
  return data
}

export async function removeMembers(
  poolId: number,
  userIds: number[]
): Promise<{ removed: number }> {
  const { data } = await apiClient.post<{ removed: number }>(
    `/admin/user-pools/${poolId}/members/remove`,
    { user_ids: userIds }
  )
  return data
}

export async function listMembers(
  poolId: number,
  page: number = 1,
  limit: number = 20,
  options?: { signal?: AbortSignal }
): Promise<UserPoolMemberListResponse> {
  const { data } = await apiClient.get<UserPoolMemberListResponse>(
    `/admin/user-pools/${poolId}/members`,
    { params: { page, limit }, signal: options?.signal }
  )
  return data
}

export async function listGroupGrants(poolId: number): Promise<UserPoolGroupGrantsResponse> {
  const { data } = await apiClient.get<UserPoolGroupGrantsResponse>(
    `/admin/user-pools/${poolId}/allowed-groups`
  )
  return data
}

export async function replaceGroupGrants(
  poolId: number,
  grants: PoolGroupGrantInput[]
): Promise<UserPoolGroupGrantsResponse> {
  const { data } = await apiClient.put<UserPoolGroupGrantsResponse>(
    `/admin/user-pools/${poolId}/allowed-groups`,
    { grants }
  )
  return data
}

export async function deleteGroupGrant(poolId: number, groupId: number): Promise<void> {
  await apiClient.delete(`/admin/user-pools/${poolId}/allowed-groups/${groupId}`)
}

export async function getUserPools(userId: number): Promise<{ pools: UserPool[] }> {
  const { data } = await apiClient.get<{ pools: UserPool[] }>(`/admin/users/${userId}/pools`)
  return data
}

export interface AddMembersByFilterRequest {
  search?: string
  status?: 'active' | 'disabled'
  role?: 'admin' | 'user'
  group_name?: string
  attributes?: Record<number, string>
}

export interface AddMembersByFilterResponse {
  added: number
  skipped: number
  matched: number
}

export async function addMembersByFilter(
  poolId: number,
  filters: AddMembersByFilterRequest
): Promise<AddMembersByFilterResponse> {
  const { data } = await apiClient.post<AddMembersByFilterResponse>(
    `/admin/user-pools/${poolId}/members/by-filter`,
    filters
  )
  return data
}

export const userPoolsAPI = {
  list,
  getById,
  create,
  update,
  delete: deletePool,
  addMembers,
  addMembersByFilter,
  removeMembers,
  listMembers,
  listGroupGrants,
  replaceGroupGrants,
  deleteGroupGrant,
  getUserPools
}

export default userPoolsAPI
