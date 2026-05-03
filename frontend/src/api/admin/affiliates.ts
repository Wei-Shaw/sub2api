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

export interface ListAffiliateRecordsParams {
  page?: number
  page_size?: number
  search?: string
  start_at?: string
  end_at?: string
  sort_by?: string
  sort_order?: 'asc' | 'desc'
  timezone?: string
REDACTED

export interface AffiliateInviteRecord {
  inviter_id: number
  inviter_email: string
  inviter_username: string
  invitee_id: number
  invitee_email: string
  invitee_username: string
  aff_code: string
  total_rebate: number
  created_at: string
REDACTED

export interface AffiliateRebateRecord {
  order_id: number
  out_trade_no: string
  inviter_id: number
  inviter_email: string
  inviter_username: string
  invitee_id: number
  invitee_email: string
  invitee_username: string
  order_amount: number
  pay_amount: number
  rebate_amount: number
  payment_type: string
  order_status: string
  created_at: string
REDACTED

export interface AffiliateTransferRecord {
  ledger_id: number
  user_id: number
  user_email: string
  username: string
  amount: number
  current_balance: number
  remaining_quota: number
  frozen_quota: number
  history_quota: number
  created_at: string
REDACTED

export interface AffiliateUserOverview {
  user_id: number
  email: string
  username: string
  aff_code: string
  rebate_rate_percent: number
  invited_count: number
  rebated_invitee_count: number
  available_quota: number
  history_quota: number
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

function recordParams(params: ListAffiliateRecordsParams = {REDACTED) {
  return {
    page: params.page ?? 1,
    page_size: params.page_size ?? 20,
    search: params.search ?? '',
    start_at: params.start_at || undefined,
    end_at: params.end_at || undefined,
    sort_by: params.sort_by || undefined,
    sort_order: params.sort_order || undefined,
    timezone: params.timezone || undefined,
  REDACTED
REDACTED

export async function listInviteRecords(
  params: ListAffiliateRecordsParams = {REDACTED,
): Promise<PaginatedResponse<AffiliateInviteRecord>> {
  const { data REDACTED = await apiClient.get<PaginatedResponse<AffiliateInviteRecord>>(
    '/admin/affiliates/invites',
    { params: recordParams(params) REDACTED,
  )
  return data
REDACTED

export async function listRebateRecords(
  params: ListAffiliateRecordsParams = {REDACTED,
): Promise<PaginatedResponse<AffiliateRebateRecord>> {
  const { data REDACTED = await apiClient.get<PaginatedResponse<AffiliateRebateRecord>>(
    '/admin/affiliates/rebates',
    { params: recordParams(params) REDACTED,
  )
  return data
REDACTED

export async function listTransferRecords(
  params: ListAffiliateRecordsParams = {REDACTED,
): Promise<PaginatedResponse<AffiliateTransferRecord>> {
  const { data REDACTED = await apiClient.get<PaginatedResponse<AffiliateTransferRecord>>(
    '/admin/affiliates/transfers',
    { params: recordParams(params) REDACTED,
  )
  return data
REDACTED

export async function getUserOverview(
  userId: number,
): Promise<AffiliateUserOverview> {
  const { data REDACTED = await apiClient.get<AffiliateUserOverview>(
    `/admin/affiliates/users/${userIdREDACTED/overview`,
  )
  return data
REDACTED

export const affiliatesAPI = {
  listUsers,
  lookupUsers,
  updateUserSettings,
  clearUserSettings,
  batchSetRate,
  listInviteRecords,
  listRebateRecords,
  listTransferRecords,
  getUserOverview,
REDACTED

export default affiliatesAPI
