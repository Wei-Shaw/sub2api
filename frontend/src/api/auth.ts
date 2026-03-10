/**
 * Authentication API endpoints
 * Handles user login, registration, and logout operations
 */

import { apiClient REDACTED from './client'
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
export interface RefreshTokenResponse {
  access_token: string
  refresh_token: string
  expires_in: number
  token_type: string
REDACTED

/**
 * Refresh the access token using the refresh token
 * @returns New token pair
 */
export async function refreshToken(): Promise<RefreshTokenResponse> {
  const currentRefreshToken = getRefreshToken()
  if (!currentRefreshToken) {
    throw new Error('No refresh token available')
  REDACTED

  const { data REDACTED = await apiClient.post<RefreshTokenResponse>('/auth/refresh', {
    refresh_token: currentRefreshToken
  REDACTED)

  // Update tokens in localStorage
  setAuthToken(data.access_token)
  setRefreshToken(data.refresh_token)
  setTokenExpiresAt(data.expires_in)

  return data
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
 * @param pendingOAuthToken - Short-lived JWT from the OAuth callback
 * @param invitationCode - Invitation code entered by the user
 * @returns Token pair on success
 */
export async function completeLinuxDoOAuthRegistration(
  pendingOAuthToken: string,
  invitationCode: string
): Promise<{ access_token: string; refresh_token: string; expires_in: number; token_type: string REDACTED> {
  const { data REDACTED = await apiClient.post<{
    access_token: string
    refresh_token: string
    expires_in: number
    token_type: string
  REDACTED>('/auth/oauth/linuxdo/complete-registration', {
    pending_oauth_token: pendingOAuthToken,
    invitation_code: invitationCode
  REDACTED)
  return data
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
  validatePromoCode,
  validateInvitationCode,
  forgotPassword,
  resetPassword,
  refreshToken,
  revokeAllSessions,
  completeLinuxDoOAuthRegistration
REDACTED

export default authAPI
