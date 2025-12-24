/**
 * Authentication API endpoints
 * Handles user login, registration, and logout operations
 */

import { apiClient } from './client';
import type { LoginRequest, RegisterRequest, AuthResponse, User, SendVerifyCodeRequest, SendVerifyCodeResponse, PublicSettings } from '@/types';

// 登录态由 HttpOnly Cookie 维护，前端不再持久化 token。

/**
 * User login
 * @param credentials - Username and password
 * @returns Authentication response with token and user data
 */
export async function login(credentials: LoginRequest): Promise<AuthResponse> {
  const { data } = await apiClient.post<AuthResponse>('/auth/login', credentials);
  return data;
}

/**
 * User registration
 * @param userData - Registration data (username, email, password)
 * @returns Authentication response with token and user data
 */
export async function register(userData: RegisterRequest): Promise<AuthResponse> {
  const { data } = await apiClient.post<AuthResponse>('/auth/register', userData);
  return data;
}

/**
 * Get current authenticated user
 * @returns User profile data
 */
export async function getCurrentUser(): Promise<User> {
  const { data } = await apiClient.get<User>('/auth/me');
  return data;
}

/**
 * User logout
 * 通知后端清理 Cookie 会话
 */
export async function logout(): Promise<void> {
  // 通过后端清理 HttpOnly Cookie。
  await apiClient.post('/auth/logout');
}

/**
 * Get public settings (no auth required)
 * @returns Public settings including registration and Turnstile config
 */
export async function getPublicSettings(): Promise<PublicSettings> {
  const { data } = await apiClient.get<PublicSettings>('/settings/public');
  return data;
}

/**
 * Send verification code to email
 * @param request - Email and optional Turnstile token
 * @returns Response with countdown seconds
 */
export async function sendVerifyCode(request: SendVerifyCodeRequest): Promise<SendVerifyCodeResponse> {
  const { data } = await apiClient.post<SendVerifyCodeResponse>('/auth/send-verify-code', request);
  return data;
}

export const authAPI = {
  login,
  register,
  getCurrentUser,
  logout,
  getPublicSettings,
  sendVerifyCode,
};

export default authAPI;
