import { apiClient } from './client'

export type CustomDomainStatus = 'pending_dns' | 'active' | 'disabled' | 'error'

export interface CustomDomainUser {
  id: number
  email: string
  username?: string
  role?: string
}

export interface CustomDomain {
  id: number
  user_id: number
  all_users: boolean
  user_ids: number[]
  domain: string
  status: CustomDomainStatus
  verification_txt_name: string
  verification_txt_value: string
  cname_target?: string | null
  last_error?: string | null
  verified_at?: string | null
  last_checked_at?: string | null
  disabled_at?: string | null
  disabled_reason?: string | null
  created_at: string
  updated_at: string
  user?: CustomDomainUser | null
  users?: CustomDomainUser[]
  can_manage?: boolean
}

export interface CustomDomainListResponse {
  enabled: boolean
  cname_target: string
  domains: CustomDomain[]
}

export interface CustomDomainConfig {
  enabled: boolean
  cname_target: string
}

export interface AdminCustomDomainFilters {
  domain?: string
  status?: string
  user_id?: number | string
  all_users?: boolean | string
}

export async function listUserCustomDomains(): Promise<CustomDomainListResponse> {
  const { data } = await apiClient.get<CustomDomainListResponse>('/custom-domains')
  return data
}

export async function createCustomDomain(domain: string): Promise<CustomDomain> {
  const { data } = await apiClient.post<CustomDomain>('/custom-domains', { domain })
  return data
}

export async function verifyCustomDomain(id: number): Promise<CustomDomain> {
  const { data } = await apiClient.post<CustomDomain>(`/custom-domains/${id}/verify`)
  return data
}

export async function deleteCustomDomain(id: number): Promise<void> {
  await apiClient.delete(`/custom-domains/${id}`)
}

export async function getCustomDomainConfig(): Promise<CustomDomainConfig> {
  const { data } = await apiClient.get<CustomDomainConfig>('/admin/custom-domains/config')
  return data
}

export async function updateCustomDomainConfig(enabled: boolean): Promise<CustomDomainConfig> {
  const { data } = await apiClient.put<CustomDomainConfig>('/admin/custom-domains/config', { enabled })
  return data
}

export async function listAdminCustomDomains(filters: AdminCustomDomainFilters = {}): Promise<CustomDomain[]> {
  const params = Object.fromEntries(
    Object.entries(filters).filter(([, value]) => value !== undefined && value !== null && String(value).trim() !== ''),
  )
  const { data } = await apiClient.get<CustomDomain[]>('/admin/custom-domains', { params })
  return data
}

export async function createAdminCustomDomain(
  userId: number,
  domain: string,
  access: { all_users?: boolean; user_ids?: number[] } = {},
): Promise<CustomDomain> {
  const { data } = await apiClient.post<CustomDomain>('/admin/custom-domains', {
    user_id: userId,
    domain,
    all_users: Boolean(access.all_users),
    user_ids: access.user_ids || [],
  })
  return data
}

export async function updateAdminCustomDomainAccess(
  id: number,
  access: { all_users: boolean; user_ids: number[] },
): Promise<CustomDomain> {
  const { data } = await apiClient.put<CustomDomain>(`/admin/custom-domains/${id}/access`, {
    all_users: access.all_users,
    user_ids: access.user_ids,
  })
  return data
}

export async function verifyAdminCustomDomain(id: number): Promise<CustomDomain> {
  const { data } = await apiClient.post<CustomDomain>(`/admin/custom-domains/${id}/verify`)
  return data
}

export async function disableAdminCustomDomain(id: number, reason = ''): Promise<CustomDomain> {
  const { data } = await apiClient.post<CustomDomain>(`/admin/custom-domains/${id}/disable`, { reason })
  return data
}

export async function enableAdminCustomDomain(id: number): Promise<CustomDomain> {
  const { data } = await apiClient.post<CustomDomain>(`/admin/custom-domains/${id}/enable`)
  return data
}

export async function deleteAdminCustomDomain(id: number): Promise<void> {
  await apiClient.delete(`/admin/custom-domains/${id}`)
}

export const customDomainsAPI = {
  listUserCustomDomains,
  createCustomDomain,
  verifyCustomDomain,
  deleteCustomDomain,
  getCustomDomainConfig,
  updateCustomDomainConfig,
  listAdminCustomDomains,
  createAdminCustomDomain,
  updateAdminCustomDomainAccess,
  verifyAdminCustomDomain,
  disableAdminCustomDomain,
  enableAdminCustomDomain,
  deleteAdminCustomDomain,
}

export default customDomainsAPI
