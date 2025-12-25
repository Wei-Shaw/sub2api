/**
 * Admin Settings API endpoints
 * Handles system settings management for administrators
 */

import { apiClient } from '../client';

/**
 * System settings interface
 */
export interface SystemSettings {
  // Registration settings
  registration_enabled: boolean;
  email_verify_enabled: boolean;
  // Default settings
  default_balance: number;
  default_concurrency: number;
  // OEM settings
  site_name: string;
  site_logo: string;
  site_subtitle: string;
  api_base_url: string;
  contact_info: string;
  doc_url: string;
  // SMTP settings
  smtp_host: string;
  smtp_port: number;
  smtp_username: string;
  smtp_password: string;
  smtp_password_set: boolean;
  smtp_from_email: string;
  smtp_from_name: string;
  smtp_use_tls: boolean;
  // Cloudflare Turnstile settings
  turnstile_enabled: boolean;
  turnstile_site_key: string;
  turnstile_secret_key: string;
  turnstile_secret_key_set: boolean;
}

// 更新时不提交“已配置”标记字段。
export type UpdateSystemSettings = Omit<SystemSettings, 'smtp_password_set' | 'turnstile_secret_key_set'>;

/**
 * Get all system settings
 * @returns System settings
 */
export async function getSettings(): Promise<SystemSettings> {
  const { data } = await apiClient.get<SystemSettings>('/admin/settings');
  return data;
}

/**
 * Update system settings
 * @param settings - Partial settings to update
 * @returns Updated settings
 */
export async function updateSettings(settings: Partial<UpdateSystemSettings>): Promise<SystemSettings> {
  const { data } = await apiClient.put<SystemSettings>('/admin/settings', settings);
  return data;
}

/**
 * Test SMTP connection request
 */
export interface TestSmtpRequest {
  smtp_host: string;
  smtp_port: number;
  smtp_username: string;
  smtp_password: string;
  smtp_use_tls: boolean;
}

/**
 * Test SMTP connection with provided config
 * @param config - SMTP configuration to test
 * @returns Test result message
 */
export async function testSmtpConnection(config: TestSmtpRequest): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>('/admin/settings/test-smtp', config);
  return data;
}

/**
 * Send test email request
 */
export interface SendTestEmailRequest {
  email: string;
  smtp_host: string;
  smtp_port: number;
  smtp_username: string;
  smtp_password: string;
  smtp_from_email: string;
  smtp_from_name: string;
  smtp_use_tls: boolean;
}

/**
 * Send test email with provided SMTP config
 * @param request - Email address and SMTP config
 * @returns Test result message
 */
export async function sendTestEmail(request: SendTestEmailRequest): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>('/admin/settings/send-test-email', request);
  return data;
}

/**
 * Admin API Key status response
 */
export interface AdminApiKeyStatus {
  exists: boolean;
  masked_key: string;
}

/**
 * Get admin API key status
 * @returns Status indicating if key exists and masked version
 */
export async function getAdminApiKey(): Promise<AdminApiKeyStatus> {
  const { data } = await apiClient.get<AdminApiKeyStatus>('/admin/settings/admin-api-key');
  return data;
}

/**
 * Regenerate admin API key
 * @returns The new full API key (only shown once)
 */
export async function regenerateAdminApiKey(): Promise<{ key: string }> {
  const { data } = await apiClient.post<{ key: string }>('/admin/settings/admin-api-key/regenerate');
  return data;
}

/**
 * Delete admin API key
 * @returns Success message
 */
export async function deleteAdminApiKey(): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>('/admin/settings/admin-api-key');
  return data;
}

export const settingsAPI = {
  getSettings,
  updateSettings,
  testSmtpConnection,
  sendTestEmail,
  getAdminApiKey,
  regenerateAdminApiKey,
  deleteAdminApiKey,
};

export default settingsAPI;
