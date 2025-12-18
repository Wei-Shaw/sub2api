<template>
  <Teleport to="body">
    <div
      class="fixed top-4 right-4 z-[9999] space-y-3 pointer-events-none"
      aria-live="polite"
      aria-atomic="true"
    >
      <TransitionGroup
        enter-active-class="transition ease-out duration-300"
        enter-from-class="opacity-0 translate-x-full"
        enter-to-class="opacity-100 translate-x-0"
        leave-active-class="transition ease-in duration-200"
        leave-from-class="opacity-100 translate-x-0"
        leave-to-class="opacity-0 translate-x-full"
      >
        <div
          v-for="toast in toasts"
          :key="toast.id"
          :class="[
            'pointer-events-auto min-w-[320px] max-w-md overflow-hidden rounded-lg shadow-lg',
            'bg-white dark:bg-dark-800',
            'border-l-4',
            getBorderColor(toast.type)
          ]"
        >
          <div class="p-4">
            <div class="flex items-start gap-3">
              <!-- Icon -->
              <div class="flex-shrink-0 mt-0.5">
                <component
                  :is="getIcon(toast.type)"
                  :class="['w-5 h-5', getIconColor(toast.type)]"
                  aria-hidden="true"
                />
              </div>

              <!-- Content -->
              <div class="flex-1 min-w-0">
                <p
                  v-if="toast.title"
                  class="text-sm font-semibold text-gray-900 dark:text-white"
                >
                  {{ toast.title REDACTEDREDACTED
                </p>
                <p
                  :class="[
                    'text-sm leading-relaxed',
                    toast.title ? 'mt-1 text-gray-600 dark:text-gray-300' : 'text-gray-900 dark:text-white'
                  ]"
                >
                  {{ toast.message REDACTEDREDACTED
                </p>
              </div>

              <!-- Close button -->
              <button
                @click="removeToast(toast.id)"
                class="flex-shrink-0 p-1 -m-1 text-gray-400 dark:text-gray-500 transition-colors rounded hover:text-gray-600 dark:hover:text-gray-300 hover:bg-gray-100 dark:hover:bg-dark-700"
                aria-label="Close notification"
              >
                <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                  <path
                    fill-rule="evenodd"
                    d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"
                    clip-rule="evenodd"
                  />
                </svg>
              </button>
            </div>
          </div>

          <!-- Progress bar -->
          <div
            v-if="toast.duration"
            class="h-1 bg-gray-100 dark:bg-dark-700"
          >
            <div
              :class="['h-full transition-all', getProgressBarColor(toast.type)]"
              :style="{ width: `${getProgress(toast)REDACTED%` REDACTED"
            ></div>
          </div>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, h REDACTED from 'vue'
import { useAppStore REDACTED from '@/stores/app'

const appStore = useAppStore()

const toasts = computed(() => appStore.toasts)

const getIcon = (type: string) => {
  const icons = {
    success: () =>
      h(
        'svg',
        {
          fill: 'currentColor',
          viewBox: '0 0 20 20'
        REDACTED,
        [
          h('path', {
            'fill-rule': 'evenodd',
            d: 'M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z',
            'clip-rule': 'evenodd'
          REDACTED)
        ]
      ),
    error: () =>
      h(
        'svg',
        {
          fill: 'currentColor',
          viewBox: '0 0 20 20'
        REDACTED,
        [
          h('path', {
            'fill-rule': 'evenodd',
            d: 'M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z',
            'clip-rule': 'evenodd'
          REDACTED)
        ]
      ),
    warning: () =>
      h(
        'svg',
        {
          fill: 'currentColor',
          viewBox: '0 0 20 20'
        REDACTED,
        [
          h('path', {
            'fill-rule': 'evenodd',
            d: 'M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z',
            'clip-rule': 'evenodd'
          REDACTED)
        ]
      ),
    info: () =>
      h(
        'svg',
        {
          fill: 'currentColor',
          viewBox: '0 0 20 20'
        REDACTED,
        [
          h('path', {
            'fill-rule': 'evenodd',
            d: 'M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z',
            'clip-rule': 'evenodd'
          REDACTED)
        ]
      )
  REDACTED
  return icons[type as keyof typeof icons] || icons.info
REDACTED

const getIconColor = (type: string): string => {
  const colors: Record<string, string> = {
    success: 'text-green-500',
    error: 'text-red-500',
    warning: 'text-yellow-500',
    info: 'text-blue-500'
  REDACTED
  return colors[type] || colors.info
REDACTED

const getBorderColor = (type: string): string => {
  const colors: Record<string, string> = {
    success: 'border-green-500',
    error: 'border-red-500',
    warning: 'border-yellow-500',
    info: 'border-blue-500'
  REDACTED
  return colors[type] || colors.info
REDACTED

const getProgressBarColor = (type: string): string => {
  const colors: Record<string, string> = {
    success: 'bg-green-500',
    error: 'bg-red-500',
    warning: 'bg-yellow-500',
    info: 'bg-blue-500'
  REDACTED
  return colors[type] || colors.info
REDACTED

const getProgress = (toast: any): number => {
  if (!toast.duration || !toast.startTime) return 100
  const elapsed = Date.now() - toast.startTime
  const progress = Math.max(0, 100 - (elapsed / toast.duration) * 100)
  return progress
REDACTED

const removeToast = (id: string) => {
  appStore.hideToast(id)
REDACTED

let intervalId: number | undefined

onMounted(() => {
  // Check for expired toasts every 100ms
  intervalId = window.setInterval(() => {
    const now = Date.now()
    toasts.value.forEach((toast) => {
      if (toast.duration && toast.startTime) {
        if (now - toast.startTime >= toast.duration) {
          removeToast(toast.id)
        REDACTED
      REDACTED
    REDACTED)
  REDACTED, 100)
REDACTED)

onUnmounted(() => {
  if (intervalId !== undefined) {
    clearInterval(intervalId)
  REDACTED
REDACTED)
</script>
