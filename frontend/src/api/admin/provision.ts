import { apiClient } from '../client'
import type { AdminGroup } from '@/types'

function randomIdempotencyKey(scope: string): string {
  const random =
    typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
      ? crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(36).slice(2)}`
  return `${scope}:${random}`
}

function provisionIdempotencyKey(payload: ProvisionAPIKeyRequest): string {
  return `provision-api-key:${payload.order_id.trim()}:${payload.plan_code.trim().toLowerCase()}`
}

function idempotencyHeaders(key: string) {
  return {
    headers: {
      'Idempotency-Key': key
    }
  }
}

export interface ProvisionPlan {
  id: number
  code: string
  name: string
  group_id: number
  balance: number
  quota: number
  expires_in_days?: number | null
  rate_limit_5h: number
  rate_limit_1d: number
  rate_limit_7d: number
  concurrency: number
  rpm_limit: number
  enabled: boolean
  group?: AdminGroup
  created_at: string
  updated_at: string
}

export interface ProvisionPlanRequest {
  code: string
  name: string
  group_id: number
  balance: number
  quota: number
  expires_in_days?: number | null
  rate_limit_5h: number
  rate_limit_1d: number
  rate_limit_7d: number
  concurrency: number
  rpm_limit: number
  enabled: boolean
}

export interface ProvisionAPIKeyRequest {
  order_id: string
  plan_code: string
  customer_label?: string
}

export interface ProvisionResult {
  order_id: string
  api_key: string
  key_id: number
  user_id: number
  plan_code: string
  group_id: number
  balance: number
  quota: number
  rate_multiplier: number
}

export async function listPlans(): Promise<ProvisionPlan[]> {
  const { data } = await apiClient.get<ProvisionPlan[]>('/admin/provision/plans')
  return data || []
}

export async function createPlan(payload: ProvisionPlanRequest): Promise<ProvisionPlan> {
  const { data } = await apiClient.post<ProvisionPlan>(
    '/admin/provision/plans',
    payload,
    idempotencyHeaders(randomIdempotencyKey('provision-plan-create'))
  )
  return data
}

export async function updatePlan(id: number, payload: ProvisionPlanRequest): Promise<ProvisionPlan> {
  const { data } = await apiClient.put<ProvisionPlan>(
    `/admin/provision/plans/${id}`,
    payload,
    idempotencyHeaders(randomIdempotencyKey(`provision-plan-update:${id}`))
  )
  return data
}

export async function deletePlan(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/provision/plans/${id}`)
  return data
}

export async function provisionAPIKey(payload: ProvisionAPIKeyRequest): Promise<ProvisionResult> {
  const { data } = await apiClient.post<ProvisionResult>(
    '/admin/provision/api-keys',
    payload,
    idempotencyHeaders(provisionIdempotencyKey(payload))
  )
  return data
}

export default {
  listPlans,
  createPlan,
  updatePlan,
  deletePlan,
  provisionAPIKey
}
