import { apiClient } from '../client'

export interface AccessRule {
  permissions: string[]
  group_ids: number[]
  all_groups: boolean
  include_ungrouped: boolean
  account_types: string[]
  platforms: string[]
}
export interface AccessPolicy { rules: AccessRule[]; global_permissions: string[] }
export interface Permission { id: string; label: string; category: string; scope: 'account' | 'group' | 'global' }
export interface AdminKeyInput {
  name: string
  description: string
  enabled: boolean
  expires_at: string | null
  allowed_ips: string[]
  policy: AccessPolicy
}
export interface AdminKey extends AdminKeyInput {
  id: string
  key_prefix: string
  created_by: number
  created_at: string
  updated_at: string
  last_used_at: string | null
  last_used_ip: string
  revoked_at: string | null
}
const base = '/admin/settings/admin-api-keys'
export async function list() { return (await apiClient.get<{ items: AdminKey[]; permissions: Permission[] }>(base)).data }
export async function create(input: AdminKeyInput) { return (await apiClient.post<{ key: string; item: AdminKey }>(base, input)).data }
export async function update(id: string, input: AdminKeyInput) { return (await apiClient.put<AdminKey>(`${base}/${encodeURIComponent(id)}`, input)).data }
export async function rotate(id: string) { return (await apiClient.post<{ key: string; item: AdminKey }>(`${base}/${encodeURIComponent(id)}/rotate`)).data }
export async function revoke(id: string) { return (await apiClient.delete<{ message: string }>(`${base}/${encodeURIComponent(id)}`)).data }
export async function deleteRecord(id: string) { return (await apiClient.delete<{ message: string }>(`${base}/${encodeURIComponent(id)}/record`)).data }
