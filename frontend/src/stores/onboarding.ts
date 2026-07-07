/**
 * Onboarding Store
 * Manages onboarding tour state and control methods
 */

import { defineStore } from 'pinia'
import { markRaw, ref, shallowRef } from 'vue'
import type { Driver } from 'driver.js'

type VoidCallback = () => void
type NextStepCallback = (delay?: number) => Promise<void>
type IsCurrentStepCallback = (selector: string) => boolean

export const useOnboardingStore = defineStore('onboarding', () => {
  const replayCallback = ref<VoidCallback | null>(null)
  const nextStepCallback = ref<NextStepCallback | null>(null)
  const isCurrentStepCallback = ref<IsCurrentStepCallback | null>(null)

  // 全局 driver 实例，跨组件保持
  const driverInstance = shallowRef<Driver | null>(null)

  function setReplayCallback(callback: VoidCallback | null): void {
    replayCallback.value = callback
  }

  function setControlMethods(methods: {
    nextStep: NextStepCallback,
    isCurrentStep: IsCurrentStepCallback
  }): void {
    nextStepCallback.value = methods.nextStep
    isCurrentStepCallback.value = methods.isCurrentStep
  }

  function clearControlMethods(): void {
    nextStepCallback.value = null
    isCurrentStepCallback.value = null
  }

  function setDriverInstance(driver: Driver | null): void {
    driverInstance.value = driver ? markRaw(driver) : null
  }

  function getDriverInstance(): Driver | null {
    return driverInstance.value
  }

  function isDriverActive(): boolean {
    return driverInstance.value?.isActive?.() ?? false
  }

  function replay(): void {
    if (replayCallback.value) {
      replayCallback.value()
    }
  }

  /**
   * Manually advance to the next step
   * @param delay Optional delay in ms (useful for waiting for animations)
   */
  async function nextStep(delay = 0): Promise<void> {
    if (nextStepCallback.value) {
      await nextStepCallback.value(delay)
    }
  }

  /**
   * Check if the tour is currently highlighting a specific element
   */
  function isCurrentStep(selector: string): boolean {
    if (isCurrentStepCallback.value) {
      return isCurrentStepCallback.value(selector)
    }
    return false
  }

  return {
    setReplayCallback,
    setControlMethods,
    clearControlMethods,
    setDriverInstance,
    getDriverInstance,
    isDriverActive,
    replay,
    nextStep,
    isCurrentStep
  }
})
