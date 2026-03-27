/**
 * Admin Settings API endpoints
 * Handles system settings management for administrators
 */

import { apiClient } from '../client'
import type { CustomMenuItem, CustomEndpoint } from '@/types'

export interface DefaultSubscriptionSetting {
  group_id: number
  validity_days: number
}

/**
 * System settings interface
 */
export interface SystemSettings {
  // Registration settings
  registration_enabled: boolean
  email_verify_enabled: boolean
  registration_email_suffix_whitelist: string[]
  promo_code_enabled: boolean
  password_reset_enabled: boolean
  frontend_url: string
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
  backend_mode_enabled: boolean
  custom_menu_items: CustomMenuItem[]
  custom_endpoints: CustomEndpoint[]
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
  max_claude_code_version: string

  // 分组隔离
  allow_ungrouped_key_scheduling: boolean

  // Gateway forwarding behavior
  enable_fingerprint_unification: boolean
  enable_metadata_passthrough: boolean
}

export interface UpdateSettingsRequest {
  registration_enabled?: boolean
  email_verify_enabled?: boolean
  registration_email_suffix_whitelist?: string[]
  promo_code_enabled?: boolean
  password_reset_enabled?: boolean
  frontend_url?: string
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
  backend_mode_enabled?: boolean
  custom_menu_items?: CustomMenuItem[]
  custom_endpoints?: CustomEndpoint[]
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
  max_claude_code_version?: string
  allow_ungrouped_key_scheduling?: boolean
  enable_fingerprint_unification?: boolean
  enable_metadata_passthrough?: boolean
}

/**
 * Get all system settings
 * @returns System settings
 */
export async function getSettings(): Promise<SystemSettings> {
  const { data } = await apiClient.get<SystemSettings>('/admin/settings')
  return data
}

/**
 * Update system settings
 * @param settings - Partial settings to update
 * @returns Updated settings
 */
export async function updateSettings(settings: UpdateSettingsRequest): Promise<SystemSettings> {
  const { data } = await apiClient.put<SystemSettings>('/admin/settings', settings)
  return data
}

/**
 * Test SMTP connection request
 */
export interface TestSmtpRequest {
  smtp_host: string
  smtp_port: number
  smtp_username: string
  smtp_password: string
  smtp_use_tls: boolean
}

/**
 * Test SMTP connection with provided config
 * @param config - SMTP configuration to test
 * @returns Test result message
 */
export async function testSmtpConnection(config: TestSmtpRequest): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>('/admin/settings/test-smtp', config)
  return data
}

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
}

/**
 * Send test email with provided SMTP config
 * @param request - Email address and SMTP config
 * @returns Test result message
 */
export async function sendTestEmail(request: SendTestEmailRequest): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>(
    '/admin/settings/send-test-email',
    request
  )
  return data
}

/**
 * Admin API Key status response
 */
export interface AdminApiKeyStatus {
  exists: boolean
  masked_key: string
}

/**
 * Get admin API key status
 * @returns Status indicating if key exists and masked version
 */
export async function getAdminApiKey(): Promise<AdminApiKeyStatus> {
  const { data } = await apiClient.get<AdminApiKeyStatus>('/admin/settings/admin-api-key')
  return data
}

/**
 * Regenerate admin API key
 * @returns The new full API key (only shown once)
 */
export async function regenerateAdminApiKey(): Promise<{ key: string }> {
  const { data } = await apiClient.post<{ key: string }>('/admin/settings/admin-api-key/regenerate')
  return data
}

/**
 * Delete admin API key
 * @returns Success message
 */
export async function deleteAdminApiKey(): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>('/admin/settings/admin-api-key')
  return data
}

// ==================== Overload Cooldown Settings ====================

/**
 * Overload cooldown settings interface (529 handling)
 */
export interface OverloadCooldownSettings {
  enabled: boolean
  cooldown_minutes: number
}

export async function getOverloadCooldownSettings(): Promise<OverloadCooldownSettings> {
  const { data } = await apiClient.get<OverloadCooldownSettings>('/admin/settings/overload-cooldown')
  return data
}

export async function updateOverloadCooldownSettings(
  settings: OverloadCooldownSettings
): Promise<OverloadCooldownSettings> {
  const { data } = await apiClient.put<OverloadCooldownSettings>(
    '/admin/settings/overload-cooldown',
    settings
  )
  return data
}

// ==================== Stream Timeout Settings ====================

/**
 * Stream timeout settings interface
 */
export interface StreamTimeoutSettings {
  enabled: boolean
  action: 'temp_unsched' | 'error' | 'none'
  temp_unsched_minutes: number
  threshold_count: number
  threshold_window_minutes: number
}

