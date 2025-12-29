/**
 * Onboarding Store
 * Manages onboarding tour state and control methods
 */

import { defineStore REDACTED from 'pinia'
import { markRaw, ref, shallowRef REDACTED from 'vue'
import type { Driver REDACTED from 'driver.js'

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
  REDACTED

  function setControlMethods(methods: {
    nextStep: NextStepCallback,
    isCurrentStep: IsCurrentStepCallback
  REDACTED): void {
    nextStepCallback.value = methods.nextStep
    isCurrentStepCallback.value = methods.isCurrentStep
  REDACTED

  function clearControlMethods(): void {
    nextStepCallback.value = null
    isCurrentStepCallback.value = null
  REDACTED

  function setDriverInstance(driver: Driver | null): void {
    driverInstance.value = driver ? markRaw(driver) : null
  REDACTED

  function getDriverInstance(): Driver | null {
    return driverInstance.value
  REDACTED

  function isDriverActive(): boolean {
    return driverInstance.value?.isActive?.() ?? false
  REDACTED

  function replay(): void {
    if (replayCallback.value) {
      replayCallback.value()
    REDACTED
  REDACTED

  /**
   * Manually advance to the next step
   * @param delay Optional delay in ms (useful for waiting for animations)
   */
  async function nextStep(delay = 0): Promise<void> {
    if (nextStepCallback.value) {
      await nextStepCallback.value(delay)
    REDACTED
  REDACTED

  /**
   * Check if the tour is currently highlighting a specific element
   */
  function isCurrentStep(selector: string): boolean {
    if (isCurrentStepCallback.value) {
      return isCurrentStepCallback.value(selector)
    REDACTED
    return false
  REDACTED

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
  REDACTED
REDACTED)
