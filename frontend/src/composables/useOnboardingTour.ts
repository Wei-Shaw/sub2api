import { onMounted, onUnmounted, nextTick } from 'vue'
import { driver, type Driver, type DriveStep } from 'driver.js'
import 'driver.js/dist/driver.css'
import { useAuthStore as useUserStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { useOnboardingStore } from '@/stores/onboarding'
import { useI18n } from 'vue-i18n'
import { getAdminSteps, getUserSteps } from '@/components/Guide/steps'

export interface OnboardingOptions {
  storageKey?: string
  autoStart?: boolean
}

/** 侧边栏在该宽度以下收进抽屉，引导改为居中弹出 */
const DESKTOP_MIN_WIDTH = 1024
const AUTO_START_DELAY_MS = 1000

/**
 * 概览式新手引导
 *
 * - 只高亮入口并说明用途，不替用户点击、不打开弹窗、不提交表单
 * - 无遮罩：页面在引导期间始终可操作
 * - 任意方式退出（完成、关闭、Esc）都记为已看过
 */
export function useOnboardingTour(options: OnboardingOptions) {
  const { t } = useI18n()
  const userStore = useUserStore()
  const appStore = useAppStore()
  const onboardingStore = useOnboardingStore()
  const storageVersion = 'v5_overview'

  let driverInstance: Driver | null = onboardingStore.getDriverInstance()
  let autoStartTimer: ReturnType<typeof setTimeout> | null = null

  const getStorageKey = () => {
    const baseKey = options.storageKey ?? 'onboarding_tour'
    const userId = userStore.user?.id ?? 'guest'
    const role = userStore.user?.role ?? 'user'
    return `${baseKey}_${userId}_${role}_${storageVersion}`
  }

  const hasSeen = () => {
    try {
      return localStorage.getItem(getStorageKey()) === 'true'
    } catch {
      return false
    }
  }

  const markAsSeen = () => {
    try {
      localStorage.setItem(getStorageKey(), 'true')
    } catch {
      // 存储不可用时仅本次会话生效
    }
  }

  const clearSeen = () => {
    try {
      localStorage.removeItem(getStorageKey())
    } catch {
      // ignore
    }
  }

  /** 普通用户在 Backend 模式下没有自助功能，不展示引导 */
  const isTourAvailable = () => {
    if (!userStore.user || userStore.isSimpleMode) return false
    if (userStore.user.role !== 'admin' && appStore.backendModeEnabled) return false
    return true
  }

  /**
   * 生成当前可用的步骤：
   * - 目标元素不存在（被功能开关隐藏）的步骤直接跳过
   * - 小屏下侧边栏不可见，侧边栏步骤改为居中弹出
   */
  const buildSteps = (): DriveStep[] => {
    const isAdmin = userStore.user?.role === 'admin'
    const isDesktop = window.innerWidth >= DESKTOP_MIN_WIDTH
    const steps = isAdmin ? getAdminSteps(t) : getUserSteps(t)

    return steps
      .filter((step) => typeof step.element !== 'string' || document.querySelector(step.element))
      .map((step) => {
        if (!isDesktop && typeof step.element === 'string' && step.element.startsWith('aside.sidebar')) {
          const { element: _element, ...rest } = step
          return { ...rest, popover: { ...rest.popover, side: undefined, align: undefined } }
        }
        return step
      })
  }

  const destroyTour = () => {
    driverInstance?.destroy()
    driverInstance = null
    onboardingStore.setDriverInstance(null)
  }

  const startTour = async (startIndex = 0) => {
    if (!isTourAvailable()) return

    await nextTick()
    const steps = buildSteps()
    if (steps.length === 0) return

    if (driverInstance?.isActive()) {
      driverInstance.destroy()
    }

    driverInstance = driver({
      steps,
      animate: true,
      smoothScroll: true,
      allowClose: true,
      allowKeyboardControl: true,
      overlayOpacity: 0,
      stagePadding: 4,
      stageRadius: 8,
      popoverClass: 'theme-tour-popover',
      showProgress: true,
      progressText: '{{current}} / {{total}}',
      showButtons: ['next', 'previous', 'close'],
      nextBtnText: t('common.next'),
      prevBtnText: t('common.back'),
      doneBtnText: t('onboarding.finish'),
      onDestroyed: () => {
        markAsSeen()
        driverInstance = null
        onboardingStore.setDriverInstance(null)
      }
    })

    onboardingStore.setDriverInstance(driverInstance)
    driverInstance.drive(Math.min(startIndex, steps.length - 1))
    // driver.css 用 `.driver-active *` 禁用了整页的点击；该类只在启动时添加一次，
    // 移除后引导期间页面保持可操作（遮罩已在 onboarding.css 中隐藏）
    document.body.classList.remove('driver-active')
  }

  /** 供业务代码在需要时推进一步 */
  const nextStep = async (delay = 300) => {
    if (!driverInstance?.isActive()) return
    if (delay > 0) {
      await new Promise((resolve) => setTimeout(resolve, delay))
    }
    driverInstance?.moveNext()
  }

  const isCurrentStep = (elementSelector: string): boolean => {
    if (!driverInstance?.isActive()) return false
    const activeElement = driverInstance.getActiveElement()
    return activeElement?.matches?.(elementSelector) ?? false
  }

  const replayTour = () => {
    clearSeen()
    void startTour()
  }

  onMounted(() => {
    onboardingStore.setControlMethods({
      nextStep,
      isCurrentStep
    })

    // 路由切换会重新挂载布局，沿用仍在进行中的引导
    if (onboardingStore.isDriverActive()) {
      driverInstance = onboardingStore.getDriverInstance()
      return
    }

    if (!options.autoStart || !isTourAvailable() || hasSeen()) return
    autoStartTimer = setTimeout(() => {
      autoStartTimer = null
      if (!onboardingStore.isDriverActive()) void startTour()
    }, AUTO_START_DELAY_MS)
  })

  onUnmounted(() => {
    if (autoStartTimer) {
      clearTimeout(autoStartTimer)
      autoStartTimer = null
    }
    onboardingStore.clearControlMethods()
  })

  return {
    startTour,
    replayTour,
    destroyTour,
    nextStep,
    isCurrentStep,
    hasSeen,
    markAsSeen,
    clearSeen
  }
}
