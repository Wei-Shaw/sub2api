/**
 * Axios HTTP Client Configuration
 * Base client with interceptors for authentication and error handling
 */

import axios, { AxiosInstance, AxiosError, InternalAxiosRequestConfig REDACTED from 'axios'
import type { ApiResponse REDACTED from '@/types'

// ==================== Axios Instance Configuration ====================

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api/v1'

export const apiClient: AxiosInstance = axios.create({
  baseURL: API_BASE_URL,
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json'
  REDACTED
REDACTED)

// ==================== Request Interceptor ====================

apiClient.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    // Attach token from localStorage
    const token = localStorage.getItem('auth_token')
    if (token && config.headers) {
      config.headers.Authorization = `Bearer ${tokenREDACTED`
    REDACTED
    return config
  REDACTED,
  (error) => {
    return Promise.reject(error)
  REDACTED
)

// ==================== Response Interceptor ====================

apiClient.interceptors.response.use(
  (response) => {
    // Unwrap standard API response format { code, message, data REDACTED
    const apiResponse = response.data as ApiResponse<unknown>
    if (apiResponse && typeof apiResponse === 'object' && 'code' in apiResponse) {
      if (apiResponse.code === 0) {
        // Success - return the data portion
        response.data = apiResponse.data
      REDACTED else {
        // API error
        return Promise.reject({
          status: response.status,
          code: apiResponse.code,
          message: apiResponse.message || 'Unknown error'
        REDACTED)
      REDACTED
    REDACTED
    return response
  REDACTED,
  (error: AxiosError<ApiResponse<unknown>>) => {
    // Handle common errors
    if (error.response) {
      const { status, data REDACTED = error.response

      // 401: Unauthorized - clear token and redirect to login
      if (status === 401) {
        localStorage.removeItem('auth_token')
        localStorage.removeItem('auth_user')
        // Only redirect if not already on login page
        if (!window.location.pathname.includes('/login')) {
          window.location.href = '/login'
        REDACTED
      REDACTED

      // Return structured error
      return Promise.reject({
        status,
        code: data?.code,
        message: data?.message || error.message
      REDACTED)
    REDACTED

    // Network error
    return Promise.reject({
      status: 0,
      message: 'Network error. Please check your connection.'
    REDACTED)
  REDACTED
)

export default apiClient
