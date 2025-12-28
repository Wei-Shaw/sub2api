/**
 * Authentication Store
 * Manages user authentication state, login/logout, and token persistence
 */

import { defineStore REDACTED from 'pinia'
import { ref, computed, readonly REDACTED from 'vue'
import { authAPI REDACTED from '@/api'
import type { User, LoginRequest, RegisterRequest REDACTED from '@/types'

const AUTH_TOKEN_KEY = 'auth_token'
const AUTH_USER_KEY = 'auth_user'
const AUTO_REFRESH_INTERVAL = 60 * 1000 // 60 seconds

export const useAuthStore = defineStore('auth', () => {
  // ==================== State ====================

  const user = ref<User | null>(null)
  const token = ref<string | null>(null)
  const runMode = ref<'standard' | 'simple'>('standard')
  let refreshIntervalId: ReturnType<typeof setInterval> | null = null

  // ==================== Computed ====================

  const isAuthenticated = computed(() => {
    return !!token.value && !!user.value
  REDACTED)

  const isAdmin = computed(() => {
    return user.value?.role === 'admin'
  REDACTED)

  const isSimpleMode = computed(() => runMode.value === 'simple')

  // ==================== Actions ====================

  /**
   * Initialize auth state from localStorage
   * Call this on app startup to restore session
   * Also starts auto-refresh and immediately fetches latest user data
   */
  function checkAuth(): void {
    const savedToken = localStorage.getItem(AUTH_TOKEN_KEY)
    const savedUser = localStorage.getItem(AUTH_USER_KEY)

    if (savedToken && savedUser) {
      try {
        token.value = savedToken
        user.value = JSON.parse(savedUser)

        // Immediately refresh user data from backend (async, don't block)
        refreshUser().catch((error) => {
          console.error('Failed to refresh user on init:', error)
        REDACTED)

        // Start auto-refresh interval
        startAutoRefresh()
      REDACTED catch (error) {
        console.error('Failed to parse saved user data:', error)
        clearAuth()
      REDACTED
    REDACTED
  REDACTED

  /**
   * Start auto-refresh interval for user data
   * Refreshes user data every 60 seconds
   */
  function startAutoRefresh(): void {
    // Clear existing interval if any
    stopAutoRefresh()

    refreshIntervalId = setInterval(() => {
      if (token.value) {
        refreshUser().catch((error) => {
          console.error('Auto-refresh user failed:', error)
        REDACTED)
      REDACTED
    REDACTED, AUTO_REFRESH_INTERVAL)
  REDACTED

  /**
   * Stop auto-refresh interval
   */
  function stopAutoRefresh(): void {
    if (refreshIntervalId) {
      clearInterval(refreshIntervalId)
      refreshIntervalId = null
    REDACTED
  REDACTED

  /**
   * User login
   * @param credentials - Login credentials (username and password)
   * @returns Promise resolving to the authenticated user
   * @throws Error if login fails
   */
  async function login(credentials: LoginRequest): Promise<User> {
    try {
      const response = await authAPI.login(credentials)

      // Store token and user
      token.value = response.access_token
      user.value = response.user

      // Persist to localStorage
      localStorage.setItem(AUTH_TOKEN_KEY, response.access_token)
      localStorage.setItem(AUTH_USER_KEY, JSON.stringify(response.user))

      // Start auto-refresh interval
      startAutoRefresh()

      return response.user
    REDACTED catch (error) {
      // Clear any partial state on error
      clearAuth()
      throw error
    REDACTED
  REDACTED

  /**
   * User registration
   * @param userData - Registration data (username, email, password)
   * @returns Promise resolving to the newly registered and authenticated user
   * @throws Error if registration fails
   */
  async function register(userData: RegisterRequest): Promise<User> {
    try {
      const response = await authAPI.register(userData)

      // Store token and user
      token.value = response.access_token
      user.value = response.user

      // Persist to localStorage
      localStorage.setItem(AUTH_TOKEN_KEY, response.access_token)
      localStorage.setItem(AUTH_USER_KEY, JSON.stringify(response.user))

      // Start auto-refresh interval
      startAutoRefresh()

      return response.user
    REDACTED catch (error) {
      // Clear any partial state on error
      clearAuth()
      throw error
    REDACTED
  REDACTED

  /**
   * User logout
   * Clears all authentication state and persisted data
   */
  function logout(): void {
    // Call API logout (client-side cleanup)
    authAPI.logout()

    // Clear state
    clearAuth()
  REDACTED

  /**
   * Refresh current user data
   * Fetches latest user info from the server
   * @returns Promise resolving to the updated user
   * @throws Error if not authenticated or request fails
   */
  async function refreshUser(): Promise<User> {
    if (!token.value) {
      throw new Error('Not authenticated')
    REDACTED

    try {
      const response = await authAPI.getCurrentUser()
      if (response.data.run_mode) {
        runMode.value = response.data.run_mode
      REDACTED
      const { run_mode, ...userData REDACTED = response.data
      user.value = userData

      // Update localStorage
      localStorage.setItem(AUTH_USER_KEY, JSON.stringify(userData))

      return userData
    REDACTED catch (error) {
      // If refresh fails with 401, clear auth state
      if ((error as { status?: number REDACTED).status === 401) {
        clearAuth()
      REDACTED
      throw error
    REDACTED
  REDACTED

  /**
   * Clear all authentication state
   * Internal helper function
   */
  function clearAuth(): void {
    // Stop auto-refresh
    stopAutoRefresh()

    token.value = null
    user.value = null
    localStorage.removeItem(AUTH_TOKEN_KEY)
    localStorage.removeItem(AUTH_USER_KEY)
  REDACTED

  // ==================== Return Store API ====================

  return {
    // State
    user,
    token,
    runMode: readonly(runMode),

    // Computed
    isAuthenticated,
    isAdmin,
    isSimpleMode,

    // Actions
    login,
    register,
    logout,
    checkAuth,
    refreshUser
  REDACTED
REDACTED)
