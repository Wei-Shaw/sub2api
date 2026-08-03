/**
 * Authentication API endpoints
 * Handles user login, registration, and logout operations
 */

import { apiClient REDACTED from './client'
import { refreshAuthTokens, type RefreshTokenResponse REDACTED from './tokenRefresh'
export type { RefreshTokenResponse REDACTED from './tokenRefresh'
import type {
  LoginRequest,
  RegisterRequest,
  AuthResponse,
  CurrentUserResponse,
  SendVerifyCodeRequest,
  SendVerifyCodeResponse,
  PublicSettings,
  TotpLoginResponse,
  TotpLogin2FARequest
REDACTED from '@/types'

/**
 * Login response type - can be either full auth or 2FA required
 */
export type LoginResponse = AuthResponse | TotpLoginResponse

/**
 * Type guard to check if login response requires 2FA
 */
export function isTotp2FARequired(response: LoginResponse): response is TotpLoginResponse {
  return 'requires_2fa' in response && response.requires_2fa === true
REDACTED

/**
 * Store authentication token in localStorage
 */
export function setAuthToken(token: string): void {
  localStorage.setItem('auth_token', token)
REDACTED

/**
 * Store refresh token in localStorage
 */
export function setRefreshToken(token: string): void {
  localStorage.setItem('refresh_token', token)
REDACTED

/**
 * Store token expiration timestamp in localStorage
 * Converts expires_in (seconds) to absolute timestamp (milliseconds)
 */
export function setTokenExpiresAt(expiresIn: number): void {
  const expiresAt = Date.now() + expiresIn * 1000
  localStorage.setItem('token_expires_at', String(expiresAt))
REDACTED

/**
 * Get authentication token from localStorage
 */
export function getAuthToken(): string | null {
  return localStorage.getItem('auth_token')
REDACTED

/**
 * Get refresh token from localStorage
 */
export function getRefreshToken(): string | null {
  return localStorage.getItem('refresh_token')
REDACTED

/**
 * Get token expiration timestamp from localStorage
 */
export function getTokenExpiresAt(): number | null {
  const value = localStorage.getItem('token_expires_at')
  return value ? parseInt(value, 10) : null
REDACTED

/**
 * Clear authentication token from localStorage
 */
export function clearAuthToken(): void {
  localStorage.removeItem('auth_token')
  localStorage.removeItem('refresh_token')
  localStorage.removeItem('auth_user')
  localStorage.removeItem('token_expires_at')
REDACTED

/**
 * User login
 * @param credentials - Email and password
 * @returns Authentication response with token and user data, or 2FA required response
 */
export async function login(credentials: LoginRequest): Promise<LoginResponse> {
  const { data REDACTED = await apiClient.post<LoginResponse>('/auth/login', credentials)

  // Only store token if 2FA is not required
  if (!isTotp2FARequired(data)) {
    setAuthToken(data.access_token)
    if (data.refresh_token) {
      setRefreshToken(data.refresh_token)
    REDACTED
    if (data.expires_in) {
      setTokenExpiresAt(data.expires_in)
    REDACTED
    localStorage.setItem('auth_user', JSON.stringify(data.user))
  REDACTED

  return data
REDACTED

/**
 * Complete login with 2FA code
 * @param request - Temp token and TOTP code
 * @returns Authentication response with token and user data
 */
export async function login2FA(request: TotpLogin2FARequest): Promise<AuthResponse> {
  const { data REDACTED = await apiClient.post<AuthResponse>('/auth/login/2fa', request)

  // Store token and user data
  setAuthToken(data.access_token)
  if (data.refresh_token) {
    setRefreshToken(data.refresh_token)
  REDACTED
  if (data.expires_in) {
    setTokenExpiresAt(data.expires_in)
  REDACTED
  localStorage.setItem('auth_user', JSON.stringify(data.user))

  return data
REDACTED

/**
 * User registration
 * @param userData - Registration data (username, email, password)
 * @returns Authentication response with token and user data
 */
export async function register(userData: RegisterRequest): Promise<AuthResponse> {
  const { data REDACTED = await apiClient.post<AuthResponse>('/auth/register', userData)

  // Store token and user data
  setAuthToken(data.access_token)
  if (data.refresh_token) {
    setRefreshToken(data.refresh_token)
  REDACTED
  if (data.expires_in) {
    setTokenExpiresAt(data.expires_in)
  REDACTED
  localStorage.setItem('auth_user', JSON.stringify(data.user))

  return data
REDACTED

/**
 * Get current authenticated user
 * @returns User profile data
 */
export async function getCurrentUser() {
  return apiClient.get<CurrentUserResponse>('/auth/me')
REDACTED

/**
 * User logout
 * Clears authentication token and user data from localStorage
 * Optionally revokes the refresh token on the server
 */
export async function logout(): Promise<void> {
  const refreshToken = getRefreshToken()

  // Try to revoke the refresh token on the server
  if (refreshToken) {
    try {
      await apiClient.post('/auth/logout', { refresh_token: refreshToken REDACTED)
    REDACTED catch {
      // Ignore errors - we still want to clear local state
    REDACTED
  REDACTED

  clearAuthToken()
REDACTED

/**
 * Refresh token response
 */
export interface OAuthTokenResponse {
  access_token: string
  refresh_token?: string
  expires_in?: number
  token_type?: string
REDACTED

export interface PendingOAuthBindLoginResponse extends Partial<OAuthTokenResponse> {
  auth_result?: string
  redirect?: string
  error?: string
  requires_2fa?: boolean
  temp_token?: string
  user_email_masked?: string
  adoption_required?: boolean
  suggested_display_name?: string
  suggested_avatar_url?: string
REDACTED

export type PendingOAuthExchangeResponse = PendingOAuthBindLoginResponse

export interface PendingOAuthCreateAccountResponse extends OAuthTokenResponse {
  auth_result?: string
REDACTED

export interface PendingOAuthSendVerifyCodeResponse extends SendVerifyCodeResponse {
  auth_result?: string
  provider?: string
  redirect?: string
REDACTED

export type OAuthCompletionKind = 'login' | 'bind'

export interface OAuthAdoptionDecision {
  adoptDisplayName?: boolean
  adoptAvatar?: boolean
REDACTED

function serializeOAuthAdoptionDecision(
  decision?: OAuthAdoptionDecision
): Record<string, boolean> {
  const payload: Record<string, boolean> = {REDACTED

  if (typeof decision?.adoptDisplayName === 'boolean') {
    payload.adopt_display_name = decision.adoptDisplayName
  REDACTED
  if (typeof decision?.adoptAvatar === 'boolean') {
    payload.adopt_avatar = decision.adoptAvatar
  REDACTED

  return payload
REDACTED

export function isOAuthLoginCompletion(
  completion: Partial<OAuthTokenResponse>
): completion is OAuthTokenResponse {
  return typeof completion.access_token === 'string' && completion.access_token.trim().length > 0
REDACTED

export function getOAuthCompletionKind(
  completion: Partial<OAuthTokenResponse>
): OAuthCompletionKind {
  return isOAuthLoginCompletion(completion) ? 'login' : 'bind'
REDACTED

export function getPendingOAuthBindLoginKind(
  completion: PendingOAuthBindLoginResponse
): OAuthCompletionKind {
  return getOAuthCompletionKind(completion)
REDACTED

export function isPendingOAuthCreateAccountRequired(
  completion: Pick<PendingOAuthBindLoginResponse, 'error'>
): boolean {
  return completion.error === 'invitation_required'
REDACTED

export function hasPendingOAuthSuggestedProfile(
  completion: Pick<
    PendingOAuthBindLoginResponse,
    'suggested_display_name' | 'suggested_avatar_url'
  >
): boolean {
  return Boolean(completion.suggested_display_name || completion.suggested_avatar_url)
REDACTED

export function persistOAuthTokenContext(tokens: Partial<OAuthTokenResponse>): void {
  if (tokens.refresh_token) {
    setRefreshToken(tokens.refresh_token)
  REDACTED
  if (tokens.expires_in) {
    setTokenExpiresAt(tokens.expires_in)
  REDACTED
REDACTED

export async function prepareOAuthBindAccessTokenCookie(): Promise<void> {
  if (!getAuthToken()) {
    return
  REDACTED
  await apiClient.post('/auth/oauth/bind-token')
REDACTED

/**
 * Refresh the access token using the refresh token
 * @returns New token pair
 */
export async function refreshToken(): Promise<RefreshTokenResponse> {
  return refreshAuthTokens()
REDACTED

/**
 * Revoke all sessions for the current user
 * @returns Response with message
 */
export async function revokeAllSessions(): Promise<{ message: string REDACTED> {
  const { data REDACTED = await apiClient.post<{ message: string REDACTED>('/auth/revoke-all-sessions')
  return data
REDACTED

/**
 * Check if user is authenticated
 * @returns True if user has valid token
 */
export function isAuthenticated(): boolean {
  return getAuthToken() !== null
REDACTED

/**
 * Get public settings (no auth required)
 * @returns Public settings including registration and Turnstile config
 */
export async function getPublicSettings(): Promise<PublicSettings> {
  const { data REDACTED = await apiClient.get<PublicSettings>('/settings/public')
  return data
REDACTED

export type WeChatOAuthMode = 'open' | 'mp'
export type WeChatOAuthUnavailableReason =
  | 'not_configured'
  | 'capability_unknown'
  | 'external_browser_required'
  | 'wechat_browser_required'
  | 'native_app_required'

export interface ResolvedWeChatOAuthStart {
  mode: WeChatOAuthMode | null
  openEnabled: boolean
  mpEnabled: boolean
  mobileEnabled: boolean
  isWeChatBrowser: boolean
  unavailableReason: WeChatOAuthUnavailableReason | null
REDACTED

export type WeChatOAuthPublicSettings = {
  wechat_oauth_enabled?: boolean
  wechat_oauth_open_enabled?: boolean
  wechat_oauth_mp_enabled?: boolean
  wechat_oauth_mobile_enabled?: boolean
REDACTED

export function isWeChatWebOAuthEnabled(
  settings: WeChatOAuthPublicSettings | null | undefined,
): boolean {
  const legacyEnabled = settings?.wechat_oauth_enabled ?? false
  const hasExplicitCapabilities =
    typeof settings?.wechat_oauth_open_enabled === 'boolean' ||
    typeof settings?.wechat_oauth_mp_enabled === 'boolean'

  if (!hasExplicitCapabilities) {
    return legacyEnabled
  REDACTED

  return settings?.wechat_oauth_open_enabled === true || settings?.wechat_oauth_mp_enabled === true
REDACTED

export function hasExplicitWeChatOAuthCapabilities(
  settings: WeChatOAuthPublicSettings | null | undefined,
): settings is WeChatOAuthPublicSettings & {
  wechat_oauth_open_enabled: boolean
  wechat_oauth_mp_enabled: boolean
REDACTED {
  return typeof settings?.wechat_oauth_open_enabled === 'boolean'
    && typeof settings?.wechat_oauth_mp_enabled === 'boolean'
REDACTED

export function resolveWeChatOAuthStart(
  settings: WeChatOAuthPublicSettings | null | undefined,
  userAgent?: string
): ResolvedWeChatOAuthStart {
  const normalizedUserAgent = (userAgent
    ?? (typeof navigator !== 'undefined' ? navigator.userAgent : '')
    ?? '').trim()
  const isWeChatBrowser = /MicroMessenger/i.test(normalizedUserAgent)
  const legacyEnabled = settings?.wechat_oauth_enabled ?? false
  const openEnabled = typeof settings?.wechat_oauth_open_enabled === 'boolean'
    ? settings.wechat_oauth_open_enabled
    : legacyEnabled
  const mpEnabled = typeof settings?.wechat_oauth_mp_enabled === 'boolean'
    ? settings.wechat_oauth_mp_enabled
    : legacyEnabled
  const mobileEnabled = typeof settings?.wechat_oauth_mobile_enabled === 'boolean'
    ? settings.wechat_oauth_mobile_enabled
    : false

  if (isWeChatBrowser) {
    if (mpEnabled) {
      return { mode: 'mp', openEnabled, mpEnabled, mobileEnabled, isWeChatBrowser, unavailableReason: null REDACTED
    REDACTED
    if (openEnabled) {
      return { mode: null, openEnabled, mpEnabled, mobileEnabled, isWeChatBrowser, unavailableReason: 'external_browser_required' REDACTED
    REDACTED
    return { mode: null, openEnabled, mpEnabled, mobileEnabled, isWeChatBrowser, unavailableReason: 'not_configured' REDACTED
  REDACTED

  if (openEnabled) {
    return { mode: 'open', openEnabled, mpEnabled, mobileEnabled, isWeChatBrowser, unavailableReason: null REDACTED
  REDACTED
  if (mpEnabled) {
    return { mode: null, openEnabled, mpEnabled, mobileEnabled, isWeChatBrowser, unavailableReason: 'wechat_browser_required' REDACTED
  REDACTED
  return { mode: null, openEnabled, mpEnabled, mobileEnabled, isWeChatBrowser, unavailableReason: 'not_configured' REDACTED
REDACTED

export function resolveWeChatOAuthStartStrict(
  settings: WeChatOAuthPublicSettings | null | undefined,
  userAgent?: string,
): ResolvedWeChatOAuthStart {
  const normalizedUserAgent = (userAgent
    ?? (typeof navigator !== 'undefined' ? navigator.userAgent : '')
    ?? '').trim()
  const isWeChatBrowser = /MicroMessenger/i.test(normalizedUserAgent)

  if (!hasExplicitWeChatOAuthCapabilities(settings)) {
    return {
      mode: null,
      openEnabled: false,
      mpEnabled: false,
      mobileEnabled: false,
      isWeChatBrowser,
      unavailableReason: 'capability_unknown',
    REDACTED
  REDACTED

  return resolveWeChatOAuthStart(settings, normalizedUserAgent)
REDACTED

/**
 * Send verification code to email
 * @param request - Email and optional Turnstile token
 * @returns Response with countdown seconds
 */
export async function sendVerifyCode(
  request: SendVerifyCodeRequest
): Promise<SendVerifyCodeResponse> {
  const { data REDACTED = await apiClient.post<SendVerifyCodeResponse>('/auth/send-verify-code', request)
  return data
REDACTED

export async function sendPendingOAuthVerifyCode(
  request: SendVerifyCodeRequest
): Promise<PendingOAuthSendVerifyCodeResponse> {
  const { data REDACTED = await apiClient.post<PendingOAuthSendVerifyCodeResponse>(
    '/auth/oauth/pending/send-verify-code',
    request
  )
  return data
REDACTED

/**
 * Validate promo code response
 */
export interface ValidatePromoCodeResponse {
  valid: boolean
  bonus_amount?: number
  error_code?: string
  message?: string
REDACTED

/**
 * Validate promo code (public endpoint, no auth required)
 * @param code - Promo code to validate
 * @returns Validation result with bonus amount if valid
 */
export async function validatePromoCode(code: string): Promise<ValidatePromoCodeResponse> {
  const { data REDACTED = await apiClient.post<ValidatePromoCodeResponse>('/auth/validate-promo-code', { code REDACTED)
  return data
REDACTED

/**
 * Validate invitation code response
 */
export interface ValidateInvitationCodeResponse {
  valid: boolean
  error_code?: string
REDACTED

/**
 * Validate invitation code (public endpoint, no auth required)
 * @param code - Invitation code to validate
 * @returns Validation result
 */
export async function validateInvitationCode(code: string): Promise<ValidateInvitationCodeResponse> {
  const { data REDACTED = await apiClient.post<ValidateInvitationCodeResponse>('/auth/validate-invitation-code', { code REDACTED)
  return data
REDACTED

/**
 * Forgot password request
 */
export interface ForgotPasswordRequest {
  email: string
  turnstile_token?: string
REDACTED

/**
 * Forgot password response
 */
export interface ForgotPasswordResponse {
  message: string
REDACTED

/**
 * Request password reset link
 * @param request - Email and optional Turnstile token
 * @returns Response with message
 */
export async function forgotPassword(request: ForgotPasswordRequest): Promise<ForgotPasswordResponse> {
  const { data REDACTED = await apiClient.post<ForgotPasswordResponse>('/auth/forgot-password', request)
  return data
REDACTED

/**
 * Reset password request
 */
export interface ResetPasswordRequest {
  email: string
  token: string
  new_password: string
REDACTED

/**
 * Reset password response
 */
export interface ResetPasswordResponse {
  message: string
REDACTED

/**
 * Reset password with token
 * @param request - Email, token, and new password
 * @returns Response with message
 */
export async function resetPassword(request: ResetPasswordRequest): Promise<ResetPasswordResponse> {
  const { data REDACTED = await apiClient.post<ResetPasswordResponse>('/auth/reset-password', request)
  return data
REDACTED

/**
 * Complete LinuxDo OAuth registration by supplying an invitation code
 * @param invitationCode - Invitation code entered by the user
 * @returns Token pair on success
 */
export async function completeLinuxDoOAuthRegistration(
  invitationCode: string,
  decision?: OAuthAdoptionDecision,
  affiliateCode?: string
): Promise<OAuthTokenResponse> {
  return createPendingLinuxDoOAuthAccount(invitationCode, decision, affiliateCode)
REDACTED

/**
 * Complete OIDC OAuth registration by supplying an invitation code
 * @param invitationCode - Invitation code entered by the user
 * @returns Token pair on success
 */
export async function completeOIDCOAuthRegistration(
  invitationCode: string,
  decision?: OAuthAdoptionDecision,
  affiliateCode?: string
): Promise<OAuthTokenResponse> {
  return createPendingOIDCOAuthAccount(invitationCode, decision, affiliateCode)
REDACTED

export async function completeWeChatOAuthRegistration(
  invitationCode: string,
  decision?: OAuthAdoptionDecision,
  affiliateCode?: string
): Promise<OAuthTokenResponse> {
  return createPendingWeChatOAuthAccount(invitationCode, decision, affiliateCode)
REDACTED

async function createPendingOAuthAccount(
  provider: 'linuxdo' | 'oidc' | 'wechat' | 'dingtalk',
  invitationCode: string,
  decision?: OAuthAdoptionDecision,
  affiliateCode?: string
): Promise<PendingOAuthCreateAccountResponse> {
  const normalizedAffiliateCode = affiliateCode?.trim()
  const { data REDACTED = await apiClient.post<PendingOAuthCreateAccountResponse>(
    `/auth/oauth/${providerREDACTED/complete-registration`,
    {
      invitation_code: invitationCode,
      ...(normalizedAffiliateCode ? { aff_code: normalizedAffiliateCode REDACTED : {REDACTED),
      ...serializeOAuthAdoptionDecision(decision)
    REDACTED
  )
  return data
REDACTED

export async function createPendingLinuxDoOAuthAccount(
  invitationCode: string,
  decision?: OAuthAdoptionDecision,
  affiliateCode?: string
): Promise<PendingOAuthCreateAccountResponse> {
  return createPendingOAuthAccount('linuxdo', invitationCode, decision, affiliateCode)
REDACTED

export async function createPendingOIDCOAuthAccount(
  invitationCode: string,
  decision?: OAuthAdoptionDecision,
  affiliateCode?: string
): Promise<PendingOAuthCreateAccountResponse> {
  return createPendingOAuthAccount('oidc', invitationCode, decision, affiliateCode)
REDACTED

export async function createPendingWeChatOAuthAccount(
  invitationCode: string,
  decision?: OAuthAdoptionDecision,
  affiliateCode?: string
): Promise<PendingOAuthCreateAccountResponse> {
  return createPendingOAuthAccount('wechat', invitationCode, decision, affiliateCode)
REDACTED

export async function createPendingDingTalkOAuthAccount(
  invitationCode: string,
  decision?: OAuthAdoptionDecision,
  affiliateCode?: string
): Promise<PendingOAuthCreateAccountResponse> {
  return createPendingOAuthAccount('dingtalk', invitationCode, decision, affiliateCode)
REDACTED

export async function completePendingOAuthBindLogin(
  decision?: OAuthAdoptionDecision
): Promise<PendingOAuthBindLoginResponse> {
  const { data REDACTED = await apiClient.post<PendingOAuthBindLoginResponse>(
    '/auth/oauth/pending/exchange',
    serializeOAuthAdoptionDecision(decision)
  )
  return data
REDACTED

export async function exchangePendingOAuthCompletion(
  decision?: OAuthAdoptionDecision
): Promise<PendingOAuthExchangeResponse> {
  return completePendingOAuthBindLogin(decision)
REDACTED

export const authAPI = {
  login,
  login2FA,
  isTotp2FARequired,
  register,
  getCurrentUser,
  logout,
  isAuthenticated,
  setAuthToken,
  setRefreshToken,
  setTokenExpiresAt,
  getAuthToken,
  getRefreshToken,
  getTokenExpiresAt,
  clearAuthToken,
  getPublicSettings,
  sendVerifyCode,
  sendPendingOAuthVerifyCode,
  validatePromoCode,
  validateInvitationCode,
  forgotPassword,
  resetPassword,
  refreshToken,
  revokeAllSessions,
  getPendingOAuthBindLoginKind,
  isPendingOAuthCreateAccountRequired,
  hasPendingOAuthSuggestedProfile,
  completePendingOAuthBindLogin,
  createPendingLinuxDoOAuthAccount,
  createPendingOIDCOAuthAccount,
  createPendingWeChatOAuthAccount,
  exchangePendingOAuthCompletion,
  completeLinuxDoOAuthRegistration,
  completeOIDCOAuthRegistration,
  completeWeChatOAuthRegistration,
  createPendingDingTalkOAuthAccount
REDACTED

export default authAPI
