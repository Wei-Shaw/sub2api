/**
 * User API endpoints
 * Handles user profile management and password changes
 */

import { apiClient REDACTED from './client'
import {
  resolveWeChatOAuthStartStrict,
  prepareOAuthBindAccessTokenCookie,
  type WeChatOAuthPublicSettings,
REDACTED from './auth'
import type {
  User,
  ChangePasswordRequest,
  NotifyEmailEntry,
  UserAuthProvider,
  UserAffiliateDetail,
  AffiliateTransferResponse,
  PlatformQuotasResponse,
REDACTED from '@/types'

/**
 * Get current user profile
 * @returns User profile data
 */
export async function getProfile(): Promise<User> {
  const { data REDACTED = await apiClient.get<User>('/user/profile')
  return data
REDACTED

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
REDACTED): Promise<User> {
  const { data REDACTED = await apiClient.put<User>('/user', profile)
  return data
REDACTED

/**
 * Change current user password
 * @param passwords - Old and new password
 * @returns Success message
 */
export async function changePassword(
  oldPassword: string,
  newPassword: string
): Promise<{ message: string REDACTED> {
  const payload: ChangePasswordRequest = {
    old_password: oldPassword,
    new_password: newPassword
  REDACTED

  const { data REDACTED = await apiClient.put<{ message: string REDACTED>('/user/password', payload)
  return data
REDACTED

/**
 * Send verification code for adding a notify email
 * @param email - Email address to verify
 */
export async function sendNotifyEmailCode(email: string): Promise<void> {
  await apiClient.post('/user/notify-email/send-code', { email REDACTED)
REDACTED

/**
 * Verify and add a notify email
 * @param email - Email address to add
 * @param code - Verification code
 */
export async function verifyNotifyEmail(email: string, code: string): Promise<void> {
  await apiClient.post('/user/notify-email/verify', { email, code REDACTED)
REDACTED

/**
 * Remove a notify email
 * @param email - Email address to remove
 */
export async function removeNotifyEmail(email: string): Promise<void> {
  await apiClient.delete('/user/notify-email', { data: { email REDACTED REDACTED)
REDACTED

/**
 * Toggle a notify email's disabled state
 * @param email - Email address (empty string for primary email placeholder)
 * @param disabled - Whether to disable the email
 */
export async function toggleNotifyEmail(email: string, disabled: boolean): Promise<User> {
  const { data REDACTED = await apiClient.put<User>('/user/notify-email/toggle', { email, disabled REDACTED)
  return data
REDACTED

export async function sendEmailBindingCode(email: string): Promise<void> {
  await apiClient.post('/user/account-bindings/email/send-code', { email REDACTED)
REDACTED

export async function bindEmailIdentity(payload: {
  email: string
  verify_code: string
  password: string
REDACTED): Promise<User> {
  const { data REDACTED = await apiClient.post<User>('/user/account-bindings/email', payload)
  return data
REDACTED

export async function unbindAuthIdentity(provider: BindableOAuthProvider): Promise<User> {
  const { data REDACTED = await apiClient.delete<User>(`/user/account-bindings/${providerREDACTED`)
  return data
REDACTED

export type BindableOAuthProvider = Exclude<UserAuthProvider, 'email'>

interface BuildOAuthBindingStartURLOptions {
  redirectTo?: string
  wechatOAuthSettings?: WeChatOAuthPublicSettings | null
REDACTED

export function resolveWeChatOAuthMode(): 'open' | 'mp' {
  if (typeof navigator === 'undefined') {
    return 'open'
  REDACTED
  return /MicroMessenger/i.test(navigator.userAgent) ? 'mp' : 'open'
REDACTED

function resolveWeChatOAuthBindingMode(
  settings?: WeChatOAuthPublicSettings | null
): 'open' | 'mp' | null {
  if (settings) {
    return resolveWeChatOAuthStartStrict(settings).mode
  REDACTED
  return resolveWeChatOAuthMode()
REDACTED

export function buildOAuthBindingStartURL(
  provider: BindableOAuthProvider,
  options: BuildOAuthBindingStartURLOptions = {REDACTED
): string | null {
  const redirectTo = options.redirectTo?.trim() || '/profile'
  const apiBase = (import.meta.env.VITE_API_BASE_URL as string | undefined) || '/api/v1'
  const normalized = apiBase.replace(/\/$/, '')
  const params = new URLSearchParams({
    redirect: redirectTo,
    intent: 'bind_current_user'
  REDACTED)

  if (provider === 'wechat') {
    const mode = resolveWeChatOAuthBindingMode(options.wechatOAuthSettings)
    if (!mode) {
      return null
    REDACTED
    params.set('mode', mode)
  REDACTED

  return `${normalizedREDACTED/auth/oauth/${providerREDACTED/bind/start?${params.toString()REDACTED`
REDACTED

export async function startOAuthBinding(
  provider: BindableOAuthProvider,
  options: BuildOAuthBindingStartURLOptions = {REDACTED
): Promise<void> {
  if (typeof window === 'undefined') {
    return
  REDACTED
  const startURL = buildOAuthBindingStartURL(provider, options)
  if (!startURL) {
    return
  REDACTED
  await prepareOAuthBindAccessTokenCookie()
  window.location.href = startURL
REDACTED

export async function getAffiliateDetail(): Promise<UserAffiliateDetail> {
  const { data REDACTED = await apiClient.get<UserAffiliateDetail>('/user/aff')
  return data
REDACTED

export async function transferAffiliateQuota(): Promise<AffiliateTransferResponse> {
  const { data REDACTED = await apiClient.post<AffiliateTransferResponse>('/user/aff/transfer')
  return data
REDACTED

/**
 * 获取当前用户的平台限额 + 用量。
 */
export async function getMyPlatformQuotas(): Promise<PlatformQuotasResponse> {
  const { data REDACTED = await apiClient.get<PlatformQuotasResponse>('/user/platform-quotas')
  return data
REDACTED

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
  startOAuthBinding,
  getAffiliateDetail,
  transferAffiliateQuota,
  getMyPlatformQuotas,
REDACTED

export default userAPI
