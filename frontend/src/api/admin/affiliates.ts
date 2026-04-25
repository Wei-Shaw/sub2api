/**
 * Admin Affiliate API endpoints
 * Manage per-user affiliate (邀请返利) configurations:
 * exclusive invite codes (overrides aff_code) and exclusive rebate rates.
 */

import { apiClient REDACTED from '../client'
import type { PaginatedResponse REDACTED from '@/types'

export interface AffiliateAdminEntry {
  user_id: number
  email: string
  username: string
  aff_code: string
  aff_code_custom: boolean
  aff_rebate_rate_percent?: number | null
  aff_count: number
REDACTED

export interface ListAffiliateUsersParams {
  page?: number
  page_size?: number
  search?: string
REDACTED

export interface UpdateAffiliateUserRequest {
  aff_code?: string
  aff_rebate_rate_percent?: number | null
  /** Set true to explicitly clear the per-user rate (sets it to NULL). */
  clear_rebate_rate?: boolean
REDACTED

export interface BatchSetRateRequest {
  user_ids: number[]
  aff_rebate_rate_percent?: number | null
  /** Set true to clear rates instead of setting. */
  clear?: boolean
REDACTED

export interface SimpleUser {
  id: number
  email: string
  username: string
REDACTED

export async function listUsers(
  params: ListAffiliateUsersParams = {REDACTED,
): Promise<PaginatedResponse<AffiliateAdminEntry>> {
  const { data REDACTED = await apiClient.get<PaginatedResponse<AffiliateAdminEntry>>(
    '/admin/affiliates/users',
    {
      params: {
        page: params.page ?? 1,
        page_size: params.page_size ?? 20,
        search: params.search ?? '',
      REDACTED,
    REDACTED,
  )
  return data
REDACTED

export async function lookupUsers(q: string): Promise<SimpleUser[]> {
  const { data REDACTED = await apiClient.get<SimpleUser[]>(
    '/admin/affiliates/users/lookup',
    { params: { q REDACTED REDACTED,
  )
  return data
REDACTED

export async function updateUserSettings(
  userId: number,
  payload: UpdateAffiliateUserRequest,
): Promise<{ user_id: number REDACTED> {
  const { data REDACTED = await apiClient.put<{ user_id: number REDACTED>(
    `/admin/affiliates/users/${userIdREDACTED`,
    payload,
  )
  return data
REDACTED

export async function clearUserSettings(
  userId: number,
): Promise<{ user_id: number REDACTED> {
  const { data REDACTED = await apiClient.delete<{ user_id: number REDACTED>(
    `/admin/affiliates/users/${userIdREDACTED`,
  )
  return data
REDACTED

export async function batchSetRate(
  payload: BatchSetRateRequest,
): Promise<{ affected: number REDACTED> {
  const { data REDACTED = await apiClient.post<{ affected: number REDACTED>(
    '/admin/affiliates/users/batch-rate',
    payload,
  )
  return data
REDACTED

export const affiliatesAPI = {
  listUsers,
  lookupUsers,
  updateUserSettings,
  clearUserSettings,
  batchSetRate,
REDACTED

export default affiliatesAPI