/**
 * Get stream timeout settings
 * @returns Stream timeout settings
 */
export async function getStreamTimeoutSettings(): Promise<StreamTimeoutSettings> {
  const { data } = await apiClient.get<StreamTimeoutSettings>('/admin/settings/stream-timeout')
  return data
}

/**
 * Update stream timeout settings
 * @param settings - Stream timeout settings to update
 * @returns Updated settings
 */
export async function updateStreamTimeoutSettings(
  settings: StreamTimeoutSettings
): Promise<StreamTimeoutSettings> {
  const { data } = await apiClient.put<StreamTimeoutSettings>(
    '/admin/settings/stream-timeout',
    settings
  )
  return data
}

// ==================== Rectifier Settings ====================

/**
 * Rectifier settings interface
 */
export interface RectifierSettings {
  enabled: boolean
  thinking_signature_enabled: boolean
  thinking_budget_enabled: boolean
  apikey_signature_enabled: boolean
  apikey_signature_patterns: string[]
}

/**
 * Get rectifier settings
 * @returns Rectifier settings
 */
export async function getRectifierSettings(): Promise<RectifierSettings> {
  const { data } = await apiClient.get<RectifierSettings>('/admin/settings/rectifier')
  return data
}

/**
 * Update rectifier settings
 * @param settings - Rectifier settings to update
 * @returns Updated settings
 */
export async function updateRectifierSettings(
  settings: RectifierSettings
): Promise<RectifierSettings> {
  const { data } = await apiClient.put<RectifierSettings>(
    '/admin/settings/rectifier',
    settings
  )
  return data
}

// ==================== Beta Policy Settings ====================

/**
 * Beta policy rule interface
 */
export interface BetaPolicyRule {
  beta_token: string
  action: 'pass' | 'filter' | 'block'
  scope: 'all' | 'oauth' | 'apikey' | 'bedrock'
  error_message?: string
}

/**
 * Beta policy settings interface
 */
export interface BetaPolicySettings {
  rules: BetaPolicyRule[]
}

/**
 * Get beta policy settings
 * @returns Beta policy settings
 */
export async function getBetaPolicySettings(): Promise<BetaPolicySettings> {
  const { data } = await apiClient.get<BetaPolicySettings>('/admin/settings/beta-policy')
  return data
}

/**
 * Update beta policy settings
 * @param settings - Beta policy settings to update
 * @returns Updated settings
 */
export async function updateBetaPolicySettings(
  settings: BetaPolicySettings
): Promise<BetaPolicySettings> {
  const { data } = await apiClient.put<BetaPolicySettings>(
    '/admin/settings/beta-policy',
    settings
  )
  return data
}

// ==================== Sora Storage Settings ====================

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
}

export interface SoraS3Profile {
  profile_id: string
  name: string
  is_active: boolean
  provider: string // "s3" | "gdrive"
  enabled: boolean
  access_mode: string // "direct" | "proxy"
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
  // Google Drive fields
  auth_type: string // "oauth2" | "service_account"
  client_id: string
  client_secret_configured: boolean
  refresh_token_configured: boolean
  service_account_configured: boolean
  folder_id: string
}

export interface ListSoraS3ProfilesResponse {
  active_profile_id: string
  items: SoraS3Profile[]
}

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
}

export interface CreateSoraS3ProfileRequest {
  profile_id: string
  name: string
  set_active?: boolean
  provider?: string
  enabled: boolean
  access_mode?: string
  endpoint?: string
  region?: string
  bucket?: string
  access_key_id?: string
  secret_access_key?: string
  prefix?: string
  force_path_style?: boolean
  cdn_url?: string
  default_storage_quota_bytes?: number
  // Google Drive fields
  auth_type?: string
  client_id?: string
  client_secret?: string
  refresh_token?: string
  service_account_json?: string
  folder_id?: string
}

export interface UpdateSoraS3ProfileRequest {
  name: string
  enabled: boolean
  access_mode?: string
  endpoint?: string
  region?: string
  bucket?: string
  access_key_id?: string
  secret_access_key?: string
  prefix?: string
  force_path_style?: boolean
  cdn_url?: string
  default_storage_quota_bytes?: number
  // Google Drive fields
  auth_type?: string
  client_id?: string
  client_secret?: string
  refresh_token?: string
  service_account_json?: string
  folder_id?: string
}

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
}

export interface StartGDriveOAuthRequest {
  client_id: string
  client_secret: string
  redirect_uri: string
}

export interface StartGDriveOAuthResponse {
  auth_url: string
  state: string
}

