/**
 * Axios HTTP Client Configuration
 * Base client with interceptors for authentication and error handling
 */

import axios, { AxiosInstance, AxiosError, InternalAxiosRequestConfig REDACTED from 'axios'
import type { ApiResponse REDACTED from '@/types'
import { getLocale REDACTED from '@/i18n'

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

// Get user's timezone
const getUserTimezone = (): string => {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone
  REDACTED catch {
    return 'UTC'
  REDACTED
REDACTED

apiClient.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    // Attach token from localStorage
    const token = localStorage.getItem('auth_token')
    if (token && config.headers) {
      config.headers.Authorization = `Bearer ${tokenREDACTED`
    REDACTED

    // Attach locale for backend translations
    if (config.headers) {
      config.headers['Accept-Language'] = getLocale()
    REDACTED

    // Attach timezone for all GET requests (backend may use it for default date ranges)
    if (config.method === 'get') {
      if (!config.params) {
        config.params = {REDACTED
      REDACTED
      config.params.timezone = getUserTimezone()
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
    // Request cancellation: keep the original axios cancellation error so callers can ignore it.
    // Otherwise we'd misclassify it as a generic "network error".
    if (error.code === 'ERR_CANCELED' || axios.isCancel(error)) {
      return Promise.reject(error)
    REDACTED

    // Handle common errors
    if (error.response) {
      const { status, data REDACTED = error.response
      const url = String(error.config?.url || '')

      // Validate `data` shape to avoid HTML error pages breaking our error handling.
      const apiData = (typeof data === 'object' && data !== null ? data : {REDACTED) as Record<string, any>

      // Ops monitoring disabled: treat as feature-flagged 404, and proactively redirect away
      // from ops pages to avoid broken UI states.
      if (status === 404 && apiData.message === 'Ops monitoring is disabled') {
        try {
          localStorage.setItem('ops_monitoring_enabled_cached', 'false')
        REDACTED catch {
          // ignore localStorage failures
        REDACTED
        try {
          window.dispatchEvent(new CustomEvent('ops-monitoring-disabled'))
        REDACTED catch {
          // ignore event failures
        REDACTED

        if (window.location.pathname.startsWith('/admin/ops')) {
          window.location.href = '/admin/settings'
        REDACTED

        return Promise.reject({
          status,
          code: 'OPS_DISABLED',
          message: apiData.message || error.message,
          url
        REDACTED)
      REDACTED

      // 401: Unauthorized - clear token and redirect to login
      if (status === 401) {
        const hasToken = !!localStorage.getItem('auth_token')
        const url = error.config?.url || ''
        const isAuthEndpoint =
          url.includes('/auth/login') || url.includes('/auth/register') || url.includes('/auth/refresh')
        const headers = error.config?.headers as Record<string, unknown> | undefined
        const authHeader = headers?.Authorization ?? headers?.authorization
        const sentAuth =
          typeof authHeader === 'string'
            ? authHeader.trim() !== ''
            : Array.isArray(authHeader)
            ? authHeader.length > 0
            : !!authHeader

        localStorage.removeItem('auth_token')
        localStorage.removeItem('auth_user')
        if ((hasToken || sentAuth) && !isAuthEndpoint) {
          sessionStorage.setItem('auth_expired', '1')
        REDACTED
        // Only redirect if not already on login page
        if (!window.location.pathname.includes('/login')) {
          window.location.href = '/login'
        REDACTED
      REDACTED

      // Return structured error
      return Promise.reject({
        status,
        code: apiData.code,
        message: apiData.message || apiData.detail || error.message
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
