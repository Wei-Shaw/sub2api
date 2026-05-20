/**
 * Admin Redeem Codes API endpoints
 * Handles redeem code generation and management for administrators
 */

import { apiClient REDACTED from '../client'
import type {
  RedeemCode,
  GenerateRedeemCodesRequest,
  BatchUpdateRedeemCodeFields,
  RedeemCodeType,
  PaginatedResponse
REDACTED from '@/types'

/**
 * List all redeem codes with pagination
 * @param page - Page number (default: 1)
 * @param pageSize - Items per page (default: 20)
 * @param filters - Optional filters
 * @returns Paginated list of redeem codes
 */
export async function list(
  page: number = 1,
  pageSize: number = 20,
  filters?: {
    type?: RedeemCodeType
    status?: 'active' | 'used' | 'expired' | 'unused' | 'disabled'
    search?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  REDACTED,
  options?: {
    signal?: AbortSignal
  REDACTED
): Promise<PaginatedResponse<RedeemCode>> {
  const { data REDACTED = await apiClient.get<PaginatedResponse<RedeemCode>>('/admin/redeem-codes', {
    params: {
      page,
      page_size: pageSize,
      ...filters
    REDACTED,
    signal: options?.signal
  REDACTED)
  return data
REDACTED

/**
 * Get redeem code by ID
 * @param id - Redeem code ID
 * @returns Redeem code details
 */
export async function getById(id: number): Promise<RedeemCode> {
  const { data REDACTED = await apiClient.get<RedeemCode>(`/admin/redeem-codes/${idREDACTED`)
  return data
REDACTED

/**
 * Generate new redeem codes
 * @param count - Number of codes to generate
 * @param type - Type of redeem code
 * @param value - Value of the code
 * @param groupId - Group ID (required for subscription type)
 * @param validityDays - Validity days (for subscription type)
 * @param expiresInDays - Days before the code itself expires
 * @returns Array of generated redeem codes
 */
export async function generate(
  count: number,
  type: RedeemCodeType,
  value: number,
  groupId?: number | null,
  validityDays?: number,
  expiresInDays?: number | null
): Promise<RedeemCode[]> {
  const payload: GenerateRedeemCodesRequest = {
    count,
    type,
    value
  REDACTED

  // 订阅类型专用字段
  if (type === 'subscription') {
    payload.group_id = groupId
    if (validityDays && validityDays > 0) {
      payload.validity_days = validityDays
    REDACTED
  REDACTED
  if (expiresInDays && expiresInDays > 0) {
    payload.expires_in_days = expiresInDays
  REDACTED

  const { data REDACTED = await apiClient.post<RedeemCode[]>('/admin/redeem-codes/generate', payload)
  return data
REDACTED

/**
 * Delete redeem code
 * @param id - Redeem code ID
 * @returns Success confirmation
 */
export async function deleteCode(id: number): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.delete<{ message: string REDACTED>(`/admin/redeem-codes/${idREDACTED`)
  return data
REDACTED

/**
 * Batch delete redeem codes
 * @param ids - Array of redeem code IDs
 * @returns Success confirmation
 */
export async function batchDelete(ids: number[]): Promise<{
  deleted: number
  message: string
REDACTED> {
  const { data REDACTED = await apiClient.post<{
    deleted: number
    message: string
  REDACTED>('/admin/redeem-codes/batch-delete', { ids REDACTED)
  return data
REDACTED

/**
 * Batch update selected redeem code fields
 * @param ids - Array of redeem code IDs
 * @param fields - Field collection to update
 * @returns Updated count
 */
export async function batchUpdate(
  ids: number[],
  fields: BatchUpdateRedeemCodeFields
): Promise<{
  updated: number
  message: string
REDACTED> {
  const { data REDACTED = await apiClient.post<{
    updated: number
    message: string
  REDACTED>('/admin/redeem-codes/batch-update', { ids, fields REDACTED)
  return data
REDACTED

/**
 * Expire redeem code
 * @param id - Redeem code ID
 * @returns Updated redeem code
 */
export async function expire(id: number): Promise<RedeemCode> {
  const { data REDACTED = await apiClient.post<RedeemCode>(`/admin/redeem-codes/${idREDACTED/expire`)
  return data
REDACTED

/**
 * Get redeem code statistics
 * @returns Statistics about redeem codes
 */
export async function getStats(): Promise<{
  total_codes: number
  active_codes: number
  used_codes: number
  expired_codes: number
  total_value_distributed: number
  by_type: Record<RedeemCodeType, number>
REDACTED> {
  const { data REDACTED = await apiClient.get<{
    total_codes: number
    active_codes: number
    used_codes: number
    expired_codes: number
    total_value_distributed: number
    by_type: Record<RedeemCodeType, number>
  REDACTED>('/admin/redeem-codes/stats')
  return data
REDACTED

/**
 * Export redeem codes to CSV
 * @param filters - Optional filters
 * @returns CSV data as blob
 */
export async function exportCodes(filters?: {
  type?: RedeemCodeType
  status?: 'used' | 'expired' | 'unused' | 'disabled'
  search?: string
  sort_by?: string
  sort_order?: 'asc' | 'desc'
REDACTED): Promise<Blob> {
  const response = await apiClient.get('/admin/redeem-codes/export', {
    params: filters,
    responseType: 'blob'
  REDACTED)
  return response.data
REDACTED

export const redeemAPI = {
  list,
  getById,
  generate,
  delete: deleteCode,
  batchDelete,
  batchUpdate,
  expire,
  getStats,
  exportCodes
REDACTED

export default redeemAPI
