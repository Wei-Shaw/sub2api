/**
 * Admin Proxies API endpoints
 * Handles proxy server management for administrators
 */

import { apiClient REDACTED from '../client'
import type {
  Proxy,
  ProxyAccountSummary,
  ProxyQualityCheckResult,
  CreateProxyRequest,
  UpdateProxyRequest,
  PaginatedResponse,
  AdminDataPayload,
  AdminDataImportResult
REDACTED from '@/types'

/**
 * List all proxies with pagination
 * @param page - Page number (default: 1)
 * @param pageSize - Items per page (default: 20)
 * @param filters - Optional filters
 * @returns Paginated list of proxies
 */
export async function list(
  page: number = 1,
  pageSize: number = 20,
  filters?: {
    protocol?: string
    status?: 'active' | 'inactive' | 'expired'
    search?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  REDACTED,
  options?: {
    signal?: AbortSignal
  REDACTED
): Promise<PaginatedResponse<Proxy>> {
  const { data REDACTED = await apiClient.get<PaginatedResponse<Proxy>>('/admin/proxies', {
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
 * Get all active proxies (without pagination)
 * @returns List of all active proxies
 */
export async function getAll(): Promise<Proxy[]> {
  const { data REDACTED = await apiClient.get<Proxy[]>('/admin/proxies/all')
  return data
REDACTED

/**
 * Get all active proxies with account count (sorted by creation time desc)
 * @returns List of all active proxies with account count
 */
export async function getAllWithCount(): Promise<Proxy[]> {
  const { data REDACTED = await apiClient.get<Proxy[]>('/admin/proxies/all', {
    params: { with_count: 'true' REDACTED
  REDACTED)
  return data
REDACTED

/**
 * Get proxy by ID
 * @param id - Proxy ID
 * @returns Proxy details
 */
export async function getById(id: number): Promise<Proxy> {
  const { data REDACTED = await apiClient.get<Proxy>(`/admin/proxies/${idREDACTED`)
  return data
REDACTED

/**
 * Create new proxy
 * @param proxyData - Proxy data
 * @returns Created proxy
 */
export async function create(proxyData: CreateProxyRequest): Promise<Proxy> {
  const { data REDACTED = await apiClient.post<Proxy>('/admin/proxies', proxyData)
  return data
REDACTED

/**
 * Update proxy
 * @param id - Proxy ID
 * @param updates - Fields to update
 * @returns Updated proxy
 */
export async function update(id: number, updates: UpdateProxyRequest): Promise<Proxy> {
  const { data REDACTED = await apiClient.put<Proxy>(`/admin/proxies/${idREDACTED`, updates)
  return data
REDACTED

/**
 * Delete proxy
 * @param id - Proxy ID
 * @returns Success confirmation
 */
export async function deleteProxy(id: number): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.delete<{ message: string REDACTED>(`/admin/proxies/${idREDACTED`)
  return data
REDACTED

/**
 * Toggle proxy status
 * @param id - Proxy ID
 * @param status - New status
 * @returns Updated proxy
 */
export async function toggleStatus(id: number, status: 'active' | 'inactive'): Promise<Proxy> {
  return update(id, { status REDACTED)
REDACTED

/**
 * Test proxy connectivity
 * @param id - Proxy ID
 * @returns Test result with IP info
 */
export async function testProxy(id: number): Promise<{
  success: boolean
  message: string
  latency_ms?: number
  ip_address?: string
  city?: string
  region?: string
  country?: string
  country_code?: string
REDACTED> {
  const { data REDACTED = await apiClient.post<{
    success: boolean
    message: string
    latency_ms?: number
    ip_address?: string
    city?: string
    region?: string
    country?: string
    country_code?: string
  REDACTED>(`/admin/proxies/${idREDACTED/test`)
  return data
REDACTED

/**
 * Check proxy quality across common AI targets
 * @param id - Proxy ID
 * @returns Quality check result
 */
export async function checkProxyQuality(id: number): Promise<ProxyQualityCheckResult> {
  const { data REDACTED = await apiClient.post<ProxyQualityCheckResult>(`/admin/proxies/${idREDACTED/quality-check`)
  return data
REDACTED

/**
 * Get proxy usage statistics
 * @param id - Proxy ID
 * @returns Proxy usage statistics
 */
export async function getStats(id: number): Promise<{
  total_accounts: number
  active_accounts: number
  total_requests: number
  success_rate: number
  average_latency: number
REDACTED> {
  const { data REDACTED = await apiClient.get<{
    total_accounts: number
    active_accounts: number
    total_requests: number
    success_rate: number
    average_latency: number
  REDACTED>(`/admin/proxies/${idREDACTED/stats`)
  return data
REDACTED

/**
 * Get accounts using a proxy
 * @param id - Proxy ID
 * @returns List of accounts using the proxy
 */
export async function getProxyAccounts(id: number): Promise<ProxyAccountSummary[]> {
  const { data REDACTED = await apiClient.get<ProxyAccountSummary[]>(`/admin/proxies/${idREDACTED/accounts`)
  return data
REDACTED

/**
 * Batch create proxies
 * @param proxies - Array of proxy data to create
 * @returns Creation result with count of created and skipped
 */
export async function batchCreate(
  proxies: Array<{
    protocol: string
    host: string
    port: number
    username?: string
    password?: string
  REDACTED>
): Promise<{
  created: number
  skipped: number
REDACTED> {
  const { data REDACTED = await apiClient.post<{
    created: number
    skipped: number
  REDACTED>('/admin/proxies/batch', { proxies REDACTED)
  return data
REDACTED

export async function batchDelete(ids: number[]): Promise<{
  deleted_ids: number[]
  skipped: Array<{ id: number; reason: string REDACTED>
REDACTED> {
  const { data REDACTED = await apiClient.post<{
    deleted_ids: number[]
    skipped: Array<{ id: number; reason: string REDACTED>
  REDACTED>('/admin/proxies/batch-delete', { ids REDACTED)
  return data
REDACTED

export async function exportData(options?: {
  ids?: number[]
  filters?: {
    protocol?: string
    status?: 'active' | 'inactive' | 'expired'
    search?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  REDACTED
REDACTED): Promise<AdminDataPayload> {
  const params: Record<string, string> = {REDACTED
  if (options?.ids && options.ids.length > 0) {
    params.ids = options.ids.join(',')
  REDACTED else if (options?.filters) {
    const { protocol, status, search, sort_by, sort_order REDACTED = options.filters
    if (protocol) params.protocol = protocol
    if (status) params.status = status
    if (search) params.search = search
    if (sort_by) params.sort_by = sort_by
    if (sort_order) params.sort_order = sort_order
  REDACTED
  const { data REDACTED = await apiClient.get<AdminDataPayload>('/admin/proxies/data', { params REDACTED)
  return data
REDACTED

export async function importData(payload: {
  data: AdminDataPayload
REDACTED): Promise<AdminDataImportResult> {
  const { data REDACTED = await apiClient.post<AdminDataImportResult>('/admin/proxies/data', payload)
  return data
REDACTED

export const proxiesAPI = {
  list,
  getAll,
  getAllWithCount,
  getById,
  create,
  update,
  delete: deleteProxy,
  toggleStatus,
  testProxy,
  checkProxyQuality,
  getStats,
  getProxyAccounts,
  batchCreate,
  batchDelete,
  exportData,
  importData
REDACTED

export default proxiesAPI
