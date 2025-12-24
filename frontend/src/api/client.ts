/**
 * Axios HTTP Client Configuration
 * Base client with interceptors for authentication and error handling
 */

import axios, { AxiosInstance, AxiosError } from 'axios';
import type { ApiResponse } from '@/types';

// ==================== Axios Instance Configuration ====================

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api/v1';

export const apiClient: AxiosInstance = axios.create({
  baseURL: API_BASE_URL,
  timeout: 30000,
  // Cookie 鉴权模式下需要携带凭据。
  withCredentials: true,
  headers: {
    'Content-Type': 'application/json',
  },
});

// 鉴权改为 HttpOnly Cookie，因此不再在拦截器里注入 Authorization 头。
// ==================== Response Interceptor ====================

apiClient.interceptors.response.use(
  (response) => {
    // Unwrap standard API response format { code, message, data }
    const apiResponse = response.data as ApiResponse<unknown>;
    if (apiResponse && typeof apiResponse === 'object' && 'code' in apiResponse) {
      if (apiResponse.code === 0) {
        // Success - return the data portion
        response.data = apiResponse.data;
      } else {
        // API error
        return Promise.reject({
          status: response.status,
          code: apiResponse.code,
          message: apiResponse.message || 'Unknown error',
        });
      }
    }
    return response;
  },
  (error: AxiosError<ApiResponse<unknown>>) => {
    // Handle common errors
    if (error.response) {
      const { status, data } = error.response;

      // 401: Unauthorized - clear token and redirect to login
      if (status === 401) {
        // Only redirect if not already on login page
        if (!window.location.pathname.includes('/login')) {
          window.location.href = '/login';
        }
      }

      // Return structured error
      return Promise.reject({
        status,
        code: data?.code,
        message: data?.message || error.message,
      });
    }

    // Network error
    return Promise.reject({
      status: 0,
      message: 'Network error. Please check your connection.',
    });
  }
);

export default apiClient;
