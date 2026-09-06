/**
 * Axios HTTP Client Configuration
 * Base client with interceptors for authentication, token refresh, and error handling
 */

import axios, { AxiosInstance, AxiosError, InternalAxiosRequestConfig, AxiosResponse } from 'axios'
import type { ApiResponse } from '@/types'
import { getLocale } from '@/i18n'
import {
  ADMIN_UI_REQUEST_HEADER,
  USER_UI_REQUEST_HEADER,
  shouldMarkAdminUIRequest,
  shouldMarkUserUIRequest,
} from './adminUIRequest'
import { refreshAuthTokens } from './tokenRefresh'
import { getAPIBaseURL } from './url'
export { buildApiUrl, buildGatewayUrl } from './url'

// ==================== Axios Instance Configuration ====================

export const apiClient: AxiosInstance = axios.create({
  baseURL: getAPIBaseURL(),
  withCredentials: true,
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json'
  }
})

// ==================== Request Interceptor ====================

// Get user's timezone
const getUserTimezone = (): string => {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone
  } catch {
    return 'UTC'
  }
}

apiClient.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    // Attach token from localStorage
    const token = localStorage.getItem('auth_token')
    if (token && config.headers) {
      config.headers.Authorization = `Bearer ${token}`
    }

    // Attach locale for backend translations
    if (config.headers) {
      config.headers['Accept-Language'] = getLocale()
    }

    // Attach timezone for all GET requests (backend may use it for default date ranges)
    if (config.method === 'get') {
      if (!config.params) {
        config.params = {}
      }
      config.params.timezone = getUserTimezone()
    }

    if (config.headers) {
      const requestURL = String(config.url || '')
      if (shouldMarkAdminUIRequest(requestURL)) {
        config.headers[ADMIN_UI_REQUEST_HEADER] = '1'
      }
      if (shouldMarkUserUIRequest(requestURL)) {
        config.headers[USER_UI_REQUEST_HEADER] = '1'
      }
    }

    return config
  },
  (error) => {
    return Promise.reject(error)
  }
)

// ==================== Response Interceptor ====================

apiClient.interceptors.response.use(
  (response: AxiosResponse) => {
    // Unwrap standard API response format { code, message, data }
    const apiResponse = response.data as ApiResponse<unknown>
    if (apiResponse && typeof apiResponse === 'object' && 'code' in apiResponse) {
      if (apiResponse.code === 0) {
        // Success - return the data portion
        response.data = apiResponse.data
      } else {
        // API error
        const resp = apiResponse as unknown as Record<string, unknown>
        return Promise.reject({
          status: response.status,
          code: apiResponse.code,
          message: apiResponse.message || 'Unknown error',
          reason: resp.reason,
          metadata: resp.metadata,
        })
      }
    }
    return response
  },
  async (error: AxiosError<ApiResponse<unknown>>) => {
    // Request cancellation: keep the original axios cancellation error so callers can ignore it.
    // Otherwise we'd misclassify it as a generic "network error".
    if (error.code === 'ERR_CANCELED' || axios.isCancel(error)) {
      return Promise.reject(error)
    }

    const originalRequest = error.config as InternalAxiosRequestConfig & { _retry?: boolean }

    // Handle common errors
    if (error.response) {
      const { status, data } = error.response
      const url = String(error.config?.url || '')

      // Validate `data` shape to avoid HTML error pages breaking our error handling.
      const apiData = (typeof data === 'object' && data !== null ? data : {}) as Record<string, any>

      // Ops monitoring disabled: treat as feature-flagged 404, and proactively redirect away
      // from ops pages to avoid broken UI states.
      if (status === 404 && apiData.message === 'Ops monitoring is disabled') {
        try {
          localStorage.setItem('ops_monitoring_enabled_cached', 'false')
        } catch {
          // ignore localStorage failures
        }
        try {
          window.dispatchEvent(new CustomEvent('ops-monitoring-disabled'))
        } catch {
          // ignore event failures
        }

        if (window.location.pathname.startsWith('/admin/ops')) {
          window.location.href = '/admin/settings'
        }

        return Promise.reject({
          status,
          code: 'OPS_DISABLED',
          message: apiData.message || error.message,
          url
        })
      }

      // 401: Try to refresh the token if we have a refresh token
      // This handles TOKEN_EXPIRED, INVALID_TOKEN, TOKEN_REVOKED, etc.
      if (status === 401 && !originalRequest._retry) {
        const refreshToken = localStorage.getItem('refresh_token')
        const isAuthEndpoint =
          url.includes('/auth/login') || url.includes('/auth/register') || url.includes('/auth/refresh')

        // If we have a refresh token and this is not an auth endpoint, try to refresh
        if (refreshToken && !isAuthEndpoint) {
          const refreshSessionUser = localStorage.getItem('auth_user')
          originalRequest._retry = true

          try {
            const headers = originalRequest.headers as Record<string, unknown> | undefined
            const authHeader = headers?.Authorization ?? headers?.authorization
            const failedAccessToken =
              typeof authHeader === 'string' && authHeader.startsWith('Bearer ')
                ? authHeader.slice('Bearer '.length)
                : null
            const tokens = await refreshAuthTokens({ failedAccessToken })

            // Retry the original request with the refreshed token
            if (originalRequest.headers) {
              originalRequest.headers.Authorization = `Bearer ${tokens.access_token}`
            }
            return apiClient(originalRequest)
          } catch {
            // A stale request must never destroy a session that was logged out or replaced while
            // its refresh was in flight (for example, when another tab signs in as another user).
            const sessionChanged =
              localStorage.getItem('refresh_token') !== refreshToken ||
              localStorage.getItem('auth_user') !== refreshSessionUser
            if (sessionChanged) {
              return Promise.reject({
                status: 401,
                code: 'AUTH_SESSION_CHANGED',
                message: 'Authentication session changed while refreshing.'
              })
            }

            // Clear tokens and redirect to login
            localStorage.removeItem('auth_token')
            localStorage.removeItem('refresh_token')
            localStorage.removeItem('auth_user')
            localStorage.removeItem('token_expires_at')
            sessionStorage.setItem('auth_expired', '1')

            if (!window.location.pathname.includes('/login')) {
              window.location.href = '/login'
            }

            return Promise.reject({
              status: 401,
              code: 'TOKEN_REFRESH_FAILED',
              message: 'Session expired. Please log in again.'
            })
          }
        }

        // No refresh token or is auth endpoint - clear auth and redirect
        const hasToken = !!localStorage.getItem('auth_token')
        const headers = error.config?.headers as Record<string, unknown> | undefined
        const authHeader = headers?.Authorization ?? headers?.authorization
        const sentAuth =
          typeof authHeader === 'string'
            ? authHeader.trim() !== ''
            : Array.isArray(authHeader)
              ? authHeader.length > 0
              : !!authHeader

        localStorage.removeItem('auth_token')
        localStorage.removeItem('refresh_token')
        localStorage.removeItem('auth_user')
        localStorage.removeItem('token_expires_at')
        if ((hasToken || sentAuth) && !isAuthEndpoint) {
          sessionStorage.setItem('auth_expired', '1')
        }
        // Only redirect if not already on login page
        if (!window.location.pathname.includes('/login')) {
          window.location.href = '/login'
        }
      }

      // Return structured error
      return Promise.reject({
        status,
        code: apiData.code,
        reason: apiData.reason,
        error: apiData.error,
        message: apiData.message || apiData.detail || error.message,
        metadata: apiData.metadata,
      })
    }

    // Network error
    return Promise.reject({
      status: 0,
      message: 'Network error. Please check your connection.'
    })
  }
)

export default apiClient
