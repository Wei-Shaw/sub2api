/**
 * User API endpoints
 * Handles user profile management and password changes
 */

import { apiClient } from './client'
import {
  resolveWeChatOAuthStartStrict,
  prepareOAuthBindAccessTokenCookie,
  type WeChatOAuthPublicSettings,
} from './auth'
import type { User, ChangePasswordRequest, NotifyEmailEntry, UserAuthProvider } from '@/types'

/**
 * Get current user profile
 * @returns User profile data
 */
export async function getProfile(): Promise<User> {
  const { data } = await apiClient.get<User>('/user/profile')
  return data
}

/**
 * Update current user profile
 * @param profile - Profile data to update
 * @returns Updated user profile data
 */
export async function updateProfile(profile: {
  username?: string
  avatar_url?: string | null
  balance_notify_enabled?: boolean
  balance_notify_threshold?: number | null
  balance_notify_extra_emails?: NotifyEmailEntry[]
}): Promise<User> {
  const { data } = await apiClient.put<User>('/user', profile)
  return data
}

/**
 * Change current user password
 * @param passwords - Old and new password
 * @returns Success message
 */
export async function changePassword(
  oldPassword: string,
  newPassword: string
): Promise<{ message: string }> {
  const payload: ChangePasswordRequest = {
    old_password: oldPassword,
    new_password: newPassword
  }

  const { data } = await apiClient.put<{ message: string }>('/user/password', payload)
  return data
}

/**
 * Send verification code for adding a notify email
 * @param email - Email address to verify
 */
export async function sendNotifyEmailCode(email: string): Promise<void> {
  await apiClient.post('/user/notify-email/send-code', { email })
}

/**
 * Verify and add a notify email
 * @param email - Email address to add
 * @param code - Verification code
 */
export async function verifyNotifyEmail(email: string, code: string): Promise<void> {
  await apiClient.post('/user/notify-email/verify', { email, code })
}

/**
 * Remove a notify email
 * @param email - Email address to remove
 */
export async function removeNotifyEmail(email: string): Promise<void> {
  await apiClient.delete('/user/notify-email', { data: { email } })
}

/**
 * Toggle a notify email's disabled state
 * @param email - Email address (empty string for primary email placeholder)
 * @param disabled - Whether to disable the email
 */
export async function toggleNotifyEmail(email: string, disabled: boolean): Promise<User> {
  const { data } = await apiClient.put<User>('/user/notify-email/toggle', { email, disabled })
  return data
}

export async function sendEmailBindingCode(email: string): Promise<void> {
  await apiClient.post('/user/account-bindings/email/send-code', { email })
}

export async function bindEmailIdentity(payload: {
  email: string
  verify_code: string
  password: string
}): Promise<User> {
  const { data } = await apiClient.post<User>('/user/account-bindings/email', payload)
  return data
}

export async function unbindAuthIdentity(provider: BindableOAuthProvider): Promise<User> {
  const { data } = await apiClient.delete<User>(`/user/account-bindings/${provider}`)
  return data
}

export type BindableOAuthProvider = Exclude<UserAuthProvider, 'email'>

interface BuildOAuthBindingStartURLOptions {
  redirectTo?: string
  wechatOAuthSettings?: WeChatOAuthPublicSettings | null
}

export function resolveWeChatOAuthMode(): 'open' | 'mp' {
  if (typeof navigator === 'undefined') {
    return 'open'
  }
  return /MicroMessenger/i.test(navigator.userAgent) ? 'mp' : 'open'
}

function resolveWeChatOAuthBindingMode(
  settings?: WeChatOAuthPublicSettings | null
): 'open' | 'mp' | null {
  if (settings) {
    return resolveWeChatOAuthStartStrict(settings).mode
  }
  return resolveWeChatOAuthMode()
}

export function buildOAuthBindingStartURL(
  provider: BindableOAuthProvider,
  options: BuildOAuthBindingStartURLOptions = {}
): string | null {
  const redirectTo = options.redirectTo?.trim() || '/profile'
  const apiBase = (import.meta.env.VITE_API_BASE_URL as string | undefined) || '/api/v1'
  const normalized = apiBase.replace(/\/$/, '')
  const params = new URLSearchParams({
    redirect: redirectTo,
    intent: 'bind_current_user'
  })

  if (provider === 'wechat') {
    const mode = resolveWeChatOAuthBindingMode(options.wechatOAuthSettings)
    if (!mode) {
      return null
    }
    params.set('mode', mode)
  }

  return `${normalized}/auth/oauth/${provider}/bind/start?${params.toString()}`
}

export async function startOAuthBinding(
  provider: BindableOAuthProvider,
  options: BuildOAuthBindingStartURLOptions = {}
): Promise<void> {
  if (typeof window === 'undefined') {
    return
  }
  const startURL = buildOAuthBindingStartURL(provider, options)
  if (!startURL) {
    return
  }
  await prepareOAuthBindAccessTokenCookie()
  window.location.href = startURL
}

export const userAPI = {
  getProfile,
  updateProfile,
  changePassword,
  sendNotifyEmailCode,
  verifyNotifyEmail,
  removeNotifyEmail,
  toggleNotifyEmail,
  sendEmailBindingCode,
  bindEmailIdentity,
  unbindAuthIdentity,
  buildOAuthBindingStartURL,
  startOAuthBinding
}

export default userAPI
