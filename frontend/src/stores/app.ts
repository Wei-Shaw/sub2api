/**
 * Application State Store
 * Manages global UI state including sidebar, loading indicators, and toast notifications
 */

import { defineStore REDACTED from 'pinia';
import { ref, computed REDACTED from 'vue';
import type { Toast, ToastType REDACTED from '@/types';

export const useAppStore = defineStore('app', () => {
  // ==================== State ====================
  
  const sidebarCollapsed = ref<boolean>(false);
  const loading = ref<boolean>(false);
  const toasts = ref<Toast[]>([]);

  // Auto-incrementing ID for toasts
  let toastIdCounter = 0;

  // ==================== Computed ====================
  
  const hasActiveToasts = computed(() => toasts.value.length > 0);
  
  const loadingCount = ref<number>(0);

  // ==================== Actions ====================

  /**
   * Toggle sidebar collapsed state
   */
  function toggleSidebar(): void {
    sidebarCollapsed.value = !sidebarCollapsed.value;
  REDACTED

  /**
   * Set sidebar collapsed state explicitly
   * @param collapsed - Whether sidebar should be collapsed
   */
  function setSidebarCollapsed(collapsed: boolean): void {
    sidebarCollapsed.value = collapsed;
  REDACTED

  /**
   * Set global loading state
   * @param isLoading - Whether app is in loading state
   */
  function setLoading(isLoading: boolean): void {
    if (isLoading) {
      loadingCount.value++;
    REDACTED else {
      loadingCount.value = Math.max(0, loadingCount.value - 1);
    REDACTED
    loading.value = loadingCount.value > 0;
  REDACTED

  /**
   * Show a toast notification
   * @param type - Type of toast (success, error, info, warning)
   * @param message - Toast message content
   * @param duration - Auto-dismiss duration in ms (undefined = no auto-dismiss)
   * @returns Toast ID for manual dismissal
   */
  function showToast(
    type: ToastType,
    message: string,
    duration?: number
  ): string {
    const id = `toast-${++toastIdCounterREDACTED`;
    const toast: Toast = {
      id,
      type,
      message,
      duration,
      startTime: duration !== undefined ? Date.now() : undefined,
    REDACTED;

    toasts.value.push(toast);

    // Auto-dismiss if duration is specified
    if (duration !== undefined) {
      setTimeout(() => {
        hideToast(id);
      REDACTED, duration);
    REDACTED

    return id;
  REDACTED

  /**
   * Show a success toast
   * @param message - Success message
   * @param duration - Auto-dismiss duration in ms (default: 3000)
   */
  function showSuccess(message: string, duration: number = 3000): string {
    return showToast('success', message, duration);
  REDACTED

  /**
   * Show an error toast
   * @param message - Error message
   * @param duration - Auto-dismiss duration in ms (default: 5000)
   */
  function showError(message: string, duration: number = 5000): string {
    return showToast('error', message, duration);
  REDACTED

  /**
   * Show an info toast
   * @param message - Info message
   * @param duration - Auto-dismiss duration in ms (default: 3000)
   */
  function showInfo(message: string, duration: number = 3000): string {
    return showToast('info', message, duration);
  REDACTED

  /**
   * Show a warning toast
   * @param message - Warning message
   * @param duration - Auto-dismiss duration in ms (default: 4000)
   */
  function showWarning(message: string, duration: number = 4000): string {
    return showToast('warning', message, duration);
  REDACTED

  /**
   * Hide a specific toast by ID
   * @param id - Toast ID to hide
   */
  function hideToast(id: string): void {
    const index = toasts.value.findIndex((t) => t.id === id);
    if (index !== -1) {
      toasts.value.splice(index, 1);
    REDACTED
  REDACTED

  /**
   * Clear all toasts
   */
  function clearAllToasts(): void {
    toasts.value = [];
  REDACTED

  /**
   * Execute an async operation with loading state
   * Automatically manages loading indicator
   * @param operation - Async operation to execute
   * @returns Promise resolving to operation result
   */
  async function withLoading<T>(operation: () => Promise<T>): Promise<T> {
    setLoading(true);
    try {
      return await operation();
    REDACTED finally {
      setLoading(false);
    REDACTED
  REDACTED

  /**
   * Execute an async operation with loading and error handling
   * Shows error toast on failure
   * @param operation - Async operation to execute
   * @param errorMessage - Custom error message (optional)
   * @returns Promise resolving to operation result or null on error
   */
  async function withLoadingAndError<T>(
    operation: () => Promise<T>,
    errorMessage?: string
  ): Promise<T | null> {
    setLoading(true);
    try {
      return await operation();
    REDACTED catch (error) {
      const message =
        errorMessage ||
        (error as { message?: string REDACTED).message ||
        'An error occurred';
      showError(message);
      return null;
    REDACTED finally {
      setLoading(false);
    REDACTED
  REDACTED

  /**
   * Reset app state to defaults
   * Useful for cleanup or testing
   */
  function reset(): void {
    sidebarCollapsed.value = false;
    loading.value = false;
    loadingCount.value = 0;
    toasts.value = [];
  REDACTED

  // ==================== Return Store API ====================

  return {
    // State
    sidebarCollapsed,
    loading,
    toasts,
    
    // Computed
    hasActiveToasts,
    
    // Actions
    toggleSidebar,
    setSidebarCollapsed,
    setLoading,
    showToast,
    showSuccess,
    showError,
    showInfo,
    showWarning,
    hideToast,
    clearAllToasts,
    withLoading,
    withLoadingAndError,
    reset,
  REDACTED;
REDACTED);
