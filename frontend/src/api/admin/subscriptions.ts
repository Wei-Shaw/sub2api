/**
 * Admin Subscriptions API endpoints
 * Handles user subscription management for administrators
 */

import { apiClient REDACTED from '../client'
import type {
  UserSubscription,
  SubscriptionProgress,
  AssignSubscriptionRequest,
  BulkAssignSubscriptionRequest,
  ExtendSubscriptionRequest,
  PaginatedResponse
REDACTED from '@/types'

/**
 * List all subscriptions with pagination
 * @param page - Page number (default: 1)
 * @param pageSize - Items per page (default: 20)
 * @param filters - Optional filters (status, user_id, group_id, sort_by, sort_order)
 * @returns Paginated list of subscriptions
 */
export async function list(
  page: number = 1,
  pageSize: number = 20,
  filters?: {
    status?: 'active' | 'expired' | 'revoked' | 'suspended'
    user_id?: number
    group_id?: number
    platform?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  REDACTED,
  options?: {
    signal?: AbortSignal
  REDACTED
): Promise<PaginatedResponse<UserSubscription>> {
  const { data REDACTED = await apiClient.get<PaginatedResponse<UserSubscription>>(
    '/admin/subscriptions',
    {
      params: {
        page,
        page_size: pageSize,
        ...filters
      REDACTED,
      signal: options?.signal
    REDACTED
  )
  return data
REDACTED

/**
 * Get subscription by ID
 * @param id - Subscription ID
 * @returns Subscription details
 */
export async function getById(id: number): Promise<UserSubscription> {
  const { data REDACTED = await apiClient.get<UserSubscription>(`/admin/subscriptions/${idREDACTED`)
  return data
REDACTED

/**
 * Get subscription progress
 * @param id - Subscription ID
 * @returns Subscription progress with usage stats
 */
export async function getProgress(id: number): Promise<SubscriptionProgress> {
  const { data REDACTED = await apiClient.get<SubscriptionProgress>(`/admin/subscriptions/${idREDACTED/progress`)
  return data
REDACTED

/**
 * Assign subscription to user
 * @param request - Assignment request
 * @returns Created subscription
 */
export async function assign(request: AssignSubscriptionRequest): Promise<UserSubscription> {
  const { data REDACTED = await apiClient.post<UserSubscription>('/admin/subscriptions/assign', request)
  return data
REDACTED

/**
 * Bulk assign subscriptions to multiple users
 * @param request - Bulk assignment request
 * @returns Created subscriptions
 */
export async function bulkAssign(
  request: BulkAssignSubscriptionRequest
): Promise<UserSubscription[]> {
  const { data REDACTED = await apiClient.post<UserSubscription[]>(
    '/admin/subscriptions/bulk-assign',
    request
  )
  return data
REDACTED

/**
 * Extend subscription validity
 * @param id - Subscription ID
 * @param request - Extension request with days
 * @returns Updated subscription
 */
export async function extend(
  id: number,
  request: ExtendSubscriptionRequest
): Promise<UserSubscription> {
  const { data REDACTED = await apiClient.post<UserSubscription>(
    `/admin/subscriptions/${idREDACTED/extend`,
    request
  )
  return data
REDACTED

/**
 * Revoke subscription
 * @param id - Subscription ID
 * @returns Success confirmation
 */
export async function revoke(id: number): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.post<{ message: string REDACTED>(`/admin/subscriptions/${idREDACTED/revoke`)
  return data
REDACTED

/**
 * Restore revoked subscription
 * @param id - Subscription ID
 * @returns Restored subscription
 */
export async function restore(id: number): Promise<UserSubscription> {
  const { data REDACTED = await apiClient.post<UserSubscription>(`/admin/subscriptions/${idREDACTED/restore`)
  return data
REDACTED

/**
 * Reset daily, weekly, and/or monthly usage quota for a subscription
 * @param id - Subscription ID
 * @param options - Which windows to reset
 * @returns Updated subscription
 */
export async function resetQuota(
  id: number,
  options: { daily: boolean; weekly: boolean; monthly: boolean REDACTED
): Promise<UserSubscription> {
  const { data REDACTED = await apiClient.post<UserSubscription>(
    `/admin/subscriptions/${idREDACTED/reset-quota`,
    options
  )
  return data
REDACTED

/**
 * List subscriptions by group
 * @param groupId - Group ID
 * @param page - Page number
 * @param pageSize - Items per page
 * @returns Paginated list of subscriptions in the group
 */
export async function listByGroup(
  groupId: number,
  page: number = 1,
  pageSize: number = 20
): Promise<PaginatedResponse<UserSubscription>> {
  const { data REDACTED = await apiClient.get<PaginatedResponse<UserSubscription>>(
    `/admin/groups/${groupIdREDACTED/subscriptions`,
    {
      params: { page, page_size: pageSize REDACTED
    REDACTED
  )
  return data
REDACTED

/**
 * List subscriptions by user
 * @param userId - User ID
 * @param page - Page number
 * @param pageSize - Items per page
 * @returns Paginated list of user's subscriptions
 */
export async function listByUser(
  userId: number,
  page: number = 1,
  pageSize: number = 20
): Promise<PaginatedResponse<UserSubscription>> {
  const { data REDACTED = await apiClient.get<PaginatedResponse<UserSubscription>>(
    `/admin/users/${userIdREDACTED/subscriptions`,
    {
      params: { page, page_size: pageSize REDACTED
    REDACTED
  )
  return data
REDACTED

export const subscriptionsAPI = {
  list,
  getById,
  getProgress,
  assign,
  bulkAssign,
  extend,
  revoke,
  restore,
  resetQuota,
  listByGroup,
  listByUser
REDACTED

export default subscriptionsAPI
