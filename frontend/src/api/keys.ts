/**
 * API Keys management endpoints
 * Handles CRUD operations for user API keys
 */

import { apiClient REDACTED from './client'
import type { ApiKey, CreateApiKeyRequest, UpdateApiKeyRequest, PaginatedResponse REDACTED from '@/types'

/**
 * List all API keys for current user
 * @param page - Page number (default: 1)
 * @param pageSize - Items per page (default: 10)
 * @param options - Optional request options
 * @returns Paginated list of API keys
 */
export async function list(
  page: number = 1,
  pageSize: number = 10,
  options?: {
    signal?: AbortSignal
  REDACTED
): Promise<PaginatedResponse<ApiKey>> {
  const { data REDACTED = await apiClient.get<PaginatedResponse<ApiKey>>('/keys', {
    params: { page, page_size: pageSize REDACTED,
    signal: options?.signal
  REDACTED)
  return data
REDACTED

/**
 * Get API key by ID
 * @param id - API key ID
 * @returns API key details
 */
export async function getById(id: number): Promise<ApiKey> {
  const { data REDACTED = await apiClient.get<ApiKey>(`/keys/${idREDACTED`)
  return data
REDACTED

/**
 * Create new API key
 * @param name - Key name
 * @param groupId - Optional group ID
 * @param customKey - Optional custom key value
 * @param ipWhitelist - Optional IP whitelist
 * @param ipBlacklist - Optional IP blacklist
 * @param quota - Optional quota limit in USD (0 = unlimited)
 * @param expiresInDays - Optional days until expiry (undefined = never expires)
 * @param rateLimitData - Optional rate limit fields
 * @returns Created API key
 */
export async function create(
  name: string,
  groupId?: number | null,
  customKey?: string,
  ipWhitelist?: string[],
  ipBlacklist?: string[],
  quota?: number,
  expiresInDays?: number,
  rateLimitData?: { rate_limit_5h?: number; rate_limit_1d?: number; rate_limit_7d?: number REDACTED
): Promise<ApiKey> {
  const payload: CreateApiKeyRequest = { name REDACTED
  if (groupId !== undefined) {
    payload.group_id = groupId
  REDACTED
  if (customKey) {
    payload.custom_key = customKey
  REDACTED
  if (ipWhitelist && ipWhitelist.length > 0) {
    payload.ip_whitelist = ipWhitelist
  REDACTED
  if (ipBlacklist && ipBlacklist.length > 0) {
    payload.ip_blacklist = ipBlacklist
  REDACTED
  if (quota !== undefined && quota > 0) {
    payload.quota = quota
  REDACTED
  if (expiresInDays !== undefined && expiresInDays > 0) {
    payload.expires_in_days = expiresInDays
  REDACTED
  if (rateLimitData?.rate_limit_5h && rateLimitData.rate_limit_5h > 0) {
    payload.rate_limit_5h = rateLimitData.rate_limit_5h
  REDACTED
  if (rateLimitData?.rate_limit_1d && rateLimitData.rate_limit_1d > 0) {
    payload.rate_limit_1d = rateLimitData.rate_limit_1d
  REDACTED
  if (rateLimitData?.rate_limit_7d && rateLimitData.rate_limit_7d > 0) {
    payload.rate_limit_7d = rateLimitData.rate_limit_7d
  REDACTED

  const { data REDACTED = await apiClient.post<ApiKey>('/keys', payload)
  return data
REDACTED

/**
 * Update API key
 * @param id - API key ID
 * @param updates - Fields to update
 * @returns Updated API key
 */
export async function update(id: number, updates: UpdateApiKeyRequest): Promise<ApiKey> {
  const { data REDACTED = await apiClient.put<ApiKey>(`/keys/${idREDACTED`, updates)
  return data
REDACTED

/**
 * Delete API key
 * @param id - API key ID
 * @returns Success confirmation
 */
export async function deleteKey(id: number): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.delete<{ message: string REDACTED>(`/keys/${idREDACTED`)
  return data
REDACTED

/**
 * Toggle API key status (active/inactive)
 * @param id - API key ID
 * @param status - New status
 * @returns Updated API key
 */
export async function toggleStatus(id: number, status: 'active' | 'inactive'): Promise<ApiKey> {
  return update(id, { status REDACTED)
REDACTED

export const keysAPI = {
  list,
  getById,
  create,
  update,
  delete: deleteKey,
  toggleStatus
REDACTED

export default keysAPI
