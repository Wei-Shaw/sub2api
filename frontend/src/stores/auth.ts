/**
 * 认证状态 Store
 * 负责会话恢复、登录/登出与自动刷新
 */

import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import { authAPI } from '@/api';
import type { User, LoginRequest, RegisterRequest } from '@/types';
const AUTO_REFRESH_INTERVAL = 60 * 1000; // 60 seconds

export const useAuthStore = defineStore('auth', () => {
  // ==================== State ====================

  const user = ref<User | null>(null);
  let refreshIntervalId: ReturnType<typeof setInterval> | null = null;
  // 避免重复发起会话恢复请求。
  const isChecking = ref(false);

  // ==================== Computed ====================

  const isAuthenticated = computed(() => {
    return !!user.value;
  });

  const isAdmin = computed(() => {
    return user.value?.role === 'admin';
  });

  // ==================== Actions ====================

  /**
   * 启动时从服务端会话恢复登录态
   * 通过 /auth/me 拉取最新用户信息并开启自动刷新
   */
  async function checkAuth(): Promise<void> {
    if (isChecking.value) return;
    isChecking.value = true;
    try {
      // 通过 /auth/me 验证 Cookie 会话，并拉取最新用户信息。
      const currentUser = await authAPI.getCurrentUser();
      user.value = currentUser;
      startAutoRefresh();
    } catch (error) {
      clearAuth();
    } finally {
      isChecking.value = false;
    }
  }

  /**
   * 启动用户信息自动刷新
   * 每 60 秒同步一次用户状态
   */
  function startAutoRefresh(): void {
    // Clear existing interval if any
    stopAutoRefresh();

    refreshIntervalId = setInterval(() => {
      if (user.value) {
        refreshUser().catch((error) => {
          console.error('Auto-refresh user failed:', error);
        });
      }
    }, AUTO_REFRESH_INTERVAL);
  }

  /**
   * 停止用户信息自动刷新
   */
  function stopAutoRefresh(): void {
    if (refreshIntervalId) {
      clearInterval(refreshIntervalId);
      refreshIntervalId = null;
    }
  }

  /**
   * User login
   * @param credentials - Login credentials (username and password)
   * @returns Promise resolving to the authenticated user
   * @throws Error if login fails
   */
  async function login(credentials: LoginRequest): Promise<User> {
    try {
      const response = await authAPI.login(credentials);

      // 登录成功后仅保留用户信息，令牌由 Cookie 管理。
      user.value = response.user;

      // Start auto-refresh interval
      startAutoRefresh();

      return response.user;
    } catch (error) {
      // Clear any partial state on error
      clearAuth();
      throw error;
    }
  }

  /**
   * User registration
   * @param userData - Registration data (username, email, password)
   * @returns Promise resolving to the newly registered and authenticated user
   * @throws Error if registration fails
   */
  async function register(userData: RegisterRequest): Promise<User> {
    try {
      const response = await authAPI.register(userData);

      // 注册成功后仅保留用户信息，令牌由 Cookie 管理。
      user.value = response.user;

      // Start auto-refresh interval
      startAutoRefresh();

      return response.user;
    } catch (error) {
      // Clear any partial state on error
      clearAuth();
      throw error;
    }
  }

  /**
   * User logout
   * Clears all authentication state and persisted data
   */
  async function logout(): Promise<void> {
    // Call API logout (client-side cleanup)
    await authAPI.logout();

    // Clear state
    clearAuth();
  }

  /**
   * Refresh current user data
   * Fetches latest user info from the server
   * @returns Promise resolving to the updated user
   * @throws Error if not authenticated or request fails
   */
  async function refreshUser(): Promise<User> {
    try {
      const updatedUser = await authAPI.getCurrentUser();
      user.value = updatedUser;

      return updatedUser;
    } catch (error) {
      // If refresh fails with 401, clear auth state
      if ((error as { status?: number }).status === 401) {
        clearAuth();
      }
      throw error;
    }
  }

  /**
   * Clear all authentication state
   * Internal helper function
   */
  function clearAuth(): void {
    // Stop auto-refresh
    stopAutoRefresh();

    user.value = null;
  }

  // ==================== Return Store API ====================

  return {
    // State
    user,

    // Computed
    isAuthenticated,
    isAdmin,

    // Actions
    login,
    register,
    logout,
    checkAuth,
    refreshUser,
  };
});
