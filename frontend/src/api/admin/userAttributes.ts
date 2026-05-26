/**
 * Admin User Attributes API endpoints
 * Handles user custom attribute definitions and values
 */

import { apiClient } from '../client'
import type {
  UserAttributeDefinition,
  UserAttributeValue,
  CreateUserAttributeRequest,
  UpdateUserAttributeRequest,
  UserAttributeValuesMap
} from '@/types'

/**
 * Get all attribute definitions
 */
export async function listDefinitions(): Promise<UserAttributeDefinition[]> {
  const { data } = await apiClient.get<UserAttributeDefinition[]>('/admin/user-attributes')
  return data
}

/**
 * Get enabled attribute definitions only
 */
export async function listEnabledDefinitions(): Promise<UserAttributeDefinition[]> {
  const { data } = await apiClient.get<UserAttributeDefinition[]>('/admin/user-attributes', {
    params: { enabled: true }
  })
  return data
}

/**
 * Create a new attribute definition
 */
export async function createDefinition(
  request: CreateUserAttributeRequest
): Promise<UserAttributeDefinition> {
  const { data } = await apiClient.post<UserAttributeDefinition>('/admin/user-attributes', request)
  return data
}

/**
 * Update an attribute definition
 */
export async function updateDefinition(
  id: number,
  request: UpdateUserAttributeRequest
): Promise<UserAttributeDefinition> {
  const { data } = await apiClient.put<UserAttributeDefinition>(
    `/admin/user-attributes/${id}`,
    request
  )
  return data
}

/**
 * Delete an attribute definition
 */
export async function deleteDefinition(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/user-attributes/${id}`)
  return data
}

/**
 * Reorder attribute definitions
 */
export async function reorderDefinitions(ids: number[]): Promise<{ message: string }> {
  const { data } = await apiClient.put<{ message: string }>('/admin/user-attributes/reorder', {
    ids
  })
  return data
}

/**
 * Get user's attribute values
 */
export async function getUserAttributeValues(userId: number): Promise<UserAttributeValue[]> {
  const { data } = await apiClient.get<UserAttributeValue[]>(
    `/admin/users/${userId}/attributes`
  )
  return data
}

/**
 * Update user's attribute values (batch)
 */
export async function updateUserAttributeValues(
  userId: number,
  values: UserAttributeValuesMap
): Promise<{ message: string }> {
  const { data } = await apiClient.put<{ message: string }>(
    `/admin/users/${userId}/attributes`,
    { values }
  )
  return data
}

/**
 * Batch response type
 */
export interface BatchUserAttributesResponse {
  attributes: Record<number, Record<number, string>>
}

/**
 * Get attribute values for multiple users
 */
export async function getBatchUserAttributes(
  userIds: number[]
): Promise<BatchUserAttributesResponse> {
  const { data } = await apiClient.post<BatchUserAttributesResponse>(
    '/admin/user-attributes/batch',
    { user_ids: userIds }
  )
  return data
}

export const userAttributesAPI = {
  listDefinitions,
  listEnabledDefinitions,
  createDefinition,
  updateDefinition,
  deleteDefinition,
  reorderDefinitions,
  getUserAttributeValues,
  updateUserAttributeValues,
  getBatchUserAttributes
}

export default userAttributesAPI
