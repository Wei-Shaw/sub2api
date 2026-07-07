/**
 * 导航加载状态组合式函数
 * 管理路由切换时的加载状态，支持防闪烁逻辑
 */
import { ref, readonly, computed } from 'vue'

/**
 * 导航加载状态管理
 *
 * 功能：
 * 1. 在路由切换时显示加载状态
 * 2. 快速导航（< 100ms）不显示加载指示器（防闪烁）
 * 3. 导航取消时正确重置状态
 */
export function useNavigationLoading() {
  // 内部加载状态
  const _isLoading = ref(false)

  // 导航开始时间（用于防闪烁计算）
  let navigationStartTime: number | null = null

  // 防闪烁延迟计时器
  let showLoadingTimer: ReturnType<typeof setTimeout> | null = null

  // 是否应该显示加载指示器（考虑防闪烁逻辑）
  const shouldShowLoading = ref(false)

  // 防闪烁延迟时间（毫秒）
  const ANTI_FLICKER_DELAY = 100

  /**
   * 清理计时器
   */
  const clearTimer = (): void => {
    if (showLoadingTimer !== null) {
      clearTimeout(showLoadingTimer)
      showLoadingTimer = null
    }
  }

  /**
   * 导航开始时调用
   */
  const startNavigation = (): void => {
    navigationStartTime = Date.now()
    _isLoading.value = true

    // 延迟显示加载指示器，实现防闪烁
    clearTimer()
    showLoadingTimer = setTimeout(() => {
      if (_isLoading.value) {
        shouldShowLoading.value = true
      }
    }, ANTI_FLICKER_DELAY)
  }

  /**
   * 导航结束时调用
   */
  const endNavigation = (): void => {
    clearTimer()
    _isLoading.value = false
    shouldShowLoading.value = false
    navigationStartTime = null
  }

  /**
   * 导航取消时调用（比如快速连续点击不同链接）
   */
  const cancelNavigation = (): void => {
    clearTimer()
    // 保持加载状态，因为新的导航会立即开始
    // 但重置导航开始时间
    navigationStartTime = null
  }

  /**
   * 重置所有状态（用于测试）
   */
  const resetState = (): void => {
    clearTimer()
    _isLoading.value = false
    shouldShowLoading.value = false
    navigationStartTime = null
  }

  /**
   * 获取导航持续时间（毫秒）
   */
  const getNavigationDuration = (): number | null => {
    if (navigationStartTime === null) {
      return null
    }
    return Date.now() - navigationStartTime
  }

  // 公开的加载状态（只读）
  const isLoading = computed(() => shouldShowLoading.value)

  // 内部加载状态（用于测试，不考虑防闪烁）
  const isNavigating = readonly(_isLoading)

  return {
    isLoading,
    isNavigating,
    startNavigation,
    endNavigation,
    cancelNavigation,
    resetState,
    getNavigationDuration,
    // 导出常量用于测试
    ANTI_FLICKER_DELAY
  }
}

// 创建单例实例，供全局使用
let navigationLoadingInstance: ReturnType<typeof useNavigationLoading> | null = null

export function useNavigationLoadingState() {
  if (!navigationLoadingInstance) {
    navigationLoadingInstance = useNavigationLoading()
  }
  return navigationLoadingInstance
}

// 导出重置函数（用于测试）
export function _resetNavigationLoadingInstance(): void {
  if (navigationLoadingInstance) {
    navigationLoadingInstance.resetState()
  }
  navigationLoadingInstance = null
}
