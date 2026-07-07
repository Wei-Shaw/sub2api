/**
 * Admin Error Passthrough Rules API endpoints
 * Handles error passthrough rule management for administrators
 */

import { apiClient } from '../client'

/**
 * Error passthrough rule interface
 */
export interface ErrorPassthroughRule {
  id: number
  name: string
  enabled: boolean
  priority: number
  error_codes: number[]
  keywords: string[]
  match_mode: 'any' | 'all'
  platforms: string[]
  passthrough_code: boolean
  response_code: number | null
  passthrough_body: boolean
  custom_message: string | null
  skip_monitoring: boolean
  description: string | null
  created_at: string
  updated_at: string
}

/**
 * Create rule request
 */
export interface CreateRuleRequest {
  name: string
  enabled?: boolean
  priority?: number
  error_codes?: number[]
  keywords?: string[]
  match_mode?: 'any' | 'all'
  platforms?: string[]
  passthrough_code?: boolean
  response_code?: number | null
  passthrough_body?: boolean
  custom_message?: string | null
  skip_monitoring?: boolean
  description?: string | null
}

/**
 * Update rule request
 */
export interface UpdateRuleRequest {
  name?: string
  enabled?: boolean
  priority?: number
  error_codes?: number[]
  keywords?: string[]
  match_mode?: 'any' | 'all'
  platforms?: string[]
  passthrough_code?: boolean
  response_code?: number | null
  passthrough_body?: boolean
  custom_message?: string | null
  skip_monitoring?: boolean
  description?: string | null
}

/**
 * List all error passthrough rules
 * @returns List of all rules sorted by priority
 */
export async function list(): Promise<ErrorPassthroughRule[]> {
  const { data } = await apiClient.get<ErrorPassthroughRule[]>('/admin/error-passthrough-rules')
  return data
}

/**
 * Get rule by ID
 * @param id - Rule ID
 * @returns Rule details
 */
export async function getById(id: number): Promise<ErrorPassthroughRule> {
  const { data } = await apiClient.get<ErrorPassthroughRule>(`/admin/error-passthrough-rules/${id}`)
  return data
}

/**
 * Create new rule
 * @param ruleData - Rule data
 * @returns Created rule
 */
export async function create(ruleData: CreateRuleRequest): Promise<ErrorPassthroughRule> {
  const { data } = await apiClient.post<ErrorPassthroughRule>('/admin/error-passthrough-rules', ruleData)
  return data
}

/**
 * Update rule
 * @param id - Rule ID
 * @param updates - Fields to update
 * @returns Updated rule
 */
export async function update(id: number, updates: UpdateRuleRequest): Promise<ErrorPassthroughRule> {
  const { data } = await apiClient.put<ErrorPassthroughRule>(`/admin/error-passthrough-rules/${id}`, updates)
  return data
}

/**
 * Delete rule
 * @param id - Rule ID
 * @returns Success confirmation
 */
export async function deleteRule(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/error-passthrough-rules/${id}`)
  return data
}

/**
 * Toggle rule enabled status
 * @param id - Rule ID
 * @param enabled - New enabled status
 * @returns Updated rule
 */
export async function toggleEnabled(id: number, enabled: boolean): Promise<ErrorPassthroughRule> {
  return update(id, { enabled })
}

export const errorPassthroughAPI = {
  list,
  getById,
  create,
  update,
  delete: deleteRule,
  toggleEnabled
}

export default errorPassthroughAPI
