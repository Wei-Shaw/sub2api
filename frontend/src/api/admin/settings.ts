/**
 * Admin Settings API endpoints
 * Handles system settings management for administrators
 */

import { apiClient REDACTED from '../client'

export interface DefaultSubscriptionSetting {
  group_id: number
  validity_days: number
REDACTED

/**
 * System settings interface
 */
export interface SystemSettings {
  // Registration settings
  registration_enabled: boolean
  email_verify_enabled: boolean
  promo_code_enabled: boolean
  password_reset_enabled: boolean
  invitation_code_enabled: boolean
  totp_enabled: boolean // TOTP 双因素认证
  totp_encryption_key_configured: boolean // TOTP 加密密钥是否已配置
  // Default settings
  default_balance: number
  default_concurrency: number
  default_subscriptions: DefaultSubscriptionSetting[]
  // OEM settings
  site_name: string
  site_logo: string
  site_subtitle: string
  api_base_url: string
  contact_info: string
  doc_url: string
  home_content: string
  hide_ccs_import_button: boolean
  purchase_subscription_enabled: boolean
  purchase_subscription_url: string
  sora_client_enabled: boolean
  // SMTP settings
  smtp_host: string
  smtp_port: number
  smtp_username: string
  smtp_password_configured: boolean
  smtp_from_email: string
  smtp_from_name: string
  smtp_use_tls: boolean
  // Cloudflare Turnstile settings
  turnstile_enabled: boolean
  turnstile_site_key: string
  turnstile_secret_key_configured: boolean

  // LinuxDo Connect OAuth settings
  linuxdo_connect_enabled: boolean
  linuxdo_connect_client_id: string
  linuxdo_connect_client_secret_configured: boolean
  linuxdo_connect_redirect_url: string

  // Model fallback configuration
  enable_model_fallback: boolean
  fallback_model_anthropic: string
  fallback_model_openai: string
  fallback_model_gemini: string
  fallback_model_antigravity: string

  // Identity patch configuration (Claude -> Gemini)
  enable_identity_patch: boolean
  identity_patch_prompt: string

  // Ops Monitoring (vNext)
  ops_monitoring_enabled: boolean
  ops_realtime_monitoring_enabled: boolean
  ops_query_mode_default: 'auto' | 'raw' | 'preagg' | string
  ops_metrics_interval_seconds: number

  // Claude Code version check
  min_claude_code_version: string
REDACTED

export interface UpdateSettingsRequest {
  registration_enabled?: boolean
  email_verify_enabled?: boolean
  promo_code_enabled?: boolean
  password_reset_enabled?: boolean
  invitation_code_enabled?: boolean
  totp_enabled?: boolean // TOTP 双因素认证
  default_balance?: number
  default_concurrency?: number
  default_subscriptions?: DefaultSubscriptionSetting[]
  site_name?: string
  site_logo?: string
  site_subtitle?: string
  api_base_url?: string
  contact_info?: string
  doc_url?: string
  home_content?: string
  hide_ccs_import_button?: boolean
  purchase_subscription_enabled?: boolean
  purchase_subscription_url?: string
  sora_client_enabled?: boolean
  smtp_host?: string
  smtp_port?: number
  smtp_username?: string
  smtp_password?: string
  smtp_from_email?: string
  smtp_from_name?: string
  smtp_use_tls?: boolean
  turnstile_enabled?: boolean
  turnstile_site_key?: string
  turnstile_secret_key?: string
  linuxdo_connect_enabled?: boolean
  linuxdo_connect_client_id?: string
  linuxdo_connect_client_secret?: string
  linuxdo_connect_redirect_url?: string
  enable_model_fallback?: boolean
  fallback_model_anthropic?: string
  fallback_model_openai?: string
  fallback_model_gemini?: string
  fallback_model_antigravity?: string
  enable_identity_patch?: boolean
  identity_patch_prompt?: string
  ops_monitoring_enabled?: boolean
  ops_realtime_monitoring_enabled?: boolean
  ops_query_mode_default?: 'auto' | 'raw' | 'preagg' | string
  ops_metrics_interval_seconds?: number
  min_claude_code_version?: string
REDACTED

/**
 * Get all system settings
 * @returns System settings
 */
export async function getSettings(): Promise<SystemSettings> {
  const { data REDACTED = await apiClient.get<SystemSettings>('/admin/settings')
  return data
REDACTED

/**
 * Update system settings
 * @param settings - Partial settings to update
 * @returns Updated settings
 */
export async function updateSettings(settings: UpdateSettingsRequest): Promise<SystemSettings> {
  const { data REDACTED = await apiClient.put<SystemSettings>('/admin/settings', settings)
  return data
REDACTED

/**
 * Test SMTP connection request
 */
export interface TestSmtpRequest {
  smtp_host: string
  smtp_port: number
  smtp_username: string
  smtp_password: string
  smtp_use_tls: boolean
REDACTED

/**
 * Test SMTP connection with provided config
 * @param config - SMTP configuration to test
 * @returns Test result message
 */
export async function testSmtpConnection(config: TestSmtpRequest): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.post<{ message: string REDACTED>('/admin/settings/test-smtp', config)
  return data
REDACTED

/**
 * Send test email request
 */
export interface SendTestEmailRequest {
  email: string
  smtp_host: string
  smtp_port: number
  smtp_username: string
  smtp_password: string
  smtp_from_email: string
  smtp_from_name: string
  smtp_use_tls: boolean
REDACTED

/**
 * Send test email with provided SMTP config
 * @param request - Email address and SMTP config
 * @returns Test result message
 */
export async function sendTestEmail(request: SendTestEmailRequest): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.post<{ message: string REDACTED>(
    '/admin/settings/send-test-email',
    request
  )
  return data
REDACTED

/**
 * Admin API Key status response
 */
export interface AdminApiKeyStatus {
  exists: boolean
  masked_key: string
REDACTED

/**
 * Get admin API key status
 * @returns Status indicating if key exists and masked version
 */
export async function getAdminApiKey(): Promise<AdminApiKeyStatus> {
  const { data REDACTED = await apiClient.get<AdminApiKeyStatus>('/admin/settings/admin-api-key')
  return data
REDACTED

/**
 * Regenerate admin API key
 * @returns The new full API key (only shown once)
 */
export async function regenerateAdminApiKey(): Promise<{ key: string REDACTED> {
  const { data REDACTED = await apiClient.post<{ key: string REDACTED>('/admin/settings/admin-api-key/regenerate')
  return data
REDACTED

/**
 * Delete admin API key
 * @returns Success message
 */
export async function deleteAdminApiKey(): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.delete<{ message: string REDACTED>('/admin/settings/admin-api-key')
  return data
REDACTED

/**
 * Stream timeout settings interface
 */
export interface StreamTimeoutSettings {
  enabled: boolean
  action: 'temp_unsched' | 'error' | 'none'
  temp_unsched_minutes: number
  threshold_count: number
  threshold_window_minutes: number
REDACTED

/**
 * Get stream timeout settings
 * @returns Stream timeout settings
 */
export async function getStreamTimeoutSettings(): Promise<StreamTimeoutSettings> {
  const { data REDACTED = await apiClient.get<StreamTimeoutSettings>('/admin/settings/stream-timeout')
  return data
REDACTED

/**
 * Update stream timeout settings
 * @param settings - Stream timeout settings to update
 * @returns Updated settings
 */
export async function updateStreamTimeoutSettings(
  settings: StreamTimeoutSettings
): Promise<StreamTimeoutSettings> {
  const { data REDACTED = await apiClient.put<StreamTimeoutSettings>(
    '/admin/settings/stream-timeout',
    settings
  )
  return data
REDACTED

// ==================== Sora S3 Settings ====================

export interface SoraS3Settings {
  enabled: boolean
  endpoint: string
  region: string
  bucket: string
  access_key_id: string
  secret_access_key_configured: boolean
  prefix: string
  force_path_style: boolean
  cdn_url: string
  default_storage_quota_bytes: number
REDACTED

export interface SoraS3Profile {
  profile_id: string
  name: string
  is_active: boolean
  enabled: boolean
  endpoint: string
  region: string
  bucket: string
  access_key_id: string
  secret_access_key_configured: boolean
  prefix: string
  force_path_style: boolean
  cdn_url: string
  default_storage_quota_bytes: number
  updated_at: string
REDACTED

export interface ListSoraS3ProfilesResponse {
  active_profile_id: string
  items: SoraS3Profile[]
REDACTED

export interface UpdateSoraS3SettingsRequest {
  profile_id?: string
  enabled: boolean
  endpoint: string
  region: string
  bucket: string
  access_key_id: string
  secret_access_key?: string
  prefix: string
  force_path_style: boolean
  cdn_url: string
  default_storage_quota_bytes: number
REDACTED

export interface CreateSoraS3ProfileRequest {
  profile_id: string
  name: string
  set_active?: boolean
  enabled: boolean
  endpoint: string
  region: string
  bucket: string
  access_key_id: string
  secret_access_key?: string
  prefix: string
  force_path_style: boolean
  cdn_url: string
  default_storage_quota_bytes: number
REDACTED

export interface UpdateSoraS3ProfileRequest {
  name: string
  enabled: boolean
  endpoint: string
  region: string
  bucket: string
  access_key_id: string
  secret_access_key?: string
  prefix: string
  force_path_style: boolean
  cdn_url: string
  default_storage_quota_bytes: number
REDACTED

export interface TestSoraS3ConnectionRequest {
  profile_id?: string
  enabled: boolean
  endpoint: string
  region: string
  bucket: string
  access_key_id: string
  secret_access_key?: string
  prefix: string
  force_path_style: boolean
  cdn_url: string
  default_storage_quota_bytes?: number
REDACTED

export async function getSoraS3Settings(): Promise<SoraS3Settings> {
  const { data REDACTED = await apiClient.get<SoraS3Settings>('/admin/settings/sora-s3')
  return data
REDACTED

export async function updateSoraS3Settings(settings: UpdateSoraS3SettingsRequest): Promise<SoraS3Settings> {
  const { data REDACTED = await apiClient.put<SoraS3Settings>('/admin/settings/sora-s3', settings)
  return data
REDACTED

export async function testSoraS3Connection(
  settings: TestSoraS3ConnectionRequest
): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.post<{ message: string REDACTED>('/admin/settings/sora-s3/test', settings)
  return data
REDACTED

export async function listSoraS3Profiles(): Promise<ListSoraS3ProfilesResponse> {
  const { data REDACTED = await apiClient.get<ListSoraS3ProfilesResponse>('/admin/settings/sora-s3/profiles')
  return data
REDACTED

export async function createSoraS3Profile(request: CreateSoraS3ProfileRequest): Promise<SoraS3Profile> {
  const { data REDACTED = await apiClient.post<SoraS3Profile>('/admin/settings/sora-s3/profiles', request)
  return data
REDACTED

export async function updateSoraS3Profile(profileID: string, request: UpdateSoraS3ProfileRequest): Promise<SoraS3Profile> {
  const { data REDACTED = await apiClient.put<SoraS3Profile>(`/admin/settings/sora-s3/profiles/${profileIDREDACTED`, request)
  return data
REDACTED

export async function deleteSoraS3Profile(profileID: string): Promise<void> {
  await apiClient.delete(`/admin/settings/sora-s3/profiles/${profileIDREDACTED`)
REDACTED

export async function setActiveSoraS3Profile(profileID: string): Promise<SoraS3Profile> {
  const { data REDACTED = await apiClient.post<SoraS3Profile>(`/admin/settings/sora-s3/profiles/${profileIDREDACTED/activate`)
  return data
REDACTED

export const settingsAPI = {
  getSettings,
  updateSettings,
  testSmtpConnection,
  sendTestEmail,
  getAdminApiKey,
  regenerateAdminApiKey,
  deleteAdminApiKey,
  getStreamTimeoutSettings,
  updateStreamTimeoutSettings,
  getSoraS3Settings,
  updateSoraS3Settings,
  testSoraS3Connection,
  listSoraS3Profiles,
  createSoraS3Profile,
  updateSoraS3Profile,
  deleteSoraS3Profile,
  setActiveSoraS3Profile
REDACTED

export default settingsAPI
