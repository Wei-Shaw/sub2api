/**
 * Axios HTTP Client Configuration
 * Base client with interceptors for authentication, token refresh, and error handling
 */

import axios, { AxiosInstance, AxiosError, InternalAxiosRequestConfig, AxiosResponse } from 'axios'
import type { ApiResponse } from '@/types'
import { getLocale } from '@/core/i18n'
import {
  ADMIN_UI_REQUEST_HEADER,
  USER_UI_REQUEST_HEADER,
  shouldMarkAdminUIRequest,
  shouldMarkUserUIRequest,
} from './adminUIRequest'
import { getAPIBaseURL } from './url'
import {
  getAccessToken,
  clearTokenMemory
} from './tokenStore'
import { refreshBrowserSession } from './sessionRefresh'
import { safeLocalStorage, safeSessionStorage } from '@/core/utils/safeStorage'
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

// ==================== Token Refresh State ====================

// Track if a token refresh is in progress to prevent multiple simultaneous refresh requests
let isRefreshing = false
// Queue of requests waiting for token refresh
let refreshSubscribers: Array<(token: string) => void> = []

/**
 * Subscribe to token refresh completion
 */
function subscribeTokenRefresh(callback: (token: string) => void): void {
  refreshSubscribers.push(callback)
}

/**
 * Notify all subscribers that token has been refreshed
 */
function onTokenRefreshed(token: string): void {
  refreshSubscribers.forEach((callback) => callback(token))
  refreshSubscribers = []
}

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
	// Attach the short-lived access token from memory. Refresh credentials stay
	// in the backend's HttpOnly cookie and are never read here.
	const token = getAccessToken()
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

const parseRetryAfterSeconds = (headers: unknown): number | undefined => {
  const raw = (headers as Record<string, unknown> | undefined)?.['retry-after']
  if (typeof raw !== 'string' || !/^\d+$/.test(raw.trim())) return undefined
  const seconds = Number(raw.trim())
  return Number.isSafeInteger(seconds) ? seconds : undefined
}


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
          safeLocalStorage.setItem('ops_monitoring_enabled_cached', 'false')
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

      if (status === 423 && apiData.code === 'ADMIN_COMPLIANCE_ACK_REQUIRED') {
        try {
          window.dispatchEvent(new CustomEvent('admin-compliance-required', {
            detail: apiData.metadata || {}
          }))
        } catch {
          // ignore event failures
        }

        return Promise.reject({
          status,
          code: apiData.code,
          message: apiData.message || error.message,
          metadata: apiData.metadata,
        })
      }

      // 401: Try to refresh via the HttpOnly cookie
      // This handles TOKEN_EXPIRED, INVALID_TOKEN, TOKEN_REVOKED, etc.
      if (status === 401 && !originalRequest._retry) {
        const isAuthEndpoint =
          url.includes('/auth/login') || url.includes('/auth/register') || url.includes('/auth/refresh')

        // If this is not an auth endpoint, try to refresh via the cookie.
        if (!isAuthEndpoint) {
          if (isRefreshing) {
            // Wait for the ongoing refresh to complete
            return new Promise((resolve, reject) => {
              subscribeTokenRefresh((newToken: string) => {
                if (newToken) {
                  // Mark as retried to prevent infinite loop if retry also returns 401
                  originalRequest._retry = true
                  if (originalRequest.headers) {
                    originalRequest.headers.Authorization = `Bearer ${newToken}`
                  }
                  resolve(apiClient(originalRequest))
                } else {
                  // Refresh failed, reject with original error
                  reject({
                    status,
                    code: apiData.code,
                    message: apiData.message || apiData.detail || error.message
                  })
                }
              })
            })
          }

          originalRequest._retry = true
          isRefreshing = true

          try {
            const refreshData = await refreshBrowserSession()

            if (refreshData.access_token) {
              const { access_token } = refreshData

              // Notify subscribers with new token
              onTokenRefreshed(access_token)

              // Retry the original request with new token
              if (originalRequest.headers) {
                originalRequest.headers.Authorization = `Bearer ${access_token}`
              }

              isRefreshing = false
              return apiClient(originalRequest)
            }

            // Refresh response was not successful, fall through to clear auth
            throw new Error('Token refresh failed')
          } catch (refreshError) {
            // Refresh failed - notify subscribers with empty token
            onTokenRefreshed('')
            isRefreshing = false

            // A temporary refresh transport failure must not destroy a valid
            // browser session. Keep the in-memory token and let the caller
            // retry; only an explicit 401/invalid-refresh response below
            // should transition the UI to the login state.
            if (axios.isAxiosError(refreshError)) {
              const refreshStatus = refreshError.response?.status ?? 0
              if (refreshStatus === 0 || refreshStatus === 429 || refreshStatus >= 500) {
                return Promise.reject({
                  status: refreshStatus,
                  code: 'TOKEN_REFRESH_UNAVAILABLE',
                  message: refreshError.response?.data?.message || refreshError.message,
                })
              }
            }
            if (refreshError instanceof Error && /network|timeout|timed out|temporar/i.test(refreshError.message)) {
              return Promise.reject({
                status: 0,
                code: 'TOKEN_REFRESH_UNAVAILABLE',
                message: refreshError.message,
              })
            }

            // Clear tokens and redirect to login
            clearTokenMemory()
            safeLocalStorage.removeItem('auth_user')
            safeSessionStorage.setItem('auth_expired', '1')

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
        const hasToken = !!getAccessToken()
        const headers = error.config?.headers as Record<string, unknown> | undefined
        const authHeader = headers?.Authorization ?? headers?.authorization
        const sentAuth =
          typeof authHeader === 'string'
            ? authHeader.trim() !== ''
            : Array.isArray(authHeader)
              ? authHeader.length > 0
              : !!authHeader

        clearTokenMemory()
        safeLocalStorage.removeItem('auth_user')
        if ((hasToken || sentAuth) && !isAuthEndpoint) {
          safeSessionStorage.setItem('auth_expired', '1')
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
        retry_after_seconds: parseRetryAfterSeconds(error.response.headers),
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
