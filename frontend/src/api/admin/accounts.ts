/**
 * Admin Accounts API endpoints
 * Handles AI platform account management for administrators
 */

import { apiClient REDACTED from '../client'
import type {
  Account,
  CreateAccountRequest,
  UpdateAccountRequest,
  PaginatedResponse,
  AccountUsageInfo,
  WindowStats,
  ClaudeModel,
  AccountUsageStatsResponse,
  TempUnschedulableStatus,
  AdminDataPayload,
  AdminDataImportResult,
  CodexSessionImportRequest,
  CodexSessionImportResult,
  CheckMixedChannelRequest,
  CheckMixedChannelResponse
REDACTED from '@/types'

/**
 * List all accounts with pagination
 * @param page - Page number (default: 1)
 * @param pageSize - Items per page (default: 20)
 * @param filters - Optional filters
 * @returns Paginated list of accounts
 */
export async function list(
  page: number = 1,
  pageSize: number = 20,
  filters?: {
    platform?: string
    type?: string
    status?: string
    group?: string
    search?: string
    privacy_mode?: string
    lite?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  REDACTED,
  options?: {
    signal?: AbortSignal
  REDACTED
): Promise<PaginatedResponse<Account>> {
  const { data REDACTED = await apiClient.get<PaginatedResponse<Account>>('/admin/accounts', {
    params: {
      page,
      page_size: pageSize,
      ...filters
    REDACTED,
    signal: options?.signal
  REDACTED)
  return data
REDACTED

export interface AccountListWithEtagResult {
  notModified: boolean
  etag: string | null
  data: PaginatedResponse<Account> | null
REDACTED

export async function listWithEtag(
  page: number = 1,
  pageSize: number = 20,
  filters?: {
    platform?: string
    type?: string
    status?: string
    group?: string
    search?: string
    privacy_mode?: string
    lite?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  REDACTED,
  options?: {
    signal?: AbortSignal
    etag?: string | null
  REDACTED
): Promise<AccountListWithEtagResult> {
  const headers: Record<string, string> = {REDACTED
  if (options?.etag) {
    headers['If-None-Match'] = options.etag
  REDACTED

  const response = await apiClient.get<PaginatedResponse<Account>>('/admin/accounts', {
    params: {
      page,
      page_size: pageSize,
      ...filters
    REDACTED,
    headers,
    signal: options?.signal,
    validateStatus: (status) => (status >= 200 && status < 300) || status === 304
  REDACTED)

  const etagHeader = typeof response.headers?.etag === 'string' ? response.headers.etag : null
  if (response.status === 304) {
    return {
      notModified: true,
      etag: etagHeader,
      data: null
    REDACTED
  REDACTED

  return {
    notModified: false,
    etag: etagHeader,
    data: response.data
  REDACTED
REDACTED

/**
 * Get account by ID
 * @param id - Account ID
 * @returns Account details
 */
export async function getById(id: number): Promise<Account> {
  const { data REDACTED = await apiClient.get<Account>(`/admin/accounts/${idREDACTED`)
  return data
REDACTED

/**
 * Create new account
 * @param accountData - Account data
 * @returns Created account
 */
export async function create(accountData: CreateAccountRequest): Promise<Account> {
  const { data REDACTED = await apiClient.post<Account>('/admin/accounts', accountData)
  return data
REDACTED

/**
 * Update account
 * @param id - Account ID
 * @param updates - Fields to update
 * @returns Updated account
 */
export async function update(id: number, updates: UpdateAccountRequest): Promise<Account> {
  const { data REDACTED = await apiClient.put<Account>(`/admin/accounts/${idREDACTED`, updates)
  return data
REDACTED

/**
 * Check mixed-channel risk for account-group binding.
 */
export async function checkMixedChannelRisk(
  payload: CheckMixedChannelRequest
): Promise<CheckMixedChannelResponse> {
  const { data REDACTED = await apiClient.post<CheckMixedChannelResponse>('/admin/accounts/check-mixed-channel', payload)
  return data
REDACTED

/**
 * Delete account
 * @param id - Account ID
 * @returns Success confirmation
 */
export async function deleteAccount(id: number): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.delete<{ message: string REDACTED>(`/admin/accounts/${idREDACTED`)
  return data
REDACTED

/**
 * Toggle account status
 * @param id - Account ID
 * @param status - New status
 * @returns Updated account
 */
export async function toggleStatus(id: number, status: 'active' | 'inactive'): Promise<Account> {
  return update(id, { status REDACTED)
REDACTED

/**
 * Test account connectivity
 * @param id - Account ID
 * @returns Test result
 */
export async function testAccount(id: number): Promise<{
  success: boolean
  message: string
  latency_ms?: number
REDACTED> {
  const { data REDACTED = await apiClient.post<{
    success: boolean
    message: string
    latency_ms?: number
  REDACTED>(`/admin/accounts/${idREDACTED/test`)
  return data
REDACTED

/**
 * Refresh account credentials
 * @param id - Account ID
 * @returns Updated account
 */
export async function refreshCredentials(id: number): Promise<Account> {
  const { data REDACTED = await apiClient.post<Account>(`/admin/accounts/${idREDACTED/refresh`)
  return data
REDACTED

/**
 * Apply OAuth credentials after re-authorization.
 *
 * Unlike `update()`, this endpoint:
 * - never overwrites the whole `extra` JSONB (merges incrementally instead),
 *   so persistent settings like `base_rpm`, `window_cost_limit`, `max_sessions`,
 *   `quota_*` and `privacy_mode` are preserved
 * - clears the account error and invalidates the token cache server-side
 */
export async function applyOAuthCredentials(
  id: number,
  payload: {
    type: 'oauth' | 'setup-token'
    credentials: Record<string, unknown>
    extra?: Record<string, unknown>
  REDACTED
): Promise<Account> {
  const { data REDACTED = await apiClient.post<Account>(
    `/admin/accounts/${idREDACTED/apply-oauth-credentials`,
    payload
  )
  return data
REDACTED

/**
 * Get account usage statistics
 * @param id - Account ID
 * @param days - Number of days (default: 30)
 * @returns Account usage statistics with history, summary, and models
 */
export async function getStats(id: number, days: number = 30): Promise<AccountUsageStatsResponse> {
  const { data REDACTED = await apiClient.get<AccountUsageStatsResponse>(`/admin/accounts/${idREDACTED/stats`, {
    params: { days REDACTED
  REDACTED)
  return data
REDACTED

/**
 * Clear account error
 * @param id - Account ID
 * @returns Updated account
 */
export async function clearError(id: number): Promise<Account> {
  const { data REDACTED = await apiClient.post<Account>(`/admin/accounts/${idREDACTED/clear-error`)
  return data
REDACTED

/**
 * Get account usage information (5h/7d window)
 * @param id - Account ID
 * @returns Account usage info
 */
export async function getUsage(id: number, source?: 'passive' | 'active', force?: boolean): Promise<AccountUsageInfo> {
  const params: Record<string, string> = {REDACTED
  if (source) params.source = source
  if (force) params.force = 'true'
  const { data REDACTED = await apiClient.get<AccountUsageInfo>(`/admin/accounts/${idREDACTED/usage`, {
    params: Object.keys(params).length > 0 ? params : undefined
  REDACTED)
  return data
REDACTED

/**
 * Clear account rate limit status
 * @param id - Account ID
 * @returns Updated account
 */
export async function clearRateLimit(id: number): Promise<Account> {
  const { data REDACTED = await apiClient.post<Account>(
    `/admin/accounts/${idREDACTED/clear-rate-limit`
  )
  return data
REDACTED

/**
 * Recover account runtime state in one call
 * @param id - Account ID
 * @returns Updated account
 */
export async function recoverState(id: number): Promise<Account> {
  const { data REDACTED = await apiClient.post<Account>(`/admin/accounts/${idREDACTED/recover-state`)
  return data
REDACTED

/**
 * Reset account quota usage
 * @param id - Account ID
 * @returns Updated account
 */
export async function resetAccountQuota(id: number): Promise<Account> {
  const { data REDACTED = await apiClient.post<Account>(
    `/admin/accounts/${idREDACTED/reset-quota`
  )
  return data
REDACTED

/**
 * Get temporary unschedulable status
 * @param id - Account ID
 * @returns Status with detail state if active
 */
export async function getTempUnschedulableStatus(id: number): Promise<TempUnschedulableStatus> {
  const { data REDACTED = await apiClient.get<TempUnschedulableStatus>(
    `/admin/accounts/${idREDACTED/temp-unschedulable`
  )
  return data
REDACTED

/**
 * Reset temporary unschedulable status
 * @param id - Account ID
 * @returns Success confirmation
 */
export async function resetTempUnschedulable(id: number): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.delete<{ message: string REDACTED>(
    `/admin/accounts/${idREDACTED/temp-unschedulable`
  )
  return data
REDACTED

/**
 * Generate OAuth authorization URL
 * @param endpoint - API endpoint path
 * @param config - Proxy configuration
 * @returns Auth URL and session ID
 */
export async function generateAuthUrl(
  endpoint: string,
  config: { proxy_id?: number REDACTED
): Promise<{ auth_url: string; session_id: string REDACTED> {
  const { data REDACTED = await apiClient.post<{ auth_url: string; session_id: string REDACTED>(endpoint, config)
  return data
REDACTED

/**
 * Exchange authorization code for tokens
 * @param endpoint - API endpoint path
 * @param exchangeData - Session ID, code, and optional proxy config
 * @returns Token information
 */
export async function exchangeCode(
  endpoint: string,
  exchangeData: { session_id: string; code: string; state?: string; proxy_id?: number REDACTED
): Promise<Record<string, unknown>> {
  const { data REDACTED = await apiClient.post<Record<string, unknown>>(endpoint, exchangeData)
  return data
REDACTED

/**
 * Batch create accounts
 * @param accounts - Array of account data
 * @returns Results of batch creation
 */
export async function batchCreate(accounts: CreateAccountRequest[]): Promise<{
  success: number
  failed: number
  results: Array<{ success: boolean; account?: Account; error?: string REDACTED>
REDACTED> {
  const { data REDACTED = await apiClient.post<{
    success: number
    failed: number
    results: Array<{ success: boolean; account?: Account; error?: string REDACTED>
  REDACTED>('/admin/accounts/batch', { accounts REDACTED)
  return data
REDACTED

/**
 * Batch update credentials fields for multiple accounts
 * @param request - Batch update request containing account IDs, field name, and value
 * @returns Results of batch update
 */
export async function batchUpdateCredentials(request: {
  account_ids: number[]
  field: string
  value: any
REDACTED): Promise<{
  success: number
  failed: number
  results: Array<{ account_id: number; success: boolean; error?: string REDACTED>
REDACTED> {
  const { data REDACTED = await apiClient.post<{
    success: number
    failed: number
    results: Array<{ account_id: number; success: boolean; error?: string REDACTED>
  REDACTED>('/admin/accounts/batch-update-credentials', request)
  return data
REDACTED

/**
 * Bulk update multiple accounts
 * @param accountIds - Array of account IDs
 * @param updates - Fields to update
 * @returns Success confirmation
 */
export async function bulkUpdate(
  accountIdsOrPayload: number[] | Record<string, unknown>,
  updates?: Record<string, unknown>
): Promise<{
  success: number
  failed: number
  success_ids?: number[]
  failed_ids?: number[]
  results: Array<{ account_id: number; success: boolean; error?: string REDACTED>
  REDACTED> {
  const payload = Array.isArray(accountIdsOrPayload)
    ? {
        account_ids: accountIdsOrPayload,
        ...(updates ?? {REDACTED)
      REDACTED
    : accountIdsOrPayload
  const { data REDACTED = await apiClient.post<{
    success: number
    failed: number
    success_ids?: number[]
    failed_ids?: number[]
    results: Array<{ account_id: number; success: boolean; error?: string REDACTED>
  REDACTED>('/admin/accounts/bulk-update', payload)
  return data
REDACTED

/**
 * Get account today statistics
 * @param id - Account ID
 * @returns Today's stats (requests, tokens, cost)
 */
export async function getTodayStats(id: number): Promise<WindowStats> {
  const { data REDACTED = await apiClient.get<WindowStats>(`/admin/accounts/${idREDACTED/today-stats`)
  return data
REDACTED

export interface BatchTodayStatsResponse {
  stats: Record<string, WindowStats>
REDACTED

/**
 * 批量获取多个账号的今日统计
 * @param accountIds - 账号 ID 列表
 * @returns 以账号 ID（字符串）为键的统计映射
 */
export async function getBatchTodayStats(accountIds: number[]): Promise<BatchTodayStatsResponse> {
  const { data REDACTED = await apiClient.post<BatchTodayStatsResponse>('/admin/accounts/today-stats/batch', {
    account_ids: accountIds
  REDACTED)
  return data
REDACTED

/**
 * Set account schedulable status
 * @param id - Account ID
 * @param schedulable - Whether the account should participate in scheduling
 * @returns Updated account
 */
export async function setSchedulable(id: number, schedulable: boolean): Promise<Account> {
  const { data REDACTED = await apiClient.post<Account>(`/admin/accounts/${idREDACTED/schedulable`, {
    schedulable
  REDACTED)
  return data
REDACTED

/**
 * Get available models for an account
 * @param id - Account ID
 * @returns List of available models for this account
 */
export async function getAvailableModels(id: number): Promise<ClaudeModel[]> {
  const { data REDACTED = await apiClient.get<ClaudeModel[]>(`/admin/accounts/${idREDACTED/models`)
  return data
REDACTED

export interface SyncUpstreamModelsResult {
  models: string[]
REDACTED

/**
 * Sync live supported models from the account's upstream model-list endpoint
 * @param id - Account ID
 * @returns List of model IDs returned by the upstream
 */
export async function syncUpstreamModels(id: number): Promise<SyncUpstreamModelsResult> {
  const { data REDACTED = await apiClient.post<SyncUpstreamModelsResult>(`/admin/accounts/${idREDACTED/models/sync-upstream`)
  return data
REDACTED

export interface SyncUpstreamPreviewParams {
  platform: string
  type: string
  base_url?: string
  api_key: string
REDACTED

/**
 * Preview upstream models without a saved account (create-flow)
 * @param params - Connection credentials
 * @returns List of model IDs returned by the upstream
 */
export async function syncUpstreamModelsPreview(params: SyncUpstreamPreviewParams): Promise<SyncUpstreamModelsResult> {
  const { data REDACTED = await apiClient.post<SyncUpstreamModelsResult>('/admin/accounts/models/sync-upstream-preview', params)
  return data
REDACTED

export interface CRSPreviewAccount {
  crs_account_id: string
  kind: string
  name: string
  platform: string
  type: string
REDACTED

export interface PreviewFromCRSResult {
  new_accounts: CRSPreviewAccount[]
  existing_accounts: CRSPreviewAccount[]
REDACTED

export async function previewFromCrs(params: {
  base_url: string
  username: string
  password: string
REDACTED): Promise<PreviewFromCRSResult> {
  const { data REDACTED = await apiClient.post<PreviewFromCRSResult>('/admin/accounts/sync/crs/preview', params)
  return data
REDACTED

export async function syncFromCrs(params: {
  base_url: string
  username: string
  password: string
  sync_proxies?: boolean
  selected_account_ids?: string[]
REDACTED): Promise<{
  created: number
  updated: number
  skipped: number
  failed: number
  items: Array<{
    crs_account_id: string
    kind: string
    name: string
    action: string
    error?: string
  REDACTED>
REDACTED> {
  const { data REDACTED = await apiClient.post<{
    created: number
    updated: number
    skipped: number
    failed: number
    items: Array<{
      crs_account_id: string
      kind: string
      name: string
      action: string
      error?: string
    REDACTED>
  REDACTED>('/admin/accounts/sync/crs', params)
  return data
REDACTED

export async function exportData(options?: {
  ids?: number[]
  filters?: {
    platform?: string
    type?: string
    status?: string
    group?: string
    privacy_mode?: string
    search?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  REDACTED
  includeProxies?: boolean
REDACTED): Promise<AdminDataPayload> {
  const params: Record<string, string> = {REDACTED
  if (options?.ids && options.ids.length > 0) {
    params.ids = options.ids.join(',')
  REDACTED else if (options?.filters) {
    const { platform, type, status, group, privacy_mode, search, sort_by, sort_order REDACTED = options.filters
    if (platform) params.platform = platform
    if (type) params.type = type
    if (status) params.status = status
    if (group) params.group = group
    if (privacy_mode) params.privacy_mode = privacy_mode
    if (search) params.search = search
    if (sort_by) params.sort_by = sort_by
    if (sort_order) params.sort_order = sort_order
  REDACTED
  if (options?.includeProxies === false) {
    params.include_proxies = 'false'
  REDACTED
  const { data REDACTED = await apiClient.get<AdminDataPayload>('/admin/accounts/data', { params REDACTED)
  return data
REDACTED

export async function importData(payload: {
  data: AdminDataPayload
  skip_default_group_bind?: boolean
REDACTED): Promise<AdminDataImportResult> {
  const { data REDACTED = await apiClient.post<AdminDataImportResult>('/admin/accounts/data', {
    data: payload.data,
    skip_default_group_bind: payload.skip_default_group_bind
  REDACTED)
  return data
REDACTED

export async function importCodexSession(payload: CodexSessionImportRequest): Promise<CodexSessionImportResult> {
  const { data REDACTED = await apiClient.post<CodexSessionImportResult>('/admin/accounts/import/codex-session', payload)
  return data
REDACTED

/**
 * Get Antigravity default model mapping from backend
 * @returns Default model mapping (from -> to)
 */
export async function getAntigravityDefaultModelMapping(): Promise<Record<string, string>> {
  const { data REDACTED = await apiClient.get<Record<string, string>>(
    '/admin/accounts/antigravity/default-model-mapping'
  )
  return data
REDACTED

/**
 * Refresh OpenAI token using refresh token
 * @param refreshToken - The refresh token
 * @param proxyId - Optional proxy ID
 * @returns Token information including access_token, email, etc.
 */
export async function refreshOpenAIToken(
  refreshToken: string,
  proxyId?: number | null,
  endpoint: string = '/admin/openai/refresh-token',
  clientId?: string
): Promise<Record<string, unknown>> {
  const payload: { refresh_token: string; proxy_id?: number; client_id?: string REDACTED = {
    refresh_token: refreshToken
  REDACTED
  if (proxyId) {
    payload.proxy_id = proxyId
  REDACTED
  if (clientId) {
    payload.client_id = clientId
  REDACTED
  const { data REDACTED = await apiClient.post<Record<string, unknown>>(endpoint, payload)
  return data
REDACTED

/**
 * Batch operation result type
 */
export interface BatchOperationResult {
  total: number
  success: number
  failed: number
  errors?: Array<{ account_id: number; error: string REDACTED>
  warnings?: Array<{ account_id: number; warning: string REDACTED>
REDACTED

/**
 * Revert account proxy to original before fallback
 * @param id - Account ID
 * @returns Success confirmation
 */
export async function revertProxyFallback(id: number): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.post<{ message: string REDACTED>(`/admin/accounts/${idREDACTED/revert-proxy-fallback`)
  return data
REDACTED

/**
 * Batch clear account errors
 * @param accountIds - Array of account IDs
 * @returns Batch operation result
 */
export async function batchClearError(accountIds: number[]): Promise<BatchOperationResult> {
  const { data REDACTED = await apiClient.post<BatchOperationResult>('/admin/accounts/batch-clear-error', {
    account_ids: accountIds
  REDACTED)
  return data
REDACTED

/**
 * Batch refresh account credentials
 * @param accountIds - Array of account IDs
 * @returns Batch operation result
 */
export async function batchRefresh(accountIds: number[]): Promise<BatchOperationResult> {
  const { data REDACTED = await apiClient.post<BatchOperationResult>('/admin/accounts/batch-refresh', {
    account_ids: accountIds,
  REDACTED, {
    timeout: 120000  // 120s timeout for large batch refreshes
  REDACTED)
  return data
REDACTED

/**
 * Set privacy for an Antigravity OAuth account
 * @param id - Account ID
 * @returns Updated account
 */
export async function setPrivacy(id: number): Promise<Account> {
  const { data REDACTED = await apiClient.post<Account>(`/admin/accounts/${idREDACTED/set-privacy`)
  return data
REDACTED

export const accountsAPI = {
  list,
  listWithEtag,
  getById,
  create,
  update,
  checkMixedChannelRisk,
  delete: deleteAccount,
  toggleStatus,
  testAccount,
  refreshCredentials,
  applyOAuthCredentials,
  getStats,
  clearError,
  getUsage,
  getTodayStats,
  getBatchTodayStats,
  clearRateLimit,
  recoverState,
  resetAccountQuota,
  getTempUnschedulableStatus,
  resetTempUnschedulable,
  setSchedulable,
  getAvailableModels,
  syncUpstreamModels,
  syncUpstreamModelsPreview,
  generateAuthUrl,
  exchangeCode,
  refreshOpenAIToken,
  batchCreate,
  batchUpdateCredentials,
  bulkUpdate,
  previewFromCrs,
  syncFromCrs,
  exportData,
  importData,
  importCodexSession,
  getAntigravityDefaultModelMapping,
  batchClearError,
  batchRefresh,
  setPrivacy,
  revertProxyFallback
REDACTED

export default accountsAPI
