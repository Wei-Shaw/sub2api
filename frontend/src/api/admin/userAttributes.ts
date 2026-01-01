/**
 * Admin User Attributes API endpoints
 * Handles user custom attribute definitions and values
 */

import { apiClient REDACTED from '../client'
import type {
  UserAttributeDefinition,
  UserAttributeValue,
  CreateUserAttributeRequest,
  UpdateUserAttributeRequest,
  UserAttributeValuesMap
REDACTED from '@/types'

/**
 * Get all attribute definitions
 */
export async function listDefinitions(): Promise<UserAttributeDefinition[]> {
  const { data REDACTED = await apiClient.get<UserAttributeDefinition[]>('/admin/user-attributes')
  return data
REDACTED

/**
 * Get enabled attribute definitions only
 */
export async function listEnabledDefinitions(): Promise<UserAttributeDefinition[]> {
  const { data REDACTED = await apiClient.get<UserAttributeDefinition[]>('/admin/user-attributes', {
    params: { enabled: true REDACTED
  REDACTED)
  return data
REDACTED

/**
 * Create a new attribute definition
 */
export async function createDefinition(
  request: CreateUserAttributeRequest
): Promise<UserAttributeDefinition> {
  const { data REDACTED = await apiClient.post<UserAttributeDefinition>('/admin/user-attributes', request)
  return data
REDACTED

/**
 * Update an attribute definition
 */
export async function updateDefinition(
  id: number,
  request: UpdateUserAttributeRequest
): Promise<UserAttributeDefinition> {
  const { data REDACTED = await apiClient.put<UserAttributeDefinition>(
    `/admin/user-attributes/${idREDACTED`,
    request
  )
  return data
REDACTED

/**
 * Delete an attribute definition
 */
export async function deleteDefinition(id: number): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.delete<{ message: string REDACTED>(`/admin/user-attributes/${idREDACTED`)
  return data
REDACTED

/**
 * Reorder attribute definitions
 */
export async function reorderDefinitions(ids: number[]): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.put<{ message: string REDACTED>('/admin/user-attributes/reorder', {
    ids
  REDACTED)
  return data
REDACTED

/**
 * Get user's attribute values
 */
export async function getUserAttributeValues(userId: number): Promise<UserAttributeValue[]> {
  const { data REDACTED = await apiClient.get<UserAttributeValue[]>(
    `/admin/users/${userIdREDACTED/attributes`
  )
  return data
REDACTED

/**
 * Update user's attribute values (batch)
 */
export async function updateUserAttributeValues(
  userId: number,
  values: UserAttributeValuesMap
): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.put<{ message: string REDACTED>(
    `/admin/users/${userIdREDACTED/attributes`,
    { values REDACTED
  )
  return data
REDACTED

/**
 * Batch response type
 */
export interface BatchUserAttributesResponse {
  attributes: Record<number, Record<number, string>>
REDACTED

/**
 * Get attribute values for multiple users
 */
export async function getBatchUserAttributes(
  userIds: number[]
): Promise<BatchUserAttributesResponse> {
  const { data REDACTED = await apiClient.post<BatchUserAttributesResponse>(
    '/admin/user-attributes/batch',
    { user_ids: userIds REDACTED
  )
  return data
REDACTED

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
REDACTED

export default userAttributesAPI
