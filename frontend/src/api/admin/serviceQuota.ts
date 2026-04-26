import apiClient from '@/api/client'

export interface ServiceQuotaRuleUserRef {
  id: number
  email: string
}

export interface ServiceQuotaLimiterDef {
  id: number
  rule_id: number
  limiter_type: string
  window_mode: string
  limit_value: number
}

export interface ServiceQuotaLimiterInput {
  limiter_type: string
  window_mode: string
  limit_value: number
}

export interface ServiceQuotaPathDef {
  id: number
  rule_id: number
  platform?: string | null
  channel_id?: number | null
  group_id?: number | null
  account_id?: number | null
  model_pattern?: string | null
}

export interface ServiceQuotaPathInput {
  platform?: string | null
  channel_id?: number | null
  group_id?: number | null
  account_id?: number | null
  model_pattern?: string | null
}

export interface ServiceQuotaRule {
  id: number
  enabled: boolean
  name?: string | null
  counter_mode: string
  is_fallback: boolean
  limiters: ServiceQuotaLimiterDef[]
  paths: ServiceQuotaPathDef[]
  target_user_ids?: number[] | null
  target_users?: ServiceQuotaRuleUserRef[] | null
  created_at: string
  updated_at: string
}

export interface ServiceQuotaRuleInput {
  enabled: boolean
  name?: string | null
  counter_mode: string
  is_fallback: boolean
  limiters: ServiceQuotaLimiterInput[]
  paths: ServiceQuotaPathInput[]
  target_user_ids?: number[] | null
}

export async function listServiceQuotaRules(): Promise<ServiceQuotaRule[]> {
  const { data } = await apiClient.get<{ items: ServiceQuotaRule[] }>('/admin/service-quotas')
  return data.items || []
}

export async function createServiceQuotaRule(input: ServiceQuotaRuleInput): Promise<ServiceQuotaRule> {
  const { data } = await apiClient.post<ServiceQuotaRule>('/admin/service-quotas', input)
  return data
}

export async function updateServiceQuotaRule(id: number, input: ServiceQuotaRuleInput): Promise<ServiceQuotaRule> {
  const { data } = await apiClient.put<ServiceQuotaRule>(`/admin/service-quotas/${id}`, input)
  return data
}

export async function deleteServiceQuotaRule(id: number): Promise<void> {
  await apiClient.delete(`/admin/service-quotas/${id}`)
}