import apiClient from '@/api/client'

export interface ServiceQuotaRule {
  id: number
  enabled: boolean
  scope_level: string
  platform?: string | null
  group_id?: number | null
  account_id?: number | null
  model_pattern?: string | null
  limiter_type: string
  target_mode: string
  target_user_id?: number | null
  window_mode: string
  limit_value: number
  created_at: string
  updated_at: string
}

export type ServiceQuotaRuleInput = Omit<ServiceQuotaRule, 'id' | 'created_at' | 'updated_at'>

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