export interface ExchangeGDriveOAuthCodeRequest {
  client_id: string
  client_secret: string
  redirect_uri: string
  code: string
  profile_id?: string
}

export interface ExchangeGDriveOAuthCodeResponse {
  refresh_token: string
  message: string
}

export async function getSoraS3Settings(): Promise<SoraS3Settings> {
  const { data } = await apiClient.get<SoraS3Settings>('/admin/settings/sora-storage')
  return data
}

export async function updateSoraS3Settings(settings: UpdateSoraS3SettingsRequest): Promise<SoraS3Settings> {
  const { data } = await apiClient.put<SoraS3Settings>('/admin/settings/sora-storage', settings)
  return data
}

export async function testSoraS3Connection(
  settings: TestSoraS3ConnectionRequest
): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>('/admin/settings/sora-storage/test', settings)
  return data
}

export async function listSoraS3Profiles(): Promise<ListSoraS3ProfilesResponse> {
  const { data } = await apiClient.get<ListSoraS3ProfilesResponse>('/admin/settings/sora-storage/profiles')
  return data
}

export async function createSoraS3Profile(request: CreateSoraS3ProfileRequest): Promise<SoraS3Profile> {
  const { data } = await apiClient.post<SoraS3Profile>('/admin/settings/sora-storage/profiles', request)
  return data
}

export async function updateSoraS3Profile(profileID: string, request: UpdateSoraS3ProfileRequest): Promise<SoraS3Profile> {
  const { data } = await apiClient.put<SoraS3Profile>(`/admin/settings/sora-storage/profiles/${profileID}`, request)
  return data
}

export async function deleteSoraS3Profile(profileID: string): Promise<void> {
  await apiClient.delete(`/admin/settings/sora-storage/profiles/${profileID}`)
}

export async function setActiveSoraS3Profile(profileID: string): Promise<SoraS3Profile> {
  const { data } = await apiClient.post<SoraS3Profile>(`/admin/settings/sora-storage/profiles/${profileID}/activate`)
  return data
}

export async function startGDriveOAuth(request: StartGDriveOAuthRequest): Promise<StartGDriveOAuthResponse> {
  const { data } = await apiClient.post<StartGDriveOAuthResponse>('/admin/settings/sora-storage/gdrive-oauth/start', request)
  return data
}

export async function exchangeGDriveOAuthCode(request: ExchangeGDriveOAuthCodeRequest): Promise<ExchangeGDriveOAuthCodeResponse> {
  const { data } = await apiClient.post<ExchangeGDriveOAuthCodeResponse>('/admin/settings/sora-storage/gdrive-oauth/callback', request)
  return data
}

export interface TestGDriveStorageResponse {
  status: string
  quota_limit_bytes?: number
  quota_used_bytes?: number
  uploaded_file_id?: string
  uploaded_file_name?: string
  uploaded_file_size?: number
  access_url?: string
  web_view_link?: string
  deleted?: boolean
  delete_warning?: string
}

export async function testGDriveStorage(): Promise<TestGDriveStorageResponse> {
  const { data } = await apiClient.post<TestGDriveStorageResponse>('/admin/settings/sora-storage/gdrive-test')
  return data
}

export interface GDriveQuotaInfo {
  limit_bytes: number
  used_bytes: number
}

export interface StorageVideoStats {
  [type: string]: { completed: number; in_progress: number }
}

export async function getGDriveQuota(): Promise<GDriveQuotaInfo> {
  const { data } = await apiClient.get<GDriveQuotaInfo>('/admin/settings/sora-storage/gdrive-quota')
  return data
}

export async function getStorageVideoStats(): Promise<StorageVideoStats> {
  const { data } = await apiClient.get<StorageVideoStats>('/admin/settings/sora-storage/video-stats')
  return data
}

export const settingsAPI = {
  getSettings,
  updateSettings,
  testSmtpConnection,
  sendTestEmail,
  getAdminApiKey,
  regenerateAdminApiKey,
  deleteAdminApiKey,
  getOverloadCooldownSettings,
  updateOverloadCooldownSettings,
  getStreamTimeoutSettings,
  updateStreamTimeoutSettings,
  getRectifierSettings,
  updateRectifierSettings,
  getBetaPolicySettings,
  updateBetaPolicySettings,
  getSoraS3Settings,
  updateSoraS3Settings,
  testSoraS3Connection,
  listSoraS3Profiles,
  createSoraS3Profile,
  updateSoraS3Profile,
  deleteSoraS3Profile,
  setActiveSoraS3Profile,
  startGDriveOAuth,
  exchangeGDriveOAuthCode,
  testGDriveStorage,
  getGDriveQuota,
  getStorageVideoStats
}

export default settingsAPI
