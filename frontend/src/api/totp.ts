/**
 * TOTP (2FA) API endpoints
 * Handles Two-Factor Authentication with Google Authenticator
 */

import { apiClient REDACTED from './client'
import type {
  TotpStatus,
  TotpSetupRequest,
  TotpSetupResponse,
  TotpEnableRequest,
  TotpEnableResponse,
  TotpDisableRequest,
  TotpVerificationMethod
REDACTED from '@/types'

/**
 * Get TOTP status for current user
 * @returns TOTP status including enabled state and feature availability
 */
export async function getStatus(): Promise<TotpStatus> {
  const { data REDACTED = await apiClient.get<TotpStatus>('/user/totp/status')
  return data
REDACTED

/**
 * Get verification method for TOTP operations
 * @returns Method ('email' or 'password') required for setup/disable
 */
export async function getVerificationMethod(): Promise<TotpVerificationMethod> {
  const { data REDACTED = await apiClient.get<TotpVerificationMethod>('/user/totp/verification-method')
  return data
REDACTED

/**
 * Send email verification code for TOTP operations
 * @returns Success response
 */
export async function sendVerifyCode(): Promise<{ success: boolean REDACTED> {
  const { data REDACTED = await apiClient.post<{ success: boolean REDACTED>('/user/totp/send-code')
  return data
REDACTED

/**
 * Initiate TOTP setup - generates secret and QR code
 * @param request - Email code or password depending on verification method
 * @returns Setup response with secret, QR code URL, and setup token
 */
export async function initiateSetup(request?: TotpSetupRequest): Promise<TotpSetupResponse> {
  const { data REDACTED = await apiClient.post<TotpSetupResponse>('/user/totp/setup', request || {REDACTED)
  return data
REDACTED

/**
 * Complete TOTP setup by verifying the code
 * @param request - TOTP code and setup token
 * @returns Enable response with success status and enabled timestamp
 */
export async function enable(request: TotpEnableRequest): Promise<TotpEnableResponse> {
  const { data REDACTED = await apiClient.post<TotpEnableResponse>('/user/totp/enable', request)
  return data
REDACTED

/**
 * Disable TOTP for current user
 * @param request - Email code or password depending on verification method
 * @returns Success response
 */
export async function disable(request: TotpDisableRequest): Promise<{ success: boolean REDACTED> {
  const { data REDACTED = await apiClient.post<{ success: boolean REDACTED>('/user/totp/disable', request)
  return data
REDACTED

/**
 * Step-up verification response
 */
export interface TotpStepUpResponse {
  verified: boolean
  expires_in: number
REDACTED

/**
 * Verify a TOTP code to grant the current session a short-lived step-up
 * (sudo) window for sensitive operations (account export, DB backup download...).
 * @param code - 6-digit TOTP code
 */
export async function stepUp(code: string): Promise<TotpStepUpResponse> {
  const { data REDACTED = await apiClient.post<TotpStepUpResponse>('/user/totp/step-up', { code REDACTED)
  return data
REDACTED

export const totpAPI = {
  getStatus,
  getVerificationMethod,
  sendVerifyCode,
  initiateSetup,
  enable,
  disable,
  stepUp
REDACTED

export default totpAPI
